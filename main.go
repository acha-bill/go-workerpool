package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	flag.UintVar(&maxGoroutines, "max-goroutines", 5, "the max number of worker goroutines")
	flag.DurationVar(&taskTimeout, "task-timeout", 1*time.Second, "the max duration a task can spend on any worker")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", 3*time.Second, "the graceful shutdown timeout")
	flag.Parse()

	wp := NewWorkerPool(maxGoroutines, taskTimeout, shutdownTimeout)
	done := make(chan error)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go wp.Start(done)
	time.Sleep(1 * time.Millisecond) // wait for wp to start
	go func() {
		<-sigs
		wp.Shutdown()
	}()
	defer func() {
		if err := <-done; err != nil {
			log.Println("error shutting done ", err.Error())
			return
		}
		log.Println("[shutdown] done")
	}()

	task := func(n int) int {
		var s int
		for i := 1; i <= n; i++ {
			s += i * i
		}
		return s
	}
	err, responses := wp.TrySubmit(task, 3)
	if err != nil {
		log.Println(err)
		return
	}

	errI := <-responses
	if errI != nil {
		log.Println(errI.(error))
		return
	}
	resI := <-responses
	log.Println(resI.(int))
}
