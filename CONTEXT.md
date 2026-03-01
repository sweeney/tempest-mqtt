# tempest-mqtt — Project Context

Everything a new session needs to start building immediately.

---

## What We're Building

A service that listens for UDP broadcasts from a WeatherFlow Tempest weather station hub on
the local network, parses the JSON messages, and re-publishes them as structured MQTT topics.

Primary use case: feed weather data into a home automation stack (Home Assistant, Node-RED, etc.)
that already speaks MQTT.

---

## Hardware (on the local network)

| Device | Role | Serial | Address |
|--------|------|--------|---------|
| Tempest Hub | UDP broadcaster | `HB-XXXXXXXX` | `192.168.X.XXX` |
| Tempest Sensor | All-in-one outdoor unit | `ST-XXXXXXXX` | (radio only, no IP) |

The hub is the only device on the network. It receives 863 MHz radio from the sensor and
re-broadcasts everything as UDP JSON to port 50222 on the local subnet.

---

## Protocol Summary

Full annotated walkthrough: `docs/protocol/walkthrough.md`
Original WeatherFlow reference: `docs/protocol/udp-docs.txt`

**Socket setup:** `SOCK_DGRAM`, bind `0.0.0.0:50222`, set `SO_BROADCAST` + `SO_REUSEADDR`.

### Message types

| type | Source | Cadence | Key payload |
|------|--------|---------|-------------|
| `rapid_wind` | Sensor | ~15s | `ob[1]` speed m/s, `ob[2]` dir degrees |
| `hub_status` | Hub | ~20s | `uptime`, `rssi` (WiFi), `seq` (drop detector) |
| `device_status` | Sensor | ~60s | `voltage`, `rssi`/`hub_rssi`, `sensor_status` bitmask |
| `obs_st` | Sensor | ~60s | 18-element array, full observation |
| `evt_precip` | Sensor | on event | rain started (no magnitude) |
| `evt_strike` | Sensor | on event | `evt[1]` distance km, `evt[2]` energy |

**`device_status` always arrives ~600ms before `obs_st`** — treat as a unit.

### obs_st array index map

```
[0]  timestamp          Unix epoch UTC (s)
[1]  wind_lull          m/s  (min over interval)
[2]  wind_avg           m/s  (mean over interval)
[3]  wind_gust          m/s  (max over interval)
[4]  wind_direction     degrees
[5]  wind_sample_interval  s  (3s active / 15s low-power)
[6]  pressure           mb  (station pressure — not MSLP)
[7]  temperature        °C
[8]  humidity           %
[9]  illuminance        lux
[10] uv                 index
[11] solar_radiation    W/m²
[12] rain_1min          mm
[13] precip_type        0=none 1=rain 2=hail 3=rain+hail
[14] lightning_dist     km  (avg)
[15] lightning_count    count
[16] battery            V  (supercap; healthy ~2.4–2.8V)
[17] report_interval    minutes (1, 3, or 5)
```

### sensor_status bitmask

Mask with `& 0x1FF` — only bits 0–8 are documented. If result is 0, all sensors are OK.
Upper bits (`0xA2800` on this device) are internal power-booster state flags; ignore them.

| Bit | Fault |
|-----|-------|
| 0 | lightning failed |
| 1 | lightning noise |
| 2 | lightning disturber |
| 3 | pressure failed |
| 4 | temperature failed |
| 5 | humidity failed |
| 6 | wind failed |
| 7 | precip failed |
| 8 | light/UV failed |

### Serial number convention

- `serial_number` = originating device (`HB-` = hub, `ST-` = Tempest sensor)
- `hub_sn` = hub that forwarded it (absent on `hub_status` since the hub speaks for itself)

---

## Test Fixtures

`tests/fixtures/captured_events.jsonl` — 22 real events captured over 3 minutes from the
live device, sanitized (serials and IP replaced with placeholders). Contains:

- 10 × `rapid_wind`
- 6 × `hub_status`
- 3 × `device_status`
- 3 × `obs_st`

Missing from fixtures (edge-triggered, didn't occur during capture):
- `evt_precip` — use the example from `docs/protocol/udp-docs.txt` to hand-craft a fixture
- `evt_strike` — same

---

## Decisions Still to Make

### Language / runtime
Python is already used for the capture tools and has good MQTT library support (`paho-mqtt`).
Alternative: Go (single binary, easy deployment). Nothing decided yet.

### MQTT topic structure
Not yet designed. Suggested starting point to discuss:
```
tempest/{hub_sn}/hub/status
tempest/{hub_sn}/{sensor_sn}/wind/rapid
tempest/{hub_sn}/{sensor_sn}/observation
tempest/{hub_sn}/{sensor_sn}/status
tempest/{hub_sn}/{sensor_sn}/evt/precip
tempest/{hub_sn}/{sensor_sn}/evt/strike
```
Alternatives to consider: flat vs nested, retain flags per topic, whether to publish
individual fields as separate topics (common in HA MQTT sensor config) vs a single JSON blob.

### MQTT payload format
- Raw JSON passthrough (minimal transformation, easy to debug)
- Normalised named-key JSON (expand the obs_st positional array into a dict)
- Both (raw + normalised on separate topics)

### Home Assistant integration
If HA is the target consumer, the easiest path is MQTT Discovery — publish a config payload
to `homeassistant/sensor/{sensor_sn}_{field}/config` and HA auto-creates entities.
This drives topic naming: each field probably wants its own topic with `retain: true`.

### Deployment
- Runs on the same host as HA / the MQTT broker, or standalone?
- Systemd unit? Docker container?
- Config: env vars, YAML, or CLI flags for MQTT host/port/credentials?

---

## Tools

| File | Purpose |
|------|---------|
| `tools/listen_udp.py` | Live monitor — prints events as they arrive |
| `tools/capture.py` | Capture to JSONL: `python3 tools/capture.py <secs> <outfile>` |

Run either from the repo root. Both need port 50222 free (kill any existing listener first):
```sh
lsof -i udp:50222 | awk 'NR>1 {print $2}' | xargs kill -9
```

---

## What's Been Done

- [x] Confirmed UDP reception from live device
- [x] Captured and documented all four active message types with real data
- [x] Built annotated walkthrough (`docs/protocol/walkthrough.md`) via showboat
- [x] Sanitized all PII from fixtures and walkthrough
- [ ] Project scaffold (language, package manager, src layout)
- [ ] Parser / decoder module
- [ ] MQTT publisher module
- [ ] Unit tests against fixtures
- [ ] Integration test (live UDP → assert MQTT publish)
- [ ] Deployment config
