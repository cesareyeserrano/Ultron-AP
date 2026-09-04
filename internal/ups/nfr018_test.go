package ups

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TC-UPS-040h (NFR-018): the UPS package references no shutdown/load.off path —
// the Client interface is read-only and the package imports nothing that could
// execute a command (os/exec, privileged helper).
func TestTC_UPS_040h_NoShutdownPath(t *testing.T) {
	// @aitri-tc TC-UPS-040h
	it := reflect.TypeOf((*Client)(nil)).Elem()
	for i := 0; i < it.NumMethod(); i++ {
		if name := it.Method(i).Name; name != "List" && name != "Close" {
			t.Errorf("Client exposes non-read method %q — a write/command surface is forbidden", name)
		}
	}
	assertNoForbiddenImports(t)
}

// TC-UPS-041e (NFR-018): across every UPS state, the client issues only read
// commands — never a SET/INSTCMD/shutdown/load.off.
func TestTC_UPS_041e_ReadOnlyAcrossStates(t *testing.T) {
	// @aitri-tc TC-UPS-041e
	states := []string{"OL", "OB", "LB", "RB", "BYPASS", "OFF", "ALARM"}
	for _, st := range states {
		f := startFakeUpsd(t, map[string]string{"ups.status": st, "battery.voltage": "24.0"})
		c := NewClient(Config{Addr: f.addr, UPSName: "powest", User: "ultron", Pass: "x"})
		if _, err := c.List(context.Background()); err != nil {
			t.Fatalf("state %s: List: %v", st, err)
		}
		for _, cmd := range f.cmds() {
			u := strings.ToUpper(cmd)
			for _, forbidden := range []string{"SET ", "INSTCMD", "FSD", "LOAD.OFF", "SHUTDOWN"} {
				if strings.Contains(u, forbidden) {
					t.Errorf("state %s: forbidden command %q in %q", st, forbidden, cmd)
				}
			}
		}
	}
}
