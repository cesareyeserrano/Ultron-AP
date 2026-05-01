package helper

import "testing"

// TestTC_NM_023h_PingWithAllowedArgsSucceeds asserts that a ping request
// whose flags are all in the allow-list passes Validate (the FR-016 happy
// path for outbound probe commands).
//
// @aitri-tc TC-NM-023h
func TestTC_NM_023h_PingWithAllowedArgsSucceeds(t *testing.T) {
	t.Parallel()
	d := Validate(Request{Bin: "ping", Args: []string{"-c", "3", "-W", "1", "-i", "0.5", "1.1.1.1"}})
	if !d.Allowed {
		t.Fatalf("Validate(ping happy path).Allowed = false (reason=%q), want true", d.Reason)
	}
}

// TestTC_NM_023f_PingWithDisallowedFlagRejected asserts that an unsafe flag
// like -D fails the validator with reason="disallowed_flag:-D".
//
// @aitri-tc TC-NM-023f
func TestTC_NM_023f_PingWithDisallowedFlagRejected(t *testing.T) {
	t.Parallel()
	d := Validate(Request{Bin: "ping", Args: []string{"-D", "-c", "3", "1.1.1.1"}})
	if d.Allowed {
		t.Fatal("Validate(ping with -D).Allowed = true, want false")
	}
	if got, want := d.Reason, "disallowed_flag:-D"; got != want {
		t.Errorf("reason = %q, want %q", got, want)
	}
}

// TestTC_NM_023e_UnknownBinaryRejected asserts that any binary outside the
// allow-list (e.g. nmap) is refused with binary_not_allowlisted.
//
// @aitri-tc TC-NM-023e
func TestTC_NM_023e_UnknownBinaryRejected(t *testing.T) {
	t.Parallel()
	d := Validate(Request{Bin: "nmap", Args: []string{"-sS", "1.1.1.1"}})
	if d.Allowed {
		t.Fatal("Validate(nmap).Allowed = true, want false")
	}
	if d.Reason != "binary_not_allowlisted" {
		t.Errorf("reason = %q, want binary_not_allowlisted", d.Reason)
	}
}
