// Package subnet resolves the local /24 to sweep from the kernel routing table
// and the chosen interface's IPv4 address.
//
// @aitri-trace FR-030 US-030 AC-030-001 AC-030-002 TC-LD-001h TC-LD-001f TC-LD-001e
package subnet

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

// Status reflects what Detect was able to determine.
type Status string

const (
	StatusOK             Status = "ok"
	StatusNoDefaultRoute Status = "no-default-route"
	StatusNoIPv4         Status = "no-ipv4"
	StatusClamped        Status = "subnet-clamped"
)

// Subnet is the resolved /24 (or narrower) plus the interface that produced it.
type Subnet struct {
	CIDR   string // e.g. "192.168.1.0/24"; empty when Status != ok and != clamped
	Iface  string // e.g. "eth0"; empty when no default route
	HostIP string // the local host's IPv4 on the chosen interface
	Status Status
}

// IfaceResolver returns the IPv4 addresses configured on a named interface.
// Tests inject a fake; production wires net.InterfaceByName.
type IfaceResolver func(name string) ([]net.Addr, error)

// DefaultIfaceResolver wires net.InterfaceByName.Addrs.
func DefaultIfaceResolver(name string) ([]net.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return iface.Addrs()
}

// Detect parses /proc/net/route at routePath and uses resolver to find the
// IPv4 of the default-route interface, then returns the /24 containing it.
// A /16 (or wider) interface is clamped to the host's /24.
func Detect(routePath string, resolver IfaceResolver) (Subnet, error) {
	iface, err := defaultRouteIface(routePath)
	if err != nil {
		return Subnet{Status: StatusNoDefaultRoute}, nil
	}
	addrs, err := resolver(iface)
	if err != nil {
		return Subnet{Iface: iface, Status: StatusNoIPv4}, fmt.Errorf("resolve iface %q: %w", iface, err)
	}
	hostIP, ipnet, ok := pickIPv4(addrs)
	if !ok {
		return Subnet{Iface: iface, Status: StatusNoIPv4}, nil
	}
	cidr, clamped := clampToSlash24(hostIP, ipnet)
	out := Subnet{
		CIDR:   cidr,
		Iface:  iface,
		HostIP: hostIP.String(),
		Status: StatusOK,
	}
	if clamped {
		out.Status = StatusClamped
	}
	return out, nil
}

// defaultRouteIface scans /proc/net/route for the interface owning the default
// route (destination 00000000). Returns "" if none found.
func defaultRouteIface(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return scanRouteForDefault(f)
}

func scanRouteForDefault(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == "00000000" {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no default route")
}

func pickIPv4(addrs []net.Addr) (net.IP, *net.IPNet, bool) {
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		v4 := ipnet.IP.To4()
		if v4 == nil {
			continue
		}
		return v4, ipnet, true
	}
	return nil, nil, false
}

// clampToSlash24 returns the /24 containing hostIP. If the original mask is
// /24 or narrower, that's the natural CIDR. If wider (/23, /16, /8...), clamp
// to the /24 around the host. Returns (cidr, wasClamped).
func clampToSlash24(hostIP net.IP, ipnet *net.IPNet) (string, bool) {
	hostV4 := hostIP.To4()
	if hostV4 == nil {
		return "", false
	}
	prefixLen, _ := ipnet.Mask.Size()
	if prefixLen >= 24 {
		network := hostV4.Mask(ipnet.Mask).String()
		return fmt.Sprintf("%s/%d", network, prefixLen), false
	}
	clamped := net.IPv4(hostV4[0], hostV4[1], hostV4[2], 0)
	return fmt.Sprintf("%s/24", clamped.String()), true
}
