package goroutinepool

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
)

var (
	debug      atomic.Value // a debug flag
	poolsCount uint64       // incremented when debug enabled only
)

func init() {
	debug.Store(false)
}

// EnableDebugLog makes further calls to RunInPool or RunInPoolWithChan to log
// trace messages.
func EnableDebugLog() {
	debug.Store(true)
}

// DisableDebugLog makes further calls to RunInPool or RunInPoolWithChan not
// log trace messages.
func DisableDebugLog() {
	debug.Store(false)
}

// An internal helper for tracing, maintains a pool related marker to dump in
// messages.
func getDebug() func(...string) {
	if !debug.Load().(bool) {
		// do nothing
		return func(s ...string) {}
	}

	marker := fmt.Sprintf("pool[#%d]", atomic.AddUint64(&poolsCount, 1))
	return func(message ...string) {
		log.Println(marker, message)
	}
}

// RunInPool concurrently runs provided functions with supplied concurrency
// level unless the context is triggered.
// Uses at least one worker.
func RunInPool(ctx context.Context, poolSize int, fns []func(context.Context)) {
	RunInPoolWithChan(ctx, poolSize, NonBufferedChan(ctx, fns))
}

// RunInPoolWithChan concurrently runs items from a provided channel unless
// till the context is triggered or channel is closed.
// Uses at least one worker.
func RunInPoolWithChan(ctx context.Context, poolSize int, ch chan func(context.Context)) {
	var done atomic.Value
	done.Store(false)

	var wg sync.WaitGroup

	debug := getDebug()

	wg.Add(1)
	go func() {
		wg.Done()

		select {
		case <-ctx.Done():
			debug("context done")
			done.Store(true)
		}
	}()

	if poolSize <= 0 {
		poolSize = 1
	}
	for i := 0; i < poolSize; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				select {
				case item, ok := <-ch:
					if !ok {
						// channel closed
						debug("no more items, channel closed")
						return
					}
					if done.Load().(bool) {
						// make sure read out channel fully
						debug("traverse queue, channel inactive")
						break
					}
					debug("new item")
					item(ctx)
				}
			}
		}()
	}

	wg.Wait()
}

// NonBufferedChan creates a non-buffered channel out of provided
// functions.
//
// Channel is auto-closed once all its elements consumed or context canceled.
func NonBufferedChan(ctx context.Context, fns []func(context.Context)) chan func(context.Context) {
	return PopulateAndCloseChan(ctx, make(chan func(context.Context)), fns)
}

// PopulateAndCloseChan passes all provided elements into the given channel. This
// approach allows providing own buffered channel. The provided channel must
// not be populated anywhere else otherwise there will be a data race.
//
// See RunInPoolWithChan examples for the use cases.
//
// Channel is auto-closed once all its elements consumed or context canceled.
func PopulateAndCloseChan(ctx context.Context, ch chan func(context.Context), fns []func(context.Context)) chan func(context.Context) {
	go func() {
		defer close(ch)

		for _, item := range fns {
			select {
			case <-ctx.Done():
				return
			default:
				ch <- item
			}
		}
	}()

	return ch
}
