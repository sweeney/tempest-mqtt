package event

// Tests in the internal package (not event_test) so they can replace jsonMarshal
// to exercise the json.Marshal error branches that are otherwise unreachable.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sweeney/tempest-mqtt/internal/parser"
)

var errFakeMarshal = errors.New("injected marshal error")

func failMarshal(_ any) ([]byte, error) {
	return nil, errFakeMarshal
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

func parseFixture(t *testing.T, name string) parser.Message {
	t.Helper()
	msg, err := parser.Parse(loadFixture(t, name))
	if err != nil {
		t.Fatalf("parser.Parse(%q): %v", name, err)
	}
	return msg
}

func withFailMarshal(t *testing.T) {
	t.Helper()
	orig := jsonMarshal
	jsonMarshal = failMarshal
	t.Cleanup(func() { jsonMarshal = orig })
}

func TestRapidWindEvent_MarshalError(t *testing.T) {
	withFailMarshal(t)
	msg := parseFixture(t, "rapid_wind.json")
	_, err := convert(msg, "test")
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
}

func TestHubStatusEvent_MarshalError(t *testing.T) {
	withFailMarshal(t)
	msg := parseFixture(t, "hub_status.json")
	_, err := convert(msg, "test")
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
}

func TestDeviceStatusEvent_MarshalError(t *testing.T) {
	withFailMarshal(t)
	msg := parseFixture(t, "device_status.json")
	_, err := convert(msg, "test")
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
}

func TestObsSTEvent_MarshalError(t *testing.T) {
	withFailMarshal(t)
	msg := parseFixture(t, "obs_st.json")
	_, err := convert(msg, "test")
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
}

func TestEvtPrecipEvent_MarshalError(t *testing.T) {
	withFailMarshal(t)
	msg := parseFixture(t, "evt_precip.json")
	_, err := convert(msg, "test")
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
}

func TestEvtStrikeEvent_MarshalError(t *testing.T) {
	withFailMarshal(t)
	msg := parseFixture(t, "evt_strike.json")
	_, err := convert(msg, "test")
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
}
