package main

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	maxG        = uint(1)
	taskT       = 200 * time.Millisecond
	shutdownT   = 1 * time.Second
	wpStartWait = 1 * time.Millisecond
)

func TestNewWorkerPool(t *testing.T) {
	wp := NewWorkerPool(maxG, taskT, shutdownT)
	require.NotNil(t, wp)
}

func TestWorkerPool_TrySubmit(t *testing.T) {
	wp := NewWorkerPool(maxG, taskT, shutdownT)
	done := make(chan error)
	go wp.Start(done)
	defer wp.Shutdown()
	time.Sleep(wpStartWait)
	err, _ := wp.TrySubmit(func() {})
	require.NoError(t, err)
}

func TestWorkerPool_TrySubmit2(t *testing.T) {
	wp := NewWorkerPool(maxG, taskT, shutdownT)
	done := make(chan error)
	go wp.Start(done)
	defer wp.Shutdown()
	time.Sleep(wpStartWait)
	err, _ := wp.TrySubmit("not a func")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidHandlerFunc))
}

func TestWorkerPool_TrySubmit3(t *testing.T) {
	wp := NewWorkerPool(maxG, taskT, shutdownT)
	err, _ := wp.TrySubmit(func() {})
	require.True(t, errors.Is(err, ErrWorkerPoolNotRunning))
}

func TestWorkerPool_TrySubmit4(t *testing.T) {
	wp := NewWorkerPool(maxG, taskT, shutdownT)
	done := make(chan error)
	go wp.Start(done)
	time.Sleep(wpStartWait)
	err, res := wp.TrySubmit(func(a int) int {
		return a
	})
	require.Nil(t, err)
	require.NotNil(t, res)
	errI := <-res // missing argument
	require.NotNil(t, errI)
	err, ok := errI.(error)
	require.True(t, ok)
	require.Error(t, err)
	wp.Shutdown()
	<-done
}

func TestWorkerPool_Start(t *testing.T) {
	wp := NewWorkerPool(maxG, taskT, shutdownT)
	done := make(chan error)
	go wp.Start(done)
	time.Sleep(wpStartWait)
	defer func() {
		wp.Shutdown()
	}()

	err, res := wp.TrySubmit(func(a int) int {
		return a * a
	}, 2)
	require.NoError(t, err)
	require.NotNil(t, res)

	errI := <-res
	require.Nil(t, errI)
	squareI := <-res
	require.NotNil(t, squareI)
	square, ok := squareI.(int)
	require.True(t, ok)
	require.Equal(t, 4, square)

	err, res = wp.TrySubmit(func(a int) int {
		time.Sleep(500 * time.Millisecond) // greater than task timeout
		return a * a
	}, 2)

	require.NoError(t, err)
	require.NotNil(t, res)
	errI = <-res
	require.NotNil(t, errI)
	err = errI.(error)
	require.Error(t, err)
}

func TestWorkerPool_Running(t *testing.T) {
	wp := NewWorkerPool(maxG, taskT, shutdownT)
	done := make(chan error)
	go wp.Start(done)
	defer wp.Shutdown()
	time.Sleep(wpStartWait)
	require.True(t, wp.Running())
}

func TestWorkerPool_RunningTasks(t *testing.T) {
	wp := NewWorkerPool(maxG, 1*time.Second, shutdownT)
	done := make(chan error)
	go wp.Start(done)
	defer wp.Shutdown()
	time.Sleep(wpStartWait)
	_, _ = wp.TrySubmit(func() {
		time.Sleep(500 * time.Millisecond)
	})
	time.Sleep(100 * time.Millisecond) // wait for task to start running.
	require.Len(t, wp.RunningTasks(), 1)
}

func TestWorkerPool_Shutdown(t *testing.T) {
	wp := NewWorkerPool(5, 500*time.Millisecond, 200*time.Millisecond)
	done := make(chan error)
	go wp.Start(done)
	time.Sleep(wpStartWait)

	square := func(a int) int {
		time.Sleep(400 * time.Millisecond)
		return a * a
	}
	for i := 0; i < 10; i++ {
		_, _ = wp.TrySubmit(square, i)
	}
	wp.Shutdown()
	err := <-done
	require.Error(t, err)
}
