package systemd

import (
	"strings"
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
