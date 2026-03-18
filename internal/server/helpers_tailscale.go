package server

import "strings"

func tailscalePeerTotal(data TailscaleData) int {
	if !data.Available || data.Status == nil {
		return 0
	}
	return len(data.Status.Peers)
}

func tailscalePeerOnline(data TailscaleData) int {
	if !data.Available || data.Status == nil {
		return 0
	}
	count := 0
	for _, p := range data.Status.Peers {
		if p.Online {
			count++
		}
	}
	return count
}

func tailscalePeerDeviceName(deviceName, friendlyName, osName string) string {
	device := strings.TrimSpace(deviceName)
	friendly := strings.TrimSpace(friendlyName)
	if device == "device-of-shared-to-user" {
		if osName != "" {
			return strings.ToUpper(strings.TrimSpace(osName))
		}
		return "mobile device"
	}
	if device == "" || device == friendly {
		if osName != "" {
			return strings.ToUpper(strings.TrimSpace(osName))
		}
		return "unknown device"
	}
	return device
}

func tailscalePeerDeviceChip(deviceName, osName string) string {
	device := strings.ToLower(strings.TrimSpace(deviceName))
	osv := strings.ToLower(strings.TrimSpace(osName))
	if device == "device-of-shared-to-user" && osv == "" {
		return "PH"
	}

	combined := device + " " + osv
	switch {
	case strings.Contains(combined, "tv"), strings.Contains(combined, "webos"), strings.Contains(combined, "tizen"), strings.Contains(combined, "android tv"):
		return "TV"
	case strings.Contains(combined, "ios"), strings.Contains(combined, "android"), strings.Contains(combined, "iphone"), strings.Contains(combined, "ipad"), strings.Contains(combined, "phone"):
		return "PH"
	default:
		return "PC"
	}
}
