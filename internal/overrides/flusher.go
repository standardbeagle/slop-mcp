package overrides

import (
	"fmt"
	"log/slog"
	"sync"
)

// flusher coalesces bank-dirty signals and writes in the background.
// A capacity-1 buffered channel is the coalescing primitive: a dropped
// send simply means a write is already scheduled.
type flusher struct {
	name    string
	write   func() error
	dirty   chan struct{}
	done    chan struct{}
	quit    chan struct{}
	errMu   sync.Mutex
	lastErr error
}

func newFlusher(name string, write func() error) *flusher {
	f := &flusher{
		name:  name,
		write: write,
		dirty: make(chan struct{}, 1),
		done:  make(chan struct{}),
		quit:  make(chan struct{}),
	}
	go f.run()
	return f
}

// markDirty signals the bank has pending changes.
// Non-blocking: if a signal is already pending, this call is dropped.
func (f *flusher) markDirty() {
	select {
	case f.dirty <- struct{}{}:
	default:
	}
}

// close flushes any pending signal and stops the goroutine.
func (f *flusher) close() error {
	close(f.quit)
	<-f.done
	f.errMu.Lock()
	defer f.errMu.Unlock()
	return f.lastErr
}

func (f *flusher) run() {
	defer close(f.done)
	for {
		select {
		case <-f.quit:
			// Drain one last pending signal if any.
			select {
			case <-f.dirty:
				f.flush("flusher shutdown write")
			default:
			}
			return
		case <-f.dirty:
			f.flush("flusher write")
		}
	}
}

func (f *flusher) flush(logMessage string) {
	err := f.write()
	f.errMu.Lock()
	if err != nil {
		f.lastErr = fmt.Errorf("%s: %w", f.name, err)
	} else {
		f.lastErr = nil
	}
	f.errMu.Unlock()
	if err != nil {
		slog.Warn(logMessage, "bank", f.name, "err", err)
	}
}
