package metrics

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

// Reader collects system metrics.
type Reader interface {
	Read(ctx context.Context) (*Snapshot, error)
}

// prevNetCounters stores the previous network I/O counters for rate calculation.
type prevNetCounters struct {
	bytesSent uint64
	bytesRecv uint64
	timestamp time.Time
}

// SystemReader implements Reader using gopsutil.
type SystemReader struct {
	prevNet      map[string]prevNetCounters
	prevNetMu    sync.Mutex
	tempWarnOnce sync.Once
}

// NewSystemReader creates a new system metrics reader.
func NewSystemReader() *SystemReader {
	return &SystemReader{
		prevNet: make(map[string]prevNetCounters),
	}
}

// Read collects all system metrics. Individual metric failures don't stop collection.
func (r *SystemReader) Read(ctx context.Context) (*Snapshot, error) {
	now := time.Now()
	s := &Snapshot{Timestamp: now}

	r.readCPU(ctx, s)
	r.readRAM(ctx, s)
	r.readDisks(ctx, s)
	r.readNetwork(ctx, s, now)
	r.readTemperature(ctx, s)

	return s, nil
}

func (r *SystemReader) readCPU(ctx context.Context, s *Snapshot) {
	// Total CPU percent (all cores combined)
	totals, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		log.Printf("metrics: failed to read CPU total: %v", err)
		return
	}
	if len(totals) > 0 {
		s.CPU.TotalPercent = totals[0]
	}

	// Per-core
	perCore, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		log.Printf("metrics: failed to read CPU per-core: %v", err)
		return
	}
	s.CPU.PerCore = perCore
}

func (r *SystemReader) readRAM(ctx context.Context, s *Snapshot) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		log.Printf("metrics: failed to read RAM: %v", err)
		return
	}
	s.RAM = RAMMetrics{
		Total:     vm.Total,
		Used:      vm.Used,
		Available: vm.Available,
		Percent:   vm.UsedPercent,
	}
}

func (r *SystemReader) readDisks(ctx context.Context, s *Snapshot) {
	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		log.Printf("metrics: failed to read disk partitions: %v", err)
		return
	}

	for _, p := range partitions {
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			log.Printf("metrics: failed to read disk usage for %s: %v", p.Mountpoint, err)
			continue
		}
		s.Disks = append(s.Disks, DiskPartition{
			Path:    p.Mountpoint,
			Total:   usage.Total,
			Used:    usage.Used,
			Free:    usage.Free,
			Percent: usage.UsedPercent,
		})
	}
}

func (r *SystemReader) readNetwork(ctx context.Context, s *Snapshot, now time.Time) {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		log.Printf("metrics: failed to read network: %v", err)
		return
	}

	r.prevNetMu.Lock()
	defer r.prevNetMu.Unlock()

	for _, c := range counters {
		iface := NetworkIface{Name: c.Name}

		if prev, ok := r.prevNet[c.Name]; ok {
			elapsed := now.Sub(prev.timestamp).Seconds()
			if elapsed > 0 {
				iface.BytesSentPS = uint64(float64(c.BytesSent-prev.bytesSent) / elapsed)
				iface.BytesRecvPS = uint64(float64(c.BytesRecv-prev.bytesRecv) / elapsed)
			}
		}
		// First reading: rates stay 0

		r.prevNet[c.Name] = prevNetCounters{
			bytesSent: c.BytesSent,
			bytesRecv: c.BytesRecv,
			timestamp: now,
		}

		s.Networks = append(s.Networks, iface)
	}
}

func (r *SystemReader) readTemperature(_ context.Context, s *Snapshot) {
	temps, err := sensors.SensorsTemperatures()
	if err == nil {
		for _, t := range temps {
			if t.Temperature > 0 {
				temp := t.Temperature
				s.Temperature = &temp
				return
			}
		}
	}

	// Fallback: read /sys/class/thermal (Linux ARM)
	temp, err := readThermalZone()
	if err == nil {
		s.Temperature = &temp
		return
	}

	// Sensor unavailable — log once, leave Temperature nil
	r.tempWarnOnce.Do(func() {
		log.Println("metrics: temperature sensor not available, reporting null")
	})
}

// readThermalZone reads CPU temperature from sysfs (Linux).
func readThermalZone() (float64, error) {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, err
	}
	raw := strings.TrimSpace(string(data))
	milliC, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return milliC / 1000.0, nil
}
