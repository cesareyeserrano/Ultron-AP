package gatewayprobe

import (
	"testing"
)

func TestDecodeProcGatewayHex(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0100007F", "127.0.0.1"},
		{"0101A8C0", "192.168.1.1"},
		{"FE01A8C0", "192.168.1.254"},
		{"00000000", "0.0.0.0"},
	}
	for _, tt := range tests {
		got, err := decodeProcGatewayHex(tt.in)
		if err != nil {
			t.Errorf("decodeProcGatewayHex(%q) returned error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("decodeProcGatewayHex(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDecodeProcGatewayHex_BadInput(t *testing.T) {
	cases := []string{"", "ABC", "GGGGGGGG", "0102A8C0FF"}
	for _, c := range cases {
		if _, err := decodeProcGatewayHex(c); err == nil {
			t.Errorf("decodeProcGatewayHex(%q) expected error, got nil", c)
		}
	}
}

func TestStrconvParseUint(t *testing.T) {
	tests := []struct {
		in   string
		want uint64
	}{
		{"0", 0},
		{"3", 3},
		{"A", 10},
		{"FF", 255},
		{"0003", 3},
	}
	for _, tt := range tests {
		got, err := strconvParseUint(tt.in)
		if err != nil {
			t.Errorf("strconvParseUint(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("strconvParseUint(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestNew_HasInitialSnapshot(t *testing.T) {
	p := New(0) // 0 → defaults to 5s
	snap := p.Latest()
	if snap == nil {
		t.Fatal("Latest() is nil before Start")
	}
	if snap.Status != StatusInit {
		t.Errorf("initial status = %q, want %q", snap.Status, StatusInit)
	}
}
