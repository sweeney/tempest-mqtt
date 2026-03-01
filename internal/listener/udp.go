package listener

import (
	"context"
	"fmt"
	"net"
)

const maxUDPSize = 65536

// UDP listens for WeatherFlow hub broadcasts on a UDP port.
// Messages are read in a background goroutine and delivered via a buffered channel,
// allowing clean context-based cancellation without blocking on net.UDPConn.ReadFrom.
type UDP struct {
	ch   chan []byte
	errCh chan error
}

// NewUDP creates a UDP listener bound to the given port and starts reading
// datagrams in a background goroutine. The goroutine exits when the connection
// is closed; call Stop to tear down cleanly.
func NewUDP(port int) (*UDP, error) {
	addr := &net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("udp listen on port %d: %w", port, err)
	}

	l := &UDP{
		ch:    make(chan []byte, 16),
		errCh: make(chan error, 1),
	}
	go l.readLoop(conn)
	return l, nil
}

func (l *UDP) readLoop(conn *net.UDPConn) {
	defer conn.Close()
	buf := make([]byte, maxUDPSize)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			l.errCh <- fmt.Errorf("udp read: %w", err)
			return
		}
		msg := make([]byte, n)
		copy(msg, buf[:n])
		l.ch <- msg
	}
}

// ReadMessage blocks until a UDP datagram arrives or the context is cancelled.
func (l *UDP) ReadMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-l.ch:
		return data, nil
	case err := <-l.errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
