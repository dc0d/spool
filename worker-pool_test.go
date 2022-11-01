// see LICENSE file

package spool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dc0d/actor"

	"github.com/stretchr/testify/assert"
)

func Test_WorkerPool_New(t *testing.T) {
	t.Run(`should set default mailbox size to zero`, func(t *testing.T) {
		pool := New(-1)
		defer pool.Stop()

		assert.True(t, len(pool) == 0)
	})

	t.Run(`should not start any initial workers`, func(t *testing.T) {
		pool := New(-1)
		defer pool.Stop()

		assert.Never(t, func() bool {
			select {
			case pool <- func() { panic("should not run") }:
				return true
			default:
			}
			return false
		}, time.Millisecond*300, time.Millisecond*20)
	})
}

func Test_WorkerPool_grow_should_spawn_workers_equal_to_growth(t *testing.T) {
	var (
		ctx     = context.Background()
		growth  = 100
		options []actor.Option
	)
	pool := New(-1)
	exec := &CallbacksSpy[T]{
		StoppedFunc: func() {},
	}
	executorFactory := func() actor.Callbacks[T] { return exec }

	pool.grow(ctx, growth, executorFactory, options...)
	pool.Stop()

	assert.Eventually(t, func() bool {
		return len(exec.StoppedCalls()) == growth
	}, time.Millisecond*300, time.Millisecond*20)
}

func Test_WorkerPool_Blocking_should_serialize_the_jobs(t *testing.T) {
	const n = 1000

	pool := New(10)
	defer pool.Stop()
	pool.Grow(context.Background(), 1)
	var (
		counter, previous int64
	)
	wg := &sync.WaitGroup{}
	wg.Add(n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			_ = pool.Blocking(context.Background(), func() {
				defer wg.Done()
				<-start

				previous = atomic.LoadInt64(&counter)
				next := atomic.AddInt64(&counter, 1)
				assert.Equal(t, previous+1, next)
			})
		}()
	}
	close(start) // signal all jobs they are green to go
	wg.Wait()

	assert.Equal(t, int64(n), counter)
}

func Test_WorkerPool_Blocking_should_respect_context_cancellation(t *testing.T) {
	pool := New(-1)
	defer pool.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := pool.Blocking(ctx, func() { panic("should not be called") })

	assert.Equal(t, context.Canceled, err)
}

func Test_WorkerPool_SemiBlocking_should_just_put_job_in_the_mailbox(t *testing.T) {
	const n = 1000
	pool := New(n)
	defer pool.Stop()
	pool.Grow(context.Background(), 1)
	var counter int64 = 0
	wg := &sync.WaitGroup{}

	wg.Add(n)
	for i := 0; i < n; i++ {
		_ = pool.SemiBlocking(context.Background(), func() {
			defer wg.Done()
			atomic.AddInt64(&counter, 1)
		})
	}
	wg.Wait()

	assert.Equal(t, int64(n), counter)
}

func Test_WorkerPool_SemiBlocking_should_respect_context_cancellation(t *testing.T) {
	pool := New(-1)
	defer pool.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := pool.SemiBlocking(ctx, func() { panic("should not be called") })

	assert.Equal(t, context.Canceled, err)
}

func Test_WorkerPool_should_not_stop_because_of_panic(t *testing.T) {
	pool := New(1)
	defer pool.Stop()
	pool.Grow(context.Background(), 1)

	_ = pool.Blocking(context.Background(), func() {
		panic("some error")
	})

	counter := 0
	_ = pool.Blocking(context.Background(), func() {
		counter++
	})

	assert.Equal(t, 1, counter)
}

func Test_WorkerPool_Grow_should_stop_extra_workers_with_absolute_timeout(t *testing.T) {
	increased := 10
	absoluteTimeout := time.Millisecond * 10
	pool := New(9)
	defer pool.Stop()
	exec := &CallbacksSpy[T]{
		StoppedFunc: func() {},
	}
	executorFactory := func() actor.Callbacks[T] { return exec }

	pool.grow(context.Background(), increased, executorFactory, actor.WithAbsoluteTimeout(absoluteTimeout))

	assert.Eventually(t, func() bool {
		return len(exec.StoppedCalls()) == increased
	}, time.Millisecond*500, time.Millisecond*50)
}

func Test_WorkerPool_Grow_should_stop_extra_workers_with_idle_timeout_when_there_are_no_more_jobs(t *testing.T) {
	const n = 1000
	increased := 10
	idleTimeout := time.Millisecond * 50
	pool := New(100)
	defer pool.Stop()
	exec := &CallbacksSpy[T]{
		StoppedFunc:  func() {},
		ReceivedFunc: func(fn func()) { fn() },
	}
	executorFactory := func() actor.Callbacks[T] { return exec }

	start := make(chan struct{}, n)
	wg := &sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			_ = pool.SemiBlocking(context.Background(), func() {
				defer wg.Done()
				<-start
			})
		}()
	}
	pool.grow(context.Background(), increased, executorFactory, actor.WithIdleTimeout(idleTimeout))

	expectedNumberOfWorkers := increased
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == len(exec.ReceivedCalls())
	}, time.Millisecond*1000, time.Millisecond*20,
		"expected %v actual %v", expectedNumberOfWorkers, func() int { return len(exec.ReceivedCalls()) }())

	close(start)
	wg.Wait()

	expectedNumberOfStoppedWorkers := 10
	assert.Eventually(t, func() bool {
		return expectedNumberOfStoppedWorkers == len(exec.StoppedCalls())
	}, time.Millisecond*5000, time.Millisecond*50)
}

func Test_WorkerPool_Grow_should_stop_extra_workers_when_context_is_canceled(t *testing.T) {
	increased := 10
	pool := New(10)
	defer pool.Stop()
	exec := &CallbacksSpy[T]{
		StoppedFunc:  func() {},
		ReceivedFunc: func(fn func()) { fn() },
	}
	executorFactory := func() actor.Callbacks[T] { return exec }
	pool.grow(context.Background(), 1, executorFactory)

	ctx, cancel := context.WithCancel(context.Background())
	pool.grow(ctx, increased, executorFactory)

	cancel()

	expectedNumberOfStoppedWorkers := increased
	assert.Eventually(t, func() bool {
		return expectedNumberOfStoppedWorkers == len(exec.StoppedCalls())
	}, time.Millisecond*5000, time.Millisecond*50)
}

func Test_WorkerPool_Stop_should_close_the_pool(t *testing.T) {
	pool := New(9)
	pool.Stop()

	assert.Panics(t, func() {
		_ = pool.SemiBlocking(context.Background(), func() {})
	})
}

func Test_WorkerPool_Stop_should_stop_the_workers(t *testing.T) {
	pool := New(9)
	increased := 10
	exec := &CallbacksSpy[T]{
		StoppedFunc:  func() {},
		ReceivedFunc: func(fn func()) { fn() },
	}
	executorFactory := func() actor.Callbacks[T] { return exec }
	pool.grow(context.Background(), increased, executorFactory)

	pool.Stop()

	expectedNumberOfStoppedWorkers := increased
	assert.Eventually(t, func() bool {
		return expectedNumberOfStoppedWorkers == len(exec.StoppedCalls())
	}, time.Millisecond*5000, time.Millisecond*50)
}

func ExampleWorkerPool_Blocking() {
	pool := New(1)
	defer pool.Stop()
	pool.Grow(context.Background(), 1)

	var state int64
	job := func() { atomic.AddInt64(&state, 19) }

	_ = pool.Blocking(context.Background(), job)

	fmt.Println(atomic.LoadInt64(&state))

	// Output:
	// 19
}

func ExampleWorkerPool_SemiBlocking() {
	pool := New(1)
	defer pool.Stop()
	pool.Grow(context.Background(), 1)

	var state int64
	jobDone := make(chan struct{})
	job := func() {
		defer close(jobDone)
		atomic.AddInt64(&state, 19)
	}

	_ = pool.SemiBlocking(context.Background(), job)
	<-jobDone

	fmt.Println(state)

	// Output:
	// 19
}

func ExampleWorkerPool_Grow() {
	const n = 19
	pool := New(1)
	defer pool.Stop()

	pool.Grow(context.Background(), 3) // spin up three new workers

	var state int64
	wg := &sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		_ = pool.SemiBlocking(context.Background(), func() { defer wg.Done(); atomic.AddInt64(&state, 1) })
	}
	wg.Wait()

	fmt.Println(state)

	// Output:
	// 19
}
