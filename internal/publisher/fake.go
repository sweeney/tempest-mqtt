package publisher

import "sync"

// Message records a single call to Fake.Publish.
type Message struct {
	Topic   string
	Payload []byte
	QoS     byte
	Retain  bool
}

// Fake is an in-memory Publisher for use in tests.
// It records all published messages and can be configured to return an error.
type Fake struct {
	mu       sync.Mutex
	messages []Message
	err      error
}

// NewFakeWithError returns a Fake that always returns err from Publish.
func NewFakeWithError(err error) *Fake {
	return &Fake{err: err}
}

// Publish records the message. Returns the configured error, or nil.
func (f *Fake) Publish(topic string, payload []byte, qos byte, retain bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	msg := Message{
		Topic:   topic,
		Payload: make([]byte, len(payload)),
		QoS:     qos,
		Retain:  retain,
	}
	copy(msg.Payload, payload)
	f.messages = append(f.messages, msg)
	return nil
}

// Messages returns a snapshot of all published messages in order.
func (f *Fake) Messages() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Message, len(f.messages))
	copy(out, f.messages)
	return out
}

// Reset clears all recorded messages.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = nil
}
