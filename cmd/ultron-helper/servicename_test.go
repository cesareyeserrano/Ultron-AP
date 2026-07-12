// Tests for the root helper's service-name validation. serviceNameRe is the
// last line of defence before a name is handed to systemctl/journalctl running
// as root, so it must reject any value that getopt could parse as an option
// token (argument-injection guard, A1).
package main

import "testing"

func TestServiceNameRe_RejectsOptionLikeNames(t *testing.T) {
	bad := []string{
		"-Mfoo",             // -M<machine>: retarget systemd instance
		"--version",         // long option
		"-Hroot@evil.com",   // -H<host>: remote over ssh (@ is otherwise allowed)
		"-r",                // shutdown-style flag
		"",                  // empty
		".hidden",           // leading dot
		"foo bar",           // space
		"foo;rm",            // shell metachar (belt-and-braces)
	}
	for _, name := range bad {
		if serviceNameRe.MatchString(name) {
			t.Errorf("serviceNameRe accepted option-like/invalid name %q", name)
		}
	}
}

func TestServiceNameRe_AcceptsRealUnitNames(t *testing.T) {
	good := []string{
		"docker",
		"nginx.service",
		"home-assistant@homeassistant",
		"pironman5-service",
		"systemd-networkd.service",
		"user@1000.service",
	}
	for _, name := range good {
		if !serviceNameRe.MatchString(name) {
			t.Errorf("serviceNameRe rejected valid unit name %q", name)
		}
	}
}
