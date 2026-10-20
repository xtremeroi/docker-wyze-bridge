# NOTICE: modified version of docker-wyze-bridge

This repository (`xtremeroi/docker-wyze-bridge`, branch `hardened`) is a
modified version of [IDisposable/docker-wyze-bridge](https://github.com/IDisposable/docker-wyze-bridge),
licensed under the GNU Affero General Public License v3.0 (see `LICENSE`).
The modifications are released under the same license. Third-party
components keep their own licenses (see `THIRD_PARTY_NOTICES.md`).

Base: upstream `dev` at `c4991b1`, with upstream `main` at `eb6f6b9`
(4.6.0-edge.20260924.1038) merged in. Modified starting 2026-10-19.

Every code change is marked with a `HARDENED:` comment.

## Changes from upstream

### go2rtc configuration (`internal/go2rtcmgr/config.go`)

- API listen changed from `:<port>` (all interfaces) to `127.0.0.1:<port>`.
- API `origin: "*"` (CORS for any web page) removed.
- API `local_auth: true`, so go2rtc's Basic auth also applies to loopback
  callers (the add-on runs with `host_network`, so every host-network
  process shares 127.0.0.1).
- WebRTC media listener changed from `:<port>` to `127.0.0.1:<port>`, and
  the `<BRIDGE_IP>:<port>` LAN ICE candidate is no longer advertised.
- SRTP/HomeKit listener (go2rtc default UDP `:8443`) disabled.
- `go2rtc.yaml` written with mode 0600 instead of 0644.
- New `ConfigBuilder.Validate()`: requires API Basic auth, exactly one
  global `STREAM_AUTH` credential, and rejects `GO2RTC_EXTRA_YAML` that
  redefines `api`, `rtsp`, `webrtc`, `srtp`, `rtmp`, `ngrok`,
  `webtorrent` or `homekit`.

### Startup checks (`internal/config/config.go`, `cmd/wyze-bridge/main.go`)

- New `Config.ValidateHardened()`, called before any Wyze login or
  listener: the bridge exits with a "refusing to start (hardened build)"
  log line unless `BRIDGE_AUTH=true` with `BRIDGE_USERNAME` +
  `BRIDGE_PASSWORD` (or `BRIDGE_API_TOKEN`), `GO2RTC_API_USERNAME` +
  `GO2RTC_API_PASSWORD`, and a single global `STREAM_AUTH=user:password`
  are set.
- `BRIDGE_PASSWORD` is no longer derived from the local part of
  `WYZE_EMAIL`.
- New `BRIDGE_TRUST_HA_INGRESS` setting (default off).

### WebUI (`internal/webui`)

- `auth.go`: Basic auth is rejected when no password is configured
  (upstream accepted an empty-password Basic header in token-only
  setups); opt-in Home Assistant ingress bypass for requests whose TCP
  peer is the Supervisor (172.30.32.2) and that carry `X-Ingress-Path`.
- `server.go`: wildcard CORS headers removed; `/metrics.prom` requires
  auth.
- `static/video-stream.js`: the player uses `mse,hls,mjpeg` and skips
  WebRTC (its media port is loopback-only).

### Home Assistant add-on

- New `home_assistant/wyze_bridge_hardened` (slug `wyze_bridge_hardened`),
  derived from upstream's `wyze_bridge_edge`. It has no `image:` key and
  is built locally from a pinned commit of this fork; the go2rtc binary
  download is SHA-256 verified. Credentials are required by the option
  schema, only ports 5080 and 8554 are published, and `run.sh` builds
  `STREAM_AUTH` from `bridge.rtsp_username` / `bridge.rtsp_password` and
  enables the ingress bypass.
- Upstream's `home_assistant/wyze_bridge` and
  `home_assistant/wyze_bridge_edge` add-ons were removed from this branch,
  and `repository.yaml` / `repository.json` point at this fork.

### Tests

- Updated go2rtc listener expectations in existing tests.
- Added `internal/go2rtcmgr/hardened_test.go`,
  `internal/config/hardened_test.go`, `internal/webui/auth_hardened_test.go`.
