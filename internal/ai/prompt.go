// Module: ai/prompt
// Purpose: Compose a bounded, secret-redacted prompt from a telemetry snapshot and
//          parse the model's reply into a structured Explanation (FR-016, FR-023,
//          FR-024). Caps keep the prompt small, fast, and cheap (NFR-006).
// Dependencies: encoding/json, fmt, strings.
package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Prompt size caps — keep the request small and the call fast/cheap.
const (
	maxVerdicts    = 25
	maxItems       = 30
	maxPromptBytes = 32768
)

// systemPrompt constrains the model to ground its answer in the supplied data,
// cite the signals it used, take no action, and reply as JSON.
const systemPrompt = "You are a diagnostic assistant for a Raspberry Pi system/network monitor. " +
	"Use ONLY the telemetry provided by the user. Do not invent data. Never instruct to run " +
	"destructive commands. You take no action yourself; you only advise. " +
	"Reply with a single JSON object: " +
	`{"cause": "<probable cause in plain language>", "remediation": "<suggested steps>", ` +
	`"cited_signals": ["<signal names you used, e.g. cpu_temp, rule_id=8, docker:nginx>"]}. ` +
	"If the data is insufficient, say so in cause and leave cited_signals empty."

// buildPrompt renders the snapshot into a capped, redacted user prompt.
//
// @aitri-trace FR-ID: FR-016, US-ID: US-016, AC-ID: AC-016-1h, TC-ID: TC-AI-016e
func buildPrompt(snap TelemetrySnapshot, scope Scope, secrets []string) string {
	var b strings.Builder

	if scope.Kind == ScopeInsight && snap.FocusInsight != nil {
		fmt.Fprintf(&b, "FOCUS INSIGHT rule_id=%s severity=%s: %s\n",
			snap.FocusInsight.RuleID, snap.FocusInsight.Severity, snap.FocusInsight.VerdictText)
	} else {
		b.WriteString("SCOPE: overall system state\n")
	}

	if m := snap.Metrics; m != nil {
		fmt.Fprintf(&b, "METRICS: cpu=%.1f%% ram=%.1f%%", m.CPU.TotalPercent, m.RAM.Percent)
		if m.Temperature != nil {
			fmt.Fprintf(&b, " temp=%.1fC", *m.Temperature)
		}
		for _, d := range m.Disks {
			fmt.Fprintf(&b, " disk[%s]=%.1f%%", d.Path, d.Percent)
		}
		b.WriteString("\n")
	}

	if n := len(snap.Verdicts); n > 0 {
		b.WriteString("ACTIVE INSIGHTS:\n")
		for i, v := range snap.Verdicts {
			if i >= maxVerdicts {
				fmt.Fprintf(&b, "  (+%d more)\n", n-maxVerdicts)
				break
			}
			fmt.Fprintf(&b, "  - rule_id=%s [%s] %s\n", v.RuleID, v.Severity, v.VerdictText)
		}
	}

	if n := len(snap.Containers); n > 0 {
		b.WriteString("CONTAINERS:\n")
		for i, c := range snap.Containers {
			if i >= maxItems {
				break
			}
			fmt.Fprintf(&b, "  - %s state=%s health=%s\n", c.Name, c.State, c.Health)
		}
	}

	if n := len(snap.Services); n > 0 {
		b.WriteString("SERVICES:\n")
		for i, s := range snap.Services {
			if i >= maxItems {
				break
			}
			fmt.Fprintf(&b, "  - %s active=%s health=%s\n", s.Name, s.ActiveState, s.Health)
		}
	}

	out := Scrub(b.String(), secrets)
	if len(out) > maxPromptBytes {
		out = out[:maxPromptBytes]
	}
	return out
}

// parseExplanation extracts the structured fields from the model reply. A reply
// that is not valid JSON is treated as an ungrounded free-text cause and flagged
// Unverified (FR-024).
//
// @aitri-trace FR-ID: FR-024, US-ID: US-022, AC-ID: AC-022-1h, TC-ID: TC-AI-016h
func parseExplanation(content string) *Explanation {
	raw := extractJSONObject(content)
	if raw != "" {
		var parsed struct {
			Cause        string   `json:"cause"`
			Remediation  string   `json:"remediation"`
			CitedSignals []string `json:"cited_signals"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil && strings.TrimSpace(parsed.Cause) != "" {
			return &Explanation{
				Cause:        strings.TrimSpace(parsed.Cause),
				Remediation:  strings.TrimSpace(parsed.Remediation),
				CitedSignals: parsed.CitedSignals,
				Unverified:   len(parsed.CitedSignals) == 0,
			}
		}
	}
	return &Explanation{Cause: strings.TrimSpace(content), Unverified: true}
}

// extractJSONObject returns the substring from the first '{' to the last '}',
// tolerating markdown code fences around the model's JSON.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return s[start : end+1]
}
