package parser_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sweeney/tempest-mqtt/internal/parser"
)

// fixtureDir returns the path to tests/fixtures relative to this file.
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

// --- RapidWind ---

func TestParse_RapidWind_Fields(t *testing.T) {
	msg, err := parser.Parse(loadFixture(t, "rapid_wind.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	m, ok := msg.(*parser.RapidWind)
	if !ok {
		t.Fatalf("Parse() returned %T, want *parser.RapidWind", msg)
	}

	if m.Type() != parser.TypeRapidWind {
		t.Errorf("Type() = %q, want %q", m.Type(), parser.TypeRapidWind)
	}
	if m.SerialNumber != "ST-00000001" {
		t.Errorf("SerialNumber = %q, want %q", m.SerialNumber, "ST-00000001")
	}
	if m.HubSN != "HB-00000001" {
		t.Errorf("HubSN = %q, want %q", m.HubSN, "HB-00000001")
	}
	if m.Timestamp() != 1772383186 {
		t.Errorf("Timestamp() = %d, want 1772383186", m.Timestamp())
	}
	if m.SpeedMS() != 0.26 {
		t.Errorf("SpeedMS() = %f, want 0.26", m.SpeedMS())
	}
	if m.DirectionDeg() != 108 {
		t.Errorf("DirectionDeg() = %d, want 108", m.DirectionDeg())
	}
}

func TestParse_RapidWind_ZeroWind(t *testing.T) {
	raw := []byte(`{"serial_number":"ST-00000001","type":"rapid_wind","hub_sn":"HB-00000001","ob":[1000000000,0.0,0]}`)
	msg, err := parser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	m := msg.(*parser.RapidWind)
	if m.SpeedMS() != 0.0 {
		t.Errorf("SpeedMS() = %f, want 0.0", m.SpeedMS())
	}
	if m.DirectionDeg() != 0 {
		t.Errorf("DirectionDeg() = %d, want 0", m.DirectionDeg())
	}
}

// --- HubStatus ---

func TestParse_HubStatus_Fields(t *testing.T) {
	msg, err := parser.Parse(loadFixture(t, "hub_status.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	m, ok := msg.(*parser.HubStatus)
	if !ok {
		t.Fatalf("Parse() returned %T, want *parser.HubStatus", msg)
	}

	if m.Type() != parser.TypeHubStatus {
		t.Errorf("Type() = %q, want %q", m.Type(), parser.TypeHubStatus)
	}
	if m.SerialNumber != "HB-00000001" {
		t.Errorf("SerialNumber = %q, want %q", m.SerialNumber, "HB-00000001")
	}
	if m.FirmwareRevision != "309" {
		t.Errorf("FirmwareRevision = %q, want %q", m.FirmwareRevision, "309")
	}
	if m.Uptime != 7370 {
		t.Errorf("Uptime = %d, want 7370", m.Uptime)
	}
	if m.RSSI != -71 {
		t.Errorf("RSSI = %d, want -71", m.RSSI)
	}
	if m.Timestamp != 1772383213 {
		t.Errorf("Timestamp = %d, want 1772383213", m.Timestamp)
	}
	if m.ResetFlags != "POR" {
		t.Errorf("ResetFlags = %q, want %q", m.ResetFlags, "POR")
	}
	if m.Seq != 548 {
		t.Errorf("Seq = %d, want 548", m.Seq)
	}
	if len(m.RadioStats) != 5 {
		t.Errorf("len(RadioStats) = %d, want 5", len(m.RadioStats))
	}
}

// --- DeviceStatus ---

func TestParse_DeviceStatus_Fields(t *testing.T) {
	msg, err := parser.Parse(loadFixture(t, "device_status.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	m, ok := msg.(*parser.DeviceStatus)
	if !ok {
		t.Fatalf("Parse() returned %T, want *parser.DeviceStatus", msg)
	}

	if m.Type() != parser.TypeDeviceStatus {
		t.Errorf("Type() = %q, want %q", m.Type(), parser.TypeDeviceStatus)
	}
	if m.SerialNumber != "ST-00000001" {
		t.Errorf("SerialNumber = %q, want %q", m.SerialNumber, "ST-00000001")
	}
	if m.HubSN != "HB-00000001" {
		t.Errorf("HubSN = %q, want %q", m.HubSN, "HB-00000001")
	}
	if m.Timestamp != 1772383230 {
		t.Errorf("Timestamp = %d, want 1772383230", m.Timestamp)
	}
	if m.Uptime != 7593 {
		t.Errorf("Uptime = %d, want 7593", m.Uptime)
	}
	if m.Voltage != 2.458 {
		t.Errorf("Voltage = %f, want 2.458", m.Voltage)
	}
	if m.FirmwareRevision != 185 {
		t.Errorf("FirmwareRevision = %d, want 185", m.FirmwareRevision)
	}
	if m.RSSI != -68 {
		t.Errorf("RSSI = %d, want -68", m.RSSI)
	}
	if m.HubRSSI != -74 {
		t.Errorf("HubRSSI = %d, want -74", m.HubRSSI)
	}
	if m.SensorStatus != 665600 {
		t.Errorf("SensorStatus = %d, want 665600", m.SensorStatus)
	}
}

func TestParse_DeviceStatus_SensorOK(t *testing.T) {
	tests := []struct {
		name         string
		sensorStatus int
		wantOK       bool
	}{
		{"all clear", 665600, true},      // upper bits set (power booster), lower 9 bits clear
		{"all faults", 0x1FF, false},     // all 9 sensor fault bits set
		{"lightning noise", 0x02, false}, // bit 1 = lightning sensor noise
		{"wind failed", 0x40, false},     // bit 6 = wind sensor failed
		{"no faults", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &parser.DeviceStatus{SensorStatus: tt.sensorStatus}
			if got := m.SensorOK(); got != tt.wantOK {
				t.Errorf("SensorOK() = %v, want %v (status=0x%X)", got, tt.wantOK, tt.sensorStatus)
			}
		})
	}
}

// --- ObsST ---

func TestParse_ObsST_Fields(t *testing.T) {
	msg, err := parser.Parse(loadFixture(t, "obs_st.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	m, ok := msg.(*parser.ObsST)
	if !ok {
		t.Fatalf("Parse() returned %T, want *parser.ObsST", msg)
	}

	if m.Type() != parser.TypeObsST {
		t.Errorf("Type() = %q, want %q", m.Type(), parser.TypeObsST)
	}
	if m.SerialNumber != "ST-00000001" {
		t.Errorf("SerialNumber = %q, want %q", m.SerialNumber, "ST-00000001")
	}
	if m.HubSN != "HB-00000001" {
		t.Errorf("HubSN = %q, want %q", m.HubSN, "HB-00000001")
	}
	if len(m.Obs) != 1 {
		t.Fatalf("len(Obs) = %d, want 1", len(m.Obs))
	}
	if len(m.Obs[0]) != 18 {
		t.Fatalf("len(Obs[0]) = %d, want 18", len(m.Obs[0]))
	}
	if m.FirmwareRevision != 185 {
		t.Errorf("FirmwareRevision = %d, want 185", m.FirmwareRevision)
	}

	obs := m.Obs[0]
	checkNumber(t, obs[0], 1772383230, "timestamp")
	checkFloat(t, obs[1], 0.1, "wind_lull")
	checkFloat(t, obs[2], 0.4, "wind_avg")
	checkFloat(t, obs[3], 0.89, "wind_gust")
	checkNumber(t, obs[4], 107, "wind_direction")
	checkNumber(t, obs[5], 15, "wind_sample_interval")
	checkFloat(t, obs[6], 995.65, "pressure")
	checkFloat(t, obs[7], 11.37, "temperature")
	checkFloat(t, obs[8], 73.64, "humidity")
	checkNumber(t, obs[9], 2883, "illuminance")
	checkFloat(t, obs[10], 0.3, "uv")
	checkNumber(t, obs[11], 24, "solar_radiation")
	checkFloat(t, obs[12], 0.0, "rain_1min")
	checkNumber(t, obs[13], 0, "precip_type")
	checkNumber(t, obs[14], 0, "lightning_dist")
	checkNumber(t, obs[15], 0, "lightning_count")
	checkFloat(t, obs[16], 2.458, "battery")
	checkNumber(t, obs[17], 1, "report_interval")
}

func TestParse_ObsST_EmptyObs(t *testing.T) {
	raw := []byte(`{"serial_number":"ST-00000001","type":"obs_st","hub_sn":"HB-00000001","obs":[],"firmware_revision":185}`)
	_, err := parser.Parse(raw)
	if err == nil {
		t.Fatal("Parse() expected error for empty obs, got nil")
	}
}

func TestParse_ObsST_TooFewFields(t *testing.T) {
	raw := []byte(`{"serial_number":"ST-00000001","type":"obs_st","hub_sn":"HB-00000001","obs":[[1,2,3]],"firmware_revision":185}`)
	_, err := parser.Parse(raw)
	if err == nil {
		t.Fatal("Parse() expected error for obs with < 18 fields, got nil")
	}
}

func TestParse_ObsST_MultipleBatched(t *testing.T) {
	// Protocol allows multiple obs in a single message; all should be accepted.
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
		t.Fatalf("Parse() error = %v", err)
	}
	m := msg.(*parser.ObsST)
	if len(m.Obs) != 2 {
		t.Errorf("len(Obs) = %d, want 2", len(m.Obs))
	}
}

// --- EvtPrecip ---

func TestParse_EvtPrecip_Fields(t *testing.T) {
	msg, err := parser.Parse(loadFixture(t, "evt_precip.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	m, ok := msg.(*parser.EvtPrecip)
	if !ok {
		t.Fatalf("Parse() returned %T, want *parser.EvtPrecip", msg)
	}

	if m.Type() != parser.TypeEvtPrecip {
		t.Errorf("Type() = %q, want %q", m.Type(), parser.TypeEvtPrecip)
	}
	if m.SerialNumber != "ST-00000001" {
		t.Errorf("SerialNumber = %q, want %q", m.SerialNumber, "ST-00000001")
	}
	if m.HubSN != "HB-00000001" {
		t.Errorf("HubSN = %q, want %q", m.HubSN, "HB-00000001")
	}
	if m.Timestamp() != 1772383500 {
		t.Errorf("Timestamp() = %d, want 1772383500", m.Timestamp())
	}
}

// --- EvtStrike ---

func TestParse_EvtStrike_Fields(t *testing.T) {
	msg, err := parser.Parse(loadFixture(t, "evt_strike.json"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	m, ok := msg.(*parser.EvtStrike)
	if !ok {
		t.Fatalf("Parse() returned %T, want *parser.EvtStrike", msg)
	}

	if m.Type() != parser.TypeEvtStrike {
		t.Errorf("Type() = %q, want %q", m.Type(), parser.TypeEvtStrike)
	}
	if m.SerialNumber != "ST-00000001" {
		t.Errorf("SerialNumber = %q, want %q", m.SerialNumber, "ST-00000001")
	}
	if m.HubSN != "HB-00000001" {
		t.Errorf("HubSN = %q, want %q", m.HubSN, "HB-00000001")
	}
	if m.Timestamp() != 1772383500 {
		t.Errorf("Timestamp() = %d, want 1772383500", m.Timestamp())
	}
	if m.DistanceKM() != 27 {
		t.Errorf("DistanceKM() = %d, want 27", m.DistanceKM())
	}
	if m.Energy() != 3849 {
		t.Errorf("Energy() = %d, want 3849", m.Energy())
	}
}

// --- Error cases ---

func TestParse_InvalidJSON(t *testing.T) {
	_, err := parser.Parse([]byte(`not json`))
	if err == nil {
		t.Fatal("Parse() expected error for invalid JSON, got nil")
	}
}

func TestParse_MissingType(t *testing.T) {
	_, err := parser.Parse([]byte(`{"serial_number":"ST-00000001"}`))
	if err == nil {
		t.Fatal("Parse() expected error for missing type, got nil")
	}
}

func TestParse_UnknownType(t *testing.T) {
	_, err := parser.Parse([]byte(`{"type":"obs_air","serial_number":"AR-00000001"}`))
	if err == nil {
		t.Fatal("Parse() expected error for unknown type, got nil")
	}
}

func TestParse_EmptyInput(t *testing.T) {
	_, err := parser.Parse([]byte{})
	if err == nil {
		t.Fatal("Parse() expected error for empty input, got nil")
	}
}

// --- PrecipTypeString ---

func TestPrecipTypeString(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{parser.PrecipTypeNone, "none"},
		{parser.PrecipTypeRain, "rain"},
		{parser.PrecipTypeHail, "hail"},
		{parser.PrecipTypeRainAndHail, "rain_and_hail"},
		{99, "unknown"},
	}
	for _, tt := range tests {
		got := parser.PrecipTypeString(tt.in)
		if got != tt.want {
			t.Errorf("PrecipTypeString(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// --- unmarshal error paths (type-correct probe, malformed payload) ---

func TestParse_RapidWind_MalformedPayload(t *testing.T) {
	_, err := parser.Parse([]byte(`{"type":"rapid_wind","ob":"not-an-array"}`))
	if err == nil {
		t.Fatal("expected error for malformed rapid_wind payload, got nil")
	}
}

func TestParse_HubStatus_MalformedPayload(t *testing.T) {
	_, err := parser.Parse([]byte(`{"type":"hub_status","uptime":"not-a-number"}`))
	if err == nil {
		t.Fatal("expected error for malformed hub_status payload, got nil")
	}
}

func TestParse_DeviceStatus_MalformedPayload(t *testing.T) {
	_, err := parser.Parse([]byte(`{"type":"device_status","voltage":"not-a-number"}`))
	if err == nil {
		t.Fatal("expected error for malformed device_status payload, got nil")
	}
}

func TestParse_ObsST_MalformedPayload(t *testing.T) {
	_, err := parser.Parse([]byte(`{"type":"obs_st","obs":"not-an-array"}`))
	if err == nil {
		t.Fatal("expected error for malformed obs_st payload, got nil")
	}
}

func TestParse_EvtPrecip_MalformedPayload(t *testing.T) {
	_, err := parser.Parse([]byte(`{"type":"evt_precip","evt":"not-an-array"}`))
	if err == nil {
		t.Fatal("expected error for malformed evt_precip payload, got nil")
	}
}

func TestParse_EvtStrike_MalformedPayload(t *testing.T) {
	_, err := parser.Parse([]byte(`{"type":"evt_strike","evt":"not-an-array"}`))
	if err == nil {
		t.Fatal("expected error for malformed evt_strike payload, got nil")
	}
}

// --- helpers ---

func checkNumber(t *testing.T, n interface{ Int64() (int64, error) }, want int64, name string) {
	t.Helper()
	got, err := n.Int64()
	if err != nil {
		t.Errorf("%s: Int64() error: %v", name, err)
		return
	}
	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func checkFloat(t *testing.T, n interface{ Float64() (float64, error) }, want float64, name string) {
	t.Helper()
	got, err := n.Float64()
	if err != nil {
		t.Errorf("%s: Float64() error: %v", name, err)
		return
	}
	if got != want {
		t.Errorf("%s = %f, want %f", name, got, want)
	}
}
