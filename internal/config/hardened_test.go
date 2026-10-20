package config

import (
	"strings"
	"testing"
)

// HARDENED: refuse-to-start checks.

func hardenedEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BRIDGE_AUTH", "true")
	t.Setenv("BRIDGE_USERNAME", "admin")
	t.Setenv("BRIDGE_PASSWORD", "webui-secret")
	t.Setenv("BRIDGE_API_TOKEN", "")
	t.Setenv("GO2RTC_API_USERNAME", "g2r")
	t.Setenv("GO2RTC_API_PASSWORD", "api-secret")
	t.Setenv("STREAM_AUTH", "viewer:rtsp-secret")
}

func TestValidateHardened_OK(t *testing.T) {
	hardenedEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.ValidateHardened(); err != nil {
		t.Errorf("ValidateHardened() = %v, want nil", err)
	}
}

func TestValidateHardened_TokenOnlyOK(t *testing.T) {
	hardenedEnv(t)
	t.Setenv("BRIDGE_PASSWORD", "")
	t.Setenv("BRIDGE_API_TOKEN", "tok")
	cfg, _ := Load()
	if err := cfg.ValidateHardened(); err != nil {
		t.Errorf("ValidateHardened() = %v, want nil", err)
	}
}

func TestValidateHardened_Refuses(t *testing.T) {
	cases := []struct {
		name, key, val, want string
	}{
		{"auth off", "BRIDGE_AUTH", "false", "BRIDGE_AUTH=true"},
		{"no webui password or token", "BRIDGE_PASSWORD", "", "BRIDGE_PASSWORD"},
		{"no go2rtc api user", "GO2RTC_API_USERNAME", "", "GO2RTC_API_USERNAME"},
		{"no go2rtc api password", "GO2RTC_API_PASSWORD", "", "GO2RTC_API_PASSWORD"},
		{"no stream auth", "STREAM_AUTH", "", "STREAM_AUTH"},
		{"per-camera stream auth", "STREAM_AUTH", "viewer:pw@front_door", "STREAM_AUTH"},
		{"multi-user stream auth", "STREAM_AUTH", "a:1|b:2", "STREAM_AUTH"},
		{"stream auth no password", "STREAM_AUTH", "viewer:", "STREAM_AUTH"},
		{"stream auth no colon", "STREAM_AUTH", "viewer", "STREAM_AUTH"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hardenedEnv(t)
			t.Setenv(c.key, c.val)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			err = cfg.ValidateHardened()
			if err == nil {
				t.Fatal("ValidateHardened() = nil, want refusal")
			}
			if !strings.Contains(err.Error(), "refusing to start") || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q should mention %q", err, c.want)
			}
		})
	}
}

func TestValidateHardened_EmptyEnvListsEverything(t *testing.T) {
	t.Setenv("BRIDGE_AUTH", "")
	t.Setenv("BRIDGE_PASSWORD", "")
	t.Setenv("BRIDGE_API_TOKEN", "")
	t.Setenv("GO2RTC_API_USERNAME", "")
	t.Setenv("GO2RTC_API_PASSWORD", "")
	t.Setenv("STREAM_AUTH", "")
	cfg, _ := Load()
	err := cfg.ValidateHardened()
	if err == nil {
		t.Fatal("want refusal with no credentials at all")
	}
	for _, want := range []string{"BRIDGE_AUTH", "BRIDGE_PASSWORD", "GO2RTC_API_USERNAME", "STREAM_AUTH"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s: %v", want, err)
		}
	}
}
