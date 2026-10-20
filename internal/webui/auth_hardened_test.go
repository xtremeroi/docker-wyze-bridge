package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// HARDENED: Home Assistant ingress bypass and empty-password Basic.

func okHandler(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func TestAuthMiddleware_HAIngressTrusted(t *testing.T) {
	auth := NewAuthMiddleware(true, "admin", "secret", "")
	auth.SetTrustHAIngress(true)
	h := auth.Wrap(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.30.32.2:43210"
	req.Header.Set("X-Ingress-Path", "/api/hassio_ingress/abc123")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("ingress request should pass, got %d", w.Code)
	}
}

func TestAuthMiddleware_HAIngressRejectedCases(t *testing.T) {
	cases := []struct {
		name, remote, ingressHdr string
		trust                    bool
	}{
		{"trust disabled", "172.30.32.2:1", "/api/hassio_ingress/x", false},
		{"wrong source ip", "192.168.68.20:1", "/api/hassio_ingress/x", true},
		{"other hassio container", "172.30.33.5:1", "/api/hassio_ingress/x", true},
		{"missing header", "172.30.32.2:1", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			auth := NewAuthMiddleware(true, "admin", "secret", "")
			auth.SetTrustHAIngress(c.trust)
			h := auth.Wrap(okHandler)
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = c.remote
			if c.ingressHdr != "" {
				req.Header.Set("X-Ingress-Path", c.ingressHdr)
			}
			w := httptest.NewRecorder()
			h(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("want 401, got %d", w.Code)
			}
		})
	}
}

func TestAuthMiddleware_TokenOnlyRejectsEmptyBasic(t *testing.T) {
	// Token-only setup: no password configured. Upstream accepted
	// "Basic <user>:" because it compared against the empty password.
	auth := NewAuthMiddleware(true, "wyze", "", "tok")
	h := auth.Wrap(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("wyze", "")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("empty-password Basic should be 401, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w = httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("bearer token should pass, got %d", w.Code)
	}
}
