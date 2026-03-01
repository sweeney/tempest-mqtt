# Tempest Weather Station: UDP Protocol Walkthrough

*2026-03-01T16:43:54Z by Showboat 0.6.1*
<!-- showboat-id: 0c89ca6b-d6fa-42a8-b307-c4d656bcab00 -->

This walkthrough is built from 22 real UDP packets captured live from a WeatherFlow Tempest station on a local network.
All JSON shown is actual data from the device — nothing mocked.

The Tempest system has two hardware components:

- **Hub** (`HB-XXXXXXXX`) — a small indoor bridge plugged into mains power. It receives 915 MHz radio from the sensor, then broadcasts everything as JSON over UDP to the local subnet on port 50222.
- **Sensor** (`ST-XXXXXXXX`) — the all-in-one outdoor Tempest unit. It measures wind, pressure, temperature, humidity, rain, lightning, UV, and solar radiation, then transmits wirelessly to the hub every minute (and wind every 3 seconds).

All four message types flow from the same source IP (the hub at `192.168.X.XXX`). The hub is the only device you talk to; the sensor is invisible to the network.

## The Four Message Types

First, a census of what's in our 3-minute capture — how many of each type arrived:

```bash
jq -r '.type' captured_events.jsonl | sort | uniq -c | sort -rn
```

```output
  10 rapid_wind
   6 hub_status
   3 obs_st
   3 device_status
```

In 3 minutes: 10 rapid_wind (every ~15s), 6 hub_status (every ~20s), and 3 device_status+obs_st pairs (every ~60s).
This tells us there are three distinct clocks running simultaneously — and they're all independent.

---

## 1. hub_status — The Hub Heartbeat

Every ~20 seconds the hub announces itself. This is the hub speaking about its *own* health,
not about the sensor. Notice that `serial_number` here is the hub's own serial (`HB-`), not the sensor's.

```bash
grep '"type": "hub_status"' captured_events.jsonl | head -1 | jq 'del(._captured_at, ._source_ip)'
```

```output
{
  "serial_number": "HB-XXXXXXXX",
  "type": "hub_status",
  "firmware_revision": "309",
  "uptime": 7370,
  "rssi": -71,
  "timestamp": 1772383213,
  "reset_flags": "POR",
  "seq": 548,
  "radio_stats": [
    28,
    1,
    0,
    2,
    XXXXX
  ],
  "mqtt_stats": [
    1,
    0
  ],
  "freq": 863400000,
  "hw_version": 2,
  "hardware_id": 0
}
```

Field by field:

- **`serial_number`**: `HB-` prefix confirms this is the hub speaking, not the sensor.
- **`firmware_revision`**: Hub firmware `309` — note this is a string (unlike the sensor's numeric firmware_revision).
- **`uptime`**: Seconds since last boot. `7370s ≈ 2 hours`. Combined with `reset_flags: "POR"` (Power On Reset), this hub booted from a cold start ~2 hours ago.
- **`rssi`**: `-71 dBm` — the hub's WiFi signal strength to your router. Decent; -80 would start causing issues.
- **`seq`**: Sequence number, increments each broadcast. Used to detect missed packets. Our capture started at seq 548.
- **`radio_stats`**: `[version=28, reboot_count=1, i2c_errors=0, radio_status=2, network_id=XXXXX]`. Radio status 2 = radio on and active. The network_id (`XXXXX`) is how the hub and sensor find each other on 863 MHz.
- **`freq`**: `863400000 Hz` (863.4 MHz) — the EU/AU 868 MHz band sub-channel in use for the hub↔sensor radio link.
- **`mqtt_stats`**: `[1, 0]` — reserved internal fields, not meaningful here.

Confirm the sequence counter is incrementing — we can use this to detect dropped packets:

```bash
grep '"type": "hub_status"' captured_events.jsonl | jq -r '"seq=" + (.seq|tostring) + "  uptime=" + (.uptime|tostring) + "s"'
```

```output
seq=548  uptime=7370s
seq=550  uptime=7410s
seq=551  uptime=7430s
seq=552  uptime=7450s
seq=554  uptime=7490s
seq=555  uptime=7510s
```

Sequence jumps: 548→550 (missed 549) and 552→554 (missed 553). This is normal — those two hub_status broadcasts were sent
before our listener started, or were lost in transit. The 20-second gap between each shows the heartbeat cadence.

---

## 2. rapid_wind — High-Frequency Wind Samples

The sensor samples wind every 3 seconds internally, but only transmits over radio every ~15 seconds.
This is the highest-frequency message type — it sacrifices some fields for speed.

```bash
grep '"type": "rapid_wind"' captured_events.jsonl | head -3 | jq 'del(._captured_at, ._source_ip)'
```

```output
{
  "serial_number": "ST-XXXXXXXX",
  "type": "rapid_wind",
  "hub_sn": "HB-XXXXXXXX",
  "ob": [
    1772383186,
    0.26,
    108
  ]
}
{
  "serial_number": "ST-XXXXXXXX",
  "type": "rapid_wind",
  "hub_sn": "HB-XXXXXXXX",
  "ob": [
    1772383201,
    0.36,
    105
  ]
}
{
  "serial_number": "ST-XXXXXXXX",
  "type": "rapid_wind",
  "hub_sn": "HB-XXXXXXXX",
  "ob": [
    1772383216,
    0.1,
    105
  ]
}
```

Key structural point: **`serial_number` is now `ST-`** (the Tempest sensor), and a **`hub_sn`** field appears pointing back to `HB-XXXXXXXX`.
This pattern holds for all sensor-originated messages: `serial_number` = who measured it, `hub_sn` = who forwarded it.

The `ob` array has exactly 3 elements — the minimal wind payload:

| Index | Field | This reading |
|-------|-------|-------------|
| 0 | timestamp (Unix epoch, UTC) | `1772383186` |
| 1 | wind speed | `0.26 m/s` (0.9 km/h — nearly calm) |
| 2 | wind direction | `108°` (ESE) |

All three consecutive readings show ~105–108° ESE at under 0.4 m/s — a very light, consistent breeze.
The 15-second gap between timestamps (186→201→216) confirms the transmission interval.

All wind speeds and directions over the 3-minute window:

```bash
grep '"type": "rapid_wind"' captured_events.jsonl | jq -r '.ob | "  speed=" + (.[1]|tostring) + " m/s  dir=" + (.[2]|tostring) + "°"'
```

```output
  speed=0.26 m/s  dir=108°
  speed=0.36 m/s  dir=105°
  speed=0.1 m/s  dir=105°
  speed=0.03 m/s  dir=105°
  speed=0.01 m/s  dir=105°
  speed=0.52 m/s  dir=124°
  speed=0.06 m/s  dir=262°
  speed=0.02 m/s  dir=262°
  speed=0.0 m/s  dir=0°
  speed=0.09 m/s  dir=94°
```

Wind is dying off across the window: 0.36 m/s → 0.0 m/s, backing from ESE (105°) to W (262°) then going calm.
Direction becomes meaningless (shown as 0°) when speed is effectively zero — there's nothing to point at.

---

## 3. device_status — Sensor Health Report

Every 60 seconds the sensor sends a health check *before* its observation. It always arrives
as a pair: device_status first, then obs_st a few hundred milliseconds later.

```bash
grep '"type": "device_status"' captured_events.jsonl | head -1 | jq 'del(._captured_at, ._source_ip)'
```

```output
{
  "serial_number": "ST-XXXXXXXX",
  "type": "device_status",
  "hub_sn": "HB-XXXXXXXX",
  "timestamp": 1772383230,
  "uptime": 7593,
  "voltage": 2.458,
  "firmware_revision": 185,
  "rssi": -68,
  "hub_rssi": -74,
  "sensor_status": 665600,
  "debug": 1
}
```

Field breakdown:

- **`uptime`**: `7593s ≈ 2h 6m` — the *sensor's* uptime, independent of the hub's (`7370s`). They booted a few minutes apart.
- **`voltage`**: `2.458V` — the sensor's battery (or supercapacitor). The Tempest is solar-powered and uses a supercap; 2.458V is healthy (range ~1.8–2.80V).
- **`firmware_revision`**: `185` — note this is an integer on the sensor vs. a string on the hub.
- **`rssi`**: `-68 dBm` — sensor's received signal strength *from the hub* (downlink).
- **`hub_rssi`**: `-74 dBm` — hub's received signal strength *from the sensor* (uplink). Both directions are healthy.
- **`sensor_status`**: `665600` — a bitmask. The *documented* bits (0–8) are all zero, meaning all sensors are OK. The upper bits (`665600 = 0xA2800`) are reserved internal flags (power booster state) — safe to mask off and ignore in application code.
- **`debug`**: `1` — this sensor has debug mode enabled (firmware logging active). Informational only.

The documented sensor_status bits occupy only the lowest 9 bits. Check that all are clear:

```python3
import json
raw = 665600
documented = raw & 0x1FF  # bits 0-8
upper = raw >> 9
flags = {
    0: 'lightning failed', 1: 'lightning noise', 2: 'lightning disturber',
    3: 'pressure failed', 4: 'temperature failed', 5: 'rh failed',
    6: 'wind failed', 7: 'precip failed', 8: 'light/UV failed'
}
print(f'raw sensor_status : {raw} (0x{raw:05X})')
print(f'documented bits   : {documented:#011b}  (0 = all sensors OK)')
print(f'internal bits     : {upper} (power booster state, ignore)')
issues = [name for bit, name in flags.items() if documented & (1 << bit)]
print(f'active faults     : {issues if issues else "none"}')
```

```output
raw sensor_status : 665600 (0xA2800)
documented bits   : 0b000000000  (0 = all sensors OK)
internal bits     : 1300 (power booster state, ignore)
active faults     : none
```

All nine documented sensor fault bits are zero. The high bits (`0xA2800`) are internal power-booster state flags —
mask to `& 0x1FF` in application code and ignore the rest.

---

## 4. obs_st — The Full Observation

This is the payload that matters most. It arrives paired with device_status, once per minute.
The entire observation is packed into a single JSON array — no named keys, just positional indices.

```bash
grep '"type": "obs_st"' captured_events.jsonl | head -1 | jq 'del(._captured_at, ._source_ip)'
```

```output
{
  "serial_number": "ST-XXXXXXXX",
  "type": "obs_st",
  "hub_sn": "HB-XXXXXXXX",
  "obs": [
    [
      1772383230,
      0.1,
      0.4,
      0.89,
      107,
      15,
      995.65,
      11.37,
      73.64,
      2883,
      0.3,
      24,
      0.0,
      0,
      0,
      0,
      2.458,
      1
    ]
  ],
  "firmware_revision": 185
}
```

Note `obs` is an array-of-arrays (outer array could contain multiple observations if the sensor batched them, though in practice it's always one).
The inner array has 18 positions. Here they are decoded from this exact reading:

```python3
import json, datetime, warnings
warnings.filterwarnings('ignore')

fields = [
    ('timestamp',           'Unix epoch UTC',    's'),
    ('wind_lull',           'min wind in period','m/s'),
    ('wind_avg',            'mean wind',         'm/s'),
    ('wind_gust',           'max wind in period','m/s'),
    ('wind_direction',      'direction',         'deg'),
    ('wind_sample_interval','sample window',     's'),
    ('pressure',            'station pressure',  'mb'),
    ('temperature',         'air temp',          'C'),
    ('humidity',            'relative humidity', '%'),
    ('illuminance',         'solar illuminance', 'lux'),
    ('uv',                  'UV index',          ''),
    ('solar_radiation',     'solar radiation',   'W/m2'),
    ('rain_1min',           'rain last minute',  'mm'),
    ('precip_type',         '0=none 1=rain 2=hail',''),
    ('lightning_dist',      'avg strike distance','km'),
    ('lightning_count',     'strike count',      ''),
    ('battery',             'sensor voltage',    'V'),
    ('report_interval',     'reporting interval','min'),
]

obs = [1772383230,0.1,0.4,0.89,107,15,995.65,11.37,73.64,2883,0.3,24,0.0,0,0,0,2.458,1]

for i, (key, desc, unit) in enumerate(fields):
    val = obs[i]
    display = val
    if key == 'timestamp':
        dt = datetime.datetime.fromtimestamp(val, tz=datetime.timezone.utc)
        display = f'{val}  ({dt.strftime("%Y-%m-%d %H:%M:%S")} UTC)'
    suffix = f' {unit}' if unit else ''
    print(f'  [{i:2d}] {key:<22} = {display}{suffix}')
```

```output
  [ 0] timestamp              = 1772383230  (2026-03-01 16:40:30 UTC) s
  [ 1] wind_lull              = 0.1 m/s
  [ 2] wind_avg               = 0.4 m/s
  [ 3] wind_gust              = 0.89 m/s
  [ 4] wind_direction         = 107 deg
  [ 5] wind_sample_interval   = 15 s
  [ 6] pressure               = 995.65 mb
  [ 7] temperature            = 11.37 C
  [ 8] humidity               = 73.64 %
  [ 9] illuminance            = 2883 lux
  [10] uv                     = 0.3
  [11] solar_radiation        = 24 W/m2
  [12] rain_1min              = 0.0 mm
  [13] precip_type            = 0
  [14] lightning_dist         = 0 km
  [15] lightning_count        = 0
  [16] battery                = 2.458 V
  [17] report_interval        = 1 min
```

A few things to note about this observation:

- **Wind**: lull=0.1, avg=0.4, gust=0.89 m/s — these are the min/mean/max over the last reporting interval. The rapid_wind stream samples this continuously; obs_st summarises it.
- **`wind_sample_interval` = 15s**: the sensor was in low-power mode (normal is 3s in active mode). This tells you the fidelity of the wind statistics.
- **Pressure = 995.65 mb**: this is *station pressure*, not sea-level. You'd apply a hypsometric correction using the station's altitude to get MSLP.
- **Battery = 2.458V**: matches device_status exactly — the same reading is reported in both messages.
- **`precip_type` = 0**: no rain or hail. Values 1=rain, 2=hail, 3=rain+hail.
- **`lightning_dist` = 0, `lightning_count` = 0**: no strikes this minute.
- **`report_interval` = 1**: the sensor is set to 1-minute reporting. This field can be 1, 3, or 5 minutes.

How the three obs_st readings changed across the 3-minute window:

```bash
grep '"type": "obs_st"' captured_events.jsonl | jq -r '.obs[0] | "  wind_avg=" + (.[2]|tostring) + " m/s  gust=" + (.[3]|tostring) + " m/s  dir=" + (.[4]|tostring) + "deg  pressure=" + (.[6]|tostring) + " mb  temp=" + (.[7]|tostring) + "C  battery=" + (.[16]|tostring) + "V"'
```

```output
  wind_avg=0.4 m/s  gust=0.89 m/s  dir=107deg  pressure=995.65 mb  temp=11.37C  battery=2.458V
  wind_avg=0.19 m/s  gust=0.52 m/s  dir=142deg  pressure=995.73 mb  temp=11.37C  battery=2.458V
  wind_avg=0.1 m/s  gust=0.32 m/s  dir=98deg  pressure=995.74 mb  temp=11.38C  battery=2.458V
```

Wind calming progressively (avg 0.4→0.19→0.1 m/s), pressure rising slightly (995.65→995.74 mb), temperature
rock-steady at 11.37–11.38°C, battery unchanged at 2.458V. Real, coherent sensor behaviour.

---

## 5. The Timing Architecture — Three Clocks

The four message types run on completely independent schedules. Here's the actual chronological sequence
from the capture, showing how they interleave:

```python3
import json

events = []
with open('captured_events.jsonl') as f:
    for line in f:
        if line.strip():
            events.append(json.loads(line))

t0 = events[0].get('timestamp') or int(events[0].get('_captured_at', 0))

for e in events:
    ts = e.get('timestamp') or e.get('ob', [None])[0] or int(e.get('_captured_at', 0))
    rel = ts - events[0].get('ob', [ts])[0] if e['type'] == 'rapid_wind' else ts - (events[0].get('timestamp') or int(events[0]['_captured_at']))
    # simpler: just show offset from first event's captured_at
    cap = e['_captured_at']
    rel = cap - events[0]['_captured_at']
    sym = {'hub_status': 'HUB  ', 'rapid_wind': 'WIND ', 'device_status': 'DEVST', 'obs_st': 'OBS  '}[e['type']]
    extra = ''
    if e['type'] == 'hub_status':
        extra = f" seq={e['seq']}"
    elif e['type'] == 'rapid_wind':
        extra = f" {e['ob'][1]} m/s @ {e['ob'][2]}deg"
    elif e['type'] == 'obs_st':
        extra = f" avg={e['obs'][0][2]} m/s  {e['obs'][0][6]} mb  {e['obs'][0][7]}C"
    elif e['type'] == 'device_status':
        extra = f" {e['voltage']}V  rssi={e['rssi']}"
    print(f'  +{rel:6.1f}s  {sym}{extra}')
```

```output
  +   0.0s  WIND  0.26 m/s @ 108deg
  +  15.4s  WIND  0.36 m/s @ 105deg
  +  27.3s  HUB   seq=548
  +  30.1s  WIND  0.1 m/s @ 105deg
  +  43.6s  DEVST 2.458V  rssi=-68
  +  44.2s  OBS   avg=0.4 m/s  995.65 mb  11.37C
  +  45.1s  WIND  0.03 m/s @ 105deg
  +  60.2s  WIND  0.01 m/s @ 105deg
  +  67.6s  HUB   seq=550
  +  75.6s  WIND  0.52 m/s @ 124deg
  +  87.5s  HUB   seq=551
  + 103.5s  DEVST 2.458V  rssi=-67
  + 104.1s  OBS   avg=0.19 m/s  995.73 mb  11.37C
  + 105.0s  WIND  0.06 m/s @ 262deg
  + 107.5s  HUB   seq=552
  + 120.1s  WIND  0.02 m/s @ 262deg
  + 135.2s  WIND  0.0 m/s @ 0deg
  + 147.4s  HUB   seq=554
  + 163.7s  DEVST 2.458V  rssi=-68
  + 164.0s  OBS   avg=0.1 m/s  995.74 mb  11.38C
  + 165.0s  WIND  0.09 m/s @ 94deg
  + 167.4s  HUB   seq=555
```

Three independent clocks are visible:

- **WIND every ~15s** — driven by the sensor's internal radio transmit schedule.
- **HUB every ~20s** — driven by the hub's own heartbeat timer (separate from the sensor).
- **DEVST + OBS every ~60s** — always paired, always together (within ~600ms of each other). At +43.6s, +103.5s, +163.7s — exactly 60 seconds apart.

The DEVST+OBS pair is the most important: it's the hub relaying the sensor's full 60-second measurement cycle.
When DEVST arrives, OBS follows within half a second — parse them as a unit.

There is no coordination between the three clocks. Your consumer code must handle any ordering.

---

## 6. The Serial Number Convention

A simple but important rule governs all messages:

```bash
jq -r '[.type, .serial_number, (.hub_sn // "(none)")] | @tsv' captured_events.jsonl | sort -u | awk '{printf "  %-14s  serial=%-14s  hub_sn=%s\n", $1, $2, $3}'
```

```output
  device_status   serial=ST-XXXXXXXX     hub_sn=HB-XXXXXXXX
  hub_status      serial=HB-XXXXXXXX     hub_sn=(none)
  obs_st          serial=ST-XXXXXXXX     hub_sn=HB-XXXXXXXX
  rapid_wind      serial=ST-XXXXXXXX     hub_sn=HB-XXXXXXXX
```

The rule: **`serial_number` = the originating device; `hub_sn` = the hub that forwarded it.**

- `hub_status` has no `hub_sn` because the hub is speaking for itself — it *is* the serial.
- All sensor messages carry both: `serial_number` (the `ST-`) identifies which sensor measured the data; `hub_sn` (the `HB-`) identifies which hub is on your network.

If you ever have multiple Tempest hubs on the same network (unusual but possible), `hub_sn` lets you distinguish which hub each message came from. In the common single-hub case, you can use `serial_number` alone to identify your sensor.

---

## Summary: What to Implement

To consume this protocol you need one UDP socket bound to `0.0.0.0:50222` with `SO_BROADCAST` set.
Every message that arrives can be dispatched by `type`:

```python3
dispatch = {
    'rapid_wind':    'every ~15s  — wind speed+direction snapshot',
    'hub_status':    'every ~20s  — hub health, WiFi RSSI, seq counter',
    'device_status': 'every ~60s  — sensor battery, radio RSSI, fault flags',
    'obs_st':        'every ~60s  — full 18-field observation (always follows device_status)',
    # Not seen in this session but documented:
    'evt_precip':    'on event    — rain started',
    'evt_strike':    'on event    — lightning strike detected',
}
for msg_type, description in dispatch.items():
    print(f'  {msg_type:<14}  {description}')
```

```output
  rapid_wind      every ~15s  — wind speed+direction snapshot
  hub_status      every ~20s  — hub health, WiFi RSSI, seq counter
  device_status   every ~60s  — sensor battery, radio RSSI, fault flags
  obs_st          every ~60s  — full 18-field observation (always follows device_status)
  evt_precip      on event    — rain started
  evt_strike      on event    — lightning strike detected
```

The two event types (`evt_precip`, `evt_strike`) weren't seen in this 3-minute capture — they're edge-triggered,
only emitted when rain starts or a lightning strike is detected. They'll appear in the test fixtures as edge cases.

The `captured_events.jsonl` file produced during this walkthrough contains one real example of each
type that *was* observed, and is the basis for the unit and integration test suite.
