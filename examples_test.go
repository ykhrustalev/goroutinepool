package goroutinepool_test

import (
	"context"
	"fmt"
	"github.com/ykhrustalev/goroutinepool"
	"sync/atomic"
	"time"
)

func ExampleRunInPool_basic() {
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
}

func ExampleRunInPool_cancellation() {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var counter int32

	// a simple job to increment a number
	increment := func(delta int32) func(context.Context) {
		return func(ctx context.Context) {
			atomic.AddInt32(&counter, delta)

			select {
			case <-ctx.Done():
				break // won't exit unless cancelled
			}

			time.Sleep(200 * time.Millisecond)
		}
	}

	// jobs
	fns := []func(context.Context){
		increment(1),   // executed and waited
		increment(10),  // executed and waited
		increment(100), // will be cancelled by timeout
	}

	// use two workers
	goroutinepool.RunInPool(ctx, 2, fns)

	fmt.Println(atomic.LoadInt32(&counter))

	// Output:
	// 11
}

func ExampleRunInPoolWithChan_bufferedChannel() {
	ctx := context.Background()

	var counter int32

	// a simple job to increment a number
	increment := func(delta int32) func(context.Context) {
		return func(ctx context.Context) {
			atomic.AddInt32(&counter, delta)
		}
	}

	// create a buffered channel
	ch := make(chan func(context.Context), 2)
	// extend channel with jobs
	// note, that channel gets closed using this function
	goroutinepool.PopulateAndCloseChan(ctx, ch, []func(context.Context){
		increment(1),
		increment(10),
		increment(100),
	})

	// use two workers
	goroutinepool.RunInPoolWithChan(ctx, 2, ch)

	fmt.Println(atomic.LoadInt32(&counter))

	// Output:
	// 111
}

func ExampleNonBufferedChan() {
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
		increment(1),   // executed and waited
		increment(10),  // executed and waited
		increment(100), // will be cancelled by timeout
	}

	// use two workers
	ch := goroutinepool.NonBufferedChan(ctx, fns)

	for fn := range ch {
		fn(ctx)
	}

	fmt.Println(atomic.LoadInt32(&counter))

	// Output:
	// 111
}
