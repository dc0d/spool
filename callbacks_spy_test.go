// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package spool

import (
	"sync"
)

// Ensure, that CallbacksSpy does implement Callbacks.
// If this is not the case, regenerate this file with moq.
var _ Callbacks = &CallbacksSpy{}

// CallbacksSpy is a mock implementation of Callbacks.
//
// 	func TestSomethingThatUsesCallbacks(t *testing.T) {
//
// 		// make and configure a mocked Callbacks
// 		mockedCallbacks := &CallbacksSpy{
// 			ReceivedFunc: func(fn func())  {
// 				panic("mock out the Received method")
// 			},
// 			StoppedFunc: func()  {
// 				panic("mock out the Stopped method")
// 			},
// 		}
//
// 		// use mockedCallbacks in code that requires Callbacks
// 		// and then make assertions.
//
// 	}
type CallbacksSpy struct {
	// ReceivedFunc mocks the Received method.
	ReceivedFunc func(fn func())

	// StoppedFunc mocks the Stopped method.
	StoppedFunc func()

	// calls tracks calls to the methods.
	calls struct {
		// Received holds details about calls to the Received method.
		Received []struct {
			// Fn is the fn argument value.
			Fn func()
		}
		// Stopped holds details about calls to the Stopped method.
		Stopped []struct {
		}
	}
	lockReceived sync.RWMutex
	lockStopped  sync.RWMutex
}

// Received calls ReceivedFunc.
func (mock *CallbacksSpy) Received(fn func()) {
	if mock.ReceivedFunc == nil {
		panic("CallbacksSpy.ReceivedFunc: method is nil but Callbacks.Received was just called")
	}
	callInfo := struct {
		Fn func()
	}{
		Fn: fn,
	}
	mock.lockReceived.Lock()
	mock.calls.Received = append(mock.calls.Received, callInfo)
	mock.lockReceived.Unlock()
	mock.ReceivedFunc(fn)
}

// ReceivedCalls gets all the calls that were made to Received.
// Check the length with:
//     len(mockedCallbacks.ReceivedCalls())
func (mock *CallbacksSpy) ReceivedCalls() []struct {
	Fn func()
} {
	var calls []struct {
		Fn func()
	}
	mock.lockReceived.RLock()
	calls = mock.calls.Received
	mock.lockReceived.RUnlock()
	return calls
}

// Stopped calls StoppedFunc.
func (mock *CallbacksSpy) Stopped() {
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
//     len(mockedCallbacks.StoppedCalls())
func (mock *CallbacksSpy) StoppedCalls() []struct {
} {
	var calls []struct {
	}
	mock.lockStopped.RLock()
	calls = mock.calls.Stopped
	mock.lockStopped.RUnlock()
	return calls
}