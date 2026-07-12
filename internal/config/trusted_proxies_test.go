// Tests for ULTRON_TRUSTED_PROXIES parsing.
//
// @aitri-trace BG-020 BL-014
package config

import (
	"net"
	"strings"
	"testing"
)

func TestParseTrustedProxies_BareIP(t *testing.T) {
	got, err := parseTrustedProxies("10.0.0.5")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if !got[0].Contains(net.ParseIP("10.0.0.5")) {
		t.Fatalf("expected 10.0.0.5 in 10.0.0.5/32")
	}
	if got[0].Contains(net.ParseIP("10.0.0.6")) {
		t.Fatalf("10.0.0.5 should not match 10.0.0.6 (must be /32)")
	}
}

func TestParseTrustedProxies_CIDR(t *testing.T) {
	got, err := parseTrustedProxies("192.168.0.0/16, 10.0.0.0/8")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got[0].Contains(net.ParseIP("192.168.42.1")) {
		t.Fatalf("192.168.42.1 should match 192.168.0.0/16")
	}
	if !got[1].Contains(net.ParseIP("10.5.5.5")) {
		t.Fatalf("10.5.5.5 should match 10.0.0.0/8")
	}
}

func TestParseTrustedProxies_IPv6(t *testing.T) {
	got, err := parseTrustedProxies("fd00::/8")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || !got[0].Contains(net.ParseIP("fd12:3456::1")) {
		t.Fatalf("IPv6 CIDR did not match expected address")
	}
}

func TestParseTrustedProxies_RejectsGarbage(t *testing.T) {
	_, err := parseTrustedProxies("not-an-ip")
	if err == nil {
		t.Fatalf("expected error for 'not-an-ip'")
	}
	if !strings.Contains(err.Error(), "neither an IP nor a CIDR") {
		t.Fatalf("error message = %q", err.Error())
	}
}

func TestParseTrustedProxies_EmptyEntriesIgnored(t *testing.T) {
	got, err := parseTrustedProxies(", 10.0.0.1 ,, ")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (empty entries should be skipped)", len(got))
	}
}

// B3 — an all-encompassing CIDR must be rejected: trusting every peer enables
// X-Forwarded-* spoofing for anyone.
func TestParseTrustedProxies_RejectsCatchAll(t *testing.T) {
	for _, cidr := range []string{"0.0.0.0/0", "::/0"} {
		if _, err := parseTrustedProxies(cidr); err == nil {
			t.Errorf("expected %q to be rejected", cidr)
		}
	}
}
