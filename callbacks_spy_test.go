package spool

import (
	"sync"

	"github.com/dc0d/actor"
)

var _ actor.Callbacks[int] = &CallbacksSpy[int]{}

type CallbacksSpy[T any] struct {
	// ReceivedFunc mocks the Received method.
	ReceivedFunc func(ifaceVal T)

	// StoppedFunc mocks the Stopped method.
	StoppedFunc func()

	// calls tracks calls to the methods.
	calls struct {
		// Received holds details about calls to the Received method.
		Received []struct {
			// IfaceVal is the ifaceVal argument value.
			IfaceVal T
		}
		// Stopped holds details about calls to the Stopped method.
		Stopped []struct {
		}
	}
	lockReceived sync.RWMutex
	lockStopped  sync.RWMutex
}

// Received calls ReceivedFunc.
func (mock *CallbacksSpy[T]) Received(ifaceVal T) {
	if mock.ReceivedFunc == nil {
		panic("CallbacksSpy.ReceivedFunc: method is nil but Callbacks.Received was just called")
	}
	callInfo := struct {
		IfaceVal T
	}{
		IfaceVal: ifaceVal,
	}
	mock.lockReceived.Lock()
	mock.calls.Received = append(mock.calls.Received, callInfo)
	mock.lockReceived.Unlock()
	mock.ReceivedFunc(ifaceVal)
}

// ReceivedCalls gets all the calls that were made to Received.
// Check the length with:
//
//	len(mockedCallbacks.ReceivedCalls())
func (mock *CallbacksSpy[T]) ReceivedCalls() []struct {
	IfaceVal T
} {
	var calls []struct {
		IfaceVal T
	}
	mock.lockReceived.RLock()
	calls = mock.calls.Received
	mock.lockReceived.RUnlock()
	return calls
}

// Stopped calls StoppedFunc.
func (mock *CallbacksSpy[T]) Stopped() {
	if mock.StoppedFunc == nil {
		panic("CallbacksSpy.StoppedFunc: method is nil but Callbacks.Stopped was just called")
	}
	callInfo := struct {
	}{}
	mock.lockStopped.Lock()
	mock.calls.Stopped = append(mock.calls.Stopped, callInfo)
	mock.lockStopped.Unlock()
	mock.StoppedFunc()
}

// StoppedCalls gets all the calls that were made to Stopped.
// Check the length with:
//
//	len(mockedCallbacks.StoppedCalls())
func (mock *CallbacksSpy[T]) StoppedCalls() []struct {
} {
	var calls []struct {
	}
	mock.lockStopped.RLock()
	calls = mock.calls.Stopped
	mock.lockStopped.RUnlock()
	return calls
}
