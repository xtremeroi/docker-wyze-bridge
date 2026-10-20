# Wyze Bridge (Hardened) for Home Assistant

Hardened fork of [IDisposable/docker-wyze-bridge](https://github.com/IDisposable/docker-wyze-bridge)
(AGPL-3.0). Stream your Wyze cameras locally over authenticated RTSP. See
[NOTICE.md](https://github.com/xtremeroi/docker-wyze-bridge/blob/hardened/NOTICE.md)
for every change from upstream.

What is different from the upstream add-on:

- go2rtc's API (port 1984, which also serves HLS and WebRTC signaling) and
  its WebRTC media port (8889) listen on 127.0.0.1 only. No CORS.
- RTSP (8554) and the WebUI / REST API (5080) require credentials. The
  bridge refuses to start without them.
- The add-on is built on your Home Assistant host from a pinned commit of
  the fork (first install takes several minutes) instead of pulling a
  prebuilt image.

## Setup

1. Install the add-on from the repository
2. In the **Configuration** tab, fill in the **Wyze** section:
   - **email** — your Wyze account email
   - **password** — your Wyze account password
   - **api_id** — API ID from [Wyze Developer Console](https://developer-api-console.wyze.com/#/apikey/view)
   - **api_key** — API Key from the same page
3. Fill in the required hardened fields (the add-on will not start without them):
   - **bridge.password** — WebUI / REST API password (12+ characters; username defaults to `wyze`)
   - **bridge.rtsp_username** and **bridge.rtsp_password** — RTSP credentials (no `@` or `|`)
   - **go2rtc.api_password** — internal go2rtc API password (12+ characters; username defaults to `go2rtc`)
   - optionally **bridge.ip** — your Home Assistant host's LAN IP, used in displayed stream URLs
4. Start the add-on
5. Open the WebUI from the sidebar (ingress does not ask for the WebUI password)
6. In Home Assistant, add cameras with `rtsp://RTSP_USER:RTSP_PASS@HOST:8554/<camera_name>`

The Configuration tab is grouped into collapsible sections: **wyze**, **bridge**, **camera**, **snapshot**, **record**, **mqtt**, **filter**, **location**, **webhooks**, **gwell**, **debug**. Anything beyond the fields above is optional — click "Show unused optional configuration options" to expose the rest.

## MQTT Auto-Detection

If the Mosquitto broker add-on is installed, MQTT is auto-configured — leave the **mqtt** section empty and the bridge picks up Mosquitto's host/credentials automatically. Home Assistant discovery messages are published so cameras appear as entities without further setup.

## Camera Filtering

Under the **filter** section:

- **names**, **models**, **macs** — each is a list; the UI gives you an "Add" button per entry. Narrows which cameras the bridge exposes.
- **blocks** — when `true`, the lists above act as a block-list instead of an allow-list.

## Recording

Under the **record** section:

- **all** — `true` records every camera; per-camera overrides via `camera.options` below still apply.
- **path** — directory template. HA default: `/media/wyze_bridge/recordings/{cam_name}/%Y/%m/%d`.
- **file_name** — filename template; `.mp4` auto-appended. Default: `%H-%M-%S`.
- **length** — segment length. Examples: `60s`, `5m`, `1h`.
- **keep** — auto-delete segments older than this. `0` = keep forever. Examples: `7d`, `72h`.

## Snapshots

Under the **snapshot** section:

- **path** — directory template. HA default: `/media/wyze_bridge/snapshots/{cam_name}/%Y/%m/%d`.
- **file_name** — filename template; `.jpg` auto-appended. Default: `%H-%M-%S` (time-of-day).
- **interval** — seconds between captures. `0` disables periodic capture.
- **keep** — auto-delete old snapshots. Same format as `record.keep`.
- **cameras** — list of camera names to restrict snapshots to. Empty list = all cameras.

Both `path` and `file_name` accept `{cam_name}`, `{CAM_NAME}`, and strftime tokens `%Y %m %d %H %M %S %s`. Subdirectories in `path` are created automatically.

## Per-Camera Options

Add per-camera overrides under `camera.options`:

```yaml
camera:
  options:
    - cam_name: front_door
      quality: hd
      record: true
    - cam_name: backyard
      audio: false
```

Camera names are matched case-insensitively; spaces become underscores. Only letters, digits, and underscores are preserved — cameras with other punctuation in their Wyze-account names can't be matched via this option.

## Location (sunrise/sunset snapshots)

Fill **location.latitude** and **location.longitude** to have the bridge capture an extra snapshot at civil sunrise and sunset.

## Webhooks

Add entries to **webhooks.urls** (the UI gives a "+" Add button per URL). The bridge POSTs JSON to each URL on every camera state change (offline → discovering → connecting → streaming → error). HA validates each entry as a URL.

## Gwell Cameras (OG, Doorbell Pro, Doorbell Duo)

The **gwell.enabled** toggle controls the proxy for Wyze's newer Gwell/IoTVideo cameras (GW_BE1, GW_GC1, GW_GC2, GW_DBD). **Default is `false` in 4.0-beta** because the proxy binary isn't shipped in the Docker image yet — flipping it on today will put GW_* cameras in a permanent retry loop. Leave it off until a release note says otherwise.

## REST API / WebUI Auth

Under the **bridge** section:

- **auth** — must be `true` (default). WebUI and REST API require HTTP Basic auth with `username` + `password`.
- **api_token** — optional bearer token accepted on REST API requests.
- **rtsp_username** / **rtsp_password** — the single RTSP credential go2rtc enforces on port 8554.

Requests that come through Home Assistant ingress (the sidebar panel) are
already authenticated by Home Assistant and do not prompt again. The bridge
recognises them by their source address (the Supervisor, 172.30.32.2) plus
the `X-Ingress-Path` header.

In-browser playback uses MSE over the WebUI's authenticated `/ws` proxy.
WebRTC playback in the browser is intentionally unavailable because the
WebRTC media port is loopback-only.

## Ports

| Port | Protocol | Reachable from LAN | Protection |
| ------ | ---------- | ------------------ | ---------- |
| 5080 | TCP | yes | WebUI Basic auth or API token (ingress trusted) |
| 8554 | TCP | yes | RTSP auth (`rtsp_username` / `rtsp_password`) |
| 1984 | TCP | no (127.0.0.1) | go2rtc API Basic auth |
| 8889 | TCP+UDP | no (127.0.0.1) | WebRTC media, signaling via the API only |

## Support

- [Fork issues](https://github.com/xtremeroi/docker-wyze-bridge/issues)
- [Upstream Migration Guide](https://github.com/IDisposable/docker-wyze-bridge/blob/main/MIGRATION.md)
