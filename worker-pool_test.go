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
	workerStarted = func(pool Workerpool) { incNumberOfWorkers(pool, 1) }
	workerStopped = func(pool Workerpool) { decNumberOfWorkers(pool, 1) }

	exitVal := m.Run()

	os.Exit(exitVal)
}

func Test_Workerpool_Blocking_should_set_default_mailbox_size_to_zero(t *testing.T) {
	pool := Init(-1)
	defer pool.Stop()

	assert.True(t, len(pool) == 0)
}

func Test_Workerpool_Blocking_should_serialize_the_jobs(t *testing.T) {
	const n = 1000

	pool := Init(10)
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

func Test_Workerpool_Nonblocking_should_just_put_job_in_the_mailbox(t *testing.T) {
	const n = 1000
	pool := Init(n)
	defer pool.Stop()

	var counter int64 = 0
	wg := &sync.WaitGroup{}

	wg.Add(n)
	for i := 0; i < n; i++ {
		pool.Nonblocking(func() {
			defer wg.Done()
			atomic.AddInt64(&counter, 1)
		})
	}
	wg.Wait()

	assert.Equal(t, int64(n), counter)
}

func Test_Workerpool_should_not_stop_because_of_panic(t *testing.T) {
	pool := Init(1)
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

func Test_Workerpool_Grow_should_spin_up_at_least_one_new_worker(t *testing.T) {
	increased := 1
	expectedNumberOfWorkers := increased /* the one extra worker */ + 1 /* the default worker */
	pool := Init(9)
	defer pool.Stop()

	negativeOrZero := 0
	pool.Grow(negativeOrZero)
	time.Sleep(time.Millisecond * 50)

	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_Workerpool_Grow_should_spin_up_multiple_new_workers(t *testing.T) {
	increased := 10
	expectedNumberOfWorkers := increased + 1
	pool := Init(9)
	defer pool.Stop()

	pool.Grow(increased)
	time.Sleep(time.Millisecond * 50)

	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_Workerpool_Grow_should_stop_extra_workers_with_absolute_timeout(t *testing.T) {
	increased := 10
	expectedNumberOfWorkers := 1
	absoluteTimeout := time.Millisecond * 10
	pool := Init(9)
	defer pool.Stop()

	pool.Grow(increased, WithAbsoluteTimeout(absoluteTimeout))
	time.Sleep(time.Millisecond * 50)

	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_Workerpool_Grow_should_stop_extra_workers_with_idle_timeout_when_there_are_no_more_jobs(t *testing.T) {
	const n = 1000
	increased := 10
	idleTimeout := time.Millisecond * 50
	pool := Init(100)
	defer pool.Stop()

	start := make(chan struct{}, n)
	wg := &sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		go pool.Nonblocking(func() {
			defer wg.Done()
			<-start
		})
	}

	pool.Grow(increased, WithIdleTimeout(idleTimeout))
	expectedNumberOfWorkers := 1 + increased
	for i := 0; i < 3; i++ {
		if expectedNumberOfWorkers > getNumberOfWorkers(pool) {
			time.Sleep(time.Millisecond * 50)
		}
	}
	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))

	go func() {
		for i := 0; i < n; i++ {
			start <- struct{}{}
		}
	}()
	wg.Wait()

	expectedNumberOfWorkers = 1
	for i := 0; i < 3; i++ {
		if expectedNumberOfWorkers < getNumberOfWorkers(pool) {
			time.Sleep(time.Millisecond * 50)
		}
	}
	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_Workerpool_Grow_should_stop_extra_workers_with_explicit_stop_signal(t *testing.T) {
	increased := 10
	stopSignal := make(chan struct{})
	pool := Init(9)
	defer pool.Stop()

	pool.Grow(increased, WithStopSignal(stopSignal))

	expectedNumberOfWorkers := 1 + increased
	for i := 0; i < 3; i++ {
		if expectedNumberOfWorkers > getNumberOfWorkers(pool) {
			time.Sleep(time.Millisecond * 50)
		}
	}
	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))

	close(stopSignal)
	expectedNumberOfWorkers = 1
	for i := 0; i < 3; i++ {
		if expectedNumberOfWorkers < getNumberOfWorkers(pool) {
			time.Sleep(time.Millisecond * 50)
		}
	}
	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

func Test_Workerpool_Grow_should_respawn_after_a_certain_number_of_requests(t *testing.T) {
	pool := Init(9, WithRespawnAfter(10))
	defer pool.Stop()

	time.Sleep(time.Millisecond * 50)
	assert.Equal(t, 1, getNumberOfStarts(pool)) // one start

	for i := 0; i < 11; i++ {
		pool.Blocking(func() {})
	}

	time.Sleep(time.Millisecond * 50)
	assert.Equal(t, 2, getNumberOfStarts(pool))
}

func Test_Workerpool_Grow_should_respawn_after_a_certain_timespan_if_reapawnAfter_is_provided(t *testing.T) {
	pool := Init(9, WithRespawnAfter(1000), WithIdleTimeout(time.Millisecond*50))
	defer pool.Stop()

	time.Sleep(time.Millisecond * 190)
	assert.Equal(t, 4, getNumberOfStarts(pool))
}

//

func Test_Workerpool_Stop_should_close_the_pool(t *testing.T) {
	pool := Init(9)
	pool.Stop()

	assert.Panics(t, func() {
		pool.Nonblocking(func() {})
	})
}

func Test_Workerpool_Stop_should_stop_the_workers(t *testing.T) {
	pool := Init(9)

	increased := 10
	pool.Grow(increased)

	expectedNumberOfWorkers := 1 + increased
	for i := 0; i < 3; i++ {
		if expectedNumberOfWorkers > getNumberOfWorkers(pool) {
			time.Sleep(time.Millisecond * 50)
		}
	}
	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))

	pool.Stop()

	assert.Panics(t, func() {
		pool.Nonblocking(func() {})
	})

	expectedNumberOfWorkers = 0
	for i := 0; i < 3; i++ {
		if expectedNumberOfWorkers < getNumberOfWorkers(pool) {
			time.Sleep(time.Millisecond * 50)
		}
	}
	assert.Equal(t, expectedNumberOfWorkers, getNumberOfWorkers(pool))
}

//

func Test_initialWorkerOptions(t *testing.T) {
	t.Run(`ignores absolute timeout`, func(t *testing.T) {
		options := initialWorkerOptions(WithAbsoluteTimeout(time.Minute))

		assert.Equal(t, time.Duration(0), options.absoluteTimeout)
	})

	t.Run(`ignores stop signal`, func(t *testing.T) {
		options := initialWorkerOptions(WithStopSignal(make(chan struct{})))

		var zeroStopSignal <-chan struct{}
		assert.Equal(t, zeroStopSignal, options.stopSignal)
	})

	t.Run(`ignores idle timeout if no respawn count is provided`, func(t *testing.T) {
		options := initialWorkerOptions(WithIdleTimeout(time.Minute))

		assert.Equal(t, time.Duration(0), options.idleTimeout)
	})

	t.Run(`does not ignore idle timeout if respawn count is provided`, func(t *testing.T) {
		options := initialWorkerOptions(WithIdleTimeout(time.Minute), WithRespawnAfter(1000))

		assert.Equal(t, time.Minute, options.idleTimeout)
	})
}

//

func getNumberOfWorkers(pool Workerpool) int {
	accessWorkerpoolState.RLock()
	defer accessWorkerpoolState.RUnlock()

	return workerpoolStateWorkerCount[pool]
}

func getNumberOfStarts(pool Workerpool) int {
	accessWorkerpoolState.RLock()
	defer accessWorkerpoolState.RUnlock()

	return workerpoolStateWorkerStartCount[pool]
}

func incNumberOfWorkers(pool Workerpool, count int) {
	accessWorkerpoolState.Lock()
	defer accessWorkerpoolState.Unlock()

	workerpoolStateWorkerCount[pool] += count
	workerpoolStateWorkerStartCount[pool] += count
}

func decNumberOfWorkers(pool Workerpool, count int) {
	accessWorkerpoolState.Lock()
	defer accessWorkerpoolState.Unlock()

	workerpoolStateWorkerCount[pool] -= count
}

var (
	workerpoolStateWorkerCount      = make(map[Workerpool]int)
	workerpoolStateWorkerStartCount = make(map[Workerpool]int)
	accessWorkerpoolState           = &sync.RWMutex{}
)

func ExampleWorkerpool_Blocking() {
	pool := Init(1)
	defer pool.Stop()

	var state int64
	job := func() { atomic.AddInt64(&state, 19) }

	pool.Blocking(job)

	fmt.Println(state)

	// Output:
	// 19
}

func ExampleWorkerpool_Nonblocking() {
	pool := Init(1)
	defer pool.Stop()

	var state int64
	jobDone := make(chan struct{})
	job := func() {
		defer close(jobDone)
		atomic.AddInt64(&state, 19)
	}

	pool.Nonblocking(job)
	<-jobDone

	fmt.Println(state)

	// Output:
	// 19
}

func ExampleWorkerpool_Grow() {
	const n = 19
	pool := Init(10)
	defer pool.Stop()

	pool.Grow(3) // spin up three new workers

	var state int64
	wg := &sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		pool.Nonblocking(func() { defer wg.Done(); atomic.AddInt64(&state, 1) })
	}
	wg.Wait()

	fmt.Println(state)

	// Output:
	// 19
}
