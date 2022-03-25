package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

var (
	maxGoroutines   uint
	taskTimeout     time.Duration
	shutdownTimeout time.Duration

	ErrInvalidHandlerFunc   = errors.New("invalid handler func")
	ErrWorkerPoolNotRunning = errors.New("worker pool is not running")
)

// WorkerPool is the definition of a worker pool.
type WorkerPool struct {
	taskTimeout     time.Duration
	shutdownTimeout time.Duration

	running          uint32
	tasks            []*Workload
	taskChan         chan *Workload
	runningTasks     map[uint64]*Workload
	runningTasksLock sync.RWMutex
	guard            chan any
	IDGen            uint64
	shutdown         chan struct{}
}

// Workload represents a workload.
type Workload struct {
	ID       uint64
	Func     reflect.Value
	Args     []any
	response chan any
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(maxGoroutines uint, taskTimeout, shutdownTimeout time.Duration) *WorkerPool {
	return &WorkerPool{
		taskTimeout:     taskTimeout,
		shutdownTimeout: shutdownTimeout,
		guard:           make(chan any, maxGoroutines),
		runningTasks:    make(map[uint64]*Workload),
		taskChan:        make(chan *Workload),
		shutdown:        make(chan struct{}),
	}
}

// Shutdown shuts down the worker pool.
func (w *WorkerPool) Shutdown() {
	if !w.Running() {
		return
	}
	w.shutdown <- struct{}{}
}

// Running returns true if the worker pool is running.
func (w *WorkerPool) Running() bool {
	return atomic.LoadUint32(&w.running) == 1
}

// setRunning sets the running state of the worker pool.
func (w *WorkerPool) setRunning(state bool) {
	val := uint32(1)
	if !state {
		val = 0
	}
	atomic.StoreUint32(&w.running, val)
}

//RunningTasks returns a list of running task
func (w *WorkerPool) RunningTasks() []uint64 {
	w.runningTasksLock.RLock()
	defer w.runningTasksLock.RUnlock()
	var l []uint64
	for ID := range w.runningTasks {
		l = append(l, ID)
	}
	return l
}

// removeRunningTask removes a running task from the map.
func (w *WorkerPool) removeRunningTask(ID uint64) {
	w.runningTasksLock.Lock()
	defer w.runningTasksLock.Unlock()
	delete(w.runningTasks, ID)
}

// addRunningTask adds a task to the running map.
func (w *WorkerPool) addRunningTask(ID uint64, task *Workload) {
	w.runningTasksLock.Lock()
	defer w.runningTasksLock.Unlock()
	w.runningTasks[ID] = task
}

// Start starts the worker pool.
// It returns silently if it's already running.
func (w *WorkerPool) Start(done chan error) {
	if w.Running() {
		return
	}
	w.setRunning(true)
	ctx, cancel := context.WithCancel(context.Background())
	var doneErr error
	for {
		select {
		case <-w.shutdown:
			w.setRunning(false)
			log.Printf("[shutdown] waiting max %s for %d running tasks to finish\n", w.shutdownTimeout.String(), len(w.RunningTasks()))
			timeout := time.NewTimer(w.shutdownTimeout)
			ticker := time.NewTicker(100 * time.Millisecond)
		L:
			for {
				select {
				case <-timeout.C:
					doneErr = errors.New("shutdown timeout")
					break L
				case <-ticker.C:
					// check if there are still running tasks.
					log.Printf("[shutdown] %d tasks remaining...\n", len(w.RunningTasks()))
					if len(w.RunningTasks()) == 0 {
						break L
					}
				}
			}
			timeout.Stop()
			ticker.Stop()
			cancel()
			done <- doneErr
			return
		case task := <-w.taskChan:
			w.tasks = append(w.tasks, task)
		default:
			// pop and execute tasks
			var task *Workload
			if len(w.tasks) > 0 {
				task = w.tasks[0]
				w.tasks = w.tasks[1:]
			}
			if task == nil {
				continue
			}
			go w.work(ctx, task)
			w.guard <- struct{}{}
		}
	}
}

func (w *WorkerPool) work(ctx context.Context, task *Workload) {
	w.addRunningTask(task.ID, task)
	defer func() {
		<-w.guard
		w.removeRunningTask(task.ID)
		close(task.response)
	}()

	subChan := make(chan []any)
	subRoutine := func(task *Workload, subChan chan []any) {
		numIn := task.Func.Type().NumIn()
		if numIn != len(task.Args) {
			task.response <- errors.New(fmt.Sprintf("mismatch between function arg length and submitted args length. %d, %d", numIn, len(task.Args)))
			return
		}
		in := make([]reflect.Value, numIn)
		for i := 0; i < numIn; i++ {
			inType := task.Func.Type().In(i)
			argValue := reflect.ValueOf(task.Args[i])
			argType := argValue.Type()
			if argType.ConvertibleTo(inType) {
				in[i] = argValue.Convert(inType)
			} else {
				task.response <- errors.New("mismatch between in and arg type")
				return
			}
		}

		responses := task.Func.Call(in)
		var rs []any
		rs = append(rs, nil) // no error
		for _, r := range responses {
			rs = append(rs, r.Interface())
		}
		subChan <- rs
	}

	timeout := time.NewTimer(w.taskTimeout)
	defer timeout.Stop()
	go subRoutine(task, subChan)

	for {
		select {
		case <-timeout.C:
			log.Println("task timeout")
			task.response <- errors.New("timeout running task")
			return
		case <-ctx.Done():
			task.response <- errors.New("context cancelled. Terminating")
			return
		case res := <-subChan:
			for _, r := range res {
				task.response <- r
			}
			return
		}
	}
}

// nextID gets the next ID for a task
func (w *WorkerPool) nextID() uint64 {
	atomic.AddUint64(&w.IDGen, 1)
	return atomic.LoadUint64(&w.IDGen)
}

// TrySubmit tries to submit a new task to the worker pool.
// It returns an error if the task submit is not a function.
// It also returns a channel where the response of the task will be sent on.
// The first value of the response is always an error. Followed by the returned values from the task.
func (w *WorkerPool) TrySubmit(f any, arguments ...any) (error, <-chan any) {
	if !w.Running() {
		return ErrWorkerPoolNotRunning, nil
	}

	v := reflect.Indirect(reflect.ValueOf(f))
	if v.Kind() != reflect.Func {
		return ErrInvalidHandlerFunc, nil
	}
	task := &Workload{
		ID:       w.nextID(),
		Func:     v,
		Args:     arguments,
		response: make(chan any, v.Type().NumOut()+1), // + 1 for error
	}
	w.taskChan <- task
	return nil, task.response
}
