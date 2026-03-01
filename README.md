# tempest-mqtt

A lightweight daemon that bridges a [WeatherFlow Tempest](https://weatherflow.com/tempest-weather-system/) weather station to an MQTT broker. The hub broadcasts raw JSON over UDP on your local network; this daemon receives those broadcasts, decodes them into named-field JSON, and publishes them to structured MQTT topics — making your weather data available to Home Assistant, Node-RED, Grafana, or any other MQTT-aware system.

```
Tempest Hub  →  UDP :50222  →  tempest-mqtt  →  MQTT broker  →  your home automation
```

---

## Contents

- [How it works](#how-it-works)
- [MQTT topics and payloads](#mqtt-topics-and-payloads)
- [Configuration](#configuration)
- [Running locally](#running-locally)
- [Installing as a systemd service](#installing-as-a-systemd-service)
- [Building](#building)
- [Architecture](#architecture)
- [CI](#ci)

---

## How it works

The WeatherFlow Tempest hub continuously broadcasts JSON messages over UDP to port 50222 on your local network — no cloud, no API key, just raw packets. There are six message types:

| Hub message | Frequency | What it contains |
|---|---|---|
| `rapid_wind` | ~every 15 s | Wind speed and direction |
| `obs_st` | ~every 60 s | Full observation: temperature, humidity, pressure, UV, solar, rain, lightning, battery |
| `device_status` | ~every 60 s | Sensor diagnostics: battery voltage, RSSI, sensor fault bitmask |
| `hub_status` | ~every 20 s | Hub diagnostics: firmware, uptime, RSSI, radio stats |
| `evt_precip` | on event | Precipitation started |
| `evt_strike` | on event | Lightning strike detected |

tempest-mqtt does four things with each packet:

1. **Listen** — binds a UDP socket and reads datagrams into a 16-message buffer
2. **Parse** — peeks at the `type` field, then unmarshals into a typed Go struct
3. **Convert** — maps the typed struct to a named-field JSON payload and assigns an MQTT topic, QoS, and retain flag
4. **Publish** — sends the payload to the broker; errors on individual publishes are logged but never crash the daemon

Parse errors and publish errors are transient — they are logged as warnings and the main loop continues. The only fatal condition is a listener I/O error (e.g. the socket dies) or clean context cancellation (SIGINT / SIGTERM).

---

## MQTT topics and payloads

All topics are rooted at `climate/{prefix}` where `prefix` is set by the `-topic-prefix` flag (default: `tempest`).

### `climate/{prefix}/wind/rapid` — rapid wind

Published roughly every 15 seconds. QoS 0, not retained (stale wind readings are meaningless).

```json
{
  "timestamp": 1690000000,
  "speed_ms": 3.45,
  "direction_deg": 247,
  "hub_sn": "HB-00012345",
  "sensor_sn": "ST-00067890"
}
```

### `climate/{prefix}/observation` — full observation

Published roughly every 60 seconds. QoS 1, retained — so a new subscriber immediately gets the last known conditions.

```json
{
  "timestamp": 1690000060,
  "wind_lull_ms": 1.2,
  "wind_avg_ms": 2.8,
  "wind_gust_ms": 4.1,
  "wind_direction_deg": 255,
  "wind_sample_interval_s": 3,
  "pressure_mb": 1013.2,
  "temperature_c": 18.4,
  "humidity_pct": 72.0,
  "illuminance_lux": 14500,
  "uv_index": 3.2,
  "solar_radiation_wm2": 210,
  "rain_1min_mm": 0.0,
  "precip_type": 0,
  "precip_type_str": "none",
  "lightning_distance_km": 0,
  "lightning_count": 0,
  "battery_v": 2.641,
  "report_interval_min": 1,
  "hub_sn": "HB-00012345",
  "sensor_sn": "ST-00067890"
}
```

`precip_type_str` is one of `none`, `rain`, `hail`, `rain_and_hail`.

If the hub batches multiple observations into a single `obs_st` message (which can happen after a reconnect), each observation is published as a separate MQTT message.

### `climate/{prefix}/device/status` — sensor diagnostics

Published roughly every 60 seconds (about 600 ms before the paired `obs_st`). QoS 1, retained.

```json
{
  "timestamp": 1690000059,
  "uptime_s": 86400,
  "battery_v": 2.641,
  "firmware_revision": 171,
  "rssi_dbm": -62,
  "hub_rssi_dbm": -55,
  "sensor_status": 0,
  "sensor_ok": true,
  "sensor_faults": {
    "lightning_sensor_failed": false,
    "lightning_sensor_noise": false,
    "lightning_sensor_disturber": false,
    "pressure_sensor_failed": false,
    "temperature_sensor_failed": false,
    "humidity_sensor_failed": false,
    "wind_sensor_failed": false,
    "precip_sensor_failed": false,
    "light_uv_sensor_failed": false
  },
  "hub_sn": "HB-00012345",
  "sensor_sn": "ST-00067890"
}
```

`sensor_ok` is `true` when none of the lower 9 bits of `sensor_status` are set. The `sensor_faults` object decodes each bit individually so automations can react to specific failures without bit-shifting.

### `climate/{prefix}/status` — hub diagnostics

Published roughly every 20 seconds. QoS 1, retained.

```json
{
  "timestamp": 1690000020,
  "uptime_s": 86400,
  "rssi_dbm": -48,
  "firmware_revision": "177",
  "reset_flags": "BOR,PIN,POR",
  "seq": 1234,
  "hub_sn": "HB-00012345"
}
```

### `climate/{prefix}/event/rain` — precipitation started

Published when the sensor detects the start of rain. QoS 1, not retained (the event is transient — retaining it would mislead new subscribers into thinking it is currently raining).

```json
{
  "timestamp": 1690001200,
  "hub_sn": "HB-00012345",
  "sensor_sn": "ST-00067890"
}
```

### `climate/{prefix}/event/lightning` — lightning strike

Published when the sensor detects a lightning strike. QoS 1, not retained.

```json
{
  "timestamp": 1690002400,
  "distance_km": 12,
  "energy": 5432,
  "hub_sn": "HB-00012345",
  "sensor_sn": "ST-00067890"
}
```

### Last-will topic

On unexpected disconnect the broker automatically publishes:

```
Topic:   tempest/{client-id}/daemon/status
Payload: {"online":false}
QoS: 1, Retain: true
```

This lets automations detect if the daemon has crashed or lost network.

---

## Configuration

All options are CLI flags. When running as a systemd service they are set via environment variables in `/etc/tempest-mqtt.env`.

| Flag | Env var | Default | Description |
|---|---|---|---|
| `-broker` | `MQTT_BROKER_URL` | `tcp://localhost:1883` | MQTT broker URL |
| `-client-id` | `MQTT_CLIENT_ID` | `tempest-mqtt` | MQTT client identifier |
| `-username` | `MQTT_USERNAME` | _(none)_ | MQTT username |
| `-password` | `MQTT_PASSWORD` | _(none)_ | MQTT password |
| `-topic-prefix` | `TOPIC_PREFIX` | `tempest` | Prefix for all topics (`climate/{prefix}/...`) |
| `-udp-port` | `TEMPEST_UDP_PORT` | `50222` | UDP port to listen on |
| `-log-level` | `LOG_LEVEL` | `info` | One of `debug`, `info`, `warn`, `error` |

---

## Running locally

```bash
# Build
make build

# Run against a local broker (e.g. Mosquitto)
./tempest-mqtt -broker tcp://192.168.1.10:1883 -topic-prefix home

# Watch what comes in
mosquitto_sub -h 192.168.1.10 -t 'climate/#' -v
```

Use `-log-level debug` to see every message and publish logged to stderr.

---

## Installing as a systemd service

Requires root. Tested on Raspberry Pi OS and Debian/Ubuntu.

```bash
# 1. Build and install binary + service file
sudo make install

# 2. Create config (optional — all values have defaults)
sudo tee /etc/tempest-mqtt.env <<'EOF'
MQTT_BROKER_URL=tcp://192.168.1.10:1883
MQTT_USERNAME=myuser
MQTT_PASSWORD=secret
TOPIC_PREFIX=home
LOG_LEVEL=info
EOF

# 3. Start
sudo systemctl start tempest-mqtt

# 4. Check logs
journalctl -u tempest-mqtt -f
```

The service restarts automatically on failure (`Restart=on-failure`, 5 s delay) and starts after `network-online.target` so it won't try to connect to the broker before the network is up.

The service unit includes a set of systemd hardening directives (`NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp`, etc.) that restrict what the process can do even if it were compromised.

---

## Building

```bash
make build         # linux/amd64 binary
make cross-build   # linux/amd64, linux/arm64, linux/armv6 (Raspberry Pi)
make test          # run tests with race detector
make coverage      # per-function coverage report
make vet           # go vet
```

Binaries are statically linked with debug symbols stripped (`-ldflags="-s -w"`), producing a ~5 MB binary suitable for embedding on Raspberry Pi or similar.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  cmd/tempest-mqtt/main.go                           │
│  Parses flags, wires up components, handles signals │
└──────────────────────┬──────────────────────────────┘
                       │
         ┌─────────────▼──────────────┐
         │     internal/daemon        │
         │     Main event loop        │
         │  ReadMessage → Parse →     │
         │  Convert → Publish         │
         └──┬──────────────────────┬──┘
            │                      │
  ┌─────────▼────────┐   ┌─────────▼──────────┐
  │ internal/listener│   │ internal/publisher  │
  │ UDP socket       │   │ MQTT client (paho)  │
  │ buffered channel │   │ auto-reconnect      │
  └──────────────────┘   └─────────────────────┘
            │
  ┌─────────▼────────┐
  │ internal/parser  │
  │ Typed structs    │
  │ for 6 msg types  │
  └─────────▼────────┘
            │
  ┌─────────▼────────┐
  │ internal/event   │
  │ Topic routing    │
  │ QoS / retain     │
  │ Named-field JSON │
  └──────────────────┘
```

### Why interfaces everywhere

Every I/O boundary — the UDP listener, the MQTT publisher, and the event converter — is hidden behind a Go interface. This lets the entire daemon run in tests without a network: a `listener.Fake` feeds pre-captured messages and a `publisher.Fake` records what was published, with zero mocking libraries. All three core packages enforce 100% statement coverage in CI.

### Why the daemon continues on errors

A single bad UDP packet (malformed JSON from a firmware bug, a stray broadcast from another device) should not kill the daemon and stop all weather data. Parse errors and publish errors are logged at `warn` level and the loop continues. The only way `Run()` exits is on a listener I/O failure or context cancellation (SIGINT/SIGTERM).

### Why QoS 0 for rapid wind

Rapid wind arrives every 15 seconds. If one message is lost it is replaced by the next in 15 seconds. QoS 0 avoids the acknowledgment round-trip and the broker's need to store in-flight messages for a high-frequency, low-value stream.

### Why retained for status and observations

`hub_status`, `device_status`, and `obs_st` are retained so that a new subscriber (e.g. Home Assistant restarting after an update) immediately receives the last known state without waiting up to 60 seconds for the next broadcast.

### Why not retained for rain / lightning events

Retaining `evt_precip` or `evt_strike` would cause any new subscriber to immediately receive the last event — which could be days old — with no way to distinguish it from a current event. Events are inherently point-in-time, not state.
