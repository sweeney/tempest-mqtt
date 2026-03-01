package event_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sweeney/tempest-mqtt/internal/event"
	"github.com/sweeney/tempest-mqtt/internal/parser"
)

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

func parseFixture(t *testing.T, name string) parser.Message {
	t.Helper()
	msg, err := parser.Parse(loadFixture(t, name))
	if err != nil {
		t.Fatalf("parser.Parse(%q): %v", name, err)
	}
	return msg
}

func singleEvent(t *testing.T, msg parser.Message) *event.Event {
	t.Helper()
	events, err := event.FromMessage(msg)
	if err != nil {
		t.Fatalf("FromMessage() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("FromMessage() returned %d events, want 1", len(events))
	}
	return events[0]
}

func unmarshalPayload(t *testing.T, e *event.Event, v any) {
	t.Helper()
	if err := json.Unmarshal(e.Payload, v); err != nil {
		t.Fatalf("unmarshal payload: %v\npayload: %s", err, e.Payload)
	}
}

// --- RapidWind ---

func TestFromMessage_RapidWind_Topic(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "rapid_wind.json"))
	want := "tempest/HB-00000001/ST-00000001/wind/rapid"
	if e.Topic != want {
		t.Errorf("Topic = %q, want %q", e.Topic, want)
	}
}

func TestFromMessage_RapidWind_QoS(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "rapid_wind.json"))
	if e.QoS != 0 {
		t.Errorf("QoS = %d, want 0 (real-time, high-frequency)", e.QoS)
	}
	if e.Retain {
		t.Error("Retain = true, want false (stale rapid wind has no value)")
	}
}

func TestFromMessage_RapidWind_Payload(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "rapid_wind.json"))

	var p event.RapidWindPayload
	unmarshalPayload(t, e, &p)

	if p.Timestamp != 1772383186 {
		t.Errorf("timestamp = %d, want 1772383186", p.Timestamp)
	}
	if p.SpeedMS != 0.26 {
		t.Errorf("speed_ms = %f, want 0.26", p.SpeedMS)
	}
	if p.DirectionDeg != 108 {
		t.Errorf("direction_deg = %d, want 108", p.DirectionDeg)
	}
	if p.HubSN != "HB-00000001" {
		t.Errorf("hub_sn = %q, want HB-00000001", p.HubSN)
	}
	if p.SensorSN != "ST-00000001" {
		t.Errorf("sensor_sn = %q, want ST-00000001", p.SensorSN)
	}
}

// --- HubStatus ---

func TestFromMessage_HubStatus_Topic(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "hub_status.json"))
	want := "tempest/HB-00000001/status"
	if e.Topic != want {
		t.Errorf("Topic = %q, want %q", e.Topic, want)
	}
}

func TestFromMessage_HubStatus_QoS(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "hub_status.json"))
	if e.QoS != 1 {
		t.Errorf("QoS = %d, want 1", e.QoS)
	}
	if !e.Retain {
		t.Error("Retain = false, want true (last hub state should persist)")
	}
}

func TestFromMessage_HubStatus_Payload(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "hub_status.json"))

	var p event.HubStatusPayload
	unmarshalPayload(t, e, &p)

	if p.Timestamp != 1772383213 {
		t.Errorf("timestamp = %d, want 1772383213", p.Timestamp)
	}
	if p.UptimeS != 7370 {
		t.Errorf("uptime_s = %d, want 7370", p.UptimeS)
	}
	if p.RSSIdbm != -71 {
		t.Errorf("rssi_dbm = %d, want -71", p.RSSIdbm)
	}
	if p.FirmwareRevision != "309" {
		t.Errorf("firmware_revision = %q, want 309", p.FirmwareRevision)
	}
	if p.ResetFlags != "POR" {
		t.Errorf("reset_flags = %q, want POR", p.ResetFlags)
	}
	if p.Seq != 548 {
		t.Errorf("seq = %d, want 548", p.Seq)
	}
	if p.HubSN != "HB-00000001" {
		t.Errorf("hub_sn = %q, want HB-00000001", p.HubSN)
	}
}

// --- DeviceStatus ---

func TestFromMessage_DeviceStatus_Topic(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "device_status.json"))
	want := "tempest/HB-00000001/ST-00000001/status"
	if e.Topic != want {
		t.Errorf("Topic = %q, want %q", e.Topic, want)
	}
}

func TestFromMessage_DeviceStatus_QoS(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "device_status.json"))
	if e.QoS != 1 {
		t.Errorf("QoS = %d, want 1", e.QoS)
	}
	if !e.Retain {
		t.Error("Retain = false, want true (last sensor state should persist)")
	}
}

func TestFromMessage_DeviceStatus_Payload(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "device_status.json"))

	var p event.DeviceStatusPayload
	unmarshalPayload(t, e, &p)

	if p.Timestamp != 1772383230 {
		t.Errorf("timestamp = %d, want 1772383230", p.Timestamp)
	}
	if p.UptimeS != 7593 {
		t.Errorf("uptime_s = %d, want 7593", p.UptimeS)
	}
	if p.BatteryV != 2.458 {
		t.Errorf("battery_v = %f, want 2.458", p.BatteryV)
	}
	if p.FirmwareRevision != 185 {
		t.Errorf("firmware_revision = %d, want 185", p.FirmwareRevision)
	}
	if p.RSSIdbm != -68 {
		t.Errorf("rssi_dbm = %d, want -68", p.RSSIdbm)
	}
	if p.HubRSSIdbm != -74 {
		t.Errorf("hub_rssi_dbm = %d, want -74", p.HubRSSIdbm)
	}
	if p.SensorStatus != 665600 {
		t.Errorf("sensor_status = %d, want 665600", p.SensorStatus)
	}
	if !p.SensorOK {
		t.Error("sensor_ok = false, want true (no fault bits set in lower 9 bits)")
	}
	if p.HubSN != "HB-00000001" {
		t.Errorf("hub_sn = %q, want HB-00000001", p.HubSN)
	}
	if p.SensorSN != "ST-00000001" {
		t.Errorf("sensor_sn = %q, want ST-00000001", p.SensorSN)
	}
}

func TestFromMessage_DeviceStatus_SensorFaults_AllClear(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "device_status.json"))

	var p event.DeviceStatusPayload
	unmarshalPayload(t, e, &p)

	f := p.SensorFaults
	if f.LightningSensorFailed || f.LightningSensorNoise || f.LightningSensorDisturber ||
		f.PressureSensorFailed || f.TemperatureSensorFailed || f.HumiditySensorFailed ||
		f.WindSensorFailed || f.PrecipSensorFailed || f.LightUVSensorFailed {
		t.Errorf("expected no sensor faults, got %+v", f)
	}
}

func TestFromMessage_DeviceStatus_SensorFaults_WindFailed(t *testing.T) {
	raw := &parser.DeviceStatus{
		SerialNumber: "ST-00000001",
		HubSN:        "HB-00000001",
		Timestamp:    1000000000,
		SensorStatus: 0x40, // bit 6 = wind sensor failed
	}
	e := singleEvent(t, raw)
	var p event.DeviceStatusPayload
	unmarshalPayload(t, e, &p)

	if !p.SensorFaults.WindSensorFailed {
		t.Error("WindSensorFailed = false, want true")
	}
	if p.SensorOK {
		t.Error("sensor_ok = true, want false when wind sensor is failed")
	}
}

func TestFromMessage_DeviceStatus_SensorFaults_LightningNoise(t *testing.T) {
	raw := &parser.DeviceStatus{
		SerialNumber: "ST-00000001",
		HubSN:        "HB-00000001",
		Timestamp:    1000000000,
		SensorStatus: 0x06, // bits 1+2 = lightning noise + disturber
	}
	e := singleEvent(t, raw)
	var p event.DeviceStatusPayload
	unmarshalPayload(t, e, &p)

	if !p.SensorFaults.LightningSensorNoise {
		t.Error("LightningSensorNoise = false, want true")
	}
	if !p.SensorFaults.LightningSensorDisturber {
		t.Error("LightningSensorDisturber = false, want true")
	}
	if p.SensorOK {
		t.Error("sensor_ok = true, want false when faults are present")
	}
}

// --- ObsST ---

func TestFromMessage_ObsST_Topic(t *testing.T) {
	events, err := event.FromMessage(parseFixture(t, "obs_st.json"))
	if err != nil {
		t.Fatalf("FromMessage() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	want := "tempest/HB-00000001/ST-00000001/observation"
	if events[0].Topic != want {
		t.Errorf("Topic = %q, want %q", events[0].Topic, want)
	}
}

func TestFromMessage_ObsST_QoS(t *testing.T) {
	events, _ := event.FromMessage(parseFixture(t, "obs_st.json"))
	e := events[0]
	if e.QoS != 1 {
		t.Errorf("QoS = %d, want 1", e.QoS)
	}
	if !e.Retain {
		t.Error("Retain = false, want true (current conditions should persist)")
	}
}

func TestFromMessage_ObsST_Payload_AllFields(t *testing.T) {
	events, _ := event.FromMessage(parseFixture(t, "obs_st.json"))
	var p event.ObservationPayload
	unmarshalPayload(t, events[0], &p)

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"timestamp", p.Timestamp, int64(1772383230)},
		{"wind_lull_ms", p.WindLullMS, 0.1},
		{"wind_avg_ms", p.WindAvgMS, 0.4},
		{"wind_gust_ms", p.WindGustMS, 0.89},
		{"wind_direction_deg", p.WindDirectionDeg, 107},
		{"wind_sample_interval_s", p.WindSampleIntervalS, 15},
		{"pressure_mb", p.PressureMB, 995.65},
		{"temperature_c", p.TemperatureC, 11.37},
		{"humidity_pct", p.HumidityPct, 73.64},
		{"illuminance_lux", p.IlluminanceLux, 2883},
		{"uv_index", p.UVIndex, 0.3},
		{"solar_radiation_wm2", p.SolarRadiationWM2, 24},
		{"rain_1min_mm", p.Rain1MinMM, 0.0},
		{"precip_type", p.PrecipType, 0},
		{"precip_type_str", p.PrecipTypeStr, "none"},
		{"lightning_distance_km", p.LightningDistKM, 0},
		{"lightning_count", p.LightningCount, 0},
		{"battery_v", p.BatteryV, 2.458},
		{"report_interval_min", p.ReportIntervalMin, 1},
		{"hub_sn", p.HubSN, "HB-00000001"},
		{"sensor_sn", p.SensorSN, "ST-00000001"},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestFromMessage_ObsST_PrecipType_Rain(t *testing.T) {
	raw := []byte(`{
		"serial_number":"ST-00000001","type":"obs_st","hub_sn":"HB-00000001",
		"obs":[[1000000000,0,0,0,0,15,1000,20,50,0,0,0,2.5,1,0,0,3.0,1]],
		"firmware_revision":185
	}`)
	msg, err := parser.Parse(raw)
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	events, err := event.FromMessage(msg)
	if err != nil {
		t.Fatalf("FromMessage: %v", err)
	}
	var p event.ObservationPayload
	unmarshalPayload(t, events[0], &p)

	if p.PrecipType != 1 {
		t.Errorf("precip_type = %d, want 1", p.PrecipType)
	}
	if p.PrecipTypeStr != "rain" {
		t.Errorf("precip_type_str = %q, want rain", p.PrecipTypeStr)
	}
	if p.Rain1MinMM != 2.5 {
		t.Errorf("rain_1min_mm = %f, want 2.5", p.Rain1MinMM)
	}
}

func TestFromMessage_ObsST_BatchedYieldsMultipleEvents(t *testing.T) {
	raw := []byte(`{
		"serial_number":"ST-00000001","type":"obs_st","hub_sn":"HB-00000001",
		"obs":[
			[1772383230,0.1,0.4,0.89,107,15,995.65,11.37,73.64,2883,0.3,24,0.0,0,0,0,2.458,1],
			[1772383290,0.0,0.2,0.45,180,15,995.70,11.40,73.50,2900,0.3,25,0.0,0,0,0,2.460,1]
		],
		"firmware_revision":185
	}`)
	msg, err := parser.Parse(raw)
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	events, err := event.FromMessage(msg)
	if err != nil {
		t.Fatalf("FromMessage: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}

	var p1, p2 event.ObservationPayload
	unmarshalPayload(t, events[0], &p1)
	unmarshalPayload(t, events[1], &p2)

	if p1.Timestamp != 1772383230 {
		t.Errorf("events[0].timestamp = %d, want 1772383230", p1.Timestamp)
	}
	if p2.Timestamp != 1772383290 {
		t.Errorf("events[1].timestamp = %d, want 1772383290", p2.Timestamp)
	}
	// Both events go to same topic
	if events[0].Topic != events[1].Topic {
		t.Errorf("expected same topic, got %q and %q", events[0].Topic, events[1].Topic)
	}
}

// --- EvtPrecip ---

func TestFromMessage_EvtPrecip_Topic(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "evt_precip.json"))
	want := "tempest/HB-00000001/ST-00000001/event/rain"
	if e.Topic != want {
		t.Errorf("Topic = %q, want %q", e.Topic, want)
	}
}

func TestFromMessage_EvtPrecip_QoS(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "evt_precip.json"))
	if e.QoS != 1 {
		t.Errorf("QoS = %d, want 1", e.QoS)
	}
	if e.Retain {
		t.Error("Retain = true, want false (transient event)")
	}
}

func TestFromMessage_EvtPrecip_Payload(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "evt_precip.json"))

	var p event.EvtPrecipPayload
	unmarshalPayload(t, e, &p)

	if p.Timestamp != 1772383500 {
		t.Errorf("timestamp = %d, want 1772383500", p.Timestamp)
	}
	if p.HubSN != "HB-00000001" {
		t.Errorf("hub_sn = %q, want HB-00000001", p.HubSN)
	}
	if p.SensorSN != "ST-00000001" {
		t.Errorf("sensor_sn = %q, want ST-00000001", p.SensorSN)
	}
}

// --- EvtStrike ---

func TestFromMessage_EvtStrike_Topic(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "evt_strike.json"))
	want := "tempest/HB-00000001/ST-00000001/event/lightning"
	if e.Topic != want {
		t.Errorf("Topic = %q, want %q", e.Topic, want)
	}
}

func TestFromMessage_EvtStrike_QoS(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "evt_strike.json"))
	if e.QoS != 1 {
		t.Errorf("QoS = %d, want 1", e.QoS)
	}
	if e.Retain {
		t.Error("Retain = true, want false (transient event)")
	}
}

func TestFromMessage_EvtStrike_Payload(t *testing.T) {
	e := singleEvent(t, parseFixture(t, "evt_strike.json"))

	var p event.EvtStrikePayload
	unmarshalPayload(t, e, &p)

	if p.Timestamp != 1772383500 {
		t.Errorf("timestamp = %d, want 1772383500", p.Timestamp)
	}
	if p.DistanceKM != 27 {
		t.Errorf("distance_km = %d, want 27", p.DistanceKM)
	}
	if p.Energy != 3849 {
		t.Errorf("energy = %d, want 3849", p.Energy)
	}
	if p.HubSN != "HB-00000001" {
		t.Errorf("hub_sn = %q, want HB-00000001", p.HubSN)
	}
	if p.SensorSN != "ST-00000001" {
		t.Errorf("sensor_sn = %q, want ST-00000001", p.SensorSN)
	}
}

// --- Unsupported type ---

// fakeMessage is a parser.Message type not handled by FromMessage.
type fakeMessage struct{}

func (f *fakeMessage) Type() string { return "fake" }

func TestFromMessage_UnsupportedType(t *testing.T) {
	_, err := event.FromMessage(&fakeMessage{})
	if err == nil {
		t.Fatal("FromMessage() expected error for unsupported type, got nil")
	}
}
