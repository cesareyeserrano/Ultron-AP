// Package arp parses /proc/net/arp and pairs responding IPs with MAC addresses.
//
// /proc/net/arp format (header line, then one row per neighbour):
//
//	IP address       HW type  Flags    HW address          Mask     Device
//	192.168.1.1      0x1      0x2      a1:b2:c3:d4:e5:f6   *        eth0
//
// Flag values of interest:
//
//	0x0  ATF_NONE     entry is incomplete / stale — MAC is not trustworthy
//	0x2  ATF_COM      complete / reachable — MAC is current
//
// @aitri-trace FR-032 US-032 AC-032-001 AC-032-002 TC-LD-003h TC-LD-003f TC-LD-003e
package arp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrARPUnavailable is returned (in the err result) when /proc/net/arp cannot
// be read. Pairs still come back with mac="" so the caller can degrade.
var ErrARPUnavailable = errors.New("arp cache unavailable")

// Pair links a responder IP with its current MAC and a status flag.
type Pair struct {
	IP     string // canonical IPv4 dotted-quad
	MAC    string // lower-hex with colons; empty when unknown
	Status string // "ok" | "no-arp"
}

// ReadCache parses arpPath and returns the map of ip → mac for entries with
// flag 0x2 (REACHABLE/complete). Returns ErrARPUnavailable wrapped if the
// file can't be opened.
func ReadCache(arpPath string) (map[string]string, error) {
	f, err := os.Open(arpPath)
	if err != nil {
		return map[string]string{}, fmt.Errorf("%w: %v", ErrARPUnavailable, err)
	}
	defer f.Close()
	return parseARP(f)
}

func parseARP(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	scanner := bufio.NewScanner(r)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ip := fields[0]
		flags := fields[2]
		mac := strings.ToLower(fields[3])
		if flags != "0x2" {
			continue
		}
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		out[ip] = mac
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// PairResponders takes the set of IPs that responded to the sweep and the
// current ARP cache map, and returns one Pair per responder. IPs without an
// ARP entry get MAC="" and Status="no-arp"; IPs with an entry get MAC
// populated and "ok".
//
// The cache parameter is the result of ReadCache; pass an empty map plus
// ErrARPUnavailable wrapped err to record degraded mode.
func PairResponders(responders []string, cache map[string]string, cacheErr error) []Pair {
	pairs := make([]Pair, 0, len(responders))
	degraded := errors.Is(cacheErr, ErrARPUnavailable)
	for _, ip := range responders {
		if degraded {
			pairs = append(pairs, Pair{IP: ip, MAC: "", Status: "no-arp"})
			continue
		}
		mac, ok := cache[ip]
		if !ok {
			pairs = append(pairs, Pair{IP: ip, MAC: "", Status: "no-arp"})
			continue
		}
		pairs = append(pairs, Pair{IP: ip, MAC: mac, Status: "ok"})
	}
	return pairs
}
