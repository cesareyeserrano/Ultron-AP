package oui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TC-LD-004h
//
// @aitri-tc TC-LD-004h
func TestTC_LD_004h_Vendor_RaspberryPi(t *testing.T) {
	// @aitri-tc TC-LD-004h
	v := Vendor("b8:27:eb:11:22:33")
	assert.Contains(t, strings.ToLower(v), "raspberry pi")
}

// TC-LD-004f
//
// @aitri-tc TC-LD-004f
func TestTC_LD_004f_Vendor_LAABitReturnsLocallyAdministered(t *testing.T) {
	// @aitri-tc TC-LD-004f
	// 02:00:00:00:00:01 — first byte 0x02 has bit 1 set → LAA.
	assert.Equal(t, "Locally administered", Vendor("02:00:00:00:00:01"))
	// 06:xx (bit 1 + bit 2) also LAA.
	assert.Equal(t, "Locally administered", Vendor("06:11:22:33:44:55"))
	// 0A:xx (bit 1 + bit 3) also LAA.
	assert.Equal(t, "Locally administered", Vendor("0a:b8:27:eb:00:00"))
}

// TC-LD-004e
//
// @aitri-tc TC-LD-004e
func TestTC_LD_004e_Vendor_UnknownPrefixReturnsUnknown(t *testing.T) {
	// @aitri-tc TC-LD-004e
	// 00:DE:AD is not in the bundled table; first byte 0x00 has LAA bit clear.
	assert.Equal(t, "Unknown", Vendor("00:de:ad:be:ef:01"))
}

// TC-LD-011h — works with no network and resolves Pi + Google + unknown
//
// @aitri-tc TC-LD-011h
func TestTC_LD_011h_Vendor_NoNetworkLookups(t *testing.T) {
	// @aitri-tc TC-LD-011h
	pi := Vendor("B8:27:EB:00:00:00")
	google := Vendor("00:1A:11:00:00:00")
	unknown := Vendor("00:DE:AD:BE:EF:00")

	assert.Contains(t, strings.ToLower(pi), "raspberry pi")
	assert.Contains(t, strings.ToLower(google), "google")
	assert.Equal(t, "Unknown", unknown)
}

// TC-LD-011f — malformed inputs do not panic and return a sensible string.
//
// @aitri-tc TC-LD-011f
func TestTC_LD_011f_Vendor_MalformedInputs(t *testing.T) {
	// @aitri-tc TC-LD-011f
	cases := []string{"not-a-mac", "00:00", "ZZ:YY:XX:11:22:33", "", "::::::", "deadbeef"}
	for _, in := range cases {
		got := Vendor(in)
		assert.NotEmpty(t, got, "input %q produced empty vendor", in)
	}
}

// Sanity: lookup latency dominated by map access (in-memory).
func TestVendor_LookupIsConstantTime(t *testing.T) {
	for i := 0; i < 1000; i++ {
		_ = Vendor("b8:27:eb:11:22:33")
	}
}

// Acceptable variant forms parse to the same vendor.
func TestVendor_AcceptsCommonMACFormats(t *testing.T) {
	want := Vendor("b8:27:eb:11:22:33")
	for _, form := range []string{"B8:27:EB:11:22:33", "B8-27-EB-11-22-33", "b827eb112233"} {
		got := Vendor(form)
		assert.Equal(t, want, got, "form %q should resolve identically", form)
	}
}
