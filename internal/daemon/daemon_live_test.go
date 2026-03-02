package daemon_test

// Live-data tests use fixtures captured directly from a real WeatherFlow
// Tempest hub (sanitised: serial numbers replaced with ST-00000001 /
// HB-00000001).  They exercise the full pipeline — parse → convert →
// publish — against production message shapes and values, complementing
// the synthetic fixtures used in daemon_test.go.
//
// Captured: 2026-03-02, firmware hub=309 sensor=185, sensor_status=665600
// Conditions: cold night (8.3 °C), very humid (90 %), calm, low illuminance.

import (
	"encoding/json"
	"testing"
)

// --- RapidWind (live) ---

func TestLive_RapidWind_CalmConditions(t *testing.T) {
	// Real capture: calm night, 0 m/s, 0°
	msgs := runDaemon(t, loadFixture(t, "rapid_wind_live.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}

	if msgs[0].Topic != "climate/test/wind/rapid" {
		t.Errorf("topic = %q, want climate/test/wind/rapid", msgs[0].Topic)
	}
	if msgs[0].QoS != 0 {
		t.Errorf("QoS = %d, want 0", msgs[0].QoS)
	}
	if msgs[0].Retain {
		t.Error("Retain = true, want false")
	}

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
	if p.Timestamp != 1772442931 {
		t.Errorf("timestamp = %d, want 1772442931", p.Timestamp)
	}
	if p.SpeedMS != 0.0 {
		t.Errorf("speed_ms = %f, want 0.0 (calm)", p.SpeedMS)
	}
	if p.DirectionDeg != 0 {
		t.Errorf("direction_deg = %d, want 0 (calm/variable)", p.DirectionDeg)
	}
	if p.HubSN != "HB-00000001" {
		t.Errorf("hub_sn = %q, want HB-00000001", p.HubSN)
	}
	if p.SensorSN != "ST-00000001" {
		t.Errorf("sensor_sn = %q, want ST-00000001", p.SensorSN)
	}
}

// --- HubStatus (live) ---

func TestLive_HubStatus_LongUptime(t *testing.T) {
	// Real capture: hub has been running 67090 s (~18.6 h), seq=3534
	msgs := runDaemon(t, loadFixture(t, "hub_status_live.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}

	if msgs[0].Topic != "climate/test/status" {
		t.Errorf("topic = %q, want climate/test/status", msgs[0].Topic)
	}
	if msgs[0].QoS != 1 {
		t.Errorf("QoS = %d, want 1", msgs[0].QoS)
	}
	if !msgs[0].Retain {
		t.Error("Retain = false, want true")
	}

	var p struct {
		Timestamp        int64  `json:"timestamp"`
		UptimeS          int64  `json:"uptime_s"`
		RSSIdbm          int    `json:"rssi_dbm"`
		FirmwareRevision string `json:"firmware_revision"`
		ResetFlags       string `json:"reset_flags"`
		Seq              int    `json:"seq"`
		HubSN            string `json:"hub_sn"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Timestamp != 1772442934 {
		t.Errorf("timestamp = %d, want 1772442934", p.Timestamp)
	}
	if p.UptimeS != 67090 {
		t.Errorf("uptime_s = %d, want 67090", p.UptimeS)
	}
	if p.RSSIdbm != -76 {
		t.Errorf("rssi_dbm = %d, want -76", p.RSSIdbm)
	}
	if p.FirmwareRevision != "309" {
		t.Errorf("firmware_revision = %q, want 309", p.FirmwareRevision)
	}
	if p.ResetFlags != "POR" {
		t.Errorf("reset_flags = %q, want POR", p.ResetFlags)
	}
	if p.Seq != 3534 {
		t.Errorf("seq = %d, want 3534", p.Seq)
	}
	if p.HubSN != "HB-00000001" {
		t.Errorf("hub_sn = %q, want HB-00000001", p.HubSN)
	}
}

// --- DeviceStatus (live) ---

func TestLive_DeviceStatus_AllSensorsOK(t *testing.T) {
	// Real capture: sensor_status=665600 (upper bits set = internal power
	// booster state; lower 9 fault bits all clear → sensor_ok=true)
	msgs := runDaemon(t, loadFixture(t, "device_status_live.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}

	if msgs[0].Topic != "climate/test/device/status" {
		t.Errorf("topic = %q, want climate/test/device/status", msgs[0].Topic)
	}
	if msgs[0].QoS != 1 {
		t.Errorf("QoS = %d, want 1", msgs[0].QoS)
	}
	if !msgs[0].Retain {
		t.Error("Retain = false, want true")
	}

	var p struct {
		Timestamp    int64   `json:"timestamp"`
		UptimeS      int64   `json:"uptime_s"`
		BatteryV     float64 `json:"battery_v"`
		RSSIdbm      int     `json:"rssi_dbm"`
		HubRSSIdbm   int     `json:"hub_rssi_dbm"`
		SensorStatus int     `json:"sensor_status"`
		SensorOK     bool    `json:"sensor_ok"`
		HubSN        string  `json:"hub_sn"`
		SensorSN     string  `json:"sensor_sn"`
		SensorFaults struct {
			LightningSensorFailed    bool `json:"lightning_sensor_failed"`
			LightningSensorNoise     bool `json:"lightning_sensor_noise"`
			LightningSensorDisturber bool `json:"lightning_sensor_disturber"`
			PressureSensorFailed     bool `json:"pressure_sensor_failed"`
			TemperatureSensorFailed  bool `json:"temperature_sensor_failed"`
			HumiditySensorFailed     bool `json:"humidity_sensor_failed"`
			WindSensorFailed         bool `json:"wind_sensor_failed"`
			PrecipSensorFailed       bool `json:"precip_sensor_failed"`
			LightUVSensorFailed      bool `json:"light_uv_sensor_failed"`
		} `json:"sensor_faults"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Timestamp != 1772442975 {
		t.Errorf("timestamp = %d, want 1772442975", p.Timestamp)
	}
	if p.UptimeS != 67354 {
		t.Errorf("uptime_s = %d, want 67354", p.UptimeS)
	}
	if p.BatteryV != 2.449 {
		t.Errorf("battery_v = %f, want 2.449", p.BatteryV)
	}
	if p.RSSIdbm != -71 {
		t.Errorf("rssi_dbm = %d, want -71", p.RSSIdbm)
	}
	if p.HubRSSIdbm != -76 {
		t.Errorf("hub_rssi_dbm = %d, want -76", p.HubRSSIdbm)
	}
	if p.SensorStatus != 665600 {
		t.Errorf("sensor_status = %d, want 665600", p.SensorStatus)
	}
	if !p.SensorOK {
		t.Error("sensor_ok = false, want true (lower 9 fault bits are all clear)")
	}
	// Verify all individual fault flags are clear
	f := p.SensorFaults
	if f.LightningSensorFailed || f.LightningSensorNoise || f.LightningSensorDisturber ||
		f.PressureSensorFailed || f.TemperatureSensorFailed || f.HumiditySensorFailed ||
		f.WindSensorFailed || f.PrecipSensorFailed || f.LightUVSensorFailed {
		t.Errorf("expected no sensor faults, got %+v", f)
	}
	if p.HubSN != "HB-00000001" {
		t.Errorf("hub_sn = %q, want HB-00000001", p.HubSN)
	}
	if p.SensorSN != "ST-00000001" {
		t.Errorf("sensor_sn = %q, want ST-00000001", p.SensorSN)
	}
}

// --- ObsST (live) ---

func TestLive_ObsST_ColdHumidNight(t *testing.T) {
	// Real capture: 8.29 °C, 90.4 % RH, calm, low light — typical cold night.
	msgs := runDaemon(t, loadFixture(t, "obs_st_live.json"))
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}

	if msgs[0].Topic != "climate/test/observation" {
		t.Errorf("topic = %q, want climate/test/observation", msgs[0].Topic)
	}
	if msgs[0].QoS != 1 {
		t.Errorf("QoS = %d, want 1", msgs[0].QoS)
	}
	if !msgs[0].Retain {
		t.Error("Retain = false, want true")
	}

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
		HubSN               string  `json:"hub_sn"`
		SensorSN            string  `json:"sensor_sn"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"timestamp", p.Timestamp, int64(1772442974)},
		{"wind_lull_ms", p.WindLullMS, 0.0},
		{"wind_avg_ms", p.WindAvgMS, 0.0},
		{"wind_gust_ms", p.WindGustMS, 0.0},
		{"wind_direction_deg", p.WindDirectionDeg, 0},
		{"wind_sample_interval_s", p.WindSampleIntervalS, 15},
		{"pressure_mb", p.PressureMB, 998.24},
		{"temperature_c", p.TemperatureC, 8.29},
		{"humidity_pct", p.HumidityPct, 90.4},
		{"illuminance_lux", p.IlluminanceLux, 2652},
		{"uv_index", p.UVIndex, 0.27},
		{"solar_radiation_wm2", p.SolarRadiationWM2, 22},
		{"rain_1min_mm", p.Rain1MinMM, 0.0},
		{"precip_type", p.PrecipType, 0},
		{"precip_type_str", p.PrecipTypeStr, "none"},
		{"lightning_distance_km", p.LightningDistKM, 0},
		{"lightning_count", p.LightningCount, 0},
		{"battery_v", p.BatteryV, 2.449},
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

// --- Full pipeline (live) ---

func TestLive_AllFourMessageTypes_EndToEnd(t *testing.T) {
	// Feed one message of each real-world type through the full pipeline
	// in the order the hub sends them (~device_status just before obs_st).
	msgs := runDaemon(t,
		loadFixture(t, "hub_status_live.json"),
		loadFixture(t, "rapid_wind_live.json"),
		loadFixture(t, "device_status_live.json"),
		loadFixture(t, "obs_st_live.json"),
	)
	if len(msgs) != 4 {
		t.Fatalf("got %d published messages, want 4", len(msgs))
	}

	wantTopics := []string{
		"climate/test/status",
		"climate/test/wind/rapid",
		"climate/test/device/status",
		"climate/test/observation",
	}
	for i, want := range wantTopics {
		if msgs[i].Topic != want {
			t.Errorf("msgs[%d].Topic = %q, want %q", i, msgs[i].Topic, want)
		}
	}
}
