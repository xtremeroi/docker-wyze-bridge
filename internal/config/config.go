// Package config loads and validates all bridge configuration from
// environment variables, Docker secrets, and optional YAML config files.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Config holds the canonical, validated configuration for the bridge.
type Config struct {
	// Wyze account credentials
	WyzeEmail    string
	WyzePassword string
	WyzeAPIID    string
	WyzeAPIKey   string
	WyzeTOTPKey  string

	// Bridge HTTP server (WebUI + REST API)
	BridgeIP       string // host IP used in WebRTC ICE candidates and URL generation
	BridgePort     int
	BridgeAuth     bool
	BridgeUsername string
	BridgePassword string
	BridgeAPIToken string // bearer token for REST API

	// HARDENED: when true, requests proxied by the Home Assistant
	// Supervisor's ingress (source 172.30.32.2 + X-Ingress-Path header)
	// skip WebUI auth, because HA has already authenticated the user.
	// Set by the HA add-on's run.sh only; default false.
	BridgeTrustHAIngress bool

	STUNServer string

	// Stream Auth — go2rtc RTSP/WebRTC consumer credentials
	StreamAuth string

	// External go2rtc. When non-empty the bridge uses this as its
	// go2rtc instead of spawning one — e.g. a shared go2rtc already
	// running for Frigate. Format: "http://host:1984". In this mode
	// the bridge does NOT write go2rtc.yaml and does NOT configure
	// recording; the user manages that upstream.
	Go2RTCURL string

	// go2rtc listener + auth knobs. Only meaningful when we're
	// spawning the sidecar (Go2RTCURL empty). Zero-valued ports fall
	// back to go2rtc's historical defaults (API 1984, RTSP 8554,
	// WebRTC HTTP 8889). Empty auth = unauthenticated. Motivated by
	// issues #123 (auth on 1984) and #108 (Frigate port collision).
	Go2RTCAPIPort      int
	Go2RTCAPIUsername  string
	Go2RTCAPIPassword  string
	Go2RTCRTSPPort     int
	Go2RTCWebRTCPort   int
	Go2RTCExtraStreams string // GO2RTC_EXTRA_STREAMS="name=src,name=src"
	Go2RTCExtraYAML    string // GO2RTC_EXTRA_YAML="…"; appended verbatim

	// MQTT
	MQTTEnabled        bool
	MQTTHost           string
	MQTTPort           int
	MQTTUsername       string
	MQTTPassword       string
	MQTTTopic          string
	MQTTDiscoveryTopic string // HA discovery prefix

	// Camera Filtering
	FilterNames  []string
	FilterModels []string
	FilterMACs   []string
	FilterBlocks bool

	// Camera Defaults
	Quality     string
	Audio       bool
	OfflineTime int

	// TUTKFallbackThreshold is the consecutive-TUTK-failure count at
	// which a camera is auto-promoted to the WebRTC path. Motivated
	// by Wyze's 2025-02 firmware disabling TUTK on newer HL_CAM4
	// units. Zero disables the auto-fallback (operators fall back to
	// the manual MODEL_OVERRIDES workaround). See
	// DOCS/TUTK_WEBRTC_FALLBACK_DESIGN.md.
	TUTKFallbackThreshold int

	// Recording
	RecordAll      bool
	RecordPath     string
	RecordFileName string
	RecordLength   time.Duration
	RecordKeep     time.Duration
	// RecordCmd overrides the default ffmpeg recording argv. Empty =
	// use the built-in default. See recording/template.go for the
	// supported token list. Per-camera overrides via RECORD_CMD_<CAM>
	// land on CamOverride.RecordCmd.
	RecordCmd string

	// Snapshots — SnapshotPath is the directory template (strftime +
	// {cam_name} subdirs OK); SnapshotFileName is the filename stem
	// template (.jpg auto-appended). Both accept the same tokens.
	SnapshotPath     string
	SnapshotFileName string
	SnapshotInterval int
	SnapshotKeep     time.Duration
	SnapshotCameras  []string

	// Sunrise/Sunset
	Latitude  float64
	Longitude float64

	// Paths
	StateDir string

	// Webhooks
	WebhookURLs string // comma-separated URLs

	// Debugging
	LogLevel        zerolog.Level
	ForceIOTCDetail bool

	// Gwell (IoTVideo) P2P proxy for GW_* cameras.
	GwellEnabled        bool
	GwellBinary         string
	GwellRTSPPort       int
	GwellControlPort    int
	GwellLogLevel       string
	GwellFFmpegLogLevel string // ffmpeg -loglevel inside gwell-proxy (quiet/panic/fatal/error/warning/info/verbose/debug/trace)
	GwellDumpDir        string // if non-empty, gwell-proxy tees raw H.264 to this dir for offline ffprobe
	GwellDeadmanTimeout time.Duration
	// ModelOverrides is the raw MODEL_OVERRIDES string applied to the
	// wyzeapi model registry at startup. See wyzeapi.ApplyModelOverrides.
	ModelOverrides string

	// Per-camera overrides keyed by normalized camera name (UPPER_CASE)
	CamOverrides map[string]CamOverride

	// Refresh interval for Wyze API camera list
	RefreshInterval time.Duration
}

// CamOverride holds per-camera setting overrides.
type CamOverride struct {
	Quality   *string
	Audio     *bool
	Record    *bool
	RecordCmd *string // per-camera RECORD_CMD_<CAM> — overrides global
}

// Load reads configuration from environment variables, Docker secrets,
// and an optional YAML config file, returning a validated Config.
//
// Env var naming was reorganized in 4.0 — see MIGRATION.md for the
// full rename table. No aliases are kept; 3.x configs must be updated.
func Load() (*Config, error) {
	cfg := &Config{
		// Wyze account credentials
		WyzeEmail:    secret("WYZE_EMAIL"),
		WyzePassword: secret("WYZE_PASSWORD"),
		WyzeAPIID:    secretWithAlias("WYZE_API_ID", "API_ID"),
		WyzeAPIKey:   secretWithAlias("WYZE_API_KEY", "API_KEY"),
		WyzeTOTPKey:  env("WYZE_TOTP_KEY", ""),

		// Bridge HTTP server
		BridgeIP:       env("BRIDGE_IP", ""),
		BridgePort:     envInt("BRIDGE_PORT", 5080),
		BridgeAuth:     envBool("BRIDGE_AUTH", false),
		BridgeUsername: env("BRIDGE_USERNAME", "wyze"),
		BridgePassword: env("BRIDGE_PASSWORD", ""),
		BridgeAPIToken: env("BRIDGE_API_TOKEN", ""),

		// HARDENED: HA ingress trust (set by the add-on's run.sh).
		BridgeTrustHAIngress: envBool("BRIDGE_TRUST_HA_INGRESS", false),

		STUNServer: env("STUN_SERVER", "stun:stun.l.google.com:19302"),

		// Stream Auth
		StreamAuth: env("STREAM_AUTH", ""),

		// External go2rtc (empty = spawn our own)
		Go2RTCURL: env("GO2RTC_URL", ""),

		Go2RTCAPIPort:      envInt("GO2RTC_API_PORT", 1984),
		Go2RTCAPIUsername:  env("GO2RTC_API_USERNAME", ""),
		Go2RTCAPIPassword:  env("GO2RTC_API_PASSWORD", ""),
		Go2RTCRTSPPort:     envInt("GO2RTC_RTSP_PORT", 8554),
		Go2RTCWebRTCPort:   envInt("GO2RTC_WEBRTC_PORT", 8889),
		Go2RTCExtraStreams: env("GO2RTC_EXTRA_STREAMS", ""),
		Go2RTCExtraYAML:    env("GO2RTC_EXTRA_YAML", ""),

		// MQTT
		MQTTEnabled:        envBool("MQTT_ENABLED", false),
		MQTTHost:           env("MQTT_HOST", ""),
		MQTTPort:           envInt("MQTT_PORT", 1883),
		MQTTUsername:       secret("MQTT_USERNAME"),
		MQTTPassword:       secret("MQTT_PASSWORD"),
		MQTTTopic:          env("MQTT_TOPIC", "wyzebridge"),
		MQTTDiscoveryTopic: env("MQTT_DISCOVERY_TOPIC", "homeassistant"),

		// Camera Filtering
		FilterNames:  envList("FILTER_NAMES"),
		FilterModels: envList("FILTER_MODELS"),
		FilterMACs:   envList("FILTER_MACS"),
		FilterBlocks: envBool("FILTER_BLOCKS", false),

		// Camera Defaults
		Quality:               env("QUALITY", "hd"),
		Audio:                 envBool("AUDIO", true),
		OfflineTime:           envInt("OFFLINE_TIME", 30),
		TUTKFallbackThreshold: envInt("TUTK_FALLBACK_THRESHOLD", 5),

		// Recording — default under /media/recordings so bare-Docker
		// users can mount a single host directory at /media and get
		// both recordings and snapshots. HA addon scopes its shared
		// /media via run.sh to /media/wyze_bridge/ subdirs.
		RecordAll:      envBool("RECORD_ALL", false),
		RecordPath:     env("RECORD_PATH", "/media/recordings/{cam_name}/%Y/%m/%d"),
		RecordFileName: env("RECORD_FILE_NAME", "%H-%M-%S"),
		RecordLength:   envDuration("RECORD_LENGTH", 60*time.Second),
		RecordKeep:     envDuration("RECORD_KEEP", 0),
		RecordCmd:      env("RECORD_CMD", ""),

		// Snapshots — per-camera/ per-day layout under /media/snapshots.
		// One date directory because snapshots are JPEGs, not multi-gig
		// recordings, and nesting Y/m/d is overkill for the file volume.
		// Override SNAPSHOT_FILE_NAME to {cam_name} for the old
		// flat-overwrite shape (single "latest" JPEG per camera) —
		// useful for integrations that poll for the current frame.
		SnapshotPath:     env("SNAPSHOT_PATH", "/media/snapshots/{cam_name}/%Y-%m-%d"),
		SnapshotFileName: env("SNAPSHOT_FILE_NAME", "%H-%M-%S"),
		SnapshotInterval: envSeconds("SNAPSHOT_INTERVAL", 0),
		SnapshotKeep:     envDuration("SNAPSHOT_KEEP", 0),
		SnapshotCameras:  envList("SNAPSHOT_CAMERAS"),

		// Sunrise/Sunset
		Latitude:  envFloat("LATITUDE", 0),
		Longitude: envFloat("LONGITUDE", 0),

		// Paths
		StateDir: env("STATE_DIR", "/config"),

		// Webhooks
		WebhookURLs: env("WEBHOOK_URLS", ""),

		// Debugging
		LogLevel:        parseLogLevel(env("LOG_LEVEL", "info")),
		ForceIOTCDetail: envBool("FORCE_IOTC_DETAIL", false),

		// Gwell (IoTVideo) P2P sidecar for OG-family cameras
		// (GW_GC1 / GW_GC2). Doorbell-lineage Gwell models
		// (GW_BE1 / GW_DBD) stream via go2rtc's native Wyze WebRTC
		// handler instead and don't need gwell-proxy at all; see
		// internal/wyzeapi/models.go IsWebRTCStreamer. Default ON —
		// the sidecar only spawns when an OG camera is actually
		// discovered, so users without OG cameras pay zero cost and
		// see no log spam. Set GWELL_ENABLED=false to opt out
		// entirely (skips OG discovery and any future Gwell work).
		GwellEnabled:        envBool("GWELL_ENABLED", true),
		GwellBinary:         env("GWELL_BINARY", ""),
		GwellRTSPPort:       envInt("GWELL_RTSP_PORT", 8564),
		GwellControlPort:    envInt("GWELL_CONTROL_PORT", 18564),
		GwellLogLevel:       env("GWELL_LOG_LEVEL", ""),
		GwellFFmpegLogLevel: env("GWELL_FFMPEG_LOGLEVEL", "warning"),
		GwellDumpDir:        env("GWELL_DUMP_DIR", ""),
		GwellDeadmanTimeout: envDuration("GWELL_DEADMAN_TIMEOUT", 2*time.Minute),
		ModelOverrides:      env("MODEL_OVERRIDES", ""),

		// Internals
		CamOverrides:    make(map[string]CamOverride),
		RefreshInterval: envDuration("REFRESH_INTERVAL", 30*time.Minute),
	}

	// HARDENED: upstream derived a default BRIDGE_PASSWORD from the
	// local part of WYZE_EMAIL. That password is guessable, so the
	// hardened build never invents one; ValidateHardened refuses to
	// start without explicit WebUI credentials.

	// MQTT_HOST presence implies MQTT_ENABLED
	if cfg.MQTTHost != "" {
		cfg.MQTTEnabled = true
	}

	// Load optional YAML config (for bare-Docker users who prefer
	// a config file to env vars; HA addon bashios options.json
	// into env vars before we get here).
	if err := cfg.loadYAML(); err != nil {
		// YAML is optional; log but don't fail
		fmt.Printf("warning: config.yml: %v\n", err)
	}

	// Load per-camera overrides from env
	cfg.loadCamOverrides()

	return cfg, nil
}

// CamQuality returns the effective quality for a camera.
func (c *Config) CamQuality(camName string) string {
	key := normalizeCamName(camName)
	if ov, ok := c.CamOverrides[key]; ok && ov.Quality != nil {
		return *ov.Quality
	}
	return c.Quality
}

// CamAudio returns the effective audio setting for a camera.
func (c *Config) CamAudio(camName string) bool {
	key := normalizeCamName(camName)
	if ov, ok := c.CamOverrides[key]; ok && ov.Audio != nil {
		return *ov.Audio
	}
	return c.Audio
}

// CamRecord returns the effective record setting for a camera.
func (c *Config) CamRecord(camName string) bool {
	key := normalizeCamName(camName)
	if ov, ok := c.CamOverrides[key]; ok && ov.Record != nil {
		return *ov.Record
	}
	return c.RecordAll
}

func normalizeCamName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(name), " ", "_"))
}

func parseLogLevel(s string) zerolog.Level {
	switch strings.ToLower(s) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// ValidateHardened is the HARDENED refuse-to-start check. It runs right
// after Load, before any Wyze login or listener, and returns one error
// listing every missing protection. It requires WebUI/REST auth
// (BRIDGE_AUTH=true plus BRIDGE_USERNAME and BRIDGE_PASSWORD, or
// BRIDGE_API_TOKEN, where token-only disables Basic), go2rtc API Basic
// auth (GO2RTC_API_USERNAME and GO2RTC_API_PASSWORD), and RTSP auth
// (STREAM_AUTH is exactly one global "user:password").
func (c *Config) ValidateHardened() error {
	var problems []string
	if !c.BridgeAuth {
		problems = append(problems, "WebUI auth is required: set BRIDGE_AUTH=true (add-on option bridge.auth)")
	}
	hasBasic := c.BridgeUsername != "" && c.BridgePassword != ""
	if !hasBasic && c.BridgeAPIToken == "" {
		problems = append(problems, "WebUI credentials are required: set BRIDGE_USERNAME + BRIDGE_PASSWORD and/or BRIDGE_API_TOKEN (add-on options bridge.username / bridge.password / bridge.api_token)")
	}
	if c.Go2RTCAPIUsername == "" || c.Go2RTCAPIPassword == "" {
		problems = append(problems, "go2rtc API auth is required: set GO2RTC_API_USERNAME and GO2RTC_API_PASSWORD (add-on options go2rtc.api_username / go2rtc.api_password)")
	}
	if !validGlobalStreamAuth(c.StreamAuth) {
		problems = append(problems, "RTSP auth is required: set STREAM_AUTH to one global \"user:password\" (no @camera list, no |, no @ in the password) (add-on options bridge.rtsp_username / bridge.rtsp_password)")
	}
	if len(problems) > 0 {
		return fmt.Errorf("refusing to start (hardened build): %s", strings.Join(problems, "; "))
	}
	return nil
}

// validGlobalStreamAuth reports whether raw is a single "user:password"
// entry that go2rtc will apply to its RTSP server. go2rtc only supports
// one global RTSP credential; per-camera entries were silently ignored
// upstream, leaving RTSP open.
func validGlobalStreamAuth(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "|@") {
		return false
	}
	user, pass, ok := strings.Cut(raw, ":")
	return ok && user != "" && pass != ""
}
