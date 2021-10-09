// see LICENSE file

package spool

import (
	"context"
	"time"
)

func Start(ctx context.Context, mailbox Mailbox, callbacks Callbacks, options ...Option) {
	opts := applyOptions(options...)
	start(ctx, mailbox, callbacks, opts)
}

func start(ctx context.Context, mailbox Mailbox, callbacks Callbacks, opts actorOptions) {
	go func() {
		if started != nil {
			started(mailbox)
		}
		if stopped != nil {
			defer stopped(mailbox)
		}

		var (
			absoluteTimeout = opts.absoluteTimeout
			idleTimeout     = opts.idleTimeout
		)

		var absoluteTimeoutSignal, idleTimeoutSignal <-chan time.Time
		if absoluteTimeout > 0 {
			absoluteTimeoutSignal = time.After(absoluteTimeout)
		}

		var requestCount RequestCount
		for {
			if requestCount > 0 && opts.respawnAfter > 0 && opts.respawnAfter <= requestCount {
				start(ctx, mailbox, callbacks, opts)
				return
			}

			if idleTimeout > 0 {
				idleTimeoutSignal = time.After(idleTimeout)
			}

			select {
			case <-absoluteTimeoutSignal:
				callbacks.Stopped()
				return
			case <-idleTimeoutSignal:
				if opts.respawnAfter > 0 {
					start(ctx, mailbox, callbacks, opts)
					return
				}
				callbacks.Stopped()
				return
			case <-ctx.Done():
				callbacks.Stopped()
				return
			case v, ok := <-mailbox:
				if !ok {
					callbacks.Stopped()
					return
				}
				callbacks.Received(v)
				requestCount++
			}
		}
	}()
}

type (
	Callbacks interface {
		Received(T)
		Stopped()
	}

	Mailbox <-chan T

	Option func(actorOptions) actorOptions

	actorOptions struct {
		absoluteTimeout time.Duration
		idleTimeout     time.Duration
		respawnAfter    RequestCount
	}

	RequestCount int
	MailboxSize  int

	T = func()
)

func WithAbsoluteTimeout(timeout time.Duration) Option {
	return func(opts actorOptions) actorOptions { opts.absoluteTimeout = timeout; return opts }
}

func WithIdleTimeout(timeout time.Duration) Option {
	return func(opts actorOptions) actorOptions { opts.idleTimeout = timeout; return opts }
}

func WithRespawnAfter(respawnAfter RequestCount) Option {
	return func(opts actorOptions) actorOptions { opts.respawnAfter = respawnAfter; return opts }
}

func applyOptions(opts ...Option) actorOptions {
	var options actorOptions
	for _, fn := range opts {
		options = fn(options)
	}
	return options
}

var (
	started func(pool Mailbox)
	stopped func(pool Mailbox)
)
