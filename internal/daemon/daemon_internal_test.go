// Internal tests (package daemon) have access to unexported fields, allowing:
//   - The dispatch() error path to be exercised with an unknown message type.
//   - The Run() dispatch-error log branch to be covered by injecting a failing converter.
package daemon

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sweeney/tempest-mqtt/internal/event"
	"github.com/sweeney/tempest-mqtt/internal/listener"
	"github.com/sweeney/tempest-mqtt/internal/parser"
	"github.com/sweeney/tempest-mqtt/internal/publisher"
)

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func fixtureDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "tests", "fixtures")
}

func loadFixtureInternal(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir(), name))
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	return data
}

// unknownMessage implements parser.Message but is not known to event.FromMessage.
type unknownMessage struct{}

func (u *unknownMessage) Type() string { return "unknown_internal_test_type" }

// TestDispatch_UnknownMessage_ReturnsError exercises the dispatch() error return
// path, which requires injecting a parser.Message that event.FromMessage rejects.
func TestDispatch_UnknownMessage_ReturnsError(t *testing.T) {
	d := &Daemon{
		publisher: &publisher.Fake{},
		log:       discardLog(),
		convert:   event.FromMessage,
	}
	err := d.dispatch(&unknownMessage{})
	if err == nil {
		t.Fatal("dispatch() expected error for unknown message type, got nil")
	}
}

// TestRun_DispatchError_LogsAndContinues exercises the dispatch-error log branch
// in Run() by injecting a converter that fails on the first call. The daemon should
// log the error and continue, processing subsequent messages normally.
func TestRun_DispatchError_LogsAndContinues(t *testing.T) {
	valid := loadFixtureInternal(t, "rapid_wind.json")
	l := listener.NewFake(valid, valid)

	pub := &publisher.Fake{}
	callCount := 0
	d := &Daemon{
		listener:  l,
		publisher: pub,
		log:       discardLog(),
		convert: func(msg parser.Message) ([]*event.Event, error) {
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf("injected converter error")
			}
			return event.FromMessage(msg)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	<-l.Drained()
	cancel()
	<-done

	// Despite the first dispatch failing, the second message was still published.
	msgs := pub.Messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d published messages, want 1 (second message should succeed after dispatch error)", len(msgs))
	}
}
