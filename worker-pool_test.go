// see LICENSE file

package spool

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	workerStarted = func(pool WorkerPool) { incNumberOfWorkers(pool, 1) }
	workerStopped = func(pool WorkerPool) { decNumberOfWorkers(pool, 1) }

	exitVal := m.Run()

	os.Exit(exitVal)
}

func Test_WorkerPool_New(t *testing.T) {
	t.Run(`should set default mailbox size to zero`, func(t *testing.T) {
		pool := New(-1, 0)
		defer pool.Stop()

		assert.True(t, len(pool) == 0)
	})

	t.Run(`should start n initial workers`, func(t *testing.T) {
		n := 100 // initial workers
		pool := New(-1, n)
		defer pool.Stop()

		var count int64
		stop := make(chan struct{})
		defer close(stop)
		for i := 0; i < n; i++ {
			pool.SemiBlocking(func() {
				atomic.AddInt64(&count, 1)
				<-stop
			})
		}

		assert.Eventually(t, func() bool {
			return atomic.LoadInt64(&count) == int64(n)
		}, time.Millisecond*300, time.Millisecond*20)

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

func Test_WorkerPool_Blocking_should_serialize_the_jobs(t *testing.T) {
	const n = 1000

	pool := New(10, 1)
	defer pool.Stop()

	var (
		counter, previous int64
	)

	wg := &sync.WaitGroup{}
	wg.Add(n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go pool.Blocking(func() {
			defer wg.Done()
			<-start

			previous = atomic.LoadInt64(&counter)
			next := atomic.AddInt64(&counter, 1)
			assert.Equal(t, previous+1, next)
		})
	}
	close(start) // signal all jobs they are green to go
	wg.Wait()

	assert.Equal(t, int64(n), counter)
}

func Test_WorkerPool_Nonblocking_should_just_put_job_in_the_mailbox(t *testing.T) {
	const n = 1000
	pool := New(n, 1)
	defer pool.Stop()

	var counter int64 = 0
	wg := &sync.WaitGroup{}

	wg.Add(n)
	for i := 0; i < n; i++ {
		pool.SemiBlocking(func() {
			defer wg.Done()
			atomic.AddInt64(&counter, 1)
		})
	}
	wg.Wait()

	assert.Equal(t, int64(n), counter)
}

func Test_WorkerPool_should_not_stop_because_of_panic(t *testing.T) {
	pool := New(1, 1)
	defer pool.Stop()

	pool.Blocking(func() {
		panic("some error")
	})

	counter := 0
	pool.Blocking(func() {
		counter++
	})

	assert.Equal(t, 1, counter)
}

// these tests are good enough for now - still the temporal dependency

func Test_WorkerPool_Grow_should_spin_up_at_least_one_new_worker(t *testing.T) {
	increased := 1
	pool := New(9, 1)
	defer pool.Stop()

	negativeOrZero := 0
	pool.Grow(negativeOrZero)

	expectedNumberOfWorkers := increased /* the one extra worker */ + 1 /* the default worker */
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_WorkerPool_Grow_should_spin_up_multiple_new_workers(t *testing.T) {
	increased := 10
	pool := New(9, 1)
	defer pool.Stop()

	pool.Grow(increased)

	expectedNumberOfWorkers := increased + 1
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_WorkerPool_Grow_should_stop_extra_workers_with_absolute_timeout(t *testing.T) {
	increased := 10
	absoluteTimeout := time.Millisecond * 10
	pool := New(9, 1)
	defer pool.Stop()

	pool.Grow(increased, WithAbsoluteTimeout(absoluteTimeout))

	expectedNumberOfWorkers := 1
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_WorkerPool_Grow_should_stop_extra_workers_with_idle_timeout_when_there_are_no_more_jobs(t *testing.T) {
	const n = 1000
	increased := 10
	idleTimeout := time.Millisecond * 50
	pool := New(100, 1)
	defer pool.Stop()

	start := make(chan struct{}, n)
	wg := &sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		go pool.SemiBlocking(func() {
			defer wg.Done()
			<-start
		})
	}

	pool.Grow(increased, WithIdleTimeout(idleTimeout))
	expectedNumberOfWorkers := 1 + increased
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))

	go func() {
		for i := 0; i < n; i++ {
			start <- struct{}{}
		}
	}()
	wg.Wait()

	expectedNumberOfWorkers = 1
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_WorkerPool_Grow_should_stop_extra_workers_with_explicit_stop_signal(t *testing.T) {
	increased := 10
	stopSignal := make(chan struct{})
	pool := New(9, 1)
	defer pool.Stop()

	pool.Grow(increased, WithStopSignal(stopSignal))

	expectedNumberOfWorkers := 1 + increased
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))

	close(stopSignal)
	expectedNumberOfWorkers = 1
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_WorkerPool_Grow_should_respawn_after_a_certain_number_of_requests(t *testing.T) {
	pool := New(9, 1, WithRespawnAfter(10))
	defer pool.Stop()

	expectedNumberOfStarts := 1 // one initial start
	assert.Eventually(t, func() bool {
		return expectedNumberOfStarts == getNumberOfStarts(pool)
	}, time.Millisecond*500, time.Millisecond*50)

	for i := 0; i < 11; i++ {
		pool.Blocking(func() {})
	}

	expectedNumberOfStarts = 2
	assert.Eventually(t, func() bool {
		return expectedNumberOfStarts == getNumberOfStarts(pool)
	}, time.Millisecond*500, time.Millisecond*50)
}

func Test_WorkerPool_Grow_should_respawn_after_a_certain_timespan_if_reapawnAfter_is_provided(t *testing.T) {
	pool := New(9, 1, WithRespawnAfter(1000), WithIdleTimeout(time.Millisecond*50))
	defer pool.Stop()

	time.Sleep(time.Millisecond * 190)
	expectedNumberOfStarts := 4
	assert.Equal(t, expectedNumberOfStarts, getNumberOfStarts(pool))
}

//

func Test_WorkerPool_Stop_should_close_the_pool(t *testing.T) {
	pool := New(9, 1)
	pool.Stop()

	assert.Panics(t, func() {
		pool.SemiBlocking(func() {})
	})
}

func Test_WorkerPool_Stop_should_stop_the_workers(t *testing.T) {
	pool := New(9, 1)

	increased := 10
	pool.Grow(increased)

	expectedNumberOfWorkers := 1 + increased
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))

	pool.Stop()

	assert.Panics(t, func() {
		pool.SemiBlocking(func() {})
	})

	expectedNumberOfWorkers = 0
	assert.Eventuallyf(t, func() bool {
		return expectedNumberOfWorkers == getNumberOfWorkers(pool)
	}, time.Millisecond*500, time.Millisecond*50,
		"expectedNumberOfWorkers: %v, actual: %v", expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

//

func getNumberOfWorkers(pool WorkerPool) int {
	accessWorkerPoolState.RLock()
	defer accessWorkerPoolState.RUnlock()

	return workerpoolStateWorkerCount[pool]
}

func getNumberOfStarts(pool WorkerPool) int {
	accessWorkerPoolState.RLock()
	defer accessWorkerPoolState.RUnlock()

	return workerpoolStateWorkerStartCount[pool]
}

func incNumberOfWorkers(pool WorkerPool, count int) {
	accessWorkerPoolState.Lock()
	defer accessWorkerPoolState.Unlock()

	workerpoolStateWorkerCount[pool] += count
	workerpoolStateWorkerStartCount[pool] += count
}

func decNumberOfWorkers(pool WorkerPool, count int) {
	accessWorkerPoolState.Lock()
	defer accessWorkerPoolState.Unlock()

	workerpoolStateWorkerCount[pool] -= count
}

var (
	workerpoolStateWorkerCount      = make(map[WorkerPool]int)
	workerpoolStateWorkerStartCount = make(map[WorkerPool]int)
	accessWorkerPoolState           = &sync.RWMutex{}
)

func ExampleWorkerPool_Blocking() {
	pool := New(1, 1)
	defer pool.Stop()

	var state int64
	job := func() { atomic.AddInt64(&state, 19) }

	pool.Blocking(job)

	fmt.Println(atomic.LoadInt64(&state))

	// Output:
	// 19
}

func ExampleWorkerPool_SemiBlocking() {
	pool := New(1, 1)
	defer pool.Stop()

	var state int64
	jobDone := make(chan struct{})
	job := func() {
		defer close(jobDone)
		atomic.AddInt64(&state, 19)
	}

	pool.SemiBlocking(job)
	<-jobDone

	fmt.Println(state)

	// Output:
	// 19
}

func ExampleWorkerPool_Grow() {
	const n = 19
	pool := New(10, 1)
	defer pool.Stop()

	pool.Grow(3) // spin up three new workers

	var state int64
	wg := &sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		pool.SemiBlocking(func() { defer wg.Done(); atomic.AddInt64(&state, 1) })
	}
	wg.Wait()

	fmt.Println(state)

	// Output:
	// 19
}
