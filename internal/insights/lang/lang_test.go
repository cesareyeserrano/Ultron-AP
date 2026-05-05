// Tests for the JSON condition language: comparison, missing-var, sustained.
//
// @aitri-trace FR-041 US-041 TC-IE-003h TC-IE-003f TC-IE-003e
package lang

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: build a Lookup from a map of name → value.
func mapLookup(m map[string]Value) func(string) Value {
	return func(name string) Value {
		if v, ok := m[name]; ok {
			return v
		}
		return None()
	}
}

// TC-IE-003h
// Comparison condition gt(cpu_pct, 90) evaluates true at 91 and false at 90.
//
// @aitri-tc TC-IE-003h
func TestTC_IE_003h_GtComparisonStrictInequality(t *testing.T) {
	// @aitri-tc TC-IE-003h
	cond := json.RawMessage(`{"op":"gt","left":{"var":"cpu_pct"},"right":{"const":90}}`)
	c, err := Compile(cond)
	require.NoError(t, err)

	cases := []struct {
		v    float64
		want bool
	}{
		{91, true},
		{90, false},
		{89, false},
	}
	for _, tc := range cases {
		ctx := &EvalCtx{Lookup: mapLookup(map[string]Value{"cpu_pct": Number(tc.v)})}
		assert.Equal(t, tc.want, c.Eval(ctx), "cpu_pct=%v", tc.v)
	}
}

// TC-IE-003f
// Missing telemetry variable resolves to false (not error) and emits
// 'skipped-missing-var' exactly once.
//
// @aitri-tc TC-IE-003f
func TestTC_IE_003f_MissingVarResolvesFalseLogsOnce(t *testing.T) {
	// @aitri-tc TC-IE-003f
	cond := json.RawMessage(`{"op":"gt","left":{"var":"cpu_zzz"},"right":{"const":90}}`)
	c, err := Compile(cond)
	require.NoError(t, err)

	var mu sync.Mutex
	var loggedCount int
	once := make(map[string]bool)
	logger := func(name string) {
		mu.Lock()
		defer mu.Unlock()
		if once[name] {
			return
		}
		once[name] = true
		loggedCount++
	}
	ctx := &EvalCtx{
		Lookup:        mapLookup(nil), // empty — every var is missing
		MissingLogger: logger,
	}
	for i := 0; i < 5; i++ {
		assert.False(t, c.Eval(ctx), "missing var must yield false")
	}
	assert.Equal(t, 1, loggedCount, "missing-var logger must be called exactly once across 5 evals")
}

// TC-IE-003e
// sustained(gt(cpu_pct,90), 30s) requires 6 consecutive in-window ticks
// before firing; falls false on first below-threshold tick.
//
// @aitri-tc TC-IE-003e
func TestTC_IE_003e_SustainedSixTicksThenDrop(t *testing.T) {
	// @aitri-tc TC-IE-003e
	cond := json.RawMessage(`{"sustained":{"var":"cpu_pct","op":"gt","value":90,"window_ms":30000}}`)
	c, err := Compile(cond)
	require.NoError(t, err)

	values := []float64{91, 91, 91, 91, 91, 91, 91, 80}
	expected := []bool{false, false, false, false, false, true, true, false}

	for i, v := range values {
		nowMS := int64(i+1) * 5000 // tick T_i, 5 s cadence
		ctx := &EvalCtx{
			Lookup: mapLookup(map[string]Value{"cpu_pct": Number(v)}),
			NowMS:  nowMS,
		}
		got := c.Eval(ctx)
		assert.Equal(t, expected[i], got, "tick %d cpu=%v", i+1, v)
	}
}

// Bonus: malformed condition syntax (FR-041 AC-003) is rejected at compile.
func TestLang_MalformedConditionRejected(t *testing.T) {
	cases := []string{
		`{"op":"unknown","args":[]}`,
		`{"op":"gt","left":{"var":"cpu"}}`, // missing right
		`{"sustained":{"var":"cpu","op":"gt"}}`, // missing window_ms
	}
	for _, raw := range cases {
		_, err := Compile(json.RawMessage(raw))
		assert.Error(t, err, "raw=%s", raw)
	}
}

// Bonus: const literal types — number, string, bool.
func TestLang_ConstLiteralTypes(t *testing.T) {
	cases := []struct {
		raw  string
		vars map[string]Value
		want bool
	}{
		{`{"op":"eq","left":{"var":"name"},"right":{"const":"ok"}}`, map[string]Value{"name": String("ok")}, true},
		{`{"op":"eq","left":{"var":"name"},"right":{"const":"ok"}}`, map[string]Value{"name": String("fail")}, false},
		{`{"op":"eq","left":{"var":"flag"},"right":{"const":true}}`, map[string]Value{"flag": Bool(true)}, true},
		{`{"op":"eq","left":{"var":"flag"},"right":{"const":1}}`, map[string]Value{"flag": Bool(true)}, true},
	}
	for _, tc := range cases {
		c, err := Compile(json.RawMessage(tc.raw))
		require.NoError(t, err)
		ctx := &EvalCtx{Lookup: mapLookup(tc.vars)}
		assert.Equal(t, tc.want, c.Eval(ctx), "raw=%s", tc.raw)
	}
}

// Bonus: unknown-field detection for compile fails (so Loader can pass these
// up as structured-log entries).
func TestLang_UnknownFieldOnComparisonNode(t *testing.T) {
	_, err := Compile(json.RawMessage(`{"op":"gt","left":{"var":"x"},"right":{"const":1},"mute":true}`))
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unknown field")
}
