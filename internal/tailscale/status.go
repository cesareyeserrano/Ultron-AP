package tailscale

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Peer represents a Tailscale node (self or remote peer).
type Peer struct {
	Hostname string
	IPs      []string
	OS       string
	Online   bool
	LastSeen time.Time
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
}

type rawPeer struct {
	HostName     string    `json:"HostName"`
	OS           string    `json:"OS"`
	TailscaleIPs []string  `json:"TailscaleIPs"`
	Online       bool      `json:"Online"`
	LastSeen     time.Time `json:"LastSeen"`
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
			Hostname: raw.Self.HostName,
			IPs:      raw.Self.TailscaleIPs,
			OS:       raw.Self.OS,
			Online:   true, // self is always online
		},
	}

	for _, p := range raw.Peer {
		status.Peers = append(status.Peers, Peer{
			Hostname: p.HostName,
			IPs:      p.TailscaleIPs,
			OS:       p.OS,
			Online:   p.Online,
			LastSeen: p.LastSeen,
		})
	}

	return status, nil
}

// Available reports whether tailscale is installed on the system.
func Available() bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}
