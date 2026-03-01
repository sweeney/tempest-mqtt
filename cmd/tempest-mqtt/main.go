// Command tempest-mqtt listens for WeatherFlow Tempest hub UDP broadcasts and
// re-publishes them as structured MQTT events for home automation integration.
//
// Usage:
//
//	tempest-mqtt [flags]
//
// Flags:
//
//	-broker string     MQTT broker URL (default "tcp://localhost:1883")
//	-client-id string  MQTT client ID (default "tempest-mqtt")
//	-username string   MQTT username (optional)
//	-password string   MQTT password (optional)
//	-udp-port int      UDP port to listen on (default 50222)
//	-log-level string  Log level: debug, info, warn, error (default "info")
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sweeney/tempest-mqtt/internal/daemon"
	"github.com/sweeney/tempest-mqtt/internal/listener"
	"github.com/sweeney/tempest-mqtt/internal/publisher"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "tempest-mqtt: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	brokerURL   := flag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
	clientID    := flag.String("client-id", "tempest-mqtt", "MQTT client ID")
	username    := flag.String("username", "", "MQTT username")
	password    := flag.String("password", "", "MQTT password")
	topicPrefix := flag.String("topic-prefix", "tempest", "MQTT topic prefix (topics: climate/{prefix}/...)")
	udpPort     := flag.Int("udp-port", 50222, "UDP port for WeatherFlow hub broadcasts")
	logLevel    := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	log := newLogger(*logLevel)

	log.Info("starting tempest-mqtt",
		slog.String("broker", *brokerURL),
		slog.String("client_id", *clientID),
		slog.String("topic_prefix", *topicPrefix),
		slog.Int("udp_port", *udpPort),
	)

	l, err := listener.NewUDP(*udpPort)
	if err != nil {
		return fmt.Errorf("create udp listener: %w", err)
	}
	log.Info("listening for WeatherFlow UDP broadcasts", slog.Int("port", *udpPort))

	p, err := publisher.NewMQTT(publisher.Config{
		BrokerURL: *brokerURL,
		ClientID:  *clientID,
		Username:  *username,
		Password:  *password,
	})
	if err != nil {
		return fmt.Errorf("connect to MQTT broker: %w", err)
	}
	defer p.Disconnect()
	log.Info("connected to MQTT broker", slog.String("broker", *brokerURL))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	d := daemon.New(l, p, log, *topicPrefix)
	if err := d.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("daemon: %w", err)
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
