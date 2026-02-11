package metrics

import "time"

// Snapshot captures all system metrics at a single point in time.
type Snapshot struct {
	Timestamp   time.Time       `json:"timestamp"`
	CPU         CPUMetrics      `json:"cpu"`
	RAM         RAMMetrics      `json:"ram"`
	Disks       []DiskPartition `json:"disks"`
	Networks    []NetworkIface  `json:"networks"`
	Temperature *float64        `json:"temperature"` // nil if sensor unavailable
}

// CPUMetrics holds CPU usage percentages.
type CPUMetrics struct {
	TotalPercent float64   `json:"total_percent"`
	PerCore      []float64 `json:"per_core"`
}

// RAMMetrics holds memory usage data.
type RAMMetrics struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Available uint64  `json:"available"`
	Percent   float64 `json:"percent"`
}

// DiskPartition holds usage data for a single mounted partition.
type DiskPartition struct {
	Path    string  `json:"path"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

// NetworkIface holds network I/O rates for a single interface.
type NetworkIface struct {
	Name        string `json:"name"`
	BytesSentPS uint64 `json:"bytes_sent_ps"` // bytes per second sent
	BytesRecvPS uint64 `json:"bytes_recv_ps"` // bytes per second received
}
