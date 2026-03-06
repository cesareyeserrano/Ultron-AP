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

	var raw rawStatus
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("tailscale parse: %w", err)
	}

	status := &Status{
		Self: Peer{
			Hostname:     raw.Self.HostName,
			FriendlyName: pickFriendlyName(raw.Self, raw.User),
			IPs:          raw.Self.TailscaleIPs,
			OS:           raw.Self.OS,
			Online:       true, // self is always online
		},
	}

	for _, p := range raw.Peer {
		status.Peers = append(status.Peers, Peer{
			Hostname:     p.HostName,
			FriendlyName: pickFriendlyName(p, raw.User),
			IPs:          p.TailscaleIPs,
			OS:           p.OS,
			Online:       p.Online,
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
