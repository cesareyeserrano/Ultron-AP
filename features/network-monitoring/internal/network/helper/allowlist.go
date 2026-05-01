// Package helper validates command requests destined for the privileged
// helper socket against a closed allow-list of (binary, flag-set) tuples.
//
// @aitri-trace FR-ID: FR-016, FR-022
package helper

import (
	"fmt"
	"strings"
)

// Request is one helper RPC call: the binary to exec and its argv.
type Request struct {
	Bin  string
	Args []string
}

// Decision is the validator output. Allowed=true ↔ Reason="". Reason values
// are machine-readable: "binary_not_allowlisted" or "disallowed_flag:-X".
type Decision struct {
	Allowed bool
	Reason  string
}

// allowList maps each permitted binary to the set of flags accepted in its
// argv. Positional values (`-c 3`, `-m 30`) only require the flag to be
// allowed; the value itself is not validated here — callers must constrain
// it (BuildTracerouteArgs is one such caller for traceroute).
var allowList = map[string]map[string]bool{
	"ping": {
		"-c": true, "-W": true, "-i": true, "-w": true, "-n": true,
	},
	"traceroute": {
		"-n": true, "-m": true,
	},
	"librespeed-cli": {
		"--json": true, "--simple": true,
	},
}

// Validate returns Decision{Allowed:true} only when req.Bin is in the
// allow-list AND every token in req.Args that starts with '-' is in the
// flag set permitted for that binary.
//
// @aitri-trace FR-ID: FR-016, TC-ID: TC-NM-023h, TC-NM-023f, TC-NM-023e
func Validate(req Request) Decision {
	flags, ok := allowList[req.Bin]
	if !ok {
		return Decision{Reason: "binary_not_allowlisted"}
	}
	for _, a := range req.Args {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		if !flags[a] {
			return Decision{Reason: fmt.Sprintf("disallowed_flag:%s", a)}
		}
	}
	return Decision{Allowed: true}
}
