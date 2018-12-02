# Goroutinepool 
[![GoDoc](https://godoc.org/github.com/ykhrustalev/goroutinepool?status.svg)](https://godoc.org/github.com/ykhrustalev/goroutinepool)
[![Build Status](https://travis-ci.org/ykhrustalev/goroutinepool.svg)](https://travis-ci.org/ykhrustalev/goroutinepool) 
[![Go Report Card](https://goreportcard.com/badge/github.com/ykhrustalev/goroutinepool)](https://goreportcard.com/report/github.com/ykhrustalev/goroutinepool)

Package goroutinepool provides handy functions for running goroutines
concurrently similar to `multiprocess.Pool` in python or
`concurrency.ThreadPoolExecutor` in java.

Example:

```go
ctx := context.Background()

var counter int32

// a simple job to increment a number
increment := func(delta int32) func(context.Context) {
    return func(ctx context.Context) {
        atomic.AddInt32(&counter, delta)
    }
}

// jobs
fns := []func(context.Context){
    increment(1),
    increment(10),
    increment(100),
}

// use two workers
goroutinepool.RunInPool(ctx, 2, fns)

fmt.Println(atomic.LoadInt32(&counter))

// Output:
// 111
```
