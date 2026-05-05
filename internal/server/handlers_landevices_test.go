// Tests for the lan-devices HTTP surface.
//
// @aitri-trace FR-036 FR-037 US-036 US-037 TC-LD-007h TC-LD-007f TC-LD-007e TC-LD-008h TC-LD-008f
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	landevicesstore "github.com/cesareyeserrano/ultron-ap/internal/network/landevices/store"
)

// seedLANDevices populates the store with 2 online and 1 offline rows. The
// online rows have distinct last_seen values so ordering can be asserted.
func seedLANDevices(t *testing.T, st *landevicesstore.Store) (online1, online2, offline string) {
	t.Helper()
	t0 := time.Now().Add(-1 * time.Hour) // initial first_seen for everyone

	require.NoError(t, st.ApplySweep(t0, []landevicesstore.Observation{
		{MAC: "aa:aa:aa:00:00:01", IP: "192.168.1.10", Vendor: "AcmeOne"},
		{MAC: "aa:aa:aa:00:00:02", IP: "192.168.1.20", Vendor: "AcmeTwo"},
		{MAC: "aa:aa:aa:00:00:03", IP: "192.168.1.30", Vendor: "AcmeThree"},
	}))

	// Re-observe the two online ones (one slightly older than the other)
	// without observing the third — and run enough sweeps to flip it offline.
	tNow := time.Now()
	tLater := tNow.Add(50 * time.Millisecond)

	// Two consecutive sweeps without #03 trigger the default-3 threshold
	// (initial sweep counted as observed; 3 misses → offline).
	for i := 0; i < 3; i++ {
		require.NoError(t, st.ApplySweep(tNow, []landevicesstore.Observation{
			{MAC: "aa:aa:aa:00:00:01", IP: "192.168.1.10", Vendor: "AcmeOne"},
			{MAC: "aa:aa:aa:00:00:02", IP: "192.168.1.20", Vendor: "AcmeTwo"},
		}))
	}
	// One last sweep where #02 has the freshest last_seen.
	require.NoError(t, st.ApplySweep(tNow, []landevicesstore.Observation{
		{MAC: "aa:aa:aa:00:00:01", IP: "192.168.1.10", Vendor: "AcmeOne"},
	}))
	require.NoError(t, st.ApplySweep(tLater, []landevicesstore.Observation{
		{MAC: "aa:aa:aa:00:00:02", IP: "192.168.1.20", Vendor: "AcmeTwo"},
	}))

	return "aa:aa:aa:00:00:01", "aa:aa:aa:00:00:02", "aa:aa:aa:00:00:03"
}

// TC-LD-007h
// GET /api/network/lan-devices returns JSON, online entries first,
// online ordered by last_seen DESC, ISO 8601 timestamps.
//
// @aitri-tc TC-LD-007h
func TestTC_LD_007h_LANDevicesAPI_OnlineFirstThenLastSeenDesc(t *testing.T) {
	// @aitri-tc TC-LD-007h
	srv, session := setupSSETestServer(t)
	st := landevicesstore.New(srv.db.DB)
	srv.SetLANDevices(nil, st)

	_, online2, offline := seedLANDevices(t, st)
	// To get a deterministic offline state we need to exceed the miss
	// threshold; the seed function leaves #03 with high missed_sweeps.
	// Verify offline state at the store level so the test is self-contained.
	d, err := st.Get(offline)
	require.NoError(t, err)
	require.False(t, d.Online, "seed should leave %s offline", offline)

	req := httptest.NewRequest(http.MethodGet, "/api/network/lan-devices", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var out []lanDeviceJSON
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 3)

	// Online entries listed first.
	assert.True(t, out[0].Online, "first entry must be online")
	assert.True(t, out[1].Online, "second entry must be online")
	assert.False(t, out[2].Online, "third entry must be offline")

	// Within online, last_seen DESC — and most recently observed was online2.
	assert.Equal(t, online2, out[0].MAC)

	// Timestamps parse as RFC3339 (ISO 8601 superset).
	for _, e := range out {
		_, err := time.Parse(time.RFC3339, e.FirstSeen)
		assert.NoError(t, err, "first_seen not ISO 8601: %s", e.FirstSeen)
		_, err = time.Parse(time.RFC3339, e.LastSeen)
		assert.NoError(t, err, "last_seen not ISO 8601: %s", e.LastSeen)
	}
}

// TC-LD-007f
// Unauthenticated GET → 401 (handled via the /api/ branch of
// redirectOrUnauthorized).
//
// @aitri-tc TC-LD-007f
func TestTC_LD_007f_LANDevicesAPI_UnauthenticatedReturns401(t *testing.T) {
	// @aitri-tc TC-LD-007f
	srv, _ := setupSSETestServer(t)
	st := landevicesstore.New(srv.db.DB)
	srv.SetLANDevices(nil, st)

	req := httptest.NewRequest(http.MethodGet, "/api/network/lan-devices", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TC-LD-007e
// API p99 latency over 100 sequential requests stays under 100ms
// for a 30-row LAN.
//
// @aitri-tc TC-LD-007e
func TestTC_LD_007e_LANDevicesAPI_LatencyP99Under100ms(t *testing.T) {
	// @aitri-tc TC-LD-007e
	if testing.Short() {
		t.Skip("skipping latency test in -short mode")
	}
	srv, session := setupSSETestServer(t)
	st := landevicesstore.New(srv.db.DB)
	srv.SetLANDevices(nil, st)

	// Seed 30 rows in one transactional sweep.
	now := time.Now()
	obs := make([]landevicesstore.Observation, 0, 30)
	for i := 0; i < 30; i++ {
		obs = append(obs, landevicesstore.Observation{
			MAC:    macFor(i),
			IP:     ipFor(i),
			Vendor: "AcmeBench",
		})
	}
	require.NoError(t, st.ApplySweep(now, obs))

	const N = 100
	durations := make([]time.Duration, 0, N)
	for i := 0; i < N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/network/lan-devices", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
		rec := httptest.NewRecorder()
		start := time.Now()
		srv.httpServer.Handler.ServeHTTP(rec, req)
		durations = append(durations, time.Since(start))
		require.Equal(t, http.StatusOK, rec.Code)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p99 := durations[98] // index 98 of sorted = 99th-percentile sample (per spec).
	assert.Less(t, p99, 100*time.Millisecond,
		"p99 latency %s must stay under 100 ms (FR-036 AC-004)", p99)
}

// TC-LD-008h
// /network/lan-devices/fragment renders a 5-row HTML table.
// Substituted from the spec's headless-browser e2e (see manifest tech-debt).
//
// @aitri-tc TC-LD-008h
func TestTC_LD_008h_LANDevicesFragment_HappyPathRendersTable(t *testing.T) {
	// @aitri-tc TC-LD-008h
	srv, session := setupSSETestServer(t)
	st := landevicesstore.New(srv.db.DB)
	srv.SetLANDevices(nil, st)

	obs := make([]landevicesstore.Observation, 0, 5)
	for i := 0; i < 5; i++ {
		obs = append(obs, landevicesstore.Observation{
			MAC:    macFor(i),
			IP:     ipFor(i),
			Vendor: "AcmeRow",
		})
	}
	require.NoError(t, st.ApplySweep(time.Now(), obs))

	req := httptest.NewRequest(http.MethodGet, "/network/lan-devices/fragment", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "<table", "fragment must contain a <table>")
	// 5 data rows in <tbody>; count <tr> tags inside tbody by counting
	// occurrences of the per-row IP literal we seeded.
	for i := 0; i < 5; i++ {
		assert.Contains(t, body, ipFor(i))
	}
	// Online badge text should appear at least once (all seeded devices online).
	assert.Contains(t, body, "online")
}

// TC-LD-008f
// Empty DB → fragment shows "No devices discovered yet" and no <table>.
//
// @aitri-tc TC-LD-008f
func TestTC_LD_008f_LANDevicesFragment_EmptyStateOmitsTable(t *testing.T) {
	// @aitri-tc TC-LD-008f
	srv, session := setupSSETestServer(t)
	st := landevicesstore.New(srv.db.DB)
	srv.SetLANDevices(nil, st)

	req := httptest.NewRequest(http.MethodGet, "/network/lan-devices/fragment", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "No devices discovered yet")
	assert.False(t, strings.Contains(body, "<table"),
		"empty-state fragment must not contain a <table> element")
}

// TC-LD-007h supporting: status endpoint returns the orchestrator envelope.
// No orchestrator wired → service unavailable. With store-only wiring, this
// confirms the handler defends against missing orchestrator.
//
// @aitri-tc TC-LD-007h
func TestTC_LD_007h_LANDevicesStatus_NotInitialisedReturns503_2(t *testing.T) {
	// @aitri-tc TC-LD-007h
	srv, session := setupSSETestServer(t)
	// landevices field stays nil; only register the routes by attaching nothing.
	req := httptest.NewRequest(http.MethodGet, "/api/network/lan-devices/status", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// macFor returns a deterministic MAC for index i (0..255).
func macFor(i int) string {
	hex := "0123456789abcdef"
	hi := hex[(i>>4)&0xf]
	lo := hex[i&0xf]
	return "02:00:00:00:00:" + string([]byte{hi, lo})
}

// ipFor returns a /24 host IP for index i (1..254).
func ipFor(i int) string {
	return "192.168.99." + formatInt(i+1)
}
