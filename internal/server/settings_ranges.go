// @aitri-trace FR-057, FR-060, FR-064 — settings-revamp single source of truth
// for numeric field ranges. Used by templates (label hint), handlers (validation),
// and tests (drift assertion).
package server

import (
	"fmt"
	"strconv"
)

// Range describes the allowed values for a numeric settings field.
type Range struct {
	Field string // form-field name, e.g. "sse_interval_sec"
	Label string // human-readable label, e.g. "Dashboard refresh"
	Min   int
	Max   int
	Unit  string // "sec" / "min" / "hr" / "" (dimensionless), or "%" / "MB"
}

// Hint returns the parenthesised range string used in the visible label and
// in the validation error. Single source of truth — do not duplicate this
// shape in templates or handlers.
//
// Uses an en-dash (U+2013) on purpose, not an ASCII hyphen — assertions
// in tests verify the en-dash is preserved.
func (r Range) Hint() string {
	if r.Unit == "" {
		return fmt.Sprintf("%d–%d", r.Min, r.Max)
	}
	return fmt.Sprintf("%d–%d %s", r.Min, r.Max, r.Unit)
}

// LabelWithHint returns "<Label> (<min>–<max> <unit>)" for the rendered field.
func (r Range) LabelWithHint() string {
	return fmt.Sprintf("%s (%s)", r.Label, r.Hint())
}

// ValidationError returns the error string the server emits when a value is
// out of range. Contains the same hint substring as the visible label, by
// construction.
func (r Range) ValidationError(value int) error {
	return fmt.Errorf("value %d out of range — %s", value, r.LabelWithHint())
}

// ParseAndValidate parses a form value as int and checks bounds.
// Returns (cleaned, true) on success; (0, false, error) on parse / bounds error.
func (r Range) ParseAndValidate(raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("value %q is not an integer — %s", raw, r.LabelWithHint())
	}
	if n < r.Min || n > r.Max {
		return 0, r.ValidationError(n)
	}
	return n, nil
}

// settingsRanges is the registry. Adding a new numeric settings field
// requires editing exactly this map.
var settingsRanges = map[string]Range{
	"sse_interval_sec":     {Field: "sse_interval_sec", Label: "Dashboard refresh", Min: 2, Max: 60, Unit: "sec"},
	"disk_interval_min":    {Field: "disk_interval_min", Label: "Disk check", Min: 1, Max: 1440, Unit: "min"},
	"docker_interval_sec":  {Field: "docker_interval_sec", Label: "Docker refresh", Min: 5, Max: 300, Unit: "sec"},
	"systemd_interval_sec": {Field: "systemd_interval_sec", Label: "Services refresh", Min: 5, Max: 300, Unit: "sec"},

	"interval_hours":     {Field: "interval_hours", Label: "Backup interval", Min: 1, Max: 720, Unit: "hr"},
	"retention_count":    {Field: "retention_count", Label: "Retention", Min: 1, Max: 200, Unit: "files"},
	"upload_timeout_sec": {Field: "upload_timeout_sec", Label: "Upload timeout", Min: 5, Max: 300, Unit: "sec"},
	"max_upload_size_mb": {Field: "max_upload_size_mb", Label: "Max size", Min: 1, Max: 1024, Unit: "MB"},
	"schedule_hour":      {Field: "schedule_hour", Label: "Schedule hour", Min: 0, Max: 23, Unit: ""},
	"schedule_minute":    {Field: "schedule_minute", Label: "Schedule minute", Min: 0, Max: 59, Unit: ""},

	"cooldown": {Field: "cooldown", Label: "Cooldown", Min: 0, Max: 1440, Unit: "min"},
}

// RangeFor returns the Range for a registered field. Panics on unknown field
// — registration is a compile-time concern, drift is a test-time concern.
func RangeFor(field string) Range {
	r, ok := settingsRanges[field]
	if !ok {
		panic("settings_ranges: unknown field " + field)
	}
	return r
}

// RegisteredRangeFields returns the set of registered field names. Used by
// drift tests that verify every numeric field on /settings has its hint
// rendered and uses RangeFor() for validation.
func RegisteredRangeFields() []string {
	out := make([]string, 0, len(settingsRanges))
	for k := range settingsRanges {
		out = append(out, k)
	}
	return out
}
