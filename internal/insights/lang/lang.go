// Package lang implements the JSON-encoded boolean condition language used by
// the insights engine. Each rule's condition tree is parsed once into a
// callable closure (Compiled.Eval) at load time so the steady-state per-tick
// evaluation does zero allocations (NFR-016).
//
// Grammar (per 02_SYSTEM_DESIGN.md):
//   - Variable literal:     {"var": "cpu_pct"}
//   - Constant literal:     {"const": 90}     (number, string, or bool)
//   - Comparison:           {"op": "gt", "left": <expr>, "right": <expr>}
//                           op ∈ {eq, ne, gt, gte, lt, lte}
//   - Logical:              {"op": "and"|"or"|"not", "args": [<expr>, ...]}
//   - Sustained window:     {"sustained": {"var": ..., "op": ..., "value": ..., "window_ms": ...}}
//
// Missing telemetry variables resolve to false (FR-041 AC-002) and emit a
// single "skipped-missing-var" log entry per process per (rule, var) pair.
//
// @aitri-trace FR-041 US-041 TC-IE-003h TC-IE-003f TC-IE-003e
package lang

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Value is the polymorphic value passed through eval. Numbers are float64
// (matching Go's encoding/json default); strings and bools are themselves;
// the zero Value (kind=kindNone) means "missing variable".
type Value struct {
	kind kind
	num  float64
	str  string
	b    bool
}

type kind uint8

const (
	kindNone kind = iota
	kindNum
	kindStr
	kindBool
)

// Number, String, Bool, and None are the constructors used by the EvalCtx.
func Number(n float64) Value  { return Value{kind: kindNum, num: n} }
func String(s string) Value   { return Value{kind: kindStr, str: s} }
func Bool(b bool) Value       { return Value{kind: kindBool, b: b} }
func None() Value             { return Value{kind: kindNone} }
func (v Value) IsMissing() bool { return v.kind == kindNone }

// EvalCtx is the per-tick evaluation context. Lookup is the variable resolver
// passed in by the engine; the engine reuses one EvalCtx per rule per tick to
// keep the steady-state allocation count at zero (NFR-016).
type EvalCtx struct {
	Lookup func(name string) Value
	// MissingLogger is called once per (ruleID, varName) when a referenced
	// variable is missing. The lang package is rule-agnostic, so the engine
	// installs a logger that captures the rule id from its own context.
	MissingLogger func(varName string)
	// NowMS is the tick timestamp in Unix milliseconds. Read by sustained-window
	// nodes to advance their continuous-truth window. Engine writes this once
	// per tick before evaluating each rule.
	NowMS int64
}

// Get looks up name and emits a one-shot missing-var log if absent.
func (c *EvalCtx) Get(name string) Value {
	if c == nil || c.Lookup == nil {
		return None()
	}
	v := c.Lookup(name)
	if v.IsMissing() && c.MissingLogger != nil {
		c.MissingLogger(name)
	}
	return v
}

// Compiled is a precompiled rule condition. Eval is the hot-path closure.
type Compiled struct {
	Eval func(*EvalCtx) bool
	// Reset clears any sustained-window state (called when the engine wants
	// to discard hysteresis between independent test runs).
	Reset func()
}

// Compile parses condition JSON into a Compiled closure. Sustained-window
// state is owned per Compiled — each top-level Compile() call gets its own
// ring buffer per sustained node.
func Compile(raw json.RawMessage) (*Compiled, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty condition")
	}
	state := &compileState{}
	fn, err := state.compileNode(raw)
	if err != nil {
		return nil, err
	}
	return &Compiled{
		Eval: func(c *EvalCtx) bool { return fn(c) },
		Reset: func() {
			for _, r := range state.sustained {
				r.reset()
			}
		},
	}, nil
}

type compileState struct {
	sustained []*sustainedState
}

type nodeFn func(*EvalCtx) bool

type rawNode struct {
	// Comparison/logical
	Op    string            `json:"op,omitempty"`
	Left  json.RawMessage   `json:"left,omitempty"`
	Right json.RawMessage   `json:"right,omitempty"`
	Args  []json.RawMessage `json:"args,omitempty"`
	// Variable literal
	Var string `json:"var,omitempty"`
	// Constant literal — use a pointer so we can detect "absent" vs "null".
	Const json.RawMessage `json:"const,omitempty"`
	// Sustained window (mutually exclusive with everything above)
	Sustained *sustainedSpec `json:"sustained,omitempty"`
}

type sustainedSpec struct {
	Var      string  `json:"var"`
	Op       string  `json:"op"`
	Value    float64 `json:"value"`
	WindowMS int64   `json:"window_ms"`
}

func (s *compileState) compileNode(raw json.RawMessage) (nodeFn, error) {
	var n rawNode
	dec := json.NewDecoder(bytesReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&n); err != nil {
		return nil, fmt.Errorf("invalid condition node: %w", err)
	}

	// Sustained window — produces a boolean closure.
	if n.Sustained != nil {
		return s.compileSustained(n.Sustained)
	}

	// Constant literal (rarely used as a top-level node, but a comparison's
	// right-hand-side is always a const expression).
	if len(n.Const) > 0 {
		return nil, fmt.Errorf("const literal is not a boolean expression")
	}

	switch n.Op {
	case "and", "or":
		return s.compileBoolN(n)
	case "not":
		return s.compileNot(n)
	case "eq", "ne", "gt", "gte", "lt", "lte":
		return s.compileCmp(n)
	case "":
		return nil, fmt.Errorf("missing 'op' / 'sustained' / 'var' / 'const' on node")
	default:
		return nil, fmt.Errorf("unknown op %q", n.Op)
	}
}

func (s *compileState) compileBoolN(n rawNode) (nodeFn, error) {
	if len(n.Args) == 0 {
		return nil, fmt.Errorf("op %q requires at least one arg", n.Op)
	}
	args := make([]nodeFn, 0, len(n.Args))
	for i, raw := range n.Args {
		f, err := s.compileNode(raw)
		if err != nil {
			return nil, fmt.Errorf("arg %d: %w", i, err)
		}
		args = append(args, f)
	}
	if n.Op == "and" {
		return func(c *EvalCtx) bool {
			for _, fn := range args {
				if !fn(c) {
					return false
				}
			}
			return true
		}, nil
	}
	// or
	return func(c *EvalCtx) bool {
		for _, fn := range args {
			if fn(c) {
				return true
			}
		}
		return false
	}, nil
}

func (s *compileState) compileNot(n rawNode) (nodeFn, error) {
	if len(n.Args) != 1 {
		return nil, fmt.Errorf("'not' requires exactly one arg, got %d", len(n.Args))
	}
	inner, err := s.compileNode(n.Args[0])
	if err != nil {
		return nil, fmt.Errorf("not arg: %w", err)
	}
	return func(c *EvalCtx) bool { return !inner(c) }, nil
}

// operandFn returns a Value for a comparison operand (var or const literal).
type operandFn func(*EvalCtx) Value

func (s *compileState) compileOperand(raw json.RawMessage) (operandFn, error) {
	var n rawNode
	dec := json.NewDecoder(bytesReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&n); err != nil {
		return nil, fmt.Errorf("invalid operand: %w", err)
	}
	if n.Var != "" {
		name := n.Var
		return func(c *EvalCtx) Value { return c.Get(name) }, nil
	}
	if len(n.Const) > 0 {
		v, err := decodeConst(n.Const)
		if err != nil {
			return nil, err
		}
		return func(*EvalCtx) Value { return v }, nil
	}
	return nil, fmt.Errorf("operand must be {\"var\":...} or {\"const\":...}")
}

func (s *compileState) compileCmp(n rawNode) (nodeFn, error) {
	if len(n.Left) == 0 || len(n.Right) == 0 {
		return nil, fmt.Errorf("op %q requires 'left' and 'right'", n.Op)
	}
	left, err := s.compileOperand(n.Left)
	if err != nil {
		return nil, fmt.Errorf("left: %w", err)
	}
	right, err := s.compileOperand(n.Right)
	if err != nil {
		return nil, fmt.Errorf("right: %w", err)
	}
	op := n.Op
	return func(c *EvalCtx) bool {
		l := left(c)
		r := right(c)
		if l.IsMissing() || r.IsMissing() {
			return false
		}
		return cmpValues(op, l, r)
	}, nil
}

// sustainedState holds the per-rule ring buffer for one sustained node. It is
// captured by reference inside the closure so successive ticks accumulate
// continuous-truth tracking without allocating.
//
// We count consecutive true ticks rather than measuring (now-start) directly
// because the parent FR-001 cadence guarantees uniform tick spacing — each
// tick "covers" cadence ms of observation, so a window of N ticks covers
// N*cadence ms regardless of how (now - first_true_tick) interpolates.
type sustainedState struct {
	mu         sync.Mutex
	startMS    int64 // first tick where the inner condition became true; 0 = not currently true
	prevMS     int64 // most recent tick timestamp seen
	lastTickMS int64
	cadenceMS  int64 // observed cadence between consecutive ticks (median estimate)
	windowMS   int64
}

func (st *sustainedState) reset() {
	st.mu.Lock()
	st.startMS = 0
	st.prevMS = 0
	st.lastTickMS = 0
	st.cadenceMS = 0
	st.mu.Unlock()
}

func (s *compileState) compileSustained(spec *sustainedSpec) (nodeFn, error) {
	if spec.Var == "" {
		return nil, fmt.Errorf("sustained: var required")
	}
	if spec.WindowMS <= 0 {
		return nil, fmt.Errorf("sustained: window_ms must be > 0")
	}
	switch spec.Op {
	case "eq", "ne", "gt", "gte", "lt", "lte":
	default:
		return nil, fmt.Errorf("sustained: unknown op %q", spec.Op)
	}
	state := &sustainedState{windowMS: spec.WindowMS}
	s.sustained = append(s.sustained, state)
	varName := spec.Var
	op := spec.Op
	threshold := Number(spec.Value)

	return func(c *EvalCtx) bool {
		// Pull tick time from the EvalCtx if the engine attached one.
		nowMS := contextNow(c)
		v := c.Get(varName)
		state.mu.Lock()
		defer state.mu.Unlock()
		// Update cadence estimate from the gap to the previous tick.
		if state.prevMS != 0 && nowMS > state.prevMS {
			gap := nowMS - state.prevMS
			if state.cadenceMS == 0 || gap < state.cadenceMS {
				state.cadenceMS = gap
			}
		}
		state.prevMS = nowMS

		if v.IsMissing() {
			state.startMS = 0
			return false
		}
		live := cmpValues(op, v, threshold)
		if !live {
			state.startMS = 0
			state.lastTickMS = nowMS
			return false
		}
		if state.startMS == 0 {
			state.startMS = nowMS
		}
		state.lastTickMS = nowMS
		// Each tick covers `cadence` ms of continuous truth. Counting the
		// start tick as one cadence-sized observation, the total covered
		// duration is (now - start) + cadence. Fire when that ≥ window.
		cadence := state.cadenceMS
		if cadence <= 0 {
			cadence = 0
		}
		covered := (nowMS - state.startMS) + cadence
		return covered >= state.windowMS
	}, nil
}

// Now returns the tick timestamp (Unix milliseconds) attached to this context.
// Sustained-window evaluators read this to advance their continuous-truth window.
func (c *EvalCtx) Now() int64 { return c.NowMS }

func contextNow(c *EvalCtx) int64 {
	if c == nil {
		return 0
	}
	return c.NowMS
}

// decodeConst decodes a JSON literal into a Value. Numbers are stored as
// float64; strings and bools are stored as-is.
func decodeConst(raw json.RawMessage) (Value, error) {
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return Number(n), nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return String(s), nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return Bool(b), nil
	}
	return Value{}, fmt.Errorf("invalid const literal %s", string(raw))
}

// cmpValues applies a comparison operator to two Values. When the kinds
// disagree (number vs string), the result is false.
func cmpValues(op string, l, r Value) bool {
	switch {
	case l.kind == kindNum && r.kind == kindNum:
		return cmpNum(op, l.num, r.num)
	case l.kind == kindStr && r.kind == kindStr:
		return cmpStr(op, l.str, r.str)
	case l.kind == kindBool && r.kind == kindBool:
		return cmpBool(op, l.b, r.b)
	case l.kind == kindBool && r.kind == kindNum:
		return cmpNum(op, boolNum(l.b), r.num)
	case l.kind == kindNum && r.kind == kindBool:
		return cmpNum(op, l.num, boolNum(r.b))
	}
	return false
}

func boolNum(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func cmpNum(op string, a, b float64) bool {
	switch op {
	case "eq":
		return a == b
	case "ne":
		return a != b
	case "gt":
		return a > b
	case "gte":
		return a >= b
	case "lt":
		return a < b
	case "lte":
		return a <= b
	}
	return false
}

func cmpStr(op string, a, b string) bool {
	switch op {
	case "eq":
		return a == b
	case "ne":
		return a != b
	}
	return false
}

func cmpBool(op string, a, b bool) bool {
	switch op {
	case "eq":
		return a == b
	case "ne":
		return a != b
	}
	return false
}

// bytesReader returns a non-allocating io.Reader wrapper over a raw JSON
// fragment. encoding/json's NewDecoder needs an io.Reader — the standard
// library bytes.Reader allocates a small wrapper but is safe for the
// non-hot-path Compile call.
type byteSliceReader struct {
	b   []byte
	off int
}

func (r *byteSliceReader) Read(p []byte) (int, error) {
	if r.off >= len(r.b) {
		return 0, ioEOF
	}
	n := copy(p, r.b[r.off:])
	r.off += n
	return n, nil
}

// minimal io.EOF stand-in to avoid importing "io" for one symbol.
var ioEOF = &eofErr{}

type eofErr struct{}

func (*eofErr) Error() string { return "EOF" }

func bytesReader(b []byte) *byteSliceReader { return &byteSliceReader{b: b} }
