package api

import (
	"errors"
	"net/url"
	"strings"
)

// PublicIPEchoAllowList is the closed set of HTTPS hosts permitted as a
// `public_ip_echo_url` value in net_settings. Defending against SSRF via
// settings: arbitrary URLs are rejected.
//
// @aitri-trace FR-ID: FR-026
var PublicIPEchoAllowList = []string{
	"ifconfig.co",
	"icanhazip.com",
	"ipify.org",
}

// ErrPublicIPEchoURLNotAllowed signals that a settings save attempted to set
// `public_ip_echo_url` to a host outside the closed allow-list.
var ErrPublicIPEchoURLNotAllowed = errors.New("public_ip_echo_url: host not in allow-list")

// ErrPublicIPEchoURLNotHTTPS signals that the URL scheme is not HTTPS.
var ErrPublicIPEchoURLNotHTTPS = errors.New("public_ip_echo_url: scheme must be https")

// ValidatePublicIPEchoURL returns nil when the URL is HTTPS and its host
// (case-insensitive, without port) is in PublicIPEchoAllowList.
//
// @aitri-trace FR-ID: FR-026, TC-ID: TC-NM-011e
func ValidatePublicIPEchoURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return ErrPublicIPEchoURLNotHTTPS
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range PublicIPEchoAllowList {
		if host == allowed {
			return nil
		}
	}
	return ErrPublicIPEchoURLNotAllowed
}
