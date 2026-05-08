package tailscale

import "testing"

func TestPickFriendlyName(t *testing.T) {
	tests := []struct {
		name string
		peer rawPeer
		user map[string]rawUser
		want string
	}{
		{
			name: "prefers display name",
			peer: rawPeer{UserID: 42, HostName: "device-of-shared-to-user"},
			user: map[string]rawUser{"42": {DisplayName: "Ximena", LoginName: "x@example.com"}},
			want: "Ximena",
		},
		{
			name: "falls back to login name",
			peer: rawPeer{UserID: 42, HostName: "device-of-shared-to-user"},
			user: map[string]rawUser{"42": {LoginName: "x@example.com"}},
			want: "x@example.com",
		},
		{
			name: "falls back to hostname",
			peer: rawPeer{HostName: "iphone-ximena"},
			user: map[string]rawUser{},
			want: "iphone-ximena",
		},
		{
			name: "falls back to first ip",
			peer: rawPeer{TailscaleIPs: []string{"100.85.52.11"}},
			user: map[string]rawUser{},
			want: "100.85.52.11",
		},
		{
			name: "unknown if no metadata",
			peer: rawPeer{},
			user: map[string]rawUser{},
			want: "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickFriendlyName(tc.peer, tc.user)
			if got != tc.want {
				t.Fatalf("pickFriendlyName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPickDeviceName(t *testing.T) {
	tests := []struct {
		name string
		peer rawPeer
		want string
	}{
		{
			name: "uses explicit hostname",
			peer: rawPeer{HostName: "iphone-ximena", OS: "iOS"},
			want: "iphone-ximena",
		},
		{
			name: "falls back to os when shared hostname",
			peer: rawPeer{HostName: "device-of-shared-to-user", OS: "iOS"},
			want: "iOS device",
		},
		{
			name: "falls back to first ip",
			peer: rawPeer{TailscaleIPs: []string{"100.85.52.11"}},
			want: "100.85.52.11",
		},
		{
			name: "unknown device when no metadata",
			peer: rawPeer{},
			want: "unknown device",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickDeviceName(tc.peer)
			if got != tc.want {
				t.Fatalf("pickDeviceName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseStatusPeerOnlineReflectsActive(t *testing.T) {
	// BG-035: a shared peer reports Online=true (daemon registered with
	// control plane) but Active=false (tunnel not in use). The panel must
	// surface tunnel use, not daemon liveness.
	raw := []byte(`{
		"Self": {"HostName": "ultron-ap", "TailscaleIPs": ["100.64.0.1"], "OS": "linux"},
		"Peer": {
			"k-shared": {
				"HostName": "device-of-shared-to-user",
				"UserID": 42,
				"OS": "iOS",
				"TailscaleIPs": ["100.85.52.11"],
				"Online": true,
				"Active": false,
				"LastSeen": "0001-01-01T00:00:00Z"
			},
			"k-active": {
				"HostName": "macbook-cesar",
				"UserID": 7,
				"OS": "macOS",
				"TailscaleIPs": ["100.64.0.5"],
				"Online": true,
				"Active": true,
				"LastSeen": "2026-05-08T12:00:00Z"
			},
			"k-offline": {
				"HostName": "old-laptop",
				"UserID": 7,
				"OS": "linux",
				"TailscaleIPs": ["100.64.0.9"],
				"Online": false,
				"Active": false,
				"LastSeen": "2026-05-01T08:00:00Z"
			}
		},
		"User": {
			"42": {"DisplayName": "Ximena", "LoginName": "x@example.com"},
			"7":  {"DisplayName": "Cesar",  "LoginName": "c@example.com"}
		}
	}`)

	st, err := parseStatus(raw)
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if !st.Self.Online {
		t.Fatalf("self should always be online")
	}

	got := map[string]Peer{}
	for _, p := range st.Peers {
		got[p.Hostname] = p
	}

	shared, ok := got["device-of-shared-to-user"]
	if !ok {
		t.Fatalf("missing shared peer")
	}
	if shared.Online {
		t.Errorf("shared peer with Active=false must report Online=false (BG-035)")
	}
	if !shared.LastSeen.IsZero() {
		t.Errorf("shared peer LastSeen should be zero, got %v", shared.LastSeen)
	}

	active, ok := got["macbook-cesar"]
	if !ok {
		t.Fatalf("missing active peer")
	}
	if !active.Online {
		t.Errorf("active peer must report Online=true")
	}

	offline, ok := got["old-laptop"]
	if !ok {
		t.Fatalf("missing offline peer")
	}
	if offline.Online {
		t.Errorf("offline peer must report Online=false")
	}
	if offline.LastSeen.IsZero() {
		t.Errorf("offline peer LastSeen should preserve non-zero timestamp")
	}
	if offline.LastSeen.Year() != 2026 {
		t.Errorf("offline peer LastSeen year = %d, want 2026", offline.LastSeen.Year())
	}
}
