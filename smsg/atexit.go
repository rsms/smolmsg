// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
)

type ExitHandler = func(context.Context) error

var (
	ExitCh chan struct{} // closes when all exit handlers have completed

	sigch          chan os.Signal
	exitExitCode   = 0
	exitHandlersMu sync.Mutex // protects exitHandlers
	exitHandlers   []ExitHandler
	exitTimeouts   = map[os.Signal]time.Duration{}
	exitSignals    = []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM}
)

const defaultExitTimeout = 5 * time.Second

func init() {
	ExitCh = make(chan struct{})
	sigch = make(chan os.Signal, 1)

	for _, sig := range exitSignals {
		exitTimeouts[sig] = defaultExitTimeout
	}

	signal.Notify(sigch, exitSignals...)
	go func() {
		sig := <-sigch

		// reset signal handler so that a second signal has the default effect
		signal.Reset(exitSignals...)

		// log that we are shutting down
		dlog("shutting down...")

		// create context for shutdown
		timeout, ok := exitTimeouts[sig]
		if !ok {
			timeout = defaultExitTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel() // just in case

		// Note: copy and don't lock to avoid deadlock in case a handler calls RegisterExitHandler
		handlers := exitHandlers[:]
		fnch := make(chan struct{}) // "done" signals
		exitCode := exitExitCode

		// invoke all shutdown handlers in goroutines
		for _, fn := range handlers {
			go func(fn ExitHandler) {
				defer func() {
					if r := recover(); r != nil {
						errlog("panic in RegisterExitHandler function: %v\n", r)
						if DEBUG {
							debug.PrintStack()
						}
						// cancel the shutdown context
						cancel()
					}
				}()

				// invoke handler and log error
				if err := fn(ctx); err != nil {
					if err != context.DeadlineExceeded && err != context.Canceled {
						errlog("RegisterExitHandler function: %v", err)
					}
					// cancel the shutdown context
					cancel()
				} else {
					// signal to outer function that this handler has completed
					fnch <- struct{}{}
				}
			}(fn)
		}

		// wait for all shutdown handler goroutines to finish
	wait_loop:
		for range handlers {
			select {
			case <-fnch: // ok
			case <-ctx.Done():
				// Context canceled
				if ctx.Err() == context.DeadlineExceeded {
					warnlog("shutdown timeout (%s)", timeout)
				}
				if exitCode == 0 {
					exitCode = 1
				}
				break wait_loop
			}
		}

		// finished
		os.Exit(exitCode)
	}()
}

// Shutdown is like os.Exit but invokes shutdown handlers before exiting
func Shutdown(exitCode int) {
	exitExitCode = exitCode
	close(sigch)
	<-ExitCh // never returns
}

func SetExitTimeout(timeout time.Duration, onlySignals ...os.Signal) {
	if onlySignals == nil {
		onlySignals = exitSignals
	}
	for _, sig := range onlySignals {
		if _, ok := exitTimeouts[sig]; ok {
			exitTimeouts[sig] = timeout
		}
	}
}

func GetExitTimeout(signal os.Signal) time.Duration {
	if signal != nil {
		if timeout, ok := exitTimeouts[signal]; ok {
			return timeout
		}
	}
	return defaultExitTimeout
}

// RegisterExitHandler adds a function to be called during program shutdown.
// The following signatures are accepted:
//   func(context.Context)error
//   func(context.Context)
//   func()error
//   func()
//
// A panics inside a handler will cause the context to be cancelled and the
// panic reported. I.e. a panic inside an exit handler is isolated to that handler
// but "speeds up" shutdown.
//
func RegisterExitHandler(handlerFunc interface{}) {
	var fn ExitHandler
	if f, ok := handlerFunc.(ExitHandler); ok {
		fn = f
	} else if f, ok := handlerFunc.(func(context.Context)); ok {
		fn = func(ctx context.Context) error {
			// log.Info("atexit synthetic handler %p for %p invoked", fn, handlerFunc)
			f(ctx)
			return nil
		}
	} else if f, ok := handlerFunc.(func() error); ok {
		fn = func(_ context.Context) error {
			// log.Info("atexit synthetic handler %p for %p invoked", fn, handlerFunc)
			return f()
		}
	} else if f, ok := handlerFunc.(func()); ok {
		fn = func(_ context.Context) error {
			// log.Info("atexit synthetic handler %p for %p invoked", fn, handlerFunc)
			f()
			return nil
		}
	} else {
		panic("invalid handler signature (see RegisterExitHandler documentation)")
	}
	exitHandlersMu.Lock()
	defer exitHandlersMu.Unlock()
	exitHandlers = append(exitHandlers, fn)
}
