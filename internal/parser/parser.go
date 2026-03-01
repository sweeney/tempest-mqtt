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
