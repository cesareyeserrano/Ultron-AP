package api

import (
	"errors"
	"testing"
)

// TestTC_NM_011e_PublicIPEchoRejectsNonAllowListed asserts that arbitrary URLs
// are rejected at settings save, defending against SSRF via settings.
//
// @aitri-tc TC-NM-011e
func TestTC_NM_011e_PublicIPEchoRejectsNonAllowListed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{"arbitrary host rejected", "https://evil.example/ip", ErrPublicIPEchoURLNotAllowed},
		{"localhost rejected", "https://127.0.0.1/ip", ErrPublicIPEchoURLNotAllowed},
		{"internal RFC1918 rejected", "https://10.0.0.5/ip", ErrPublicIPEchoURLNotAllowed},
		{"http scheme rejected", "http://ifconfig.co/ip", ErrPublicIPEchoURLNotHTTPS},
		{"file scheme rejected", "file:///etc/passwd", ErrPublicIPEchoURLNotHTTPS},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePublicIPEchoURL(tc.raw)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidatePublicIPEchoURL(%q) = %v, want %v", tc.raw, err, tc.wantErr)
			}
		})
	}
}

// TestTC_NM_011e_PublicIPEchoAcceptsAllowListed asserts that the three
// configured allow-list hosts pass validation and can be saved.
//
// @aitri-tc TC-NM-011e
func TestTC_NM_011e_PublicIPEchoAcceptsAllowListed(t *testing.T) {
	t.Parallel()
	cases := []string{
		"https://ifconfig.co/ip",
		"https://icanhazip.com",
		"https://ipify.org/?format=text",
		"https://IFCONFIG.CO/ip", // case-insensitive host
	}
	for _, raw := range cases {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if err := ValidatePublicIPEchoURL(raw); err != nil {
				t.Errorf("ValidatePublicIPEchoURL(%q) returned %v, want nil", raw, err)
			}
		})
	}
}
