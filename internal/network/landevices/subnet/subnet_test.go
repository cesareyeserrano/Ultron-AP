package subnet

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /proc/net/route format: header line, then space-delimited rows.
// Iface | Destination | Gateway   | Flags | RefCnt | Use | Metric | Mask     | MTU | Window | IRTT
//
// Destination/Gateway/Mask are little-endian hex of the 4-byte IPv4 address.
// 0.0.0.0 → 00000000. 192.168.1.1 → 0101A8C0.

const routeHeader = "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n"

func writeRouteFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "route")
	require.NoError(t, os.WriteFile(path, []byte(routeHeader+body), 0o644))
	return path
}

func ipnet(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(cidr)
	require.NoError(t, err)
	return n
}

// TC-LD-001h
//
// @aitri-tc TC-LD-001h
func TestTC_LD_001h_Detect_HappyPath_192_168_1_42_24(t *testing.T) {
	// @aitri-tc TC-LD-001h
	routePath := writeRouteFile(t, "eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")

	resolver := func(name string) ([]net.Addr, error) {
		assert.Equal(t, "eth0", name)
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.168.1.42").To4(), Mask: ipnet(t, "192.168.1.0/24").Mask},
		}, nil
	}

	got, err := Detect(routePath, resolver)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.0/24", got.CIDR)
	assert.Equal(t, "eth0", got.Iface)
	assert.Equal(t, "192.168.1.42", got.HostIP)
	assert.Equal(t, StatusOK, got.Status)
}

// TC-LD-001f
//
// @aitri-tc TC-LD-001f
func TestTC_LD_001f_Detect_NoDefaultRoute(t *testing.T) {
	// @aitri-tc TC-LD-001f
	// Only a non-default route present
	routePath := writeRouteFile(t, "eth0\t0001A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n")

	got, err := Detect(routePath, func(string) ([]net.Addr, error) {
		t.Fatalf("resolver should not be called when no default route")
		return nil, nil
	})
	require.NoError(t, err)
	assert.Equal(t, StatusNoDefaultRoute, got.Status)
	assert.Empty(t, got.CIDR)
	assert.Empty(t, got.Iface)
}

// TC-LD-001e
//
// @aitri-tc TC-LD-001e
func TestTC_LD_001e_Detect_ClampSlash16ToHostSlash24(t *testing.T) {
	// @aitri-tc TC-LD-001e
	routePath := writeRouteFile(t, "eth0\t00000000\t0101320A\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")

	resolver := func(name string) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("10.50.7.42").To4(), Mask: ipnet(t, "10.50.0.0/16").Mask},
		}, nil
	}
	got, err := Detect(routePath, resolver)
	require.NoError(t, err)
	assert.Equal(t, "10.50.7.0/24", got.CIDR)
	assert.Equal(t, StatusClamped, got.Status)
}

// Additional coverage: route file unreadable behaves like no-default-route.
func TestDetect_RouteFileMissing(t *testing.T) {
	got, err := Detect("/non/existent/route/path", func(string) ([]net.Addr, error) {
		t.Fatal("resolver should not be called")
		return nil, nil
	})
	require.NoError(t, err)
	assert.Equal(t, StatusNoDefaultRoute, got.Status)
}

// Additional: a /24 interface (already at the target prefix) must not be flagged as clamped.
func TestDetect_NaturalSlash24NotClamped(t *testing.T) {
	routePath := writeRouteFile(t, "eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n")
	resolver := func(name string) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("192.168.1.50").To4(), Mask: ipnet(t, "192.168.1.0/24").Mask},
		}, nil
	}
	got, err := Detect(routePath, resolver)
	require.NoError(t, err)
	assert.Equal(t, StatusOK, got.Status)
	assert.Equal(t, "192.168.1.0/24", got.CIDR)
}
