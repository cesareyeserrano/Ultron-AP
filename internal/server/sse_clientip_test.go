// Tests for the per-IP rate-limit input — guards FR-007 (SSE per-IP cap)
// against X-Forwarded-For spoofing when the binary is reached directly.
//
// @aitri-trace BG-020 BL-014
// TC-BG-020-001 .. 005
package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
)

// withCfg builds a minimal *Server stamped with the given Config so the
// method receiver clientIPFromRequest can resolve trusted proxies.
func withCfg(cfg *config.Config) *Server {
	return &Server{cfg: cfg}
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return n
}

// TC-BG-020-001 — default config (no trusted proxies) ignores XFF entirely.
//
// @aitri-tc TC-BG-020-001
func TestClientIPFromRequest_DefaultIgnoresXFF(t *testing.T) {
	s := withCfg(&config.Config{}) // empty TrustedProxies
	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	req.Header.Set("X-Forwarded-For", "10.20.30.40")

	got := s.clientIPFromRequest(req)
	if got != "203.0.113.7" {
		t.Fatalf("default: got %q, want 203.0.113.7 (XFF must be ignored)", got)
	}
}

// TC-BG-020-002 — XFF is honoured only when the TCP peer is in the
// trusted-proxy allowlist.
//
// @aitri-tc TC-BG-020-002
func TestClientIPFromRequest_TrustedProxyHonoursXFF(t *testing.T) {
	s := withCfg(&config.Config{
		TrustedProxies: []*net.IPNet{mustCIDR(t, "10.0.0.0/8")},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	req.RemoteAddr = "10.5.5.5:33333"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	got := s.clientIPFromRequest(req)
	if got != "203.0.113.50" {
		t.Fatalf("trusted proxy: got %q, want 203.0.113.50 (XFF must be honoured)", got)
	}
}

// TC-BG-020-003 — untrusted TCP peer ignores XFF even when set.
//
// @aitri-tc TC-BG-020-003
func TestClientIPFromRequest_UntrustedPeerIgnoresXFF(t *testing.T) {
	s := withCfg(&config.Config{
		TrustedProxies: []*net.IPNet{mustCIDR(t, "10.0.0.0/8")},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	req.RemoteAddr = "203.0.113.7:54321" // public IP, not in 10/8
	req.Header.Set("X-Forwarded-For", "10.20.30.40")

	got := s.clientIPFromRequest(req)
	if got != "203.0.113.7" {
		t.Fatalf("untrusted peer: got %q, want 203.0.113.7 (XFF must be dropped)", got)
	}
}

// TC-BG-020-004 — XFF chain walked right-to-left, skipping trusted hops, so
// the first untrusted address (the original client) is the rate-limit key.
//
// @aitri-tc TC-BG-020-004
func TestClientIPFromRequest_ChainSkipsTrustedHops(t *testing.T) {
	s := withCfg(&config.Config{
		TrustedProxies: []*net.IPNet{
			mustCIDR(t, "10.0.0.0/8"),
			mustCIDR(t, "192.168.0.0/16"),
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	req.RemoteAddr = "10.0.0.1:443" // last-hop reverse proxy
	// Chain: real-client → another-trusted-proxy → last-hop.
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 192.168.1.5")

	got := s.clientIPFromRequest(req)
	if got != "198.51.100.7" {
		t.Fatalf("chain walk: got %q, want 198.51.100.7", got)
	}
}

// TC-BG-020-005 — XFF entry that is empty or unparseable falls back cleanly
// to the next entry; if every entry is trusted, peer wins.
//
// @aitri-tc TC-BG-020-005
func TestClientIPFromRequest_AllTrustedFallsBackToPeer(t *testing.T) {
	s := withCfg(&config.Config{
		TrustedProxies: []*net.IPNet{mustCIDR(t, "10.0.0.0/8")},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Forwarded-For", "10.0.0.2, 10.0.0.3")

	got := s.clientIPFromRequest(req)
	if got != "10.0.0.1" {
		t.Fatalf("all-trusted: got %q, want peer 10.0.0.1", got)
	}
}

// Sanity: malformed RemoteAddr (no port) doesn't panic and falls back.
func TestClientIPFromRequest_MalformedRemoteAddr(t *testing.T) {
	s := withCfg(&config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	req.RemoteAddr = "garbage-no-port"

	got := s.clientIPFromRequest(req)
	if got != "garbage-no-port" {
		t.Fatalf("malformed peer: got %q, want raw RemoteAddr passthrough", got)
	}
}
