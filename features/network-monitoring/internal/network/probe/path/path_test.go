package path

import (
	"errors"
	"reflect"
	"testing"
)

// TestTC_NM_014e_TracerouteArgsBounded asserts the closed argv shape for a
// well-formed IPv4 target — no shell metacharacters, no extra flags, exact
// match against the helper allow-list grammar.
//
// @aitri-tc TC-NM-014e
func TestTC_NM_014e_TracerouteArgsBounded(t *testing.T) {
	t.Parallel()
	got, err := BuildTracerouteArgs("1.1.1.1")
	if err != nil {
		t.Fatalf("BuildTracerouteArgs(1.1.1.1) returned %v, want nil", err)
	}
	want := []string{"-n", "-m", "30", "1.1.1.1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

// TestTC_NM_014e_TracerouteRejectsUnsafeTargets asserts that any non-IP
// target — hostnames, shell-metacharacter-laden strings, or flag-shaped
// inputs — is refused before reaching the helper socket.
//
// @aitri-tc TC-NM-014e
func TestTC_NM_014e_TracerouteRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()
	cases := []string{
		"1.1.1.1; rm -rf /",
		"$(curl evil)",
		"google.com",       // hostname, not an IP literal
		"--malicious-flag", // would be parsed as a flag
		"",
	}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			if _, err := BuildTracerouteArgs(c); !errors.Is(err, ErrInvalidTracerouteTarget) {
				t.Errorf("BuildTracerouteArgs(%q) = %v, want %v", c, err, ErrInvalidTracerouteTarget)
			}
		})
	}
}
