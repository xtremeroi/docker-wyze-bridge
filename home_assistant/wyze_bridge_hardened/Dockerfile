# Wyze Bridge (Hardened) - Home Assistant add-on, built locally by the
# Supervisor (config.yaml has no `image:` key).
#
# HARDENED: upstream's add-on Dockerfile copied binaries out of the
# prebuilt ghcr.io/idisposable image. This one compiles wyze-bridge and
# gwell-proxy from a pinned commit of the xtremeroi fork, and downloads
# go2rtc with a SHA-256 check, so what runs is exactly the reviewed code.
#
# The Supervisor's build context is this folder only, so the source is
# fetched with git. To ship new code: commit it, then bump
# WYZE_BRIDGE_REF below (and `version` in config.yaml) in a later commit.

ARG BUILD_FROM=ghcr.io/home-assistant/base:3.21

# ── Stage 1: build from source ──────────────────────────────────────────────
FROM golang:1.26.4-alpine AS builder

# Supervisor passes BUILD_ARCH (aarch64 / amd64). Default to aarch64 so a
# plain `docker build` on Apple Silicon works too.
ARG BUILD_ARCH=aarch64
ARG WYZE_BRIDGE_REPO=https://github.com/xtremeroi/docker-wyze-bridge.git
ARG WYZE_BRIDGE_REF=d50c92d63c18814884335b622ccc1a8db43cf7d6
ARG GO2RTC_VERSION=1.9.14
# Commit the v1.9.14 tag points at (video-rtc.js is fetched from it).
ARG GO2RTC_COMMIT=b5948cfb25404cc5cb37b166ecaa2dca20b11d4b
# SHA-256 digests of the v1.9.14 release assets (GitHub release API).
ARG GO2RTC_SHA256_AARCH64=359fabade8a7a51e81a55fe6df6b0ef81764a5e1d63179577534eaaa71904b50
ARG GO2RTC_SHA256_AMD64=32d616af226bd731678ffde328b94cfb94e30339bfefc469cfb76323144615a6

RUN apk add --no-cache git curl ca-certificates

WORKDIR /src
RUN git init -q . && \
    git remote add origin "${WYZE_BRIDGE_REPO}" && \
    git fetch -q --depth 1 origin "${WYZE_BRIDGE_REF}" && \
    git checkout -q FETCH_HEAD && \
    git log -1 --format='building wyze-bridge from %H (%s)'

RUN case "${BUILD_ARCH}" in \
      aarch64) GOARCH_DL=arm64; SUM="${GO2RTC_SHA256_AARCH64}" ;; \
      amd64)   GOARCH_DL=amd64; SUM="${GO2RTC_SHA256_AMD64}" ;; \
      *) echo "unsupported BUILD_ARCH ${BUILD_ARCH}" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/AlexxIT/go2rtc/releases/download/v${GO2RTC_VERSION}/go2rtc_linux_${GOARCH_DL}" -o /go2rtc && \
    echo "${SUM}  /go2rtc" | sha256sum -c - && \
    chmod +x /go2rtc && \
    curl -fsSL "https://raw.githubusercontent.com/AlexxIT/go2rtc/${GO2RTC_COMMIT}/www/video-rtc.js" \
      -o internal/webui/static/video-rtc.js

RUN go mod download

ARG BUILD_VERSION=4.6.0-hardened.1
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w -X main.Version=${BUILD_VERSION}" \
      -o /wyze-bridge ./cmd/wyze-bridge && \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /gwell-proxy ./cmd/gwell-proxy

# ── Stage 2: Home Assistant runtime ─────────────────────────────────────────
FROM ${BUILD_FROM}

# ffmpeg serves go2rtc JPEG snapshots and the gwell-proxy RTSP publish.
# ca-certificates + tzdata for Wyze API TLS and timestamp handling.
RUN apk add --no-cache ca-certificates tzdata ffmpeg && \
    mkdir -p /config /media/wyze_bridge/snapshots /media/wyze_bridge/recordings

COPY --from=builder /wyze-bridge /usr/local/bin/wyze-bridge
COPY --from=builder /gwell-proxy /usr/local/bin/gwell-proxy
COPY --from=builder /go2rtc      /usr/local/bin/go2rtc

COPY run.sh /
RUN chmod +x /run.sh

CMD [ "/run.sh" ]
