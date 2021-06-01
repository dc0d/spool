// see LICENSE file

package spool

import (
	"log"
	"time"
)

type Workerpool chan func()

// Init creates a new workerpool which has one default worker (the minimum number of workers is one).
// To have more workers, the Grow method should be used.
// For initial worker absolute timeout and stop signal are ignored.
// Also, idle timeout is ignored, if no respawnAfter is provided.
func Init(mailboxSize MailboxSize, opts ...GrowthOption) Workerpool {
	if mailboxSize < 0 {
		mailboxSize = 0
	}

	var pool Workerpool = make(chan func(), mailboxSize)
	pool.start(initialWorkerOptions(opts...))

	return pool
}

func initialWorkerOptions(opts ...GrowthOption) growthOptions {
	options := applyOptions(opts...)
	options.absoluteTimeout = 0
	options.stopSignal = nil
	if options.respawnAfter == 0 && options.idleTimeout > 0 {
		options.idleTimeout = 0
	}
	return options
}

func (pool Workerpool) Stop() {
	close(pool)
}

// Blocking will panic, if the workerpool is stopped.
func (pool Workerpool) Blocking(callback func()) {
	done := make(chan struct{})
	pool <- func() { defer close(done); callback() }
	<-done
}

// SemiBlocking sends the job to the worker in a non-blocking manner, as long as the mailbox is not full.
// After that, it becomes blocking until there is an empty space in the mailbox.
// If the workerpool is stopped, SemiBlocking will panic.
func (pool Workerpool) SemiBlocking(callback func()) {
	pool <- callback
}

func (pool Workerpool) Grow(growth int, opts ...GrowthOption) {
	options := applyOptions(opts...)

	if growth <= 0 {
		growth = 1
	}

	for i := 0; i < growth; i++ {
		pool.start(options)
	}
}

func (pool Workerpool) start(options growthOptions) {
	go pool.worker(options)
}

func (pool Workerpool) worker(options growthOptions) {
	if workerStarted != nil {
		workerStarted(pool)
	}
	if workerStopped != nil {
		defer workerStopped(pool)
	}

	var (
		absoluteTimeout = options.absoluteTimeout
		idleTimeout     = options.idleTimeout
		stopSignal      = options.stopSignal
	)

	var absoluteTimeoutSignal, idleTimeoutSignal <-chan time.Time
	if absoluteTimeout > 0 {
		absoluteTimeoutSignal = time.After(absoluteTimeout)
	}

	var requestCount RequestCount
	for {
		if options.respawnAfter > 0 && options.respawnAfter <= requestCount {
			pool.start(options)
			return
		}

		if idleTimeout > 0 {
			idleTimeoutSignal = time.After(idleTimeout)
		}

		select {
		case <-absoluteTimeoutSignal:
			return
		case <-idleTimeoutSignal:
			if options.respawnAfter > 0 {
				pool.start(options)
			}
			return
		case <-stopSignal:
			return
		case callback, ok := <-pool:
			if !ok {
				return
			}
			execCallback(callback)
			requestCount++
		}
	}
}

func execCallback(callback func()) {
	defer func() {
		if e := recover(); e != nil {
			log.Println(e) // TODO:
		}
	}()

	callback()
}

var (
	workerStarted func(pool Workerpool)
	workerStopped func(pool Workerpool)
)

// growth options

func WithAbsoluteTimeout(timeout time.Duration) GrowthOption {
	return func(opts growthOptions) growthOptions { opts.absoluteTimeout = timeout; return opts }
}

func WithIdleTimeout(timeout time.Duration) GrowthOption {
	return func(opts growthOptions) growthOptions { opts.idleTimeout = timeout; return opts }
}

func WithStopSignal(stopSignal <-chan struct{}) GrowthOption {
	return func(opts growthOptions) growthOptions { opts.stopSignal = stopSignal; return opts }
}

func WithRespawnAfter(respawnAfter RequestCount) GrowthOption {
	return func(opts growthOptions) growthOptions { opts.respawnAfter = respawnAfter; return opts }
}

type GrowthOption func(growthOptions) growthOptions

type growthOptions struct {
	absoluteTimeout time.Duration
	idleTimeout     time.Duration
	stopSignal      <-chan struct{}
	respawnAfter    RequestCount
}

func applyOptions(opts ...GrowthOption) growthOptions {
	var options growthOptions
	for _, fn := range opts {
		options = fn(options)
	}
	return options
}

type (
	RequestCount int
	MailboxSize  int
)
