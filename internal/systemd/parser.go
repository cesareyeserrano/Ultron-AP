package systemd

import (
	"errors"
	"strings"
	"time"
)

// parseListUnits parses the output of `systemctl list-units --type=service --all --no-pager --plain`.
// Each line has the format: UNIT LOAD ACTIVE SUB DESCRIPTION...
// Lines starting with empty or whitespace, or containing summary text, are skipped.
func parseListUnits(output string) []ServiceInfo {
	lines := strings.Split(output, "\n")
	var services []ServiceInfo

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip summary footer lines like "LOAD   = ...", "123 loaded units listed."
		if strings.HasPrefix(line, "LOAD") || strings.HasPrefix(line, "To show") || strings.Contains(line, " units listed.") || strings.Contains(line, " unit files listed.") {
			continue
		}

		// Split into at least 5 parts: UNIT LOAD ACTIVE SUB DESCRIPTION...
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		unit := fields[0]
		// Only process .service units
		if !strings.HasSuffix(unit, ".service") {
			continue
		}

		// Trim .service suffix for display name
		name := strings.TrimSuffix(unit, ".service")

		loadState := fields[1]
		activeState := fields[2]
		subState := fields[3]

		description := ""
		if len(fields) > 4 {
			description = strings.Join(fields[4:], " ")
		}

		services = append(services, ServiceInfo{
			Name:        name,
			LoadState:   loadState,
			ActiveState: activeState,
			SubState:    subState,
			Description: description,
			Health:      MapServiceHealth(activeState),
		})
	}

	return services
}

// parseShowTimestamps parses `systemctl show -p Id -p ActiveEnterTimestamp`
// output for one or more units. Blocks are separated by a blank line:
//
//	Id=nginx.service
//	ActiveEnterTimestamp=Mon 2026-07-13 10:04:11 UTC
//
// A unit that has never activated reports an empty timestamp; those are
// skipped so the caller leaves ServiceInfo.Since as the zero value.
func parseShowTimestamps(output string) map[string]time.Time {
	result := make(map[string]time.Time)

	var id string
	var stamp time.Time
	flush := func() {
		if id != "" && !stamp.IsZero() {
			result[strings.TrimSuffix(id, ".service")] = stamp
		}
		id, stamp = "", time.Time{}
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Id":
			// A new Id starts a new block even without a blank separator.
			if id != "" {
				flush()
			}
			id = value
		case "ActiveEnterTimestamp":
			if t, err := parseSystemdTimestamp(value); err == nil {
				stamp = t
			}
		}
	}
	flush()

	return result
}

// systemdTimestampLayouts covers the formats systemd emits depending on the
// host locale/timezone setting (e.g. "Mon 2026-07-13 10:04:11 UTC").
var systemdTimestampLayouts = []string{
	"Mon 2006-01-02 15:04:05 MST",
	"Mon 2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05 MST",
}

func parseSystemdTimestamp(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "n/a" {
		return time.Time{}, errNoTimestamp
	}
	var lastErr error
	for _, layout := range systemdTimestampLayouts {
		t, err := time.Parse(layout, v)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

var errNoTimestamp = errors.New("systemd: no activation timestamp")
