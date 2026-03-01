package listener

import (
	"context"
	"sync"
)

// Fake is an in-memory Listener for use in tests.
// It delivers a pre-loaded queue of messages then signals when drained,
// allowing deterministic test synchronisation without sleeps.
type Fake struct {
	mu       sync.Mutex
	messages [][]byte
	idx      int
	drained  chan struct{}
	once     sync.Once
}

// NewFake returns a Fake listener pre-loaded with the given messages.
func NewFake(messages ...[]byte) *Fake {
	return &Fake{
		messages: messages,
		drained:  make(chan struct{}),
	}
}

// ReadMessage returns the next queued message. Once all messages are consumed,
// it closes the Drained channel and then blocks until the context is cancelled.
func (f *Fake) ReadMessage(ctx context.Context) ([]byte, error) {
	f.mu.Lock()
	if f.idx < len(f.messages) {
		msg := f.messages[f.idx]
		f.idx++
		if f.idx == len(f.messages) {
			f.once.Do(func() { close(f.drained) })
		}
		f.mu.Unlock()
		return msg, nil
	}
	f.once.Do(func() { close(f.drained) })
	f.mu.Unlock()

	<-ctx.Done()
	return nil, ctx.Err()
}

// Drained returns a channel that is closed once all pre-loaded messages have
// been delivered. Tests can use this to cancel the daemon context at the right moment:
//
//	<-listener.Drained()
//	cancel()
func (f *Fake) Drained() <-chan struct{} {
	return f.drained
}
