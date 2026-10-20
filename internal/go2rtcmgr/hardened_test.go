package go2rtcmgr

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// HARDENED: tests for the loopback-only / auth-required go2rtc config.

func hardenedBuilder() *ConfigBuilder {
	b := NewConfigBuilder("info", "stun:stun.l.google.com:19302", "192.168.68.75")
	b.SetAPIAuth("g2r", "api-secret")
	b.SetStreamAuth(ParseStreamAuth("viewer:rtsp-secret"))
	return b
}

func TestHardened_BuildBindsAndAuth(t *testing.T) {
	cfg := hardenedBuilder().Build()

	if cfg.API.Listen != "127.0.0.1:1984" {
		t.Errorf("API.Listen = %q, want 127.0.0.1:1984", cfg.API.Listen)
	}
	if cfg.API.Origin != "" {
		t.Errorf("API.Origin = %q, want empty", cfg.API.Origin)
	}
	if !cfg.API.LocalAuth {
		t.Error("API.LocalAuth = false, want true (auth for loopback callers too)")
	}
	if cfg.API.Username != "g2r" || cfg.API.Password != "api-secret" {
		t.Errorf("API auth = %q/%q, want g2r/api-secret", cfg.API.Username, cfg.API.Password)
	}
	if cfg.RTSP.Listen != ":8554" {
		t.Errorf("RTSP.Listen = %q, want :8554", cfg.RTSP.Listen)
	}
	if cfg.RTSP.Username != "viewer" || cfg.RTSP.Password != "rtsp-secret" {
		t.Errorf("RTSP auth = %q/%q, want viewer/rtsp-secret", cfg.RTSP.Username, cfg.RTSP.Password)
	}
	if cfg.WebRTC.Listen != "127.0.0.1:8889" {
		t.Errorf("WebRTC.Listen = %q, want 127.0.0.1:8889", cfg.WebRTC.Listen)
	}
	if len(cfg.WebRTC.Candidates) != 0 {
		t.Errorf("WebRTC.Candidates = %v, want none", cfg.WebRTC.Candidates)
	}
	if cfg.SRTP.Listen != "" {
		t.Errorf("SRTP.Listen = %q, want empty (disabled)", cfg.SRTP.Listen)
	}
}

func TestHardened_WrittenYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go2rtc.yaml")
	if err := hardenedBuilder().WriteConfig(path); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "origin") {
		t.Errorf("YAML must not contain an origin key:\n%s", text)
	}

	// Parse generically to check the exact keys go2rtc will read.
	var raw map[string]map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw["api"]["listen"] != "127.0.0.1:1984" {
		t.Errorf("api.listen = %v", raw["api"]["listen"])
	}
	if raw["api"]["local_auth"] != true {
		t.Errorf("api.local_auth = %v, want true", raw["api"]["local_auth"])
	}
	if raw["webrtc"]["listen"] != "127.0.0.1:8889" {
		t.Errorf("webrtc.listen = %v", raw["webrtc"]["listen"])
	}
	srtp, ok := raw["srtp"]
	if !ok {
		t.Fatal("srtp section missing; go2rtc would fall back to its :8443 default")
	}
	if v, ok := srtp["listen"]; !ok || v != "" {
		t.Errorf("srtp.listen = %v (present=%v), want empty string", v, ok)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("go2rtc.yaml perm = %o, want 600", perm)
		}
	}
}

func TestHardened_ValidateOK(t *testing.T) {
	if err := hardenedBuilder().Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestHardened_ValidateRefuses(t *testing.T) {
	cases := []struct {
		name  string
		setup func(b *ConfigBuilder)
		want  string
	}{
		{"no api auth", func(b *ConfigBuilder) {
			b.apiUsername, b.apiPassword = "", ""
		}, "GO2RTC_API_USERNAME"},
		{"no stream auth", func(b *ConfigBuilder) {
			b.SetStreamAuth(nil)
		}, "STREAM_AUTH"},
		{"per-camera stream auth only", func(b *ConfigBuilder) {
			b.SetStreamAuth(ParseStreamAuth("viewer:pw@front_door"))
		}, "STREAM_AUTH"},
		{"multiple stream auth users", func(b *ConfigBuilder) {
			b.SetStreamAuth(ParseStreamAuth("a:1|b:2"))
		}, "STREAM_AUTH"},
		{"empty stream password", func(b *ConfigBuilder) {
			b.SetStreamAuth([]StreamAuthEntry{{Username: "viewer"}})
		}, "STREAM_AUTH"},
		{"extra yaml reopens api", func(b *ConfigBuilder) {
			b.AppendRawYAML("api:\n  listen: \":1984\"\n")
		}, `"api"`},
		{"extra yaml enables rtmp", func(b *ConfigBuilder) {
			b.AppendRawYAML("# comment\nrtmp:\n  listen: \":1935\"\n")
		}, `"rtmp"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := hardenedBuilder()
			c.setup(b)
			err := b.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want refusal")
			}
			if !strings.Contains(err.Error(), "refusing to start") || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q should mention %q", err, c.want)
			}
		})
	}
}

func TestHardened_ExtraYAMLAllowsStreamsAndIndentedKeys(t *testing.T) {
	b := hardenedBuilder()
	// Nested `api:` under another section is not a top-level override.
	b.AppendRawYAML("ffmpeg:\n  api: something\nlog:\n  level: debug\n")
	if err := b.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}
