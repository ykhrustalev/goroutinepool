package goroutinepool_test

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/ykhrustalev/goroutinepool"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

type buffer struct {
	items []string
	mx    sync.Mutex
}

func (b *buffer) Append(item string) {
	b.mx.Lock()
	defer b.mx.Unlock()

	b.items = append(b.items, item)
}

func (b *buffer) Get() []string {
	b.mx.Lock()
	defer b.mx.Unlock()

	return b.items
}

func (b *buffer) GetSorted() []string {
	b.mx.Lock()
	defer b.mx.Unlock()

	c := make([]string, len(b.items))
	copy(c, b.items)
	sort.Strings(c)
	return c
}

func newAppend(buf *buffer, item string) func(context.Context) {
	return func(_ context.Context) {
		buf.Append(item)
	}
}

func newAppends(buf *buffer, items []string) (r []func(ctx context.Context)) {
	for _, item := range items {
		r = append(r, newAppend(buf, item))
	}
	return
}

func newAppendWithLock(buf *buffer, item string, wait time.Duration) func(ctx context.Context) {
	return func(ctx context.Context) {
		buf.Append(item)

		// won't return unless parent context cancelled
		select {
		case <-ctx.Done():
			break
		}

		// make sure done signal received
		time.Sleep(wait)
	}
}

func TestRunInPool(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	ctx := context.Background()

	testRead := func(t *testing.T, poolSize int) {
		var buf buffer

		goroutinepool.RunInPool(ctx, poolSize, newAppends(&buf, items))

		assert.ElementsMatch(t, items, buf.Get())
	}

	t.Run("run all", func(t *testing.T) {
		testRead(t, 2)
	})

	t.Run("pool size 0", func(t *testing.T) {
		testRead(t, 0)
	})

	t.Run("pull size large than jobs", func(t *testing.T) {
		testRead(t, 20)
	})

	t.Run("cancel", func(t *testing.T) {
		var buf buffer

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		var fns []func(context.Context)
		for _, item := range items {
			fns = append(fns, newAppendWithLock(&buf, item, 200*time.Millisecond))
		}

		goroutinepool.RunInPool(ctx, 2, fns)

		// due to the sleep in handler there are predictable items
		assert.Equal(t, []string{"a", "b"}, buf.GetSorted())
	})
}

func TestRunInPoolWithChan(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	t.Run("cancel", func(t *testing.T) {
		var buf buffer

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		var fns []func(context.Context)
		for _, item := range items {
			fns = append(fns, newAppendWithLock(&buf, item, 200*time.Millisecond))
		}

		ch := goroutinepool.PopulateAndCloseChan(
			ctx,
			make(chan func(context.Context), 100),
			fns,
		)

		goroutinepool.RunInPoolWithChan(ctx, 2, ch)

		// due to the sleep in handler there are predictable items
		assert.Equal(t, []string{"a", "b"}, buf.GetSorted())
	})
}

func TestDebug(t *testing.T) {
	goroutinepool.EnableDebugLog()

	goroutinepool.RunInPool(context.Background(), 1, []func(ctx context.Context){
		func(ctx context.Context) { fmt.Println(1) },
		func(ctx context.Context) { fmt.Println(2) },
		func(ctx context.Context) { fmt.Println(3) },
	})

	goroutinepool.DisableDebugLog()

	goroutinepool.RunInPool(context.Background(), 1, []func(ctx context.Context){
		func(ctx context.Context) { fmt.Println(4) },
		func(ctx context.Context) { fmt.Println(5) },
		func(ctx context.Context) { fmt.Println(6) },
	})
}

func TestNonBufferedChan(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	testRead := func(t *testing.T, args, expected []string) {
		var buf buffer

		ctx := context.Background()
		ch := goroutinepool.NonBufferedChan(ctx, newAppends(&buf, args))

		assert.Empty(t, buf.Get())

		for fn := range ch {
			fn(ctx)
		}

		assert.Equal(t, expected, buf.Get())
	}

	t.Run("full read", func(t *testing.T) {
		testRead(t, items, items)
	})

	t.Run("read large", func(t *testing.T) {
		items := func() (r []string) {
			for i := 0; i < 99999; i++ {
				items = append(items, strconv.Itoa(i))
			}
			return
		}()

		testRead(t, items, items)
	})

	t.Run("cancellation", func(t *testing.T) {
		var buf buffer

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := goroutinepool.NonBufferedChan(ctx, newAppends(&buf, items))

		// read `a`
		fn := <-ch
		fn(ctx)
		// `b` might be put in a queue right after the cancel call
		cancel()
		// at this point, no items put in a queue
		for fn = range ch {
			fn(ctx)
		}

		if len(buf.Get()) == 1 {
			assert.Equal(t, []string{"a"}, buf.Get())
		} else {
			assert.Equal(t, []string{"a", "b"}, buf.Get())
		}
	})
}

func TestPopulateAndCloseChan(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	t.Run("cancellation partially buffered", func(t *testing.T) {
		var buf buffer

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := goroutinepool.PopulateAndCloseChan(
			ctx,
			make(chan func(context.Context), 2),
			newAppends(&buf, items),
		)

		time.Sleep(100 * time.Millisecond)
		// chan contains `a`, `b`

		cancel()
		// race, `c` might be put in a chan before cancel call

		// only buffered items read
		for fn := range ch {
			fn(ctx)
		}

		if len(buf.Get()) == 2 {
			assert.Equal(t, []string{"a", "b"}, buf.Get())
		} else {
			assert.Equal(t, []string{"a", "b", "c"}, buf.Get())
		}
	})

	t.Run("cancellation full buffer", func(t *testing.T) {
		var buf buffer

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		ch := goroutinepool.PopulateAndCloseChan(
			ctx,
			make(chan func(context.Context), 10),
			newAppends(&buf, items),
		)
		// chan contains all items

		// read all buffered items
		for fn := range ch {
			fn(ctx)
		}
		assert.Equal(t, []string{"a", "b", "c", "d", "e"}, buf.Get())
	})
}
