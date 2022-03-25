package main

import (
	"sync"
	"testing"
	"time"
)

var wp *WorkerPool

func setup(maxG uint) {
	wp = NewWorkerPool(maxG, taskT, shutdownT)
	done := make(chan error)
	go wp.Start(done)
	time.Sleep(wpStartWait)
}
func f() {
	var wg sync.WaitGroup
	wg.Add(1000)
	for j := 0; j < 1000; j++ {
		go func(j int) {
			_, res := wp.TrySubmit(func(a int) int {
				return a * a
			}, j)
			go func(res chan any) {
				_, _ = <-res, <-res
				wg.Done()
			}(res)
		}(j)
	}
	wg.Wait()
}

func BenchmarkWorkerPool(b *testing.B) {
	setup(1)
	for i := 0; i < b.N; i++ {
		f()
	}
	wp.Shutdown()
}

func BenchmarkWorkerPool2(b *testing.B) {
	setup(10)
	for i := 0; i < b.N; i++ {
		f()
	}
	wp.Shutdown()
}

func BenchmarkWorkerPool3(b *testing.B) {
	setup(100)
	for i := 0; i < b.N; i++ {
		f()
	}
	wp.Shutdown()
}
