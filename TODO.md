# TODO / Roadmap

Work remaining before this can be considered production-ready. Roughly in priority order.

---

## Deployment

- [ ] **Choose a host machine** — A Raspberry Pi on the same LAN as the Tempest hub is ideal (low power, always-on, close to the UDP broadcast). Alternatively any always-on Linux box on the same subnet works.
- [ ] **Confirm the host can receive UDP broadcasts** — The hub broadcasts to `255.255.255.255:50222`. Verify no firewall rule drops it: `sudo tcpdump -i any udp port 50222` should show packets while the hub is powered.
- [ ] **Install an MQTT broker if needed** — Mosquitto is the simplest option:
  ```bash
  sudo apt install mosquitto mosquitto-clients
  sudo systemctl enable --now mosquitto
  ```
- [ ] **Run `sudo make install`** on the target host — installs binary to `/usr/local/bin/` and service file to `/etc/systemd/system/`.
- [ ] **Create `/etc/tempest-mqtt.env`** with broker URL, credentials, and topic prefix.
- [ ] **Enable and start the service** — `sudo systemctl enable --now tempest-mqtt`.
- [ ] **Verify data is flowing** — `mosquitto_sub -h localhost -t 'climate/#' -v` should show messages within 20 seconds.

---

## Live testing

- [ ] **Smoke test with real hub** — Let it run for at least one full observation cycle (60 s) and confirm all six topic types appear. Check `journalctl -u tempest-mqtt -f` for any warn/error lines.
- [ ] **Test rapid wind** — `climate/{prefix}/wind/rapid` should arrive every ~15 s.
- [ ] **Test batched obs_st** — Disconnect the hub briefly (unplug for 2 min, reconnect). The hub may send a batched `obs_st` with multiple observations on reconnect. Verify each observation is published as a separate MQTT message.
- [ ] **Test evt_precip** — Wait for rain, or use a WeatherFlow test mode to simulate it. Confirm the event arrives and is not retained (disconnect and reconnect your MQTT client — the rain event should not replay).
- [ ] **Test evt_strike** — Same approach; confirm the event is not retained.
- [ ] **Test last-will** — Kill the daemon with `sudo kill -9 $(pidof tempest-mqtt)` and subscribe to `tempest/{client-id}/daemon/status`. The broker should immediately publish `{"online":false}`.
- [ ] **Test graceful restart** — `sudo systemctl restart tempest-mqtt`. Verify no messages are lost (compare timestamps before and after).
- [ ] **Test broker reconnect** — Restart Mosquitto while the daemon is running. Paho auto-reconnects; verify data resumes within a few seconds.
- [ ] **Test host reboot** — Reboot the host. Verify the service starts automatically and data resumes without manual intervention.

---

## Failure modes to harden

- [ ] **Broker permanently unavailable at startup** — Paho will retry but the daemon starts without confirmation the broker is reachable. Consider adding a startup health check / readiness log line once the first publish succeeds.
- [ ] **Hub on a different subnet** — UDP broadcasts don't cross router boundaries. If the hub and daemon are on different VLANs this silently produces no data. Document the requirement clearly; consider adding a "no messages received in N minutes" log warning.
- [ ] **Clock skew** — Tempest timestamps are Unix epoch from the hub's own clock. If the hub clock drifts (it syncs via the WeatherFlow cloud), published timestamps may be inconsistent with broker receive time. Consider logging a warning when the message timestamp is more than 60 s from wall clock.
- [ ] **High-frequency duplicate publish on reconnect** — After a hub reconnect, batched `obs_st` messages may flood the broker with many retained observations. The broker will handle it, but downstream consumers may be surprised. Document this behaviour.
- [ ] **MQTT client ID collision** — If two instances run with the same `-client-id`, the broker will kick one off and they will fight each other. Document that the client ID must be unique per instance.
- [ ] **Sensor fault alerting** — The `sensor_faults` payload fields are there but no alerting is wired. Consider publishing a dedicated `climate/{prefix}/device/alert` topic when `sensor_ok` is false, with QoS 1 and retain true, so a home automation system can trigger a notification.

---

## Observability

- [ ] **Structured log review** — Run with `-log-level debug` in production for one full day. Review logs for any unexpected warn/error lines before switching back to `info`.
- [ ] **Metrics** — There are currently no counters exposed. A future improvement would be a Prometheus `/metrics` endpoint (or log-based counter) tracking: messages received, parse errors, publish errors, and publish latency.
- [ ] **Grafana dashboard** — Once data is flowing into a time-series store (e.g. InfluxDB via Node-RED or Telegraf MQTT consumer), build a dashboard for temperature, pressure, wind, UV, battery, and RSSI trends.
- [ ] **Battery voltage alert** — `battery_v` below ~2.35 V indicates the sensor is at risk of losing power. Wire an automation to alert when this threshold is crossed.

---

## Security

- [ ] **MQTT credentials** — If the broker is exposed beyond localhost, enable authentication. Mosquitto: `mosquitto_passwd -c /etc/mosquitto/passwd tempest-mqtt`.
- [ ] **TLS** — For production or remote brokers, switch the broker URL to `ssl://host:8883` and configure the CA certificate. The paho client supports TLS but it is not wired up in the current publisher; would require adding `tls.Config` to the client options.
- [ ] **Review `/etc/tempest-mqtt.env` permissions** — It contains the MQTT password in plaintext. Ensure it is readable only by root: `sudo chmod 600 /etc/tempest-mqtt.env`.

---

## Code / project hygiene

- [ ] **Integration test against a real broker** — The current test suite uses fakes for both the listener and publisher. A `_integration_test.go` (build-tag gated) that spins up a real Mosquitto process and a real UDP socket would catch paho wiring bugs and connection-handling edge cases.
- [ ] **Listener reconnect on UDP socket error** — If `ReadFromUDP` returns a permanent error (e.g. "network is down"), the daemon currently exits. The service restarts it after 5 s, which is fine, but a smarter listener could re-bind the socket after a backoff without losing uptime.
- [ ] **Configuration via file** — Currently all config is CLI flags + env vars. A YAML/TOML config file might be friendlier for more complex setups (multiple prefixes, multiple brokers).
- [ ] **Release tagging and binary artefacts** — Add a GitHub Actions release workflow that triggers on `git tag v*`, builds cross-compiled binaries, and attaches them to a GitHub Release. Currently builds are only available as CI artefacts.
- [ ] **Dependabot or Renovate** — Automate dependency update PRs so paho and the Go toolchain stay current.
