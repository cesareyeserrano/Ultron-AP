package lan

import (
	"go/build"
	"strings"
	"testing"
)

// forbiddenImports lists packages that LAN discovery MUST NOT pull in.
// Listed in the spec security design as a hard package boundary: discovery
// is passive (ARP + mDNS) and never opens raw packet capture.
var forbiddenImports = []string{
	"github.com/google/gopacket",
	"github.com/google/gopacket/pcap",
	"github.com/packetcap/go-pcap",
	"github.com/google/gopacket/pcapgo",
}

// TestTC_NM_012e_NoPcapImports enforces the security boundary: this package
// must never import gopacket or any pcap library, transitively or directly,
// at the immediate import level. Stronger transitive checks belong in CI.
//
// @aitri-tc TC-NM-012e
func TestTC_NM_012e_NoPcapImports(t *testing.T) {
	t.Parallel()
	pkg, err := build.Default.ImportDir(".", build.ImportComment)
	if err != nil {
		t.Fatalf("ImportDir(.): %v", err)
	}
	all := append([]string{}, pkg.Imports...)
	all = append(all, pkg.TestImports...)
	for _, imp := range all {
		for _, banned := range forbiddenImports {
			if imp == banned || strings.HasPrefix(imp, banned+"/") {
				t.Errorf("forbidden import %q in package boundary (FR-027 passive-only)", imp)
			}
		}
	}
}
