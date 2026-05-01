package wifi

import (
	"errors"
	"testing"
)

const procWirelessFixture = `Inter-| sta-|   Quality        |   Discarded packets               |Missed | WE
 face | tus | link level noise |  nwid  crypt   frag  retry   misc | beacon | 22
wlan0: 0000   70.  -58.    0      0      0      0     12      0        0`

// TestTC_NM_013e_ParseProcWirelessNoHelperCall asserts the WiFi sampler
// produces a usable Sample from /proc/net/wireless contents alone — no
// privileged helper invocation, no root. This is the FR-028 no-root
// invariant.
//
// @aitri-tc TC-NM-013e
func TestTC_NM_013e_ParseProcWirelessNoHelperCall(t *testing.T) {
	t.Parallel()
	got, err := ParseProcWireless(procWirelessFixture)
	if err != nil {
		t.Fatalf("ParseProcWireless(fixture) returned %v, want nil", err)
	}
	if !got.Applicable {
		t.Errorf("Applicable = false, want true")
	}
	if got.RSSIDBm != -58 {
		t.Errorf("RSSIDBm = %d, want -58", got.RSSIDBm)
	}
	if got.LinkQuality != 70 {
		t.Errorf("LinkQuality = %d, want 70", got.LinkQuality)
	}
	if got.Retries != 12 {
		t.Errorf("Retries = %d, want 12", got.Retries)
	}
}

// TestTC_NM_013e_ParseProcWirelessEthernetOnly returns
// ErrProcWirelessNotApplicable when the kernel listing has no wlan*
// interface — the Ethernet-only Pi case.
//
// @aitri-tc TC-NM-013e
func TestTC_NM_013e_ParseProcWirelessEthernetOnly(t *testing.T) {
	t.Parallel()
	headerOnly := "Inter-| sta-|   Quality        |   Discarded packets\n face | tus | link level noise |  nwid  crypt"
	if _, err := ParseProcWireless(headerOnly); !errors.Is(err, ErrProcWirelessNotApplicable) {
		t.Errorf("ParseProcWireless(header-only) = %v, want %v", err, ErrProcWirelessNotApplicable)
	}
}
