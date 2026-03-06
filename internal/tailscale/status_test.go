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
