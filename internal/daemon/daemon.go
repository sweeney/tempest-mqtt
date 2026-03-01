// Package daemon is the main event loop for tempest-mqtt.
// It wires together the listener, parser, event converter, and publisher.
// All I/O is behind interfaces, making this package 100% unit-testable
// without network access or a real MQTT broker.
package daemon

import (
	"context"
	"log/slog"

	"github.com/sweeney/tempest-mqtt/internal/event"
	"github.com/sweeney/tempest-mqtt/internal/listener"
	"github.com/sweeney/tempest-mqtt/internal/parser"
	"github.com/sweeney/tempest-mqtt/internal/publisher"
)

// convertFn is the signature of event.NewConverter. Stored as a field to allow
// injection of a stub in tests without complicating the public API.
type convertFn func(parser.Message) ([]*event.Event, error)

// Daemon listens for WeatherFlow UDP messages and publishes them as MQTT events.
type Daemon struct {
	listener  listener.Listener
	publisher publisher.Publisher
	log       *slog.Logger
	convert   convertFn // defaults to event.NewConverter(topicPrefix)
}

// New creates a Daemon that reads from l, publishes via p, and roots all MQTT
// topics at climate/{topicPrefix}.
func New(l listener.Listener, p publisher.Publisher, log *slog.Logger, topicPrefix string) *Daemon {
	return &Daemon{
		listener:  l,
		publisher: p,
		log:       log,
		convert:   event.NewConverter(topicPrefix),
	}
}

// Run starts the main event loop. It blocks until ctx is cancelled, at which
// point it returns ctx.Err(). Transient errors (parse failures, dispatch failures)
// are logged and the loop continues; only context cancellation or a fatal listener
// error causes Run to return.
func (d *Daemon) Run(ctx context.Context) error {
	d.log.Info("daemon starting")
	defer d.log.Info("daemon stopped")

	for {
		data, err := d.listener.ReadMessage(ctx)
		if err != nil {
			// Context cancellation is the normal shutdown path.
			return err
		}

		msg, err := parser.Parse(data)
		if err != nil {
			d.log.Warn("parse error",
				slog.String("error", err.Error()),
				slog.String("raw", string(data)))
			continue
		}

		if err := d.dispatch(msg); err != nil {
			d.log.Warn("dispatch error",
				slog.String("type", msg.Type()),
				slog.String("error", err.Error()))
		}
	}
}

// dispatch converts a parsed message into MQTT events and publishes them.
// A non-nil error indicates an event conversion failure; publish errors are
// logged individually but do not prevent other events in the same batch.
func (d *Daemon) dispatch(msg parser.Message) error {
	events, err := d.convert(msg)
	if err != nil {
		return err
	}

	for _, e := range events {
		if err := d.publisher.Publish(e.Topic, e.Payload, e.QoS, e.Retain); err != nil {
			d.log.Warn("publish error",
				slog.String("topic", e.Topic),
				slog.String("error", err.Error()))
			// Continue publishing remaining events from the same message.
		}
	}

	d.log.Debug("published",
		slog.String("type", msg.Type()),
		slog.Int("events", len(events)))

	return nil
}
