// see LICENSE file

package spool

import (
	"context"
	"log"

	"github.com/dc0d/spool/actor"
)

type WorkerPool chan func()

// New creates a new WorkerPool without any initial workers. To spawn workers, Grow must be called.
func New(mailboxSize actor.MailboxSize) WorkerPool {
	if mailboxSize < 0 {
		mailboxSize = 0
	}
	return make(chan func(), mailboxSize)
}

func (pool WorkerPool) Stop() {
	close(pool)
}

// Blocking will panic, if the workerpool is stopped.
func (pool WorkerPool) Blocking(ctx context.Context, callback func()) error {
	done := make(chan struct{})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case pool <- func() { defer close(done); callback() }:
	}
	<-done
	return nil
}

// SemiBlocking sends the job to the worker in a non-blocking manner, as long as the mailbox is not full.
// After that, it becomes blocking until there is an empty space in the mailbox.
// If the workerpool is stopped, SemiBlocking will panic.
func (pool WorkerPool) SemiBlocking(ctx context.Context, callback func()) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case pool <- callback:
	}
	return nil
}

func (pool WorkerPool) Grow(ctx context.Context, growth int, options ...actor.Option) {
	pool.grow(ctx, growth, nil, options...)
}

func (pool WorkerPool) grow(ctx context.Context, growth int, executorFactory func() actor.Callbacks[T], options ...actor.Option) {
	var mailbox <-chan func() = pool
	for i := 0; i < growth; i++ {
		var exec actor.Callbacks[T] = defaultExecutor{}
		if executorFactory != nil {
			exec = executorFactory()
		}
		actor.Start(ctx, mailbox, exec, options...)
	}
}

type defaultExecutor struct{}

func (obj defaultExecutor) Received(fn T) {
	defer func() {
		if e := recover(); e != nil {
			log.Println(e) // TODO:
		}
	}()
	fn()
}

func (obj defaultExecutor) Stopped() {}

type (
	T = func()
)
