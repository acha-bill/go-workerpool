# Workerpool
This is a simple worker pool with constant input and variable throughput. The variability of the throuput is configured by a `maxGoroutines` parameter.

`taskTimeout` param defines the max duration a task should spend on a worker.
Likewise, `shutdownTimeout` defines the max duration it takes to shut down the worker gracefully.

```go
wp := NewWorkerPool(maxGoroutines, taskTimeout, shutdownTimeout)
sigs := make(chan os.Signal, 1)
signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
done := make(chan error)
go wp.Start(done)
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

time.Sleep(1 * time.Millisecond) // wait for wp to start
err, responses := wp.TrySubmit(task, 3)
if err != nil {
    log.Println(err)
    return
}

// responses in order of task return list (prefixed with error). so [error, int]
errI := <-responses
if errI != nil {
log.Println(errI.(error))
return
}
resI := <-responses
log.Println(resI.(int))
```
## Benchmark
1000 tasks were submited to the worker pool and their time to finish was calculated.

The task:
```go
square = func(a int) int {
	return a * a
}
```

Results:

| Max Goroutines | n tasks | time taken    | %      |
|----------------|---------| ------------- |--------|
| 1              | 1000    | 3993085 ns/op | -  
| 10             | 1000    | 2128894 ns/op | 46.69%
| 100            | 1000    | 1975701 ns/op | 7.2%(10), 50.52% (1)
