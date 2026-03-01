// Package listener provides an abstraction over UDP reception for WeatherFlow hub messages.
// The Listener interface allows the daemon to be tested without real network I/O.
package listener

import "context"

// Listener receives raw UDP message bytes from a WeatherFlow Tempest hub.
type Listener interface {
	// ReadMessage blocks until a message is available or the context is cancelled.
	// Returns the raw JSON bytes of a single UDP datagram.
	ReadMessage(ctx context.Context) ([]byte, error)
}
