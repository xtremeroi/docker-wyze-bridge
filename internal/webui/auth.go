package webui

import (
	"crypto/subtle"
	"net"
	"net/http"
)

// haSupervisorIngressIP is the fixed address of the Home Assistant
// Supervisor on the hassio Docker network. HA's add-on docs say ingress
// traffic always comes from this IP and add-ons should only trust it.
const haSupervisorIngressIP = "172.30.32.2"

// AuthMiddleware provides Basic Auth and Bearer token authentication.
type AuthMiddleware struct {
	enabled  bool
	username string
	password string
	apiKey   string // for Bearer token access

	// HARDENED: trustIngress lets Home Assistant ingress requests (which
	// HA has already authenticated) through without a Basic prompt.
	trustIngress bool
}

// SetTrustHAIngress enables the HARDENED Home Assistant ingress bypass.
func (a *AuthMiddleware) SetTrustHAIngress(v bool) {
	a.trustIngress = v
}

// isHAIngress reports whether r was proxied by the HA Supervisor's
// ingress: TCP peer is 172.30.32.2 and the X-Ingress-Path header the
// Supervisor always sets is present. The peer address comes from the
// TCP connection (not a header), so a LAN client cannot spoof it.
func isHAIngress(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	return host == haSupervisorIngressIP && r.Header.Get("X-Ingress-Path") != ""
}

// NewAuthMiddleware creates a new auth middleware.
func NewAuthMiddleware(enabled bool, username, password, apiKey string) *AuthMiddleware {
	return &AuthMiddleware{
		enabled:  enabled,
		username: username,
		password: password,
		apiKey:   apiKey,
	}
}

// Wrap wraps an HTTP handler with authentication.
func (a *AuthMiddleware) Wrap(handler http.HandlerFunc) http.HandlerFunc {
	if !a.enabled {
		return handler
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// HARDENED: HA ingress (already authenticated by Home Assistant).
		if a.trustIngress && isHAIngress(r) {
			handler(w, r)
			return
		}

		// Check Bearer token first
		if a.apiKey != "" {
			bearer := r.Header.Get("Authorization")
			if len(bearer) > 7 && bearer[:7] == "Bearer " {
				token := bearer[7:]
				if subtle.ConstantTimeCompare([]byte(token), []byte(a.apiKey)) == 1 {
					handler(w, r)
					return
				}
			}
		}

		// Fall back to Basic Auth
		// HARDENED: Basic auth is only accepted when a password is
		// configured. Upstream compared against an empty password, so a
		// token-only setup accepted "Authorization: Basic <user>:".
		user, pass, ok := r.BasicAuth()
		if !ok || a.password == "" ||
			subtle.ConstantTimeCompare([]byte(user), []byte(a.username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(a.password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Wyze Bridge"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		handler(w, r)
	}
}
