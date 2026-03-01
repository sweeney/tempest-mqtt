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
