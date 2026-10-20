# Wyze Bridge (Hardened)

Hardened fork of the [IDisposable docker-wyze-bridge](https://github.com/IDisposable/docker-wyze-bridge)
Home Assistant add-on. go2rtc's API, HLS and WebRTC are loopback-only; RTSP
and the WebUI require credentials, and the bridge refuses to start without
them. Built locally by the Supervisor from a pinned commit of
[xtremeroi/docker-wyze-bridge](https://github.com/xtremeroi/docker-wyze-bridge/tree/hardened).

Licensed AGPL-3.0, like upstream. See [DOCS.md](DOCS.md) for setup.
