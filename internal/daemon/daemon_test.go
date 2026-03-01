package daemon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sweeney/tempest-mqtt/internal/daemon"
	"github.com/sweeney/tempest-mqtt/internal/listener"
	"github.com/sweeney/tempest-mqtt/internal/publisher"
)

// discardLogger returns a slog.Logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func fixtureDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "tests", "fixtures")
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir(), name))
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	return data
}

// testPrefix is the topic prefix used across all daemon tests.
// Resulting topics take the form climate/test/...
const testPrefix = "test"

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

// --- RapidWind ---

func TestDaemon_RapidWind_PublishesToCorrectTopic(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "rapid_wind.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := "climate/test/ST-00000001/wind/rapid"
	if msgs[0].Topic != want {
		t.Errorf("topic = %q, want %q", msgs[0].Topic, want)
	}
}

func TestDaemon_RapidWind_PayloadFields(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "rapid_wind.json"))

	var p struct {
		Timestamp    int64   `json:"timestamp"`
		SpeedMS      float64 `json:"speed_ms"`
		DirectionDeg int     `json:"direction_deg"`
		HubSN        string  `json:"hub_sn"`
		SensorSN     string  `json:"sensor_sn"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Timestamp != 1772383186 {
		t.Errorf("timestamp = %d, want 1772383186", p.Timestamp)
	}
	if p.SpeedMS != 0.26 {
		t.Errorf("speed_ms = %f, want 0.26", p.SpeedMS)
	}
	if p.DirectionDeg != 108 {
		t.Errorf("direction_deg = %d, want 108", p.DirectionDeg)
	}
}

func TestDaemon_RapidWind_QoSAndRetain(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "rapid_wind.json"))
	if msgs[0].QoS != 0 {
		t.Errorf("QoS = %d, want 0", msgs[0].QoS)
	}
	if msgs[0].Retain {
		t.Error("Retain = true, want false")
	}
}

// --- HubStatus ---

func TestDaemon_HubStatus_PublishesToCorrectTopic(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "hub_status.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := "climate/test/status"
	if msgs[0].Topic != want {
		t.Errorf("topic = %q, want %q", msgs[0].Topic, want)
	}
}

func TestDaemon_HubStatus_QoSAndRetain(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "hub_status.json"))
	if msgs[0].QoS != 1 {
		t.Errorf("QoS = %d, want 1", msgs[0].QoS)
	}
	if !msgs[0].Retain {
		t.Error("Retain = false, want true")
	}
}

func TestDaemon_HubStatus_PayloadFields(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "hub_status.json"))

	var p struct {
		Timestamp int64  `json:"timestamp"`
		UptimeS   int64  `json:"uptime_s"`
		RSSIdbm   int    `json:"rssi_dbm"`
		Seq       int    `json:"seq"`
		HubSN     string `json:"hub_sn"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Timestamp != 1772383213 {
		t.Errorf("timestamp = %d, want 1772383213", p.Timestamp)
	}
	if p.UptimeS != 7370 {
		t.Errorf("uptime_s = %d, want 7370", p.UptimeS)
	}
	if p.RSSIdbm != -71 {
		t.Errorf("rssi_dbm = %d, want -71", p.RSSIdbm)
	}
	if p.Seq != 548 {
		t.Errorf("seq = %d, want 548", p.Seq)
	}
}

// --- DeviceStatus ---

func TestDaemon_DeviceStatus_PublishesToCorrectTopic(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "device_status.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := "climate/test/ST-00000001/status"
	if msgs[0].Topic != want {
		t.Errorf("topic = %q, want %q", msgs[0].Topic, want)
	}
}

func TestDaemon_DeviceStatus_QoSAndRetain(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "device_status.json"))
	if msgs[0].QoS != 1 {
		t.Errorf("QoS = %d, want 1", msgs[0].QoS)
	}
	if !msgs[0].Retain {
		t.Error("Retain = false, want true")
	}
}

func TestDaemon_DeviceStatus_PayloadContainsSensorHealth(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "device_status.json"))

	var p struct {
		BatteryV  float64 `json:"battery_v"`
		SensorOK  bool    `json:"sensor_ok"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.BatteryV != 2.458 {
		t.Errorf("battery_v = %f, want 2.458", p.BatteryV)
	}
	if !p.SensorOK {
		t.Error("sensor_ok = false, want true")
	}
}

// --- ObsST ---

func TestDaemon_ObsST_PublishesToCorrectTopic(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "obs_st.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := "climate/test/ST-00000001/observation"
	if msgs[0].Topic != want {
		t.Errorf("topic = %q, want %q", msgs[0].Topic, want)
	}
}

func TestDaemon_ObsST_QoSAndRetain(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "obs_st.json"))
	if msgs[0].QoS != 1 {
		t.Errorf("QoS = %d, want 1", msgs[0].QoS)
	}
	if !msgs[0].Retain {
		t.Error("Retain = false, want true")
	}
}

func TestDaemon_ObsST_PayloadAllWeatherFields(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "obs_st.json"))

	var p struct {
		Timestamp           int64   `json:"timestamp"`
		WindLullMS          float64 `json:"wind_lull_ms"`
		WindAvgMS           float64 `json:"wind_avg_ms"`
		WindGustMS          float64 `json:"wind_gust_ms"`
		WindDirectionDeg    int     `json:"wind_direction_deg"`
		WindSampleIntervalS int     `json:"wind_sample_interval_s"`
		PressureMB          float64 `json:"pressure_mb"`
		TemperatureC        float64 `json:"temperature_c"`
		HumidityPct         float64 `json:"humidity_pct"`
		IlluminanceLux      int     `json:"illuminance_lux"`
		UVIndex             float64 `json:"uv_index"`
		SolarRadiationWM2   int     `json:"solar_radiation_wm2"`
		Rain1MinMM          float64 `json:"rain_1min_mm"`
		PrecipType          int     `json:"precip_type"`
		PrecipTypeStr       string  `json:"precip_type_str"`
		LightningDistKM     int     `json:"lightning_distance_km"`
		LightningCount      int     `json:"lightning_count"`
		BatteryV            float64 `json:"battery_v"`
		ReportIntervalMin   int     `json:"report_interval_min"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if p.Timestamp != 1772383230 {
		t.Errorf("timestamp = %d, want 1772383230", p.Timestamp)
	}
	if p.WindLullMS != 0.1 {
		t.Errorf("wind_lull_ms = %f, want 0.1", p.WindLullMS)
	}
	if p.WindGustMS != 0.89 {
		t.Errorf("wind_gust_ms = %f, want 0.89", p.WindGustMS)
	}
	if p.PressureMB != 995.65 {
		t.Errorf("pressure_mb = %f, want 995.65", p.PressureMB)
	}
	if p.TemperatureC != 11.37 {
		t.Errorf("temperature_c = %f, want 11.37", p.TemperatureC)
	}
	if p.HumidityPct != 73.64 {
		t.Errorf("humidity_pct = %f, want 73.64", p.HumidityPct)
	}
	if p.PrecipTypeStr != "none" {
		t.Errorf("precip_type_str = %q, want none", p.PrecipTypeStr)
	}
	if p.BatteryV != 2.458 {
		t.Errorf("battery_v = %f, want 2.458", p.BatteryV)
	}
}

// --- EvtPrecip ---

func TestDaemon_EvtPrecip_PublishesToCorrectTopic(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "evt_precip.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := "climate/test/ST-00000001/event/rain"
	if msgs[0].Topic != want {
		t.Errorf("topic = %q, want %q", msgs[0].Topic, want)
	}
}

func TestDaemon_EvtPrecip_QoSAndRetain(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "evt_precip.json"))
	if msgs[0].QoS != 1 {
		t.Errorf("QoS = %d, want 1", msgs[0].QoS)
	}
	if msgs[0].Retain {
		t.Error("Retain = true, want false (transient event)")
	}
}

func TestDaemon_EvtPrecip_PayloadFields(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "evt_precip.json"))

	var p struct {
		Timestamp int64  `json:"timestamp"`
		HubSN     string `json:"hub_sn"`
		SensorSN  string `json:"sensor_sn"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Timestamp != 1772383500 {
		t.Errorf("timestamp = %d, want 1772383500", p.Timestamp)
	}
}

// --- EvtStrike ---

func TestDaemon_EvtStrike_PublishesToCorrectTopic(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "evt_strike.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := "climate/test/ST-00000001/event/lightning"
	if msgs[0].Topic != want {
		t.Errorf("topic = %q, want %q", msgs[0].Topic, want)
	}
}

func TestDaemon_EvtStrike_QoSAndRetain(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "evt_strike.json"))
	if msgs[0].QoS != 1 {
		t.Errorf("QoS = %d, want 1", msgs[0].QoS)
	}
	if msgs[0].Retain {
		t.Error("Retain = true, want false (transient event)")
	}
}

func TestDaemon_EvtStrike_PayloadFields(t *testing.T) {
	msgs := runDaemon(t, loadFixture(t, "evt_strike.json"))

	var p struct {
		Timestamp  int64  `json:"timestamp"`
		DistanceKM int    `json:"distance_km"`
		Energy     int64  `json:"energy"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Timestamp != 1772383500 {
		t.Errorf("timestamp = %d, want 1772383500", p.Timestamp)
	}
	if p.DistanceKM != 27 {
		t.Errorf("distance_km = %d, want 27", p.DistanceKM)
	}
	if p.Energy != 3849 {
		t.Errorf("energy = %d, want 3849", p.Energy)
	}
}

// --- Error resilience ---

func TestDaemon_InvalidJSON_ContinuesProcessing(t *testing.T) {
	// Feed invalid JSON followed by a valid message.
	// Daemon should log the error and process the valid message.
	valid := loadFixture(t, "rapid_wind.json")
	msgs := runDaemon(t,
		[]byte(`not valid json`),
		valid,
	)
	// Only the valid message should have been published.
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (invalid JSON should be skipped)", len(msgs))
	}
	if msgs[0].Topic != "climate/test/ST-00000001/wind/rapid" {
		t.Errorf("unexpected topic %q", msgs[0].Topic)
	}
}

func TestDaemon_UnknownMessageType_ContinuesProcessing(t *testing.T) {
	valid := loadFixture(t, "hub_status.json")
	msgs := runDaemon(t,
		[]byte(`{"type":"obs_air","serial_number":"AR-00000001"}`),
		valid,
	)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (unknown type should be skipped)", len(msgs))
	}
	if msgs[0].Topic != "climate/test/status" {
		t.Errorf("unexpected topic %q", msgs[0].Topic)
	}
}

func TestDaemon_PublishError_ContinuesProcessing(t *testing.T) {
	// Even when the publisher returns an error, the daemon should continue.
	l := listener.NewFake(
		loadFixture(t, "rapid_wind.json"),
		loadFixture(t, "hub_status.json"),
	)
	pub := publisher.NewFakeWithError(fmt.Errorf("broker unavailable"))
	d := daemon.New(l, pub, discardLogger(), testPrefix)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	<-l.Drained()
	cancel()
	<-done

	// Fake publisher with error records no messages.
	if len(pub.Messages()) != 0 {
		t.Errorf("expected 0 recorded messages (all errored), got %d", len(pub.Messages()))
	}
}

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
		"climate/test/ST-00000001/wind/rapid",
		"climate/test/status",
		"climate/test/ST-00000001/status",
		"climate/test/ST-00000001/observation",
		"climate/test/ST-00000001/event/rain",
		"climate/test/ST-00000001/event/lightning",
	}
	for i, want := range wantTopics {
		if msgs[i].Topic != want {
			t.Errorf("msgs[%d].Topic = %q, want %q", i, msgs[i].Topic, want)
		}
	}
}

func TestDaemon_ContextCancel_ReturnsContextError(t *testing.T) {
	l := listener.NewFake() // no messages; blocks immediately
	d := daemon.New(l, &publisher.Fake{}, discardLogger(), testPrefix)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	cancel()
	err := <-done
	if err != context.Canceled {
		t.Errorf("Run() returned %v, want context.Canceled", err)
	}
}

func TestDaemon_ObsST_Batched_PublishesMultipleEvents(t *testing.T) {
	raw := []byte(`{
		"serial_number":"ST-00000001","type":"obs_st","hub_sn":"HB-00000001",
		"obs":[
			[1772383230,0.1,0.4,0.89,107,15,995.65,11.37,73.64,2883,0.3,24,0.0,0,0,0,2.458,1],
			[1772383290,0.0,0.2,0.45,180,15,995.70,11.40,73.50,2900,0.3,25,0.0,0,0,0,2.460,1]
		],
		"firmware_revision":185
	}`)
	msgs := runDaemon(t, raw)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2 (one per batched obs)", len(msgs))
	}

	var p1, p2 struct {
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p1); err != nil {
		t.Fatalf("unmarshal p1: %v", err)
	}
	if err := json.Unmarshal(msgs[1].Payload, &p2); err != nil {
		t.Fatalf("unmarshal p2: %v", err)
	}
	if p1.Timestamp != 1772383230 {
		t.Errorf("msgs[0].timestamp = %d, want 1772383230", p1.Timestamp)
	}
	if p2.Timestamp != 1772383290 {
		t.Errorf("msgs[1].timestamp = %d, want 1772383290", p2.Timestamp)
	}
}
