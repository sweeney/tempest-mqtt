# tempest-mqtt — A Code Walkthrough

*2026-03-02T10:17:32Z by Showboat 0.6.1*
<!-- showboat-id: cf8ce554-91cc-4ef8-82e6-020cd39c3f07 -->

This document walks through the `tempest-mqtt` codebase from entry point to infrastructure, explaining every layer and the design decisions behind it. The daemon listens on a UDP socket for broadcasts from a WeatherFlow Tempest weather station hub, parses the JSON messages into typed Go structs, converts them into named MQTT events, and publishes them to a broker for downstream home automation use.

The architecture enforces a strict separation of concerns: every layer except the hardware driver is testable without a real hub, a real network, or a real broker.

```bash
find . -name '*.go' | grep -v _test | sort
```

```output
./cmd/tempest-mqtt/main.go
./internal/daemon/daemon.go
./internal/event/event.go
./internal/listener/fake.go
./internal/listener/listener.go
./internal/listener/udp.go
./internal/parser/parser.go
./internal/parser/types.go
./internal/publisher/fake.go
./internal/publisher/mqtt.go
./internal/publisher/publisher.go
```

Each directory is a single responsibility. `cmd/tempest-mqtt` is pure wiring — no business logic lives there. The five internal packages form a one-way dependency chain:

`listener` → `parser` → `event` → `daemon` → `publisher`

Each package defines an interface (or pure data types) so every dependency can be swapped for a test double. The `fake.go` files in `listener` and `publisher` are in-memory test doubles; there are no real-hardware stubs because UDP and MQTT are not platform-specific.

## Entry point — pure wiring

`cmd/tempest-mqtt/main.go` is deliberately thin. It parses flags, constructs a UDP listener and an MQTT publisher, wires them together into a `Daemon`, and waits for a SIGINT/SIGTERM to shut down cleanly. No business logic lives here — the `main` package is a composition root and nothing more.

```bash
cat cmd/tempest-mqtt/main.go
```

```output
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
```

The `run()` wrapper pattern (calling `os.Exit(1)` only from `main`) means the deferred `p.Disconnect()` always fires on the way out, giving the broker time to distribute the in-flight last-will message before the process dies. `signal.NotifyContext` converts SIGINT/SIGTERM into context cancellation, which propagates cleanly through every blocking call in the daemon.

## Listener layer — the network boundary

The listener layer hides all UDP socket work behind a one-method interface. This is the only layer that touches the OS network stack.

```bash
cat internal/listener/listener.go
```

```output
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
```

A single method, a single concern. The interface takes a `context.Context` so callers can cancel it cleanly without polling. The real implementation below moves the blocking `ReadFromUDP` call off the main goroutine so that context cancellation never blocks waiting for the next datagram.

```bash
cat internal/listener/udp.go
```

```output
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
```

`readLoop` runs in its own goroutine and feeds raw datagram bytes into a buffered channel (capacity 16 — enough to absorb a burst). `ReadMessage` does a three-way `select`: data channel, error channel, or context cancelled. Because `ReadFromUDP` is never in the `select`, the goroutine can block freely on the socket without interfering with clean shutdown. Each message is copied out of the shared read buffer before being sent to the channel, so there are no data races.

The test double for this layer solves a different problem: deterministic synchronisation.

```bash
cat internal/listener/fake.go
```

```output
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
```

`Fake` is loaded with a fixed sequence of messages at construction time. Once the queue is exhausted it closes a `drained` channel — tests wait on that channel before cancelling the context, which gives the daemon time to process everything before shutting down. This pattern eliminates sleeps and `time.After` from the test suite entirely: the test is told the exact moment to stop, not asked to guess.

## Parser layer — typed decoding with zero I/O

The parser converts raw bytes into typed Go structs. It has no I/O dependencies at all — just `encoding/json` — so it can be unit-tested by passing byte slices directly.

```bash
cat internal/parser/parser.go
```

```output
package parser

import (
	"encoding/json"
	"fmt"
)

// probe extracts only the "type" field from raw JSON without full decoding.
type probe struct {
	Type string `json:"type"`
}

// Parse decodes a raw Tempest hub UDP JSON message into a typed Message.
// It peeks at the "type" field and delegates to the appropriate decoder.
// Returns an error for invalid JSON, unknown types, or malformed payloads.
func Parse(data []byte) (Message, error) {
	var p probe
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse: invalid JSON: %w", err)
	}

	switch p.Type {
	case TypeRapidWind:
		return parseRapidWind(data)
	case TypeHubStatus:
		return parseHubStatus(data)
	case TypeDeviceStatus:
		return parseDeviceStatus(data)
	case TypeObsST:
		return parseObsST(data)
	case TypeEvtPrecip:
		return parseEvtPrecip(data)
	case TypeEvtStrike:
		return parseEvtStrike(data)
	case "":
		return nil, fmt.Errorf("parse: missing \"type\" field")
	default:
		return nil, fmt.Errorf("parse: unknown message type %q", p.Type)
	}
}

func parseRapidWind(data []byte) (*RapidWind, error) {
	var m RapidWind
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse rapid_wind: %w", err)
	}
	return &m, nil
}

func parseHubStatus(data []byte) (*HubStatus, error) {
	var m HubStatus
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse hub_status: %w", err)
	}
	return &m, nil
}

func parseDeviceStatus(data []byte) (*DeviceStatus, error) {
	var m DeviceStatus
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse device_status: %w", err)
	}
	return &m, nil
}

func parseObsST(data []byte) (*ObsST, error) {
	var m ObsST
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse obs_st: %w", err)
	}
	if len(m.Obs) == 0 {
		return nil, fmt.Errorf("parse obs_st: obs array is empty")
	}
	if len(m.Obs[0]) < obsFieldCount {
		return nil, fmt.Errorf("parse obs_st: obs[0] has %d fields, want %d", len(m.Obs[0]), obsFieldCount)
	}
	return &m, nil
}

func parseEvtPrecip(data []byte) (*EvtPrecip, error) {
	var m EvtPrecip
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse evt_precip: %w", err)
	}
	return &m, nil
}

func parseEvtStrike(data []byte) (*EvtStrike, error) {
	var m EvtStrike
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse evt_strike: %w", err)
	}
	return &m, nil
}
```

The two-pass decode is intentional. The first pass (the `probe` struct) extracts only the `"type"` field. The switch statement then picks the right concrete struct for the second, full decode. JSON is decoded twice, but the hub sends at most one message per second — the overhead is negligible and the code stays readable. Each `parseXxx` helper wraps any unmarshal error with its message type, giving clear diagnostics when real hardware sends something unexpected.

`parseObsST` has two extra guards: it checks that the `obs` array is non-empty and that the first inner array has all 18 expected fields. The WeatherFlow protocol allows the hub to batch multiple observations in a single message, but the parser validates the structure before the event layer ever sees it.

The types are defined in a separate file to keep the parser logic easy to scan.

```bash
cat internal/parser/types.go
```

```output
// Package parser decodes WeatherFlow Tempest hub UDP JSON messages into typed Go structs.
// All types are pure data structures with no I/O dependencies, making them fully unit-testable.
package parser

import "encoding/json"

// MessageType constants mirror the "type" field in Tempest hub UDP messages.
const (
	TypeRapidWind    = "rapid_wind"
	TypeHubStatus    = "hub_status"
	TypeDeviceStatus = "device_status"
	TypeObsST        = "obs_st"
	TypeEvtPrecip    = "evt_precip"
	TypeEvtStrike    = "evt_strike"
)

// Message is implemented by all parsed Tempest UDP message types.
// The Type() method returns the raw Tempest message type string.
type Message interface {
	Type() string
}

// RapidWind is emitted by the sensor approximately every 15 seconds.
// Ob contains [timestamp_unix, speed_m_s, direction_deg].
type RapidWind struct {
	SerialNumber string            `json:"serial_number"`
	HubSN        string            `json:"hub_sn"`
	Ob           [3]json.Number    `json:"ob"`
}

func (m *RapidWind) Type() string { return TypeRapidWind }

// Timestamp returns the Unix epoch observation time.
func (m *RapidWind) Timestamp() int64 {
	t, _ := m.Ob[0].Int64()
	return t
}

// SpeedMS returns wind speed in metres per second.
func (m *RapidWind) SpeedMS() float64 {
	v, _ := m.Ob[1].Float64()
	return v
}

// DirectionDeg returns wind direction in degrees (0–360).
func (m *RapidWind) DirectionDeg() int {
	v, _ := m.Ob[2].Int64()
	return int(v)
}

// HubStatus is emitted by the hub approximately every 20 seconds.
type HubStatus struct {
	SerialNumber     string        `json:"serial_number"`
	FirmwareRevision string        `json:"firmware_revision"`
	Uptime           int64         `json:"uptime"`
	RSSI             int           `json:"rssi"`
	Timestamp        int64         `json:"timestamp"`
	ResetFlags       string        `json:"reset_flags"`
	Seq              int           `json:"seq"`
	RadioStats       []json.Number `json:"radio_stats"`
	MQTTStats        []int         `json:"mqtt_stats"`
	Freq             int64         `json:"freq"`
	HWVersion        int           `json:"hw_version"`
	HardwareID       int           `json:"hardware_id"`
}

func (m *HubStatus) Type() string { return TypeHubStatus }

// DeviceStatus is emitted by the sensor approximately every 60 seconds,
// typically ~600ms before the paired obs_st message.
type DeviceStatus struct {
	SerialNumber     string  `json:"serial_number"`
	HubSN            string  `json:"hub_sn"`
	Timestamp        int64   `json:"timestamp"`
	Uptime           int64   `json:"uptime"`
	Voltage          float64 `json:"voltage"`
	FirmwareRevision int     `json:"firmware_revision"`
	RSSI             int     `json:"rssi"`
	HubRSSI          int     `json:"hub_rssi"`
	SensorStatus     int     `json:"sensor_status"`
	Debug            int     `json:"debug"`
}

func (m *DeviceStatus) Type() string { return TypeDeviceStatus }

// SensorOK returns true when no sensor fault bits (bits 0–8) are set.
func (m *DeviceStatus) SensorOK() bool {
	return m.SensorStatus&0x1FF == 0
}

// ObsST is the full 60-second observation from the Tempest sensor.
// Obs is an array of observation arrays; each inner array has exactly 18 elements
// at fixed positional indices (see constants below).
type ObsST struct {
	SerialNumber     string          `json:"serial_number"`
	HubSN            string          `json:"hub_sn"`
	Obs              [][]json.Number `json:"obs"`
	FirmwareRevision int             `json:"firmware_revision"`
}

func (m *ObsST) Type() string { return TypeObsST }

// obs_st inner array indices (WeatherFlow Tempest UDP API).
const (
	obsIdxTimestamp           = 0
	obsIdxWindLull            = 1
	obsIdxWindAvg             = 2
	obsIdxWindGust            = 3
	obsIdxWindDirection       = 4
	obsIdxWindSampleInterval  = 5
	obsIdxPressure            = 6
	obsIdxTemperature         = 7
	obsIdxHumidity            = 8
	obsIdxIlluminance         = 9
	obsIdxUV                  = 10
	obsIdxSolarRadiation      = 11
	obsIdxRain1Min            = 12
	obsIdxPrecipType          = 13
	obsIdxLightningDistance   = 14
	obsIdxLightningCount      = 15
	obsIdxBattery             = 16
	obsIdxReportInterval      = 17
	obsFieldCount             = 18
)

// PrecipType codes from the WeatherFlow protocol.
const (
	PrecipTypeNone        = 0
	PrecipTypeRain        = 1
	PrecipTypeHail        = 2
	PrecipTypeRainAndHail = 3
)

// PrecipTypeString returns a human-readable precipitation type label.
func PrecipTypeString(t int) string {
	switch t {
	case PrecipTypeNone:
		return "none"
	case PrecipTypeRain:
		return "rain"
	case PrecipTypeHail:
		return "hail"
	case PrecipTypeRainAndHail:
		return "rain_and_hail"
	default:
		return "unknown"
	}
}

// EvtPrecip is emitted when the sensor detects the start of precipitation.
// Evt contains [timestamp_unix].
type EvtPrecip struct {
	SerialNumber string   `json:"serial_number"`
	HubSN        string   `json:"hub_sn"`
	Evt          [1]int64 `json:"evt"`
}

func (m *EvtPrecip) Type() string { return TypeEvtPrecip }

// Timestamp returns the Unix epoch time when precipitation started.
func (m *EvtPrecip) Timestamp() int64 {
	return m.Evt[0]
}

// EvtStrike is emitted when the sensor detects a lightning strike.
// Evt contains [timestamp_unix, distance_km, energy].
type EvtStrike struct {
	SerialNumber string            `json:"serial_number"`
	HubSN        string            `json:"hub_sn"`
	Evt          [3]json.Number    `json:"evt"`
}

func (m *EvtStrike) Type() string { return TypeEvtStrike }

// Timestamp returns the Unix epoch time of the strike.
func (m *EvtStrike) Timestamp() int64 {
	v, _ := m.Evt[0].Int64()
	return v
}

// DistanceKM returns the estimated distance to the strike in kilometres.
func (m *EvtStrike) DistanceKM() int {
	v, _ := m.Evt[1].Int64()
	return int(v)
}

// Energy returns the dimensionless energy value of the strike.
func (m *EvtStrike) Energy() int64 {
	v, _ := m.Evt[2].Int64()
	return v
}
```

A few details worth calling out.

**`json.Number` for positional arrays.** `RapidWind.Ob`, `ObsST.Obs`, and `EvtStrike.Evt` all use `json.Number` instead of `float64`. The WeatherFlow protocol encodes wind direction as an integer and wind speed as a float in the same fixed-position array. Using `json.Number` preserves the raw token and lets the typed accessor methods (`Int64()`, `Float64()`) apply the correct conversion without loss of precision.

**Named index constants for `obs_st`.** The full observation arrives as an 18-element positional array. The constants (`obsIdxTemperature = 7`, `obsIdxPressure = 6`, etc.) make the event-converter code readable and prevent off-by-one errors if the indices ever need updating.

**`SensorOK()` bitmask.** The `sensor_status` field is a bitmask. Bits 0–8 are the documented fault bits; the upper bits reflect an internal power-booster state and are masked out. `SensorOK()` applies the mask `0x1FF` so home-automation rules get a clean boolean without needing to understand the hardware internals.

Here is what a real `rapid_wind` datagram looks like on the wire — this is from the test fixture, sanitised from a live capture:

```bash
cat tests/fixtures/rapid_wind.json
```

```output
{
  "serial_number": "ST-00000001",
  "type": "rapid_wind",
  "hub_sn": "HB-00000001",
  "ob": [1772383186, 0.26, 108]
}
```

Compact: three values in a fixed-position array. Timestamp, speed in m/s, direction in degrees. The parser extracts them by index via the typed accessors and the event layer re-emits them as a named JSON object. Subscribers never need to know which array index is which.

## Event layer — protocol-to-MQTT translation

The event package converts a typed `parser.Message` into one or more `Event` structs ready for the broker. This is where all the MQTT-specific decisions live: topic names, QoS levels, and retain flags.

```bash
sed -n '1,67p' internal/event/event.go
```

```output
// Package event converts parsed WeatherFlow Tempest messages into MQTT events.
//
// Topic structure (prefix is set via NewConverter):
//
//	climate/{prefix}/status          — hub health (~20s)
//	climate/{prefix}/device/status   — sensor health (~60s)
//	climate/{prefix}/wind/rapid      — rapid wind (~15s)
//	climate/{prefix}/observation     — full observation (~60s)
//	climate/{prefix}/event/rain      — precipitation started
//	climate/{prefix}/event/lightning — lightning strike
//
// All payloads are JSON objects with named fields (no positional arrays).
// hub_sn and sensor_sn are included in the JSON body for traceability.
package event

import (
	"encoding/json"
	"fmt"

	"github.com/sweeney/tempest-mqtt/internal/parser"
)

// jsonMarshal is the JSON marshalling function used by all event builders.
// Tests may replace it to exercise error paths.
var jsonMarshal = json.Marshal

// Event is an MQTT message ready to publish.
type Event struct {
	Topic   string
	Payload []byte
	QoS     byte
	Retain  bool
}

// NewConverter returns a function that converts a parsed Tempest message into
// one or more MQTT events with topics rooted at climate/{prefix}.
// obs_st may yield multiple events when the hub batches observations.
func NewConverter(prefix string) func(parser.Message) ([]*Event, error) {
	return func(msg parser.Message) ([]*Event, error) {
		return convert(msg, prefix)
	}
}

func convert(msg parser.Message, prefix string) ([]*Event, error) {
	switch m := msg.(type) {
	case *parser.RapidWind:
		e, err := rapidWindEvent(m, prefix)
		return []*Event{e}, err
	case *parser.HubStatus:
		e, err := hubStatusEvent(m, prefix)
		return []*Event{e}, err
	case *parser.DeviceStatus:
		e, err := deviceStatusEvent(m, prefix)
		return []*Event{e}, err
	case *parser.ObsST:
		return obsSTEvents(m, prefix)
	case *parser.EvtPrecip:
		e, err := evtPrecipEvent(m, prefix)
		return []*Event{e}, err
	case *parser.EvtStrike:
		e, err := evtStrikeEvent(m, prefix)
		return []*Event{e}, err
	default:
		return nil, fmt.Errorf("event: unsupported message type %T", msg)
	}
}

```

`NewConverter` returns a closure rather than a method on a struct. The prefix is captured at construction time, keeping each call site clean. The returned function takes a `parser.Message` (interface) and returns `[]*Event` because `obs_st` can carry a batch of observations — most messages return a slice of one, but the API is uniform.

The `jsonMarshal` package-level variable deserves a note: it defaults to `json.Marshal` but can be replaced by internal tests to inject a failing stub. Plain structs never fail to marshal, so without this hook the error branches in every event builder would be unreachable and coverage would drop below 100%. The injectable function is the only way to exercise those paths without making the public API more complicated.

The QoS and retain policy is different for each message type, and the comments in the code explain why:

```bash
grep -n 'QoS\|Retain\|retain\|// ' internal/event/event.go | grep -v '^.*//.*json\|SensorFaults\|LightningS\|PressureS\|Temperat\|HumidityS\|WindS\|Precip\|LightUV\|status&\|LightningSens'
```

```output
1:// Package event converts parsed WeatherFlow Tempest messages into MQTT events.
3:// Topic structure (prefix is set via NewConverter):
12:// All payloads are JSON objects with named fields (no positional arrays).
13:// hub_sn and sensor_sn are included in the JSON body for traceability.
24:// Tests may replace it to exercise error paths.
27:// Event is an MQTT message ready to publish.
31:	QoS     byte
32:	Retain  bool
35:// NewConverter returns a function that converts a parsed Tempest message into
36:// one or more MQTT events with topics rooted at climate/{prefix}.
37:// obs_st may yield multiple events when the hub batches observations.
68:// --- rapid_wind ---
70:// RapidWindPayload is the MQTT payload for rapid_wind events.
94:		QoS:     0,     // real-time; delivery not critical
95:		Retain:  false, // high-frequency; no value in retaining stale wind
99:// --- hub_status ---
101:// HubStatusPayload is the MQTT payload for hub_status events.
129:		QoS:     1,
130:		Retain:  true, // last known hub state should persist for new subscribers
134:// --- device_status ---
137:// into individual named fault flags.
164:// DeviceStatusPayload is the MQTT payload for device_status events.
200:		QoS:     1,
201:		Retain:  true, // last known sensor state should persist for new subscribers
205:// --- obs_st ---
207:// ObservationPayload is the MQTT payload for obs_st events.
208:// All array positional fields from the protocol are mapped to named JSON keys.
290:		QoS:     1,
291:		Retain:  true, // current conditions should persist for new subscribers
295:// --- evt_precip ---
317:		QoS:     1,
318:		Retain:  false, // transient event; no value retaining after rain stops
322:// --- evt_strike ---
324:// EvtStrikePayload is the MQTT payload for evt_strike events.
348:		QoS:     1,
349:		Retain:  false, // transient event
```

The policy in summary:

| Topic | QoS | Retain | Rationale |
|---|---|---|---|
| `climate/{prefix}/wind/rapid` | 0 | false | 15s updates; stale wind is misleading |
| `climate/{prefix}/status` | 1 | true | Hub health: new subscribers need the last value |
| `climate/{prefix}/device/status` | 1 | true | Sensor health: same reasoning |
| `climate/{prefix}/observation` | 1 | true | Current conditions: new subscribers need the last value |
| `climate/{prefix}/event/rain` | 1 | false | Edge trigger; meaningless after the event |
| `climate/{prefix}/event/lightning` | 1 | false | Edge trigger; meaningless after the event |

Rapid wind is the only message that drops to QoS 0. A missed rapid-wind reading is harmless — the full observation arrives within 60 seconds anyway. All the others use QoS 1 so the broker can guarantee delivery even across transient reconnects.

The `device_status` event does the most interesting transformation: it decodes the raw integer bitmask into a struct of named boolean fields so consumers get structured data rather than a magic number.

```bash
sed -n '136,203p' internal/event/event.go
```

```output
// SensorFaults decodes the lower 9 bits of the sensor_status bitmask
// into individual named fault flags.
type SensorFaults struct {
	LightningSensorFailed    bool `json:"lightning_sensor_failed"`
	LightningSensorNoise     bool `json:"lightning_sensor_noise"`
	LightningSensorDisturber bool `json:"lightning_sensor_disturber"`
	PressureSensorFailed     bool `json:"pressure_sensor_failed"`
	TemperatureSensorFailed  bool `json:"temperature_sensor_failed"`
	HumiditySensorFailed     bool `json:"humidity_sensor_failed"`
	WindSensorFailed         bool `json:"wind_sensor_failed"`
	PrecipSensorFailed       bool `json:"precip_sensor_failed"`
	LightUVSensorFailed      bool `json:"light_uv_sensor_failed"`
}

func decodeSensorFaults(status int) SensorFaults {
	return SensorFaults{
		LightningSensorFailed:    status&(1<<0) != 0,
		LightningSensorNoise:     status&(1<<1) != 0,
		LightningSensorDisturber: status&(1<<2) != 0,
		PressureSensorFailed:     status&(1<<3) != 0,
		TemperatureSensorFailed:  status&(1<<4) != 0,
		HumiditySensorFailed:     status&(1<<5) != 0,
		WindSensorFailed:         status&(1<<6) != 0,
		PrecipSensorFailed:       status&(1<<7) != 0,
		LightUVSensorFailed:      status&(1<<8) != 0,
	}
}

// DeviceStatusPayload is the MQTT payload for device_status events.
type DeviceStatusPayload struct {
	Timestamp        int64        `json:"timestamp"`
	UptimeS          int64        `json:"uptime_s"`
	BatteryV         float64      `json:"battery_v"`
	FirmwareRevision int          `json:"firmware_revision"`
	RSSIdbm          int          `json:"rssi_dbm"`
	HubRSSIdbm       int          `json:"hub_rssi_dbm"`
	SensorStatus     int          `json:"sensor_status"`
	SensorFaults     SensorFaults `json:"sensor_faults"`
	SensorOK         bool         `json:"sensor_ok"`
	HubSN            string       `json:"hub_sn"`
	SensorSN         string       `json:"sensor_sn"`
}

func deviceStatusEvent(m *parser.DeviceStatus, prefix string) (*Event, error) {
	p := DeviceStatusPayload{
		Timestamp:        m.Timestamp,
		UptimeS:          m.Uptime,
		BatteryV:         m.Voltage,
		FirmwareRevision: m.FirmwareRevision,
		RSSIdbm:          m.RSSI,
		HubRSSIdbm:       m.HubRSSI,
		SensorStatus:     m.SensorStatus,
		SensorFaults:     decodeSensorFaults(m.SensorStatus),
		SensorOK:         m.SensorOK(),
		HubSN:            m.HubSN,
		SensorSN:         m.SerialNumber,
	}
	payload, err := jsonMarshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal device_status payload: %w", err)
	}
	return &Event{
		Topic:   fmt.Sprintf("climate/%s/device/status", prefix),
		Payload: payload,
		QoS:     1,
		Retain:  true, // last known sensor state should persist for new subscribers
	}, nil
}
```

The payload includes both `sensor_status` (the raw integer, for completeness) and `sensor_faults` (the decoded struct) and `sensor_ok` (the top-level boolean). Home automation rules can check `sensor_ok` for a simple pass/fail; deeper diagnostics can inspect the individual fault fields without reverse-engineering the bitmask.

## Daemon — the event loop

The daemon is the only layer that knows about all the others. It owns the main processing loop, wires everything together, and decides what counts as a fatal error versus a transient one.

```bash
cat internal/daemon/daemon.go
```

```output
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
```

Error handling is tiered:

1. **Listener error** — fatal. If `ReadMessage` returns a non-nil error the loop exits. In practice this only happens when the context is cancelled (normal shutdown) or the UDP socket is broken (hardware problem). Both are unrecoverable from the daemon's point of view.

2. **Parse error** — transient. The hub occasionally broadcasts messages from other sensors on the same network, or may send a future firmware message type the parser doesn't know yet. The daemon logs and continues so that one bad datagram cannot interrupt the flow of valid ones.

3. **Dispatch error** — transient. Event conversion errors (e.g. a marshal failure) are logged but the loop continues. Within `dispatch`, publish errors are also logged individually rather than aborting the batch — if one topic fails the others are still attempted.

The `convertFn` field on `Daemon` follows the same injectable-function pattern as `jsonMarshal` in the event package. It defaults to `event.NewConverter(topicPrefix)` at construction time, but the internal test file can replace it with a stub to exercise the dispatch-error path without constructing real events.

## Publisher layer — the broker boundary

The publisher layer mirrors the listener layer in structure: a one-method interface, a real implementation backed by the Paho MQTT library, and a test double that records every call.

```bash
cat internal/publisher/publisher.go
```

```output
// Package publisher provides an abstraction over MQTT publishing.
// The Publisher interface allows the daemon to be tested without a real broker.
package publisher

// Publisher sends MQTT messages.
type Publisher interface {
	// Publish sends payload to topic with the given QoS level and retain flag.
	Publish(topic string, payload []byte, qos byte, retain bool) error
}
```

```bash
cat internal/publisher/mqtt.go
```

```output
package publisher

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	connectTimeout  = 10 * time.Second
	publishTimeout  = 5 * time.Second
	disconnectQuiesce = 250 // ms; time allowed for in-flight messages before disconnect
)

// MQTT publishes to an MQTT broker via the Paho client library.
type MQTT struct {
	client mqtt.Client
}

// Config holds MQTT connection parameters.
type Config struct {
	BrokerURL string
	ClientID  string
	Username  string
	Password  string
}

// NewMQTT connects to the broker described by cfg and returns a ready Publisher.
// It will retry connections automatically on transient disconnects.
func NewMQTT(cfg Config) (*MQTT, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.BrokerURL)
	opts.SetClientID(cfg.ClientID)
	opts.SetConnectTimeout(connectTimeout)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(2 * time.Second)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	// Last Will: mark daemon as offline if connection drops unexpectedly.
	opts.SetWill(
		fmt.Sprintf("tempest/%s/daemon/status", cfg.ClientID),
		`{"online":false}`,
		1,
		true,
	)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(connectTimeout) {
		return nil, fmt.Errorf("mqtt connect timeout after %s", connectTimeout)
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("mqtt connect to %q: %w", cfg.BrokerURL, err)
	}

	return &MQTT{client: client}, nil
}

// Publish sends payload to topic with qos and retain settings.
func (p *MQTT) Publish(topic string, payload []byte, qos byte, retain bool) error {
	token := p.client.Publish(topic, qos, retain, payload)
	if !token.WaitTimeout(publishTimeout) {
		return fmt.Errorf("mqtt publish timeout to %q", topic)
	}
	return token.Error()
}

// Disconnect gracefully shuts down the MQTT client, allowing in-flight messages
// to complete.
func (p *MQTT) Disconnect() {
	p.client.Disconnect(disconnectQuiesce)
}
```

A few points worth noting in the real publisher.

**Auto-reconnect.** `SetAutoReconnect(true)` and `SetConnectRetry(true)` mean the Paho client quietly handles transient broker outages in the background. Publish calls will block up to `publishTimeout` (5 s) while the client attempts to reconnect; after that the timeout error bubbles up to the daemon's dispatch loop, which logs it and continues.

**Last Will and Testament.** The MQTT LWT (`SetWill`) publishes `{"online":false}` to `tempest/{clientID}/daemon/status` with QoS 1 and retain if the client connection drops unexpectedly. Subscribers can use this to detect when the daemon goes offline — useful for home automation alerting.

**`Disconnect` quiesce.** The 250 ms quiesce period in `Disconnect` allows in-flight QoS 1 messages to complete acknowledgement before the connection closes. The systemd `TimeoutStopSec=10` in the service file gives the daemon enough time for this to complete.

```bash
cat internal/publisher/fake.go
```

```output
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
```

`Fake` has two modes: normal (records every call) and error (always returns the configured error). `NewFakeWithError` is used to test that the daemon logs publish errors and keeps running rather than aborting. Both modes are safe for concurrent access via `sync.Mutex` — the daemon's event loop and the test's assertion code can call `Messages()` and `Publish` concurrently without data races.

## Testing strategy

All three internal packages with logic (`parser`, `event`, `daemon`) are held to 100% statement coverage, enforced in CI. No sleeps, no network access, no real broker required.

```bash
go test ./... -coverprofile=/tmp/cov.out > /dev/null && go tool cover -func=/tmp/cov.out | grep -E 'total|internal'
```

```output
github.com/sweeney/tempest-mqtt/internal/daemon/daemon.go:31:	New			100.0%
github.com/sweeney/tempest-mqtt/internal/daemon/daemon.go:44:	Run			100.0%
github.com/sweeney/tempest-mqtt/internal/daemon/daemon.go:74:	dispatch		100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:38:	NewConverter		100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:44:	convert			100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:79:	rapidWindEvent		100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:112:	hubStatusEvent		100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:150:	decodeSensorFaults	100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:179:	deviceStatusEvent	100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:233:	obsSTEvents		100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:245:	singleObsEvent		100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:304:	evtPrecipEvent		100.0%
github.com/sweeney/tempest-mqtt/internal/event/event.go:333:	evtStrikeEvent		100.0%
github.com/sweeney/tempest-mqtt/internal/listener/fake.go:20:	NewFake			0.0%
github.com/sweeney/tempest-mqtt/internal/listener/fake.go:29:	ReadMessage		0.0%
github.com/sweeney/tempest-mqtt/internal/listener/fake.go:52:	Drained			0.0%
github.com/sweeney/tempest-mqtt/internal/listener/udp.go:22:	NewUDP			0.0%
github.com/sweeney/tempest-mqtt/internal/listener/udp.go:37:	readLoop		0.0%
github.com/sweeney/tempest-mqtt/internal/listener/udp.go:53:	ReadMessage		0.0%
github.com/sweeney/tempest-mqtt/internal/parser/parser.go:16:	Parse			100.0%
github.com/sweeney/tempest-mqtt/internal/parser/parser.go:42:	parseRapidWind		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/parser.go:50:	parseHubStatus		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/parser.go:58:	parseDeviceStatus	100.0%
github.com/sweeney/tempest-mqtt/internal/parser/parser.go:66:	parseObsST		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/parser.go:80:	parseEvtPrecip		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/parser.go:88:	parseEvtStrike		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:31:	Type			100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:34:	Timestamp		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:40:	SpeedMS			100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:46:	DirectionDeg		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:67:	Type			100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:84:	Type			100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:87:	SensorOK		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:101:	Type			100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:135:	PrecipTypeString	100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:158:	Type			100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:161:	Timestamp		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:173:	Type			100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:176:	Timestamp		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:182:	DistanceKM		100.0%
github.com/sweeney/tempest-mqtt/internal/parser/types.go:188:	Energy			100.0%
github.com/sweeney/tempest-mqtt/internal/publisher/fake.go:22:	NewFakeWithError	0.0%
github.com/sweeney/tempest-mqtt/internal/publisher/fake.go:27:	Publish			0.0%
github.com/sweeney/tempest-mqtt/internal/publisher/fake.go:45:	Messages		0.0%
github.com/sweeney/tempest-mqtt/internal/publisher/fake.go:54:	Reset			0.0%
github.com/sweeney/tempest-mqtt/internal/publisher/mqtt.go:31:	NewMQTT			0.0%
github.com/sweeney/tempest-mqtt/internal/publisher/mqtt.go:66:	Publish			0.0%
github.com/sweeney/tempest-mqtt/internal/publisher/mqtt.go:76:	Disconnect		0.0%
total:								(statements)		57.3%
```

The infrastructure packages (`listener`, `publisher`) show 0% because they contain I/O code that would require a live UDP socket or a live broker to execute. They are not included in the coverage requirement. The `fake.go` files in those packages are exercised indirectly by the daemon tests — they just appear as uncovered because Go measures coverage per-package and the daemons tests are in `package daemon_test`.

The three logic packages (`internal/parser`, `internal/event`, `internal/daemon`) all show 100%. This is enforced by the CI script:

```bash
cat .github/workflows/ci.yml
```

```output
name: CI

on:
  push:
    branches: [main, master]
  pull_request:
    branches: [main, master]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
          cache: true

      - name: Download dependencies
        run: go mod download

      - name: Run go vet
        run: |
          VET_OUTPUT=$(go vet ./... 2>&1) || true
          echo "$VET_OUTPUT"
          if [ -n "$VET_OUTPUT" ]; then
            echo "## ⚠️ Vet Warnings" >> $GITHUB_STEP_SUMMARY
            echo '```' >> $GITHUB_STEP_SUMMARY
            echo "$VET_OUTPUT" >> $GITHUB_STEP_SUMMARY
            echo '```' >> $GITHUB_STEP_SUMMARY
            exit 1
          fi

      - name: Run tests with JSON output
        run: |
          set -o pipefail
          go test -v -race -coverprofile=coverage.out -json ./... 2>&1 | tee test-results.json \
            || echo "TEST_FAILED=true" >> $GITHUB_ENV

      - name: Enforce 100% coverage on core logic packages
        run: |
          for pkg in internal/parser internal/event internal/daemon; do
            go test -coverprofile=${pkg//\//_}.out ./$pkg/
            COVERAGE=$(go tool cover -func=${pkg//\//_}.out | grep total | awk '{print $3}' | sed 's/%//')
            echo "$pkg coverage: ${COVERAGE}%"
            if [ "$(echo "$COVERAGE < 100" | bc)" -eq 1 ]; then
              echo "ERROR: $pkg coverage must be 100%, got ${COVERAGE}%"
              exit 1
            fi
          done

      - name: Generate HTML coverage report
        run: |
          go tool cover -html=coverage.out -o coverage.html
          go tool cover -func=coverage.out > coverage.txt

      - name: Generate test summary
        if: always()
        run: |
          echo "## 🧪 Test Results" >> $GITHUB_STEP_SUMMARY
          echo "" >> $GITHUB_STEP_SUMMARY

          PASSED=$(grep -c '"Action":"pass"' test-results.json 2>/dev/null || echo 0)
          FAILED=$(grep -c '"Action":"fail"' test-results.json 2>/dev/null || echo 0)
          SKIPPED=$(grep -c '"Action":"skip"' test-results.json 2>/dev/null || echo 0)

          echo "| ✅ Passed | ❌ Failed | ⏭️ Skipped |" >> $GITHUB_STEP_SUMMARY
          echo "|:--------:|:--------:|:---------:|" >> $GITHUB_STEP_SUMMARY
          echo "| $PASSED | $FAILED | $SKIPPED |" >> $GITHUB_STEP_SUMMARY
          echo "" >> $GITHUB_STEP_SUMMARY

          if [ "$FAILED" -gt 0 ]; then
            echo "<details open>" >> $GITHUB_STEP_SUMMARY
            echo "<summary>❌ Failed Test Output</summary>" >> $GITHUB_STEP_SUMMARY
            echo "" >> $GITHUB_STEP_SUMMARY
            echo '```' >> $GITHUB_STEP_SUMMARY
            jq -r 'select(.Action=="output") | select(.Test != null) | .Output // empty' \
              test-results.json 2>/dev/null | head -200 >> $GITHUB_STEP_SUMMARY || true
            echo '```' >> $GITHUB_STEP_SUMMARY
            echo "</details>" >> $GITHUB_STEP_SUMMARY
            echo "" >> $GITHUB_STEP_SUMMARY
          fi

          echo "## 📊 Coverage Report" >> $GITHUB_STEP_SUMMARY
          echo "" >> $GITHUB_STEP_SUMMARY
          TOTAL=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
          echo "**Overall Coverage: $TOTAL**" >> $GITHUB_STEP_SUMMARY
          echo "" >> $GITHUB_STEP_SUMMARY

          echo "### Coverage by Package" >> $GITHUB_STEP_SUMMARY
          echo '```' >> $GITHUB_STEP_SUMMARY
          for pkg in internal/parser internal/event internal/daemon internal/listener internal/publisher; do
            if go test -coverprofile=tmp.out ./$pkg/ 2>/dev/null; then
              COV=$(go tool cover -func=tmp.out | grep total | awk '{print $3}' | tr -d '%')
              if [ -n "$COV" ]; then
                FILLED=$(echo "$COV / 10" | bc)
                EMPTY=$((10 - FILLED))
                BAR=""
                for i in $(seq 1 $FILLED 2>/dev/null); do BAR+="█"; done
                for i in $(seq 1 $EMPTY 2>/dev/null); do BAR+="░"; done
                printf "%-30s %s %5.1f%%\n" "$pkg" "$BAR" "$COV" >> $GITHUB_STEP_SUMMARY
              fi
            fi
          done
          echo '```' >> $GITHUB_STEP_SUMMARY

          echo "<details>" >> $GITHUB_STEP_SUMMARY
          echo "<summary>📋 Full Function Coverage</summary>" >> $GITHUB_STEP_SUMMARY
          echo '```' >> $GITHUB_STEP_SUMMARY
          go tool cover -func=coverage.out >> $GITHUB_STEP_SUMMARY
          echo '```' >> $GITHUB_STEP_SUMMARY
          echo "</details>" >> $GITHUB_STEP_SUMMARY

          echo "<details>" >> $GITHUB_STEP_SUMMARY
          echo "<summary>🐢 Slowest Tests (top 10)</summary>" >> $GITHUB_STEP_SUMMARY
          echo "| Test | Duration |" >> $GITHUB_STEP_SUMMARY
          echo "|------|----------|" >> $GITHUB_STEP_SUMMARY
          jq -r 'select(.Action=="pass" and .Test != null and .Elapsed != null) |
            "\(.Package | split("/") | .[-1])/\(.Test)|\(.Elapsed)s"' \
            test-results.json 2>/dev/null | sort -t'|' -k2 -rn | head -10 | \
          while IFS='|' read test dur; do
            echo "| \`$test\` | $dur |" >> $GITHUB_STEP_SUMMARY
          done || true
          echo "</details>" >> $GITHUB_STEP_SUMMARY

      - name: Fail if tests failed
        if: env.TEST_FAILED == 'true'
        run: exit 1

      - name: Upload coverage report
        uses: actions/upload-artifact@v4
        with:
          name: coverage-report
          path: |
            coverage.html
            coverage.txt
          retention-days: 30

  build:
    runs-on: ubuntu-latest
    needs: test
    strategy:
      matrix:
        include:
          - goos: linux
            goarch: amd64
            suffix: linux-amd64
          - goos: linux
            goarch: arm64
            suffix: linux-arm64
          - goos: linux
            goarch: arm
            goarm: "6"
            suffix: linux-armv6

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
          cache: true

      - name: Download dependencies
        run: go mod download

      - name: Build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          GOARM: ${{ matrix.goarm }}
        run: |
          go build -ldflags="-s -w" -o tempest-mqtt-${{ matrix.suffix }} ./cmd/tempest-mqtt
          chmod +x tempest-mqtt-${{ matrix.suffix }}

      - name: Report binary size
        run: |
          SIZE=$(ls -lh tempest-mqtt-${{ matrix.suffix }} | awk '{print $5}')
          SIZE_BYTES=$(ls -l tempest-mqtt-${{ matrix.suffix }} | awk '{print $5}')
          echo "### 📦 Binary: \`${{ matrix.suffix }}\`" >> $GITHUB_STEP_SUMMARY
          echo "| Metric | Value |" >> $GITHUB_STEP_SUMMARY
          echo "|--------|-------|" >> $GITHUB_STEP_SUMMARY
          echo "| Size | **$SIZE** ($SIZE_BYTES bytes) |" >> $GITHUB_STEP_SUMMARY

      - name: Upload binary
        uses: actions/upload-artifact@v4
        with:
          name: tempest-mqtt-${{ matrix.suffix }}
          path: tempest-mqtt-${{ matrix.suffix }}
          retention-days: 90
```

The coverage step loops over the three core packages and uses `bc` to enforce that each one is exactly 100%. The `-race` flag on the full test run detects data races — relevant for the concurrency in `listener.Fake` and `publisher.Fake`. The CI also builds release binaries for `linux/amd64`, `linux/arm64`, and `linux/armv6` (for Raspberry Pi Zero) as build-matrix jobs after the tests pass.

The most instructive test to read is the end-to-end daemon test:

```bash
sed -n '42,60p' internal/daemon/daemon_test.go
```

```output
// runDaemon sends messages through the daemon and returns the published MQTT messages.
// It cancels the daemon context after the fake listener is drained.
func runDaemon(t *testing.T, messages ...[]byte) []publisher.Message {
	t.Helper()

	l := listener.NewFake(messages...)
	pub := &publisher.Fake{}
	d := daemon.New(l, pub, discardLogger(), testPrefix)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	<-l.Drained()
	cancel()
	<-done

	return pub.Messages()
}
```

`runDaemon` is the testing heart of the project. It:

1. Loads a `listener.Fake` with whatever fixture bytes the test provides
2. Creates a `publisher.Fake` to capture output
3. Starts the daemon in a background goroutine
4. Waits on `l.Drained()` — the channel closed by `listener.Fake` once every message has been delivered
5. Cancels the context, waits for the daemon to exit, then returns all recorded MQTT messages

No sleeps. No polling. No flaky timing windows. The test is told exactly when to stop.

```bash
sed -n '417,443p' internal/daemon/daemon_test.go
```

```output
func TestDaemon_MultipleMessages_AllProcessed(t *testing.T) {
	msgs := runDaemon(t,
		loadFixture(t, "rapid_wind.json"),
		loadFixture(t, "hub_status.json"),
		loadFixture(t, "device_status.json"),
		loadFixture(t, "obs_st.json"),
		loadFixture(t, "evt_precip.json"),
		loadFixture(t, "evt_strike.json"),
	)
	if len(msgs) != 6 {
		t.Fatalf("got %d published messages, want 6 (one per fixture)", len(msgs))
	}

	wantTopics := []string{
		"climate/test/wind/rapid",
		"climate/test/status",
		"climate/test/device/status",
		"climate/test/observation",
		"climate/test/event/rain",
		"climate/test/event/lightning",
	}
	for i, want := range wantTopics {
		if msgs[i].Topic != want {
			t.Errorf("msgs[%d].Topic = %q, want %q", i, msgs[i].Topic, want)
		}
	}
}
```

This test drives all six message types through the full pipeline in a single run and asserts that each produced the right MQTT topic in the right order. It is the single highest-value test in the suite: if the wiring between any two layers breaks, this test fails.

## Deployment

The daemon runs as a systemd user service on the target host. A `deploy.sh` script handles the full lifecycle: cross-compile for `linux/amd64`, upload a timestamped binary, atomically symlink it into place, restart the service, and prune old versions (keeping the last three).

```bash
cat tempest-mqtt.service
```

```output
[Unit]
Description=WeatherFlow Tempest to MQTT Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
# Optional: place config in /etc/tempest-mqtt.env
# MQTT_USERNAME=myuser
# MQTT_PASSWORD=secret
# TOPIC_PREFIX=home          # → topics: climate/home/...
EnvironmentFile=-/etc/tempest-mqtt.env
ExecStart=/usr/local/bin/tempest-mqtt \
    -broker ${MQTT_BROKER_URL:-tcp://localhost:1883} \
    -client-id ${MQTT_CLIENT_ID:-tempest-mqtt} \
    -username ${MQTT_USERNAME:-} \
    -password ${MQTT_PASSWORD:-} \
    -topic-prefix ${TOPIC_PREFIX:-tempest} \
    -udp-port ${TEMPEST_UDP_PORT:-50222} \
    -log-level ${LOG_LEVEL:-info}
Restart=on-failure
RestartSec=5
# Allow time for MQTT last-will to publish on clean shutdown
TimeoutStopSec=10

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictNamespaces=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
```

The service file is included for users who want to run as a system service (`WantedBy=multi-user.target`). For passwordless deploy without sudo, run as a systemd user service instead (`systemctl --user`) with `loginctl enable-linger` for boot persistence. The `TimeoutStopSec=10` gives the daemon enough time to complete the MQTT last-will handshake before systemd sends SIGKILL.

## Summary

The codebase follows a strict layered architecture with a one-way dependency graph:

```
cmd/tempest-mqtt  (wiring only)
      │
      ├── listener  ──▶  listener.Listener interface
      │       │              UDP (real)
      │       └──────────    Fake (tests)
      │
      ├── parser    ──▶  parser.Message interface
      │       │              all six concrete types
      │       └──────────    zero I/O; pure JSON decoding
      │
      ├── event     ──▶  event.Event struct
      │       │              NewConverter closure
      │       └──────────    injectable jsonMarshal for error coverage
      │
      ├── daemon    ──▶  the event loop
      │       │              tiered error handling
      │       └──────────    injectable convertFn for error coverage
      │
      └── publisher ──▶  publisher.Publisher interface
              │              MQTT (Paho, auto-reconnect, LWT)
              └──────────    Fake (records all calls)
```

Every layer except `listener/udp.go` and `publisher/mqtt.go` is unit-testable without network access. The fake implementations live alongside the real ones in the same package, available to any test that imports the package. Coverage is enforced to 100% on the three logic packages. Deployment is a single shell script and a systemd service — no containers, no orchestration.
