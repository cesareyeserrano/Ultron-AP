// Module:       internal/dockerapi (models)
// Purpose:      Narrow decode targets for the Docker Engine API. Each struct
//
//	is a deliberate subset: only fields the panel actually renders
//	are decoded, so a change in an unrelated part of the daemon's
//	response cannot break us, and a hostile response has a smaller
//	surface to work with.
//
// Dependencies: standard library only.
//
// @aitri-trace FR-089, FR-090
package dockerapi

// Container is one entry of GET /containers/json?all=1.
type Container struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
}

// StatsSnapshot is the subset of GET /containers/{id}/stats?stream=false
// needed to compute CPU and memory percentages.
type StatsSnapshot struct {
	CPUStats    CPUStats    `json:"cpu_stats"`
	PreCPUStats CPUStats    `json:"precpu_stats"`
	MemoryStats MemoryStats `json:"memory_stats"`
}

// CPUStats carries the cumulative counters a CPU percentage is derived from.
type CPUStats struct {
	CPUUsage struct {
		TotalUsage uint64 `json:"total_usage"`
	} `json:"cpu_usage"`
	SystemUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs  uint32 `json:"online_cpus"`
}

// MemoryStats carries the memory counters the panel displays.
type MemoryStats struct {
	Usage uint64 `json:"usage"`
	Limit uint64 `json:"limit"`
}

// Inspect is the subset of GET /containers/{id}/json the detail view needs.
//
// Config.Env is decoded here but its VALUES never leave the helper: the
// dispatch splits each entry on the first '=' and keeps only the left side.
// See AC-090-001 and TestTC_DVH_021f.
type Inspect struct {
	NetworkSettings struct {
		Ports map[string][]PortBinding `json:"Ports"`
	} `json:"NetworkSettings"`
	Mounts []Mount `json:"Mounts"`
	Config struct {
		Env []string `json:"Env"`
	} `json:"Config"`
}

// PortBinding is one host-side binding of a published container port.
type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// Mount is one volume or bind mount attached to a container.
type Mount struct {
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Mode        string `json:"Mode"`
}

// EnvNames projects Docker's "KEY=VALUE" environment slice down to the keys
// alone. This is the single chokepoint where env values are dropped, and it is
// applied inside the helper, so a value never reaches the unprivileged
// process even if a template later changed.
//
// Params:
//   - env: raw entries as the daemon reports them.
//
// Returns the names in their original order. An entry with no '=' is kept
// whole (it is already a bare name).
//
// @aitri-trace FR-090, AC-090-001, TC-DVH-021f
func EnvNames(env []string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for _, e := range env {
		if i := indexByte(e, '='); i >= 0 {
			out = append(out, e[:i])
			continue
		}
		out = append(out, e)
	}
	return out
}

// ProtoOf extracts the protocol from a Docker port key such as "8123/tcp".
// Returns "tcp" when the key carries no suffix, which is Docker's own default.
func ProtoOf(portKey string) string {
	if i := indexByte(portKey, '/'); i >= 0 && i+1 < len(portKey) {
		return portKey[i+1:]
	}
	return "tcp"
}

// indexByte is strings.IndexByte, inlined to keep this file dependency-free.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// CPUPercent computes the container's CPU usage as a percentage of one core
// times the number of online CPUs, from the cumulative counters in two
// consecutive samples the daemon returns inside a single stats document.
//
// Returns 0 when either delta is non-positive — that is the normal reading for
// a container that has just started and has no previous sample yet, not an
// error.
func (s *StatsSnapshot) CPUPercent() float64 {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(s.CPUStats.SystemUsage - s.PreCPUStats.SystemUsage)
	if systemDelta <= 0 || cpuDelta <= 0 {
		return 0
	}
	cpus := float64(s.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = 1
	}
	return (cpuDelta / systemDelta) * cpus * 100.0
}

// MemPercent computes memory usage as a percentage of the container's limit.
// Returns 0 when no limit is reported, rather than dividing by zero.
func (s *StatsSnapshot) MemPercent() float64 {
	if s.MemoryStats.Limit == 0 {
		return 0
	}
	return float64(s.MemoryStats.Usage) / float64(s.MemoryStats.Limit) * 100.0
}
