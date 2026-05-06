// @aitri-tc TC-SR-060h, TC-SR-060e (partially), TC-SR-060f
package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-SR-060h — Validation error message for an out-of-range value contains
// the exact range string from the label. The en-dash (U+2013) MUST be
// preserved, not normalised to ASCII.
func TestRange_ValidationError_ContainsExactHintStringWithEnDash(t *testing.T) {
	r := RangeFor("sse_interval_sec")
	require.Equal(t, "Dashboard refresh", r.Label)
	require.Equal(t, 2, r.Min)
	require.Equal(t, 60, r.Max)

	err := r.ValidationError(999)
	require.Error(t, err)

	got := err.Error()
	want := "value 999 out of range — Dashboard refresh (2–60 sec)"
	assert.Equal(t, want, got, "validation error must match exact string with en-dash")

	// Defensive: the en-dash must be present, NOT replaced with ASCII '-'.
	assert.True(t, strings.Contains(got, "2–60"), "en-dash 2–60 expected, got: %q", got)
	assert.False(t, strings.Contains(got, "2-60 sec"), "ASCII hyphen variant must NOT appear; got: %q", got)
}

// Hint() returns the parenthesised range string used inside the label.
func TestRange_Hint_FormatPerUnit(t *testing.T) {
	cases := []struct {
		field string
		want  string
	}{
		{"sse_interval_sec", "2–60 sec"},
		{"disk_interval_min", "1–1440 min"},
		{"docker_interval_sec", "5–300 sec"},
		{"systemd_interval_sec", "5–300 sec"},
		{"interval_hours", "1–720 hr"},
		{"retention_count", "1–200 files"},
		{"upload_timeout_sec", "5–300 sec"},
		{"max_upload_size_mb", "1–1024 MB"},
		{"schedule_hour", "0–23"},
		{"schedule_minute", "0–59"},
		{"cooldown", "0–1440 min"},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			assert.Equal(t, c.want, RangeFor(c.field).Hint())
		})
	}
}

// LabelWithHint() is the visible label string — same shape used in template
// and validation error.
func TestRange_LabelWithHint(t *testing.T) {
	r := RangeFor("disk_interval_min")
	assert.Equal(t, "Disk check (1–1440 min)", r.LabelWithHint())
}

// ParseAndValidate accepts in-range values, rejects out-of-range with the
// same hint substring. Boundary values (min, max) are accepted.
func TestRange_ParseAndValidate_BoundariesAndBeyond(t *testing.T) {
	r := RangeFor("sse_interval_sec") // 2..60

	// Min boundary OK
	v, err := r.ParseAndValidate("2")
	require.NoError(t, err)
	assert.Equal(t, 2, v)

	// Max boundary OK
	v, err = r.ParseAndValidate("60")
	require.NoError(t, err)
	assert.Equal(t, 60, v)

	// Just below min
	_, err = r.ParseAndValidate("1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2–60 sec")

	// Just above max
	_, err = r.ParseAndValidate("61")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2–60 sec")

	// Non-integer
	_, err = r.ParseAndValidate("banana")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not an integer")
	assert.Contains(t, err.Error(), "2–60 sec") // hint still surfaces
}

// RangeFor on an unknown field panics — drift detection: the registry is the
// only place these are declared, so a typo at the call site fails loudly.
func TestRangeFor_UnknownFieldPanics(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic on unknown field")
		msg, _ := r.(string)
		assert.Contains(t, msg, "unknown field")
	}()
	_ = RangeFor("not_a_real_field")
}

// TC-SR-060f — drift detection: any future contributor inlining a min/max
// literal in the settings template (outside the rangeHint helper) would be
// caught by an HTML-scan test. We assert the registry covers all fields the
// template currently references; the live template scan happens in
// templates_test.go (drift test on rendered HTML).
func TestRegisteredRangeFields_CoversAllExpectedFields(t *testing.T) {
	want := map[string]bool{
		"sse_interval_sec":     true,
		"disk_interval_min":    true,
		"docker_interval_sec":  true,
		"systemd_interval_sec": true,
		"interval_hours":       true,
		"retention_count":      true,
		"upload_timeout_sec":   true,
		"max_upload_size_mb":   true,
		"schedule_hour":        true,
		"schedule_minute":      true,
		"cooldown":             true,
	}
	for _, f := range RegisteredRangeFields() {
		delete(want, f)
	}
	assert.Empty(t, want, "registry is missing expected fields: %v", want)
}
