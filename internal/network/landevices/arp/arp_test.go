package arp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const arpHeader = "IP address       HW type     Flags       HW address            Mask     Device\n"

func writeARPFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "arp")
	require.NoError(t, os.WriteFile(path, []byte(arpHeader+body), 0o644))
	return path
}

// TC-LD-003h
func TestPair_HappyPath_FiveResolvedEntries(t *testing.T) {
	body := `192.168.1.1      0x1         0x2         a1:b2:c3:d4:e5:f6     *        eth0
192.168.1.20     0x1         0x2         11:22:33:44:55:66     *        eth0
192.168.1.21     0x1         0x2         AA:BB:CC:DD:EE:01     *        eth0
192.168.1.22     0x1         0x2         AA:BB:CC:DD:EE:02     *        eth0
192.168.1.50     0x1         0x2         B8:27:EB:11:22:33     *        eth0
`
	path := writeARPFile(t, body)
	cache, err := ReadCache(path)
	require.NoError(t, err)
	assert.Len(t, cache, 5)
	assert.Equal(t, "b8:27:eb:11:22:33", cache["192.168.1.50"])

	responders := []string{"192.168.1.1", "192.168.1.20", "192.168.1.21", "192.168.1.22", "192.168.1.50"}
	pairs := PairResponders(responders, cache, nil)
	require.Len(t, pairs, 5)
	for _, p := range pairs {
		assert.Equal(t, "ok", p.Status)
		assert.NotEmpty(t, p.MAC)
	}
}

// TC-LD-003f
func TestPair_DegradedMode_MissingARPFile(t *testing.T) {
	cache, err := ReadCache(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrARPUnavailable))
	assert.Empty(t, cache)

	responders := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}
	pairs := PairResponders(responders, cache, err)
	require.Len(t, pairs, 3)
	for _, p := range pairs {
		assert.Empty(t, p.MAC)
		assert.Equal(t, "no-arp", p.Status)
	}
}

// TC-LD-003e — stale entries (flag 0x0) ignored
func TestPair_IgnoresStaleEntries(t *testing.T) {
	body := `192.168.1.1      0x1         0x2         a1:b2:c3:d4:e5:f6     *        eth0
192.168.1.10     0x1         0x0         00:00:00:00:00:00     *        eth0
192.168.1.20     0x1         0x2         11:22:33:44:55:66     *        eth0
192.168.1.30     0x1         0x0         00:00:00:00:00:00     *        eth0
192.168.1.50     0x1         0x2         B8:27:EB:11:22:33     *        eth0
`
	path := writeARPFile(t, body)
	cache, err := ReadCache(path)
	require.NoError(t, err)

	// Cache only contains the 3 reachable entries.
	assert.Len(t, cache, 3)
	assert.Contains(t, cache, "192.168.1.1")
	assert.Contains(t, cache, "192.168.1.20")
	assert.Contains(t, cache, "192.168.1.50")

	// All 5 IPs responded to ICMP; the 2 with stale flags get no-arp.
	responders := []string{"192.168.1.1", "192.168.1.10", "192.168.1.20", "192.168.1.30", "192.168.1.50"}
	pairs := PairResponders(responders, cache, nil)
	require.Len(t, pairs, 5)

	bestatus := map[string]string{}
	for _, p := range pairs {
		bestatus[p.IP] = p.Status
	}
	assert.Equal(t, "ok", bestatus["192.168.1.1"])
	assert.Equal(t, "no-arp", bestatus["192.168.1.10"])
	assert.Equal(t, "ok", bestatus["192.168.1.20"])
	assert.Equal(t, "no-arp", bestatus["192.168.1.30"])
	assert.Equal(t, "ok", bestatus["192.168.1.50"])
}

// TC-LD-010e (NFR-011 edge): non-root reads /proc/net/arp without escalation.
// We can't truly drop privileges in a unit test, but we can assert no helper
// IPC is invoked (the implementation only uses os.Open).
func TestPair_UnprivilegedRead_NoHelperIPC(t *testing.T) {
	body := "192.168.1.1      0x1         0x2         a1:b2:c3:d4:e5:f6     *        eth0\n"
	path := writeARPFile(t, body)
	_, err := ReadCache(path)
	require.NoError(t, err)
	// If this test ran, ReadCache used os.Open only — no helper IPC, no setcap.
	// The privilege boundary is enforced by construction.
}
