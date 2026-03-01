// Package event converts parsed WeatherFlow Tempest messages into MQTT events.
//
// Topic structure:
//
//	tempest/{hub_sn}/status                        — hub health (~20s)
//	tempest/{hub_sn}/{sensor_sn}/status            — sensor health (~60s)
//	tempest/{hub_sn}/{sensor_sn}/wind/rapid        — rapid wind (~15s)
//	tempest/{hub_sn}/{sensor_sn}/observation       — full observation (~60s)
//	tempest/{hub_sn}/{sensor_sn}/event/rain        — precipitation started
//	tempest/{hub_sn}/{sensor_sn}/event/lightning   — lightning strike
//
// All payloads are JSON objects with named fields (no positional arrays).
package event

import (
	"encoding/json"
	"fmt"

	"github.com/sweeney/tempest-mqtt/internal/parser"
)

// Event is an MQTT message ready to publish.
type Event struct {
	Topic   string
	Payload []byte
	QoS     byte
	Retain  bool
}

// FromMessage converts a parsed Tempest message into one or more MQTT events.
// obs_st may yield multiple events when the hub batches observations.
func FromMessage(msg parser.Message) ([]*Event, error) {
	switch m := msg.(type) {
	case *parser.RapidWind:
		e, err := rapidWindEvent(m)
		return []*Event{e}, err
	case *parser.HubStatus:
		e, err := hubStatusEvent(m)
		return []*Event{e}, err
	case *parser.DeviceStatus:
		e, err := deviceStatusEvent(m)
		return []*Event{e}, err
	case *parser.ObsST:
		return obsSTEvents(m)
	case *parser.EvtPrecip:
		e, err := evtPrecipEvent(m)
		return []*Event{e}, err
	case *parser.EvtStrike:
		e, err := evtStrikeEvent(m)
		return []*Event{e}, err
	default:
		return nil, fmt.Errorf("event: unsupported message type %T", msg)
	}
}

// --- rapid_wind ---

// RapidWindPayload is the MQTT payload for rapid_wind events.
type RapidWindPayload struct {
	Timestamp    int64   `json:"timestamp"`
	SpeedMS      float64 `json:"speed_ms"`
	DirectionDeg int     `json:"direction_deg"`
	HubSN        string  `json:"hub_sn"`
	SensorSN     string  `json:"sensor_sn"`
}

func rapidWindEvent(m *parser.RapidWind) (*Event, error) {
	p := RapidWindPayload{
		Timestamp:    m.Timestamp(),
		SpeedMS:      m.SpeedMS(),
		DirectionDeg: m.DirectionDeg(),
		HubSN:        m.HubSN,
		SensorSN:     m.SerialNumber,
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal rapid_wind payload: %w", err)
	}
	return &Event{
		Topic:   fmt.Sprintf("tempest/%s/%s/wind/rapid", m.HubSN, m.SerialNumber),
		Payload: payload,
		QoS:     0,     // real-time; delivery not critical
		Retain:  false, // high-frequency; no value in retaining stale wind
	}, nil
}

// --- hub_status ---

// HubStatusPayload is the MQTT payload for hub_status events.
type HubStatusPayload struct {
	Timestamp        int64  `json:"timestamp"`
	UptimeS          int64  `json:"uptime_s"`
	RSSIdbm          int    `json:"rssi_dbm"`
	FirmwareRevision string `json:"firmware_revision"`
	ResetFlags       string `json:"reset_flags"`
	Seq              int    `json:"seq"`
	HubSN            string `json:"hub_sn"`
}

func hubStatusEvent(m *parser.HubStatus) (*Event, error) {
	p := HubStatusPayload{
		Timestamp:        m.Timestamp,
		UptimeS:          m.Uptime,
		RSSIdbm:          m.RSSI,
		FirmwareRevision: m.FirmwareRevision,
		ResetFlags:       m.ResetFlags,
		Seq:              m.Seq,
		HubSN:            m.SerialNumber,
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal hub_status payload: %w", err)
	}
	return &Event{
		Topic:   fmt.Sprintf("tempest/%s/status", m.SerialNumber),
		Payload: payload,
		QoS:     1,
		Retain:  true, // last known hub state should persist for new subscribers
	}, nil
}

// --- device_status ---

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

func deviceStatusEvent(m *parser.DeviceStatus) (*Event, error) {
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
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal device_status payload: %w", err)
	}
	return &Event{
		Topic:   fmt.Sprintf("tempest/%s/%s/status", m.HubSN, m.SerialNumber),
		Payload: payload,
		QoS:     1,
		Retain:  true, // last known sensor state should persist for new subscribers
	}, nil
}

// --- obs_st ---

// ObservationPayload is the MQTT payload for obs_st events.
// All array positional fields from the protocol are mapped to named JSON keys.
type ObservationPayload struct {
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

func obsSTEvents(m *parser.ObsST) ([]*Event, error) {
	events := make([]*Event, 0, len(m.Obs))
	for i, obs := range m.Obs {
		e, err := singleObsEvent(m.HubSN, m.SerialNumber, obs)
		if err != nil {
			return nil, fmt.Errorf("obs_st[%d]: %w", i, err)
		}
		events = append(events, e)
	}
	return events, nil
}

func singleObsEvent(hubSN, sensorSN string, obs []json.Number) (*Event, error) {
	intAt := func(idx int) int {
		v, _ := obs[idx].Int64()
		return int(v)
	}
	int64At := func(idx int) int64 {
		v, _ := obs[idx].Int64()
		return v
	}
	floatAt := func(idx int) float64 {
		v, _ := obs[idx].Float64()
		return v
	}

	precipType := intAt(13)
	p := ObservationPayload{
		Timestamp:           int64At(0),
		WindLullMS:          floatAt(1),
		WindAvgMS:           floatAt(2),
		WindGustMS:          floatAt(3),
		WindDirectionDeg:    intAt(4),
		WindSampleIntervalS: intAt(5),
		PressureMB:          floatAt(6),
		TemperatureC:        floatAt(7),
		HumidityPct:         floatAt(8),
		IlluminanceLux:      intAt(9),
		UVIndex:             floatAt(10),
		SolarRadiationWM2:   intAt(11),
		Rain1MinMM:          floatAt(12),
		PrecipType:          precipType,
		PrecipTypeStr:       parser.PrecipTypeString(precipType),
		LightningDistKM:     intAt(14),
		LightningCount:      intAt(15),
		BatteryV:            floatAt(16),
		ReportIntervalMin:   intAt(17),
		HubSN:               hubSN,
		SensorSN:            sensorSN,
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal observation payload: %w", err)
	}
	return &Event{
		Topic:   fmt.Sprintf("tempest/%s/%s/observation", hubSN, sensorSN),
		Payload: payload,
		QoS:     1,
		Retain:  true, // current conditions should persist for new subscribers
	}, nil
}

// --- evt_precip ---

// EvtPrecipPayload is the MQTT payload for evt_precip events.
type EvtPrecipPayload struct {
	Timestamp int64  `json:"timestamp"`
	HubSN     string `json:"hub_sn"`
	SensorSN  string `json:"sensor_sn"`
}

func evtPrecipEvent(m *parser.EvtPrecip) (*Event, error) {
	p := EvtPrecipPayload{
		Timestamp: m.Timestamp(),
		HubSN:     m.HubSN,
		SensorSN:  m.SerialNumber,
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal evt_precip payload: %w", err)
	}
	return &Event{
		Topic:   fmt.Sprintf("tempest/%s/%s/event/rain", m.HubSN, m.SerialNumber),
		Payload: payload,
		QoS:     1,
		Retain:  false, // transient event; no value retaining after rain stops
	}, nil
}

// --- evt_strike ---

// EvtStrikePayload is the MQTT payload for evt_strike events.
type EvtStrikePayload struct {
	Timestamp  int64  `json:"timestamp"`
	DistanceKM int    `json:"distance_km"`
	Energy     int64  `json:"energy"`
	HubSN      string `json:"hub_sn"`
	SensorSN   string `json:"sensor_sn"`
}

func evtStrikeEvent(m *parser.EvtStrike) (*Event, error) {
	p := EvtStrikePayload{
		Timestamp:  m.Timestamp(),
		DistanceKM: m.DistanceKM(),
		Energy:     m.Energy(),
		HubSN:      m.HubSN,
		SensorSN:   m.SerialNumber,
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal evt_strike payload: %w", err)
	}
	return &Event{
		Topic:   fmt.Sprintf("tempest/%s/%s/event/lightning", m.HubSN, m.SerialNumber),
		Payload: payload,
		QoS:     1,
		Retain:  false, // transient event
	}, nil
}
