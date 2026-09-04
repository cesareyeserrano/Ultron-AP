// Acceptance-criterion coverage for the insights engine: missing-variable
// tolerance, verdict ordering within a severity, orphan rule state, the
// flapping log payload, and the WAN/LAN disambiguation verdict.
//
// @aitri-trace FR-041 FR-044 FR-045 FR-046 FR-047
// TC-IE-014f TC-IE-015h TC-IE-016f TC-IE-017f TC-IE-018h
package insights

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/insights/lang"
	insightsstore "github.com/cesareyeserrano/ultron-ap/internal/insights/store"
)

// logLinesContaining counts captured lines holding every one of the substrings.
func logLinesContaining(lines *[]string, subs ...string) int {
	n := 0
outer:
	for _, l := range *lines {
		for _, s := range subs {
			if !strings.Contains(l, s) {
				continue outer
			}
		}
		n++
	}
	return n
}

// TC-IE-014f / AC-041-003
// A rule whose condition references a variable the snapshot does not carry
// must evaluate FALSE rather than error, and say so exactly once per process.
//
// The acceptance criterion names the variable 'lan_device_count'; the shipped
// rule (lan_offline_burst) and buildEvalCtx both call it
// 'lan_device_offline_count'. The behaviour under test is the same — this
// asserts against the name the code actually uses.
//
// @aitri-tc TC-IE-014f
func TestTC_IE_014f_MissingLanDeviceVarIsFalseAndLoggedOnce(t *testing.T) {
	// @aitri-tc TC-IE-014f
	svc, _, clk, lines, mu := newTestEngine(t)

	// An otherwise healthy Pi whose lan-devices feature is off: the variable
	// is absent from the map entirely, not present-and-zero.
	vars := idleVars()
	delete(vars, "lan_device_offline_count")

	for i := 0; i < 5; i++ {
		verdicts := svc.EvalWithVars(clk.Now(), vars)
		for _, v := range verdicts {
			assert.NotEqual(t, "lan_offline_burst", v.RuleID,
				"a rule over a missing variable must resolve false, never fire")
		}
		clk.Advance(5 * time.Second)
	}

	// The engine must still be running normally — a missing variable is not an
	// error path that stops evaluation.
	require.Empty(t, svc.Active(), "an idle snapshot yields no verdicts")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1,
		logLinesContaining(lines, "skipped-missing-var", "lan_device_offline_count"),
		"exactly one skipped-missing-var line across 5 ticks")
}

// TC-IE-015h / AC-044-002
// Within one severity, verdicts are ordered by FirstEmittedAt descending —
// newest first. Severity ordering is covered elsewhere; this pins the
// tie-break, which is the half that silently degrades if the comparator is
// ever simplified to sort on severity alone.
//
// @aitri-tc TC-IE-015h
func TestTC_IE_015h_WithinSeverityVerdictsAreNewestFirst(t *testing.T) {
	// @aitri-tc TC-IE-015h
	svc, _, clk, _, _ := newTestEngine(t)

	// Tick 1 — wan_lan_disambig (warn) starts firing.
	vars := idleVars()
	vars["wan_gateway_ok"] = lang.Number(1)
	vars["wan_cloudflare_ok"] = lang.Number(0)
	svc.EvalWithVars(clk.Now(), vars)

	older := svc.Active()
	require.Len(t, older, 1, "only the WAN/LAN verdict is active yet")
	require.Equal(t, "wan_lan_disambig", older[0].RuleID)

	// Tick 2, a minute later — memory_pressure (also warn) joins it, so the two
	// share a severity and differ only in when they first appeared. It is a
	// plain conjunction, so it fires on the tick it is set; the other warn
	// rules sit behind sustained() windows and would not be active yet.
	clk.Advance(1 * time.Minute)
	vars["ram_pct"] = lang.Number(95)
	vars["swap_pct"] = lang.Number(10)
	svc.EvalWithVars(clk.Now(), vars)

	active := svc.Active()
	require.Len(t, active, 2, "both warn verdicts are active")
	require.Equal(t, active[0].Severity, active[1].Severity,
		"the fixture is only meaningful if both sit in the same severity")

	// Active() iterates a map, so the order it starts from differs run to run.
	// A comparator that had simply dropped the tie-break would still land the
	// right way round about half the time — assert over repeated calls so that
	// coin flip cannot pass for correct ordering.
	for i := 0; i < 20; i++ {
		got := svc.Active()
		require.Len(t, got, 2)
		assert.Equal(t, "memory_pressure", got[0].RuleID,
			"call %d: the newer verdict sorts first within its severity", i)
		assert.True(t, got[0].FirstEmittedAt.After(got[1].FirstEmittedAt),
			"call %d: ordering is by FirstEmittedAt descending", i)
	}
}

// TC-IE-016f / AC-045-003
// A RuleState row whose rule was removed in a later build is an orphan: it
// survives in the database, must not break startup, and must be ignored when
// the engine evaluates.
//
// @aitri-tc TC-IE-016f
func TestTC_IE_016f_OrphanRuleStateDoesNotBreakStartupOrEvaluation(t *testing.T) {
	// @aitri-tc TC-IE-016f
	svc, st, clk, _, _ := newTestEngine(t)

	// A rule that shipped once and no longer exists in bundled.json.
	const removed = "rule_removed_in_a_later_build"
	now := clk.Now()
	require.NoError(t, st.PersistState(insightsstore.State{
		RuleID:              removed,
		LastEvaluatedAt:     now,
		LastValue:           true,
		LastChangeAt:        now,
		TransitionsInWindow: 3,
	}))

	// Startup over a database carrying the orphan must not fail.
	require.NoError(t, svc.LoadBundled(), "an orphan row must not fail startup")

	// The orphan must not evaluate — it has no rule and therefore no condition.
	verdicts := svc.EvalWithVars(clk.Now(), idleVars())
	for _, v := range verdicts {
		assert.NotEqual(t, removed, v.RuleID, "an orphan row must never produce a verdict")
	}
	for _, v := range svc.Active() {
		assert.NotEqual(t, removed, v.RuleID, "an orphan row must never reach the active set")
	}

	// And the engine still works: a real rule fires normally alongside it.
	vars := idleVars()
	vars["disk_root_pct"] = lang.Number(99)
	clk.Advance(5 * time.Second)
	svc.EvalWithVars(clk.Now(), vars)

	fired := false
	for _, v := range svc.Active() {
		if v.RuleID == "disk_critical" {
			fired = true
		}
	}
	assert.True(t, fired, "evaluation continues normally with an orphan row present")

	// The orphan row is still on disk — tolerated, not silently deleted.
	state, err := st.LoadState()
	require.NoError(t, err)
	_, present := state[removed]
	assert.True(t, present, "the orphan row is left alone, not cleaned up behind the operator's back")
}

// TC-IE-017f / AC-046-002
// The flapping hold emits ONE structured line per window, and that line carries
// both the rule_id and the transition_count. TC-IE-008h pins the "exactly one"
// half; this pins the payload, without which the line cannot be acted on.
//
// @aitri-tc TC-IE-017f
func TestTC_IE_017f_FlappingLogCarriesRuleIDAndTransitionCount(t *testing.T) {
	// @aitri-tc TC-IE-017f
	svc, _, clk, lines, mu := newTestEngine(t)

	vars := idleVars()
	svc.EvalWithVars(clk.Now(), vars)

	// Flip disk_critical either side of its threshold once per second — a
	// single comparison with no sustained operator, so it moves every tick.
	for _, v := range []float64{98, 20, 98, 20, 98, 20} {
		clk.Advance(1 * time.Second)
		vars["disk_root_pct"] = lang.Number(v)
		svc.EvalWithVars(clk.Now(), vars)
	}

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, 1, logLinesContaining(lines, "rule-flapping", "disk_critical"),
		"one rule-flapping line per window")
	assert.Equal(t, 1, logLinesContaining(lines, "rule-flapping", "disk_critical", "rule_id"),
		"the line is structured — it names the rule_id field")
	assert.Equal(t, 1, logLinesContaining(lines, "rule-flapping", "disk_critical", "transition_count"),
		"the line carries transition_count, so the hold can be judged from logs alone")
	// The hold engages the moment transitions reach FlapThreshold (default 5),
	// so the count reports the threshold crossing rather than the total number
	// of flips that eventually happen in the window.
	assert.Equal(t, 1, logLinesContaining(lines, "rule-flapping", "disk_critical", "5"),
		"transition_count carries the real count that engaged the hold, not a placeholder")
}

// TC-IE-018h / AC-047-004
// wan_gateway_ok=true with wan_cloudflare_ok=false held over two consecutive
// ticks yields the 'LAN ok, ISP/WAN down' verdict at severity warn — the
// distinction that stops a local-network alarm being raised for an ISP outage.
//
// @aitri-tc TC-IE-018h
func TestTC_IE_018h_WANLANDisambigIsWarnOverTwoConsecutiveTicks(t *testing.T) {
	// @aitri-tc TC-IE-018h
	svc, _, clk, _, _ := newTestEngine(t)

	vars := idleVars()
	vars["wan_gateway_ok"] = lang.Number(1)    // the LAN side is fine
	vars["wan_cloudflare_ok"] = lang.Number(0) // the public probe is not

	var found *Verdict
	for tick := 1; tick <= 2; tick++ {
		svc.EvalWithVars(clk.Now(), vars)
		clk.Advance(5 * time.Second)

		found = nil
		for i, v := range svc.Active() {
			if v.RuleID == "wan_lan_disambig" {
				found = &svc.Active()[i]
				break
			}
		}
		require.NotNil(t, found, "the verdict must hold across tick %d, not flicker", tick)
	}

	assert.Equal(t, "warn", string(found.Severity),
		"an ISP-side outage is a warning, not a critical — the Pi itself is healthy")
	assert.Contains(t, strings.ToLower(found.VerdictText), "internet",
		"the verdict text points at the public internet, not the LAN")
}
