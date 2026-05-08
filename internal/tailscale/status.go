package tailscale

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Peer represents a Tailscale node (self or remote peer).
type Peer struct {
	Hostname     string
	FriendlyName string
	DeviceName   string
	IPs          []string
	OS           string
	Online       bool
	LastSeen     time.Time
}

// Status holds the result of `tailscale status --json`.
type Status struct {
	Self  Peer
	Peers []Peer
}

// rawStatus mirrors the relevant fields from `tailscale status --json`.
type rawStatus struct {
	Self rawPeer            `json:"Self"`
	Peer map[string]rawPeer `json:"Peer"`
	User map[string]rawUser `json:"User"`
}

type rawPeer struct {
	HostName     string    `json:"HostName"`
	UserID       int64     `json:"UserID"`
	OS           string    `json:"OS"`
	TailscaleIPs []string  `json:"TailscaleIPs"`
	Online       bool      `json:"Online"`
	Active       bool      `json:"Active"`
	LastSeen     time.Time `json:"LastSeen"`
}

type rawUser struct {
	LoginName   string `json:"LoginName"`
	DisplayName string `json:"DisplayName"`
}

// GetStatus runs `tailscale status --json` and returns parsed data.
func GetStatus() (*Status, error) {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale status: %w", err)
	}
	return parseStatus(out)
}

// parseStatus decodes `tailscale status --json` output into Status.
// Online on Peer reflects active tunnel use (Tailscale's Active field), not
// just whether the remote daemon is registered with the control plane.
func parseStatus(out []byte) (*Status, error) {
	var raw rawStatus
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("tailscale parse: %w", err)
	}

	status := &Status{
		Self: Peer{
			Hostname:     raw.Self.HostName,
			FriendlyName: pickFriendlyName(raw.Self, raw.User),
			DeviceName:   pickDeviceName(raw.Self),
			IPs:          raw.Self.TailscaleIPs,
			OS:           raw.Self.OS,
			Online:       true, // self is always active
		},
	}

	for _, p := range raw.Peer {
		status.Peers = append(status.Peers, Peer{
			Hostname:     p.HostName,
			FriendlyName: pickFriendlyName(p, raw.User),
			DeviceName:   pickDeviceName(p),
			IPs:          p.TailscaleIPs,
			OS:           p.OS,
			Online:       p.Active,
			LastSeen:     p.LastSeen,
		})
	}

	return status, nil
}

// Available reports whether tailscale is installed on the system.
func Available() bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}

func pickFriendlyName(peer rawPeer, users map[string]rawUser) string {
	if peer.UserID > 0 {
		if user, ok := users[fmt.Sprintf("%d", peer.UserID)]; ok {
			if user.DisplayName != "" {
				return user.DisplayName
			}
			if user.LoginName != "" {
				return user.LoginName
			}
		}
	}
	if peer.HostName != "" {
		return peer.HostName
	}
	if len(peer.TailscaleIPs) > 0 {
		return peer.TailscaleIPs[0]
	}
	return "unknown"
}

func pickDeviceName(peer rawPeer) string {
	if peer.HostName != "" && peer.HostName != "device-of-shared-to-user" {
		return peer.HostName
	}
	if peer.OS != "" {
		return peer.OS + " device"
	}
	if len(peer.TailscaleIPs) > 0 {
		return peer.TailscaleIPs[0]
	}
	return "unknown device"
}
