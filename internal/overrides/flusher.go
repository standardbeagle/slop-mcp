package overrides

import (
	"log/slog"
)

// flusher coalesces bank-dirty signals and writes in the background.
// A capacity-1 buffered channel is the coalescing primitive: a dropped
// send simply means a write is already scheduled.
type flusher struct {
	name  string
	write func() error
	dirty chan struct{}
	done  chan struct{}
	quit  chan struct{}
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
func (f *flusher) close() {
	close(f.quit)
	<-f.done
}

func (f *flusher) run() {
	defer close(f.done)
	for {
		select {
		case <-f.quit:
			// Drain one last pending signal if any.
			select {
			case <-f.dirty:
				if err := f.write(); err != nil {
					slog.Warn("flusher shutdown write", "bank", f.name, "err", err)
				}
			default:
			}
			return
		case <-f.dirty:
			if err := f.write(); err != nil {
				slog.Warn("flusher write", "bank", f.name, "err", err)
			}
		}
	}
}
