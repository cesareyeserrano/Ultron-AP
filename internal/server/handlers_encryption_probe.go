// @aitri-trace FR-068, NFR-032 — encryption-key reference probe.
//
// Resolves a backup encryption-key reference (env|file|kms) and returns
// {ok, reason} with NO key bytes, length, or hash leaked. Auth-gated.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const probeReasonMaxLen = 120

// Locked enum of probe-reason strings. Adding a new reason requires updating
// this list AND its corresponding test (TC-SR-068sec1).
var probeReasonEnum = map[string]bool{
	"file readable":                   true,
	"file not found":                  true,
	"file not readable":               true,
	"env var not set":                 true,
	"kms scheme not supported in v1":  true,
	"scheme required":                 true,
	"value required":                  true,
}

// envFoundReason is generated dynamically — listed separately because the
// env var name is part of the response. The variable name itself comes from
// user input, but is sanitised (uppercase letters / digits / underscore only)
// before being embedded.
func envFoundReason(name string) string {
	return fmt.Sprintf("env var %s found", name)
}

// probeResult is the canonical response shape. Keys MUST stay {ok, reason}.
type probeResult struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

// handleEncryptionKeyProbe handles GET /api/settings/encryption-key/probe.
// Query: ?scheme=env|file|kms&value=<string>
func (s *Server) handleEncryptionKeyProbe(w http.ResponseWriter, r *http.Request) {
	scheme := strings.TrimSpace(r.URL.Query().Get("scheme"))
	value := strings.TrimSpace(r.URL.Query().Get("value"))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if scheme == "" {
		writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "scheme required"})
		return
	}

	switch scheme {
	case "env":
		probeEnv(w, value)
	case "file":
		probeFile(w, value)
	case "kms":
		writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "kms scheme not supported in v1"})
	default:
		// Unknown scheme — keep the response shape; do not echo the scheme.
		writeProbe(w, http.StatusBadRequest, probeResult{OK: false, Reason: "scheme required"})
	}
}

func probeEnv(w http.ResponseWriter, name string) {
	if name == "" {
		writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "value required"})
		return
	}
	if !isSafeEnvName(name) {
		// Reject silently with the canonical "env var not set" — do NOT echo
		// the (potentially attacker-controlled) raw name in the response.
		writeProbe(w, http.StatusBadRequest, probeResult{OK: false, Reason: "env var not set"})
		return
	}
	if _, ok := os.LookupEnv(name); !ok {
		writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "env var not set"})
		return
	}
	writeProbe(w, http.StatusOK, probeResult{OK: true, Reason: envFoundReason(name)})
}

// isSafeEnvName allows uppercase letters, digits, and underscore — the
// POSIX env-var name conventions, which prevents injection of garbage into
// the response body or the log.
func isSafeEnvName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

func probeFile(w http.ResponseWriter, raw string) {
	if raw == "" {
		writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "value required"})
		return
	}
	if strings.ContainsRune(raw, 0) {
		writeProbe(w, http.StatusBadRequest, probeResult{OK: false, Reason: "file not found"})
		return
	}
	if !filepath.IsAbs(raw) {
		writeProbe(w, http.StatusBadRequest, probeResult{OK: false, Reason: "file not found"})
		return
	}
	cleaned := filepath.Clean(raw)
	if cleaned != raw || strings.Contains(raw, "..") {
		// Path traversal defence — reject anything that is not lexically clean
		// or that contains ".." components.
		writeProbe(w, http.StatusBadRequest, probeResult{OK: false, Reason: "file not found"})
		return
	}
	st, err := os.Stat(cleaned)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "file not readable"})
			return
		}
		writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "file not found"})
		return
	}
	// Only regular files qualify. Rejecting non-regular modes (directories,
	// FIFOs, devices, sockets) also closes an O_RDONLY-on-a-FIFO hang: opening
	// a named pipe for read blocks until a writer appears, which would pin the
	// handler goroutine indefinitely (M8). O_NONBLOCK is belt-and-braces
	// against a TOCTOU swap between Stat and Open.
	if !st.Mode().IsRegular() {
		writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "file not readable"})
		return
	}
	// Verify readable by opening O_RDONLY|O_NONBLOCK and immediately closing —
	// DOES NOT read content (no key material is ever read here).
	f, err := os.OpenFile(cleaned, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		writeProbe(w, http.StatusOK, probeResult{OK: false, Reason: "file not readable"})
		return
	}
	_ = f.Close()
	writeProbe(w, http.StatusOK, probeResult{OK: true, Reason: "file readable"})
}

func writeProbe(w http.ResponseWriter, status int, result probeResult) {
	if len(result.Reason) > probeReasonMaxLen {
		result.Reason = result.Reason[:probeReasonMaxLen]
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}
