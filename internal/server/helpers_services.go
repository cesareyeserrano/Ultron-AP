package server

import (
	"sort"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

type serviceGroupData struct {
	Key      string
	Label    string
	Services []systemd.ServiceInfo
}

var serviceGroupOrder = []string{
	"core",
	"container",
	"network",
	"hardware",
	"observability",
	"system",
	"other",
}

func serviceGroup(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "ultron"):
		return "core"
	case strings.Contains(n, "docker") || strings.Contains(n, "containerd"):
		return "container"
	case strings.Contains(n, "tailscale") || strings.Contains(n, "network") || strings.Contains(n, "wpa") || strings.Contains(n, "dhcp"):
		return "network"
	case strings.Contains(n, "pironman") || strings.Contains(n, "fan") || strings.Contains(n, "gpio") || strings.Contains(n, "i2c"):
		return "hardware"
	case strings.Contains(n, "influx") || strings.Contains(n, "prometheus") || strings.Contains(n, "grafana"):
		return "observability"
	case strings.Contains(n, "systemd") || strings.Contains(n, "dbus") || strings.Contains(n, "ssh") || strings.Contains(n, "udev"):
		return "system"
	default:
		return "other"
	}
}

func serviceGroupLabel(key string) string {
	switch key {
	case "core":
		return "Ultron Core"
	case "container":
		return "Containers"
	case "network":
		return "Network"
	case "hardware":
		return "Hardware"
	case "observability":
		return "Observability"
	case "system":
		return "System"
	default:
		return "Other"
	}
}

func serviceGroupPillClass(key string) string {
	_ = key
	return "bg-accent/20 text-accent border-accent/20"
}

func groupServices(services []systemd.ServiceInfo) []serviceGroupData {
	if len(services) == 0 {
		return nil
	}
	buckets := map[string][]systemd.ServiceInfo{}
	for _, svc := range services {
		k := serviceGroup(svc.Name)
		buckets[k] = append(buckets[k], svc)
	}
	result := make([]serviceGroupData, 0, len(buckets))
	for _, key := range serviceGroupOrder {
		group, ok := buckets[key]
		if !ok {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].Name < group[j].Name
		})
		result = append(result, serviceGroupData{
			Key:      key,
			Label:    serviceGroupLabel(key),
			Services: group,
		})
	}
	return result
}

func serviceInfo(name, description string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSuffix(n, ".service")
	if strings.HasPrefix(n, "docker") {
		n = "docker"
	}
	if strings.Contains(n, "pironman5") {
		n = "pironman5"
	}
	if v, ok := map[string]string{
		"ultron-ap":         "Main Ultron web admin process.",
		"ultron-helper":     "Privileged helper boundary for controlled host actions.",
		"pironman5":         "Pironman hardware control daemon (fan/RGB/OLED).",
		"pironman5-service": "Pironman hardware control daemon (fan/RGB/OLED).",
		"influxdb":          "Time-series database used by monitoring stack.",
		"influxd":           "InfluxDB time-series engine process.",
		"docker":            "Docker daemon for containers runtime.",
		"containerd":        "Container runtime used by Docker.",
		"home-assistant":    "Home Assistant core automation platform.",
		"homeassistant":     "Home Assistant core automation platform.",
		"mosquitto":         "MQTT broker for IoT messaging.",
		"redis":             "In-memory cache and queue backend.",
		"postgresql":        "PostgreSQL relational database engine.",
		"mysql":             "MySQL relational database engine.",
		"node-exporter":     "Prometheus node metrics exporter.",
		"cadvisor":          "Container resource metrics collector.",
		"prometheus":        "Metrics scraper and alerting engine.",
		"promtail":          "Log shipping agent.",
		"grafana":           "Monitoring and observability dashboard UI.",
		"nginx":             "Web reverse proxy and static server.",
		"caddy":             "Web server and reverse proxy.",
		"tailscaled":        "Tailscale secure mesh VPN service.",
		"networkmanager":    "Network interface manager.",
		"rpi-connectd":      "Raspberry Pi remote connectivity agent.",
		"sshd":              "Secure shell access daemon.",
		"ssh":               "Secure shell access daemon.",
		"systemd-logind":    "Session and user login manager.",
		"systemd-journald":  "System logs collector.",
	}[n]; ok {
		return v
	}
	if strings.TrimSpace(description) != "" {
		return description
	}
	return "System service."
}

func serviceProcessKey(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSuffix(n, ".service")
	switch n {
	case "influxdb":
		return "influxdb"
	case "docker":
		return "docker"
	case "pironman5":
		return "pironman5"
	case "dockerd":
		return "docker"
	default:
		if strings.HasPrefix(n, "docker-") || strings.HasPrefix(n, "docker.") {
			return "docker"
		}
		if strings.Contains(n, "pironman5") {
			return "pironman5"
		}
		return n
	}
}

func serviceHasRuntime(name string, stats map[string]ProcessConsumer) bool {
	if stats == nil {
		return false
	}
	_, ok := stats[serviceProcessKey(name)]
	return ok
}

func serviceCPU(name string, stats map[string]ProcessConsumer) float64 {
	if !serviceHasRuntime(name, stats) {
		return -1
	}
	return stats[serviceProcessKey(name)].CPUPercent
}

func serviceRSS(name string, stats map[string]ProcessConsumer) uint64 {
	if !serviceHasRuntime(name, stats) {
		return 0
	}
	return stats[serviceProcessKey(name)].RSSBytes
}

func countServicesState(services []systemd.ServiceInfo, state string) int {
	if len(services) == 0 {
		return 0
	}
	want := strings.ToLower(strings.TrimSpace(state))
	count := 0
	for _, svc := range services {
		if strings.ToLower(svc.ActiveState) == want {
			count++
		}
	}
	return count
}

func countContainersState(containers []docker.ContainerInfo, state string) int {
	if len(containers) == 0 {
		return 0
	}
	want := strings.ToLower(strings.TrimSpace(state))
	count := 0
	for _, c := range containers {
		if strings.ToLower(c.State) == want {
			count++
		}
	}
	return count
}
