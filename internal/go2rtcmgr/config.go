package go2rtcmgr

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Go2RTCConfig is the YAML config for go2rtc.
type Go2RTCConfig struct {
	Log     LogConfig              `yaml:"log"`
	API     APIConfig              `yaml:"api"`
	RTSP    RTSPConfig             `yaml:"rtsp"`
	WebRTC  WebRTCConfig           `yaml:"webrtc"`
	SRTP    SRTPConfig             `yaml:"srtp"` // HARDENED: always emitted, disables go2rtc's default UDP :8443 listener
	Streams map[string]interface{} `yaml:"streams,omitempty"`
	Record  *RecordGlobalConfig    `yaml:"record,omitempty"`
}

// LogConfig controls go2rtc logging.
type LogConfig struct {
	Level string `yaml:"level"`
}

// APIConfig controls the go2rtc HTTP API.
type APIConfig struct {
	Listen string `yaml:"listen"`
	// HARDENED: omitempty so no `origin:` key is emitted at all. go2rtc
	// only adds CORS headers when origin == "*"; we never set it.
	Origin string `yaml:"origin,omitempty"`
	// HARDENED: go2rtc skips API Basic auth for loopback callers unless
	// local_auth is true. The add-on runs with host_network, so every
	// host-network process shares 127.0.0.1; require creds from them too.
	LocalAuth bool `yaml:"local_auth,omitempty"`
	// Username/Password enable HTTP Basic auth on /api/*. Empty =
	// unauthenticated (go2rtc's historical default). Loopback traffic
	// from wyze-bridge still authenticates the same way — the bridge
	// composes the same creds into its APIClient.
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// RTSPConfig controls the go2rtc RTSP server.
type RTSPConfig struct {
	Listen   string `yaml:"listen"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// WebRTCConfig controls WebRTC settings.
type WebRTCConfig struct {
	Listen     string      `yaml:"listen"`
	ICEServers []ICEServer `yaml:"ice_servers,omitempty"`
	Candidates []string    `yaml:"candidates,omitempty"`
}

// SRTPConfig controls go2rtc's SRTP (HomeKit) listener. HARDENED:
// Listen is deliberately not omitempty so an empty value is written
// and overrides go2rtc's built-in ":8443" default (empty = disabled).
type SRTPConfig struct {
	Listen string `yaml:"listen"`
}

// ICEServer represents a STUN/TURN server.
type ICEServer struct {
	URLs []string `yaml:"urls"`
}

// RecordGlobalConfig controls go2rtc's global recording.
type RecordGlobalConfig struct {
	OutputDir string `yaml:"output,omitempty"`
}

// StreamEntry contains the config for a single camera stream in go2rtc.
type StreamEntry struct {
	Name           string
	URL            string
	Record         bool
	RecordPath     string
	RecordDuration string
	RecordKeep     int // seconds, 0 = forever
}

// StreamAuthEntry represents a parsed STREAM_AUTH user credential.
type StreamAuthEntry struct {
	Username string
	Password string
	Cameras  []string // empty = all cameras
}

// ConfigBuilder builds go2rtc YAML configuration.
type ConfigBuilder struct {
	logLevel   string
	stunServer string
	wbIP       string
	streams    []StreamEntry
	streamAuth []StreamAuthEntry

	// Listener ports (defaults populated by NewConfigBuilder).
	apiPort    int
	rtspPort   int
	webrtcPort int

	// go2rtc /api/* Basic-auth. Empty = no auth (default).
	apiUsername string
	apiPassword string

	// Extra streams supplied by the operator via GO2RTC_EXTRA_STREAMS
	// or the HA add-on's `go2rtc.extra_streams` option. Rendered into
	// the `streams:` map alongside camera streams; last-writer wins on
	// name collision (extras loaded before camera streams so a camera
	// name always beats an operator-defined stub).
	extraStreams []StreamEntry
	// extraYAML is appended verbatim to the generated YAML after the
	// managed section. Escape hatch for anything the typed struct
	// doesn't model (custom go2rtc `publish:`/`hass:` blocks, ONVIF
	// discovery, etc.). No validation beyond parse-time YAML checks.
	extraYAML string
}

// NewConfigBuilder creates a new builder for go2rtc config.
func NewConfigBuilder(logLevel, stunServer, wbIP string) *ConfigBuilder {
	return &ConfigBuilder{
		logLevel:   logLevel,
		stunServer: stunServer,
		wbIP:       wbIP,
		apiPort:    1984,
		rtspPort:   8554,
		webrtcPort: 8889,
	}
}

// SetAPIPort overrides the go2rtc HTTP API listen port. Zero = keep default.
func (b *ConfigBuilder) SetAPIPort(port int) {
	if port > 0 {
		b.apiPort = port
	}
}

// SetRTSPPort overrides the go2rtc RTSP listen port. Zero = keep default.
func (b *ConfigBuilder) SetRTSPPort(port int) {
	if port > 0 {
		b.rtspPort = port
	}
}

// SetWebRTCPort overrides the go2rtc WebRTC HTTP listen port. Zero = keep default.
func (b *ConfigBuilder) SetWebRTCPort(port int) {
	if port > 0 {
		b.webrtcPort = port
	}
}

// APIPort returns the currently configured API port. main.go uses
// this to build the APIClient base URL after calling SetAPIPort.
func (b *ConfigBuilder) APIPort() int { return b.apiPort }

// SetAPIAuth enables HTTP Basic auth on go2rtc's /api/* endpoints.
// Both fields must be non-empty to take effect; an empty pair is a no-op
// so the operator can leave the env vars unset without accidentally
// enabling auth with blank credentials.
func (b *ConfigBuilder) SetAPIAuth(username, password string) {
	if username == "" || password == "" {
		return
	}
	b.apiUsername = username
	b.apiPassword = password
}

// APIUsername / APIPassword expose the resolved API auth so main.go can
// mint an APIClient that speaks the same credentials to go2rtc.
func (b *ConfigBuilder) APIUsername() string { return b.apiUsername }
func (b *ConfigBuilder) APIPassword() string { return b.apiPassword }

// AddExtraStream registers an operator-supplied stream (from
// GO2RTC_EXTRA_STREAMS or HA options). Same wire format as a camera
// stream but tracked separately so future ClearStreams calls (used
// for per-discovery-cycle regeneration) don't wipe them.
func (b *ConfigBuilder) AddExtraStream(entry StreamEntry) {
	b.extraStreams = append(b.extraStreams, entry)
}

// AppendRawYAML appends a fragment to the emitted YAML after the
// managed section. Called last during WriteConfig; fragment is not
// re-indented or re-validated by us — pass a valid go2rtc YAML block.
func (b *ConfigBuilder) AppendRawYAML(fragment string) {
	b.extraYAML = fragment
}

// ParseExtraStreams parses the GO2RTC_EXTRA_STREAMS format:
//
//	"name=source,name=source[;name=source]"
//
// Comma OR semicolon separates entries; each entry is name=source_url.
// Names are trimmed; sources are taken as-is (may contain any URL/scheme
// go2rtc understands). Entries missing an `=` are skipped silently — the
// intent is that operators can hand the string back through unchanged from
// the HA add-on without a schema round-trip.
func ParseExtraStreams(raw string) []StreamEntry {
	if raw == "" {
		return nil
	}
	// Normalize semicolons to commas so we accept either separator.
	normalized := strings.ReplaceAll(raw, ";", ",")
	var entries []StreamEntry
	for _, tok := range strings.Split(normalized, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		eq := strings.Index(tok, "=")
		if eq <= 0 || eq == len(tok)-1 {
			continue
		}
		name := strings.TrimSpace(tok[:eq])
		src := strings.TrimSpace(tok[eq+1:])
		if name == "" || src == "" {
			continue
		}
		entries = append(entries, StreamEntry{Name: name, URL: src})
	}
	return entries
}

// AddStream adds a camera stream to the config.
func (b *ConfigBuilder) AddStream(entry StreamEntry) {
	b.streams = append(b.streams, entry)
}

// ClearStreams removes all streams.
func (b *ConfigBuilder) ClearStreams() {
	b.streams = nil
}

// SetStreamAuth sets the parsed STREAM_AUTH entries.
func (b *ConfigBuilder) SetStreamAuth(entries []StreamAuthEntry) {
	b.streamAuth = entries
}

// ParseStreamAuth parses the STREAM_AUTH format: "user:pass@cam1,cam2|user2:pass2"
func ParseStreamAuth(raw string) []StreamAuthEntry {
	if raw == "" {
		return nil
	}
	var entries []StreamAuthEntry
	for _, segment := range strings.Split(raw, "|") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		// Split on @ to separate creds from camera list
		credPart := segment
		var cams []string
		if atIdx := strings.Index(segment, "@"); atIdx >= 0 {
			credPart = segment[:atIdx]
			camStr := segment[atIdx+1:]
			for _, c := range strings.Split(camStr, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					cams = append(cams, c)
				}
			}
		}
		// Split creds on : for user:pass
		parts := strings.SplitN(credPart, ":", 2)
		if len(parts) != 2 {
			continue
		}
		entries = append(entries, StreamAuthEntry{
			Username: parts[0],
			Password: parts[1],
			Cameras:  cams,
		})
	}
	return entries
}

// HARDENED: loopbackHost is the only interface the go2rtc API and the
// WebRTC media listener bind to. The bridge reaches go2rtc via
// http://127.0.0.1:<api_port> (Manager.APIURL), and browsers reach
// HLS / MSE / WebRTC signaling only through the authenticated WebUI
// proxies on :5080 (/hls/, /ws).
const loopbackHost = "127.0.0.1"

// Build generates the Go2RTCConfig struct.
func (b *ConfigBuilder) Build() *Go2RTCConfig {
	cfg := &Go2RTCConfig{
		Log: LogConfig{Level: b.logLevel},
		API: APIConfig{
			// HARDENED: was ":%d" (all interfaces) with Origin "*".
			// Now loopback-only, no CORS, Basic auth enforced for
			// loopback callers too (local_auth).
			Listen:    fmt.Sprintf("%s:%d", loopbackHost, b.apiPort),
			LocalAuth: true,
			Username:  b.apiUsername,
			Password:  b.apiPassword,
		},
		// RTSP stays on all interfaces (Home Assistant consumes it over
		// the LAN) but Validate() refuses to run without STREAM_AUTH.
		RTSP: RTSPConfig{Listen: fmt.Sprintf(":%d", b.rtspPort)},
		WebRTC: WebRTCConfig{
			// HARDENED: was ":%d". The WebRTC listener is the ICE media
			// mux (TCP+UDP), not signaling; signaling goes over the API
			// (/api/ws, /api/webrtc), now loopback-only. LAN browsers
			// therefore cannot complete WebRTC and the WebUI player uses
			// MSE over the authenticated /ws proxy instead. Outgoing
			// WebRTC (Wyze KVS cameras) uses go2rtc's separate client
			// API on ephemeral ports and is unaffected.
			Listen: fmt.Sprintf("%s:%d", loopbackHost, b.webrtcPort),
		},
		// HARDENED: disable the SRTP/HomeKit listener (default :8443).
		SRTP: SRTPConfig{Listen: ""},
		// Streams is nil when empty so YAML omits the key entirely.
		// go2rtc parses an empty flow-style `streams: {}` unreliably.
	}

	if b.stunServer != "" {
		cfg.WebRTC.ICEServers = []ICEServer{
			{URLs: []string{b.stunServer}},
		}
	}

	// HARDENED: no LAN ICE candidate (was "<BRIDGE_IP>:<webrtc_port>").
	// Advertising a LAN address for a loopback-bound listener would only
	// make browsers wait on a candidate that can never connect.

	// Apply STREAM_AUTH to RTSP config
	// If there's a single global auth entry (no per-camera restriction), set it on RTSP
	if len(b.streamAuth) == 1 && len(b.streamAuth[0].Cameras) == 0 {
		cfg.RTSP.Username = b.streamAuth[0].Username
		cfg.RTSP.Password = b.streamAuth[0].Password
	}

	total := len(b.streams) + len(b.extraStreams)
	if total > 0 {
		cfg.Streams = make(map[string]interface{}, total)
	}
	// Extras first so a camera name always overrides an operator-defined
	// stub — cameras are the primary product; extras are auxiliary.
	for _, s := range b.extraStreams {
		var sources []string
		if s.URL != "" {
			sources = []string{s.URL}
		} else {
			sources = []string{}
		}
		cfg.Streams[s.Name] = sources
	}
	for _, s := range b.streams {
		// Empty URL means publish-only slot (Gwell cameras: go2rtc
		// reserves the stream name with no source, accepts RTSP PUBLISH
		// into it). YAML form is `name: []` — not `name: [""]`, which
		// go2rtc would treat as a single malformed source and spam
		// errors at startup.
		var sources []string
		if s.URL != "" {
			sources = []string{s.URL}
		} else {
			sources = []string{}
		}
		if s.Record && s.RecordPath != "" {
			cfg.Streams[s.Name] = map[string]interface{}{
				"sources":         sources,
				"record":          true,
				"record_path":     s.RecordPath,
				"record_duration": s.RecordDuration,
			}
		} else {
			cfg.Streams[s.Name] = sources
		}
	}

	return cfg
}

// WriteConfig writes the go2rtc YAML config file.
func (b *ConfigBuilder) WriteConfig(path string) error {
	cfg := b.Build()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Add recording config as comments/structured entries for streams that need it
	// go2rtc handles recording per-stream via its API, but we set global config here
	var recordStreams []StreamEntry
	for _, s := range b.streams {
		if s.Record {
			recordStreams = append(recordStreams, s)
		}
	}

	if len(recordStreams) > 0 {
		// Append recording configuration as YAML
		var extra strings.Builder
		extra.WriteString("\n# Recording configuration\n")
		for _, s := range recordStreams {
			extra.WriteString(fmt.Sprintf("# Stream %q: record to %s\n", s.Name, s.RecordPath))
		}
		data = append(data, []byte(extra.String())...)
	}

	// Operator's GO2RTC_EXTRA_YAML escape hatch. Emitted after a
	// visible marker so anyone reading the file understands what's
	// managed vs. what came in verbatim.
	if strings.TrimSpace(b.extraYAML) != "" {
		data = append(data, []byte(
			"\n# --- managed by wyze-bridge; anything above is regenerated ---\n"+
				"# GO2RTC_EXTRA_YAML (verbatim operator input):\n")...)
		if !strings.HasSuffix(b.extraYAML, "\n") {
			b.extraYAML += "\n"
		}
		data = append(data, []byte(b.extraYAML)...)
	}

	// HARDENED: 0600 (was 0644). The file holds the go2rtc API and RTSP
	// credentials.
	return os.WriteFile(path, data, 0600)
}

// hardenedReservedKeys are top-level go2rtc sections GO2RTC_EXTRA_YAML
// may not redefine, because doing so could re-open a listener on the
// LAN or drop authentication.
var hardenedReservedKeys = []string{"api", "rtsp", "webrtc", "srtp", "rtmp", "ngrok", "webtorrent", "homekit"}

// Validate is the HARDENED pre-flight check run before the go2rtc YAML
// is written. It returns an error describing every missing protection
// so the bridge can refuse to start instead of running exposed.
func (b *ConfigBuilder) Validate() error {
	var problems []string
	if b.apiUsername == "" || b.apiPassword == "" {
		problems = append(problems, "go2rtc API Basic auth is required: set GO2RTC_API_USERNAME and GO2RTC_API_PASSWORD (add-on options go2rtc.api_username / go2rtc.api_password)")
	}
	if len(b.streamAuth) != 1 || len(b.streamAuth[0].Cameras) != 0 ||
		b.streamAuth[0].Username == "" || b.streamAuth[0].Password == "" {
		problems = append(problems, "RTSP auth is required: set STREAM_AUTH to exactly one global \"user:password\" entry with no @camera list and no | separators (add-on options bridge.rtsp_username / bridge.rtsp_password)")
	}
	if key := reservedTopLevelKey(b.extraYAML); key != "" {
		problems = append(problems, fmt.Sprintf("GO2RTC_EXTRA_YAML may not define the top-level %q section in the hardened build", key))
	}
	if len(problems) > 0 {
		return fmt.Errorf("refusing to start (hardened build): %s", strings.Join(problems, "; "))
	}
	return nil
}

// reservedTopLevelKey returns the first reserved top-level key found in
// a YAML fragment, or "". Only unindented `key:` lines count.
func reservedTopLevelKey(fragment string) string {
	for _, line := range strings.Split(fragment, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.Trim(strings.TrimSpace(name), `"'`)
		for _, k := range hardenedReservedKeys {
			if strings.EqualFold(name, k) {
				return k
			}
		}
	}
	return ""
}
