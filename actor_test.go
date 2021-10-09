//go:generate moq -out callbacks_spy_test.go . Callbacks:CallbacksSpy
// see LICENSE file

// install moq:
// $ go install github.com/matryer/moq@latest

package spool

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	started = func(box Mailbox) {
		accessMailboxState.Lock()
		defer accessMailboxState.Unlock()
		mailboxStateWorkerStartCount[box]++
	}
	stopped = func(box Mailbox) {
		accessMailboxState.Lock()
		defer accessMailboxState.Unlock()
		mailboxStateWorkerStopCount[box]++
	}

	exitVal := m.Run()

	os.Exit(exitVal)
}

func Test_should_call_received_concurrently(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	callbacks.ReceivedFunc = func(T) {}

	Start(context.TODO(), mailbox, callbacks)

	ids := make(chan int, 6)
	fn1 := func() { ids <- 1 }
	fn2 := func() { ids <- 2 }
	fn3 := func() { ids <- 3 }

	mailbox <- fn1
	mailbox <- fn2
	mailbox <- fn3

	assert.Eventually(t, func() bool { return len(callbacks.ReceivedCalls()) == 3 },
		time.Millisecond*300, time.Millisecond*20)

	fn1()
	callbacks.ReceivedCalls()[0].Fn()
	assert.EqualValues(t, <-ids, <-ids)

	fn2()
	callbacks.ReceivedCalls()[1].Fn()
	assert.EqualValues(t, <-ids, <-ids)

	fn3()
	callbacks.ReceivedCalls()[2].Fn()
	assert.EqualValues(t, <-ids, <-ids)
}

func Test_should_call_stopped_when_context_is_canceled(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	ctx, cancel := context.WithCancel(context.Background())
	callbacks.StoppedFunc = func() {}

	Start(ctx, mailbox, callbacks)
	cancel()

	assert.Eventually(t, func() bool { return len(callbacks.StoppedCalls()) == 1 },
		time.Millisecond*300, time.Millisecond*20)
}

func Test_should_not_call_received_when_context_is_canceled(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	ctx, cancel := context.WithCancel(context.Background())
	callbacks.StoppedFunc = func() {}

	Start(ctx, mailbox, callbacks)
	cancel()

	assert.Eventually(t, func() bool { return len(callbacks.StoppedCalls()) == 1 },
		time.Millisecond*300, time.Millisecond*20)

	sendingStarted := make(chan struct{})
	go func() {
		close(sendingStarted)
		mailbox <- func() {}
	}()
	<-sendingStarted

	assert.Never(t, func() bool { return len(callbacks.ReceivedCalls()) > 0 },
		time.Millisecond*300, time.Millisecond*20)
}

func Test_should_stop_after_absolute_timeout(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	callbacks.StoppedFunc = func() {}

	Start(context.Background(), mailbox, callbacks, WithAbsoluteTimeout(time.Millisecond*50))

	assert.Eventually(t, func() bool { return len(callbacks.StoppedCalls()) == 1 },
		time.Millisecond*300, time.Millisecond*20)
}

func Test_should_stop_when_mailbox_is_closed(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	callbacks.StoppedFunc = func() {}

	Start(context.Background(), mailbox, callbacks)
	close(mailbox)

	assert.Eventually(t, func() bool { return len(callbacks.StoppedCalls()) == 1 },
		time.Millisecond*300, time.Millisecond*20)
	assert.Equal(t, 0, len(callbacks.ReceivedCalls()))
}

func Test_should_stop_after_idle_timeout_elapsed(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	callbacks.StoppedFunc = func() {}

	Start(context.Background(), mailbox, callbacks, WithIdleTimeout(time.Millisecond*100))

	assert.Never(t, func() bool { return len(callbacks.StoppedCalls()) > 0 },
		time.Millisecond*100, time.Millisecond*20)

	assert.Eventually(t, func() bool { return len(callbacks.StoppedCalls()) == 1 },
		time.Millisecond*300, time.Millisecond*20)
}

func Test_should_respawn_after_receiving_n_messages(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	callbacks.StoppedFunc = func() {}
	callbacks.ReceivedFunc = func(T) {}

	Start(context.Background(), mailbox, callbacks, WithRespawnAfter(10))

	go func() {
		for i := 0; i < 20; i++ {
			mailbox <- func() {}
		}
	}()

	assert.Eventually(t, func() bool { return assert.EqualValues(t, 3, getNumberOfStarts(mailbox)) },
		time.Millisecond*300, time.Millisecond*20)
	assert.Eventually(t, func() bool { return assert.EqualValues(t, 2, getNumberOfStops(mailbox)) },
		time.Millisecond*300, time.Millisecond*20)
}

func Test_should_not_respawn_if_not_provided(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	callbacks.StoppedFunc = func() {}
	callbacks.ReceivedFunc = func(T) {}

	Start(context.Background(), mailbox, callbacks)

	go func() {
		for i := 0; i < 20; i++ {
			mailbox <- func() {}
		}
	}()

	assert.Eventually(t, func() bool { return assert.EqualValues(t, 1, getNumberOfStarts(mailbox)) },
		time.Millisecond*300, time.Millisecond*20)
	assert.Eventually(t, func() bool { return assert.EqualValues(t, 0, getNumberOfStops(mailbox)) },
		time.Millisecond*300, time.Millisecond*20)
}

func Test_should_respawn_after_idle_timeout_elapsed_if_respawn_count_is_provided(t *testing.T) {
	var (
		mailbox   = make(chan T)
		callbacks = &CallbacksSpy{}
	)
	callbacks.StoppedFunc = func() {}

	Start(context.Background(), mailbox, callbacks,
		WithIdleTimeout(time.Millisecond*100),
		WithRespawnAfter(100))

	assert.Eventually(t, func() bool { return getNumberOfStarts(mailbox) == 2 },
		time.Millisecond*300, time.Millisecond*20)
	assert.Eventually(t, func() bool { return getNumberOfStops(mailbox) == 1 },
		time.Millisecond*300, time.Millisecond*20)

	assert.Never(t, func() bool { return len(callbacks.StoppedCalls()) > 0 },
		time.Millisecond*100, time.Millisecond*20)
}

func getNumberOfStarts(box Mailbox) int {
	accessMailboxState.Lock()
	defer accessMailboxState.Unlock()

	return mailboxStateWorkerStartCount[box]
}

func getNumberOfStops(box Mailbox) int {
	accessMailboxState.Lock()
	defer accessMailboxState.Unlock()

	return mailboxStateWorkerStopCount[box]
}

var (
	mailboxStateWorkerStopCount  = make(map[Mailbox]int)
	mailboxStateWorkerStartCount = make(map[Mailbox]int)
	accessMailboxState           = &sync.Mutex{}
)
