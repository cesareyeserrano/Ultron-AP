// LAN-devices feature HTTP surface.
//
// @aitri-trace FR-036 FR-037 US-036 US-037 TC-LD-007h TC-LD-007f TC-LD-007e TC-LD-008h TC-LD-008f TC-LD-008e
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices"
	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices/store"
)

// lanDeviceJSON is the wire format for GET /api/network/lan-devices.
// Timestamps are emitted as ISO 8601 UTC strings (FR-036 AC-002).
type lanDeviceJSON struct {
	IP           string `json:"ip"`
	MAC          string `json:"mac"`
	Vendor       string `json:"vendor"`
	Online       bool   `json:"online"`
	FirstSeen    string `json:"first_seen"`
	LastSeen     string `json:"last_seen"`
	MissedSweeps int    `json:"missed_sweeps"`
}

// lanDevicesStatusJSON is the wire format for GET /api/network/lan-devices/status.
// Durations are emitted as integer milliseconds (the orchestrator's internal
// time.Duration encodes as nanoseconds, which violates the spec's `_ms` suffix
// — convert here so the wire contract matches 02_SYSTEM_DESIGN.md).
type lanDevicesStatusJSON struct {
	Subnet              string `json:"subnet"`
	Interface           string `json:"interface"`
	SubnetStatus        string `json:"subnet_status"`
	LastSweepAt         string `json:"last_sweep_at,omitempty"`
	LastSweepDurationMS int64  `json:"last_sweep_duration_ms"`
	LastSweepResponders int    `json:"last_sweep_responders"`
	OverrunCount        int    `json:"overrun_count"`
	SelfThrottled       bool   `json:"self_throttled"`
	CurrentCadenceMS    int64  `json:"current_cadence_ms"`
	DeviceCount         int    `json:"device_count"`
	Disabled            bool   `json:"disabled"`
}

// handleLANDevicesAPI serves the JSON device list (FR-036).
func (s *Server) handleLANDevicesAPI(w http.ResponseWriter, r *http.Request) {
	if s.landevicesStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lan-devices not initialized"})
		return
	}
	devices, err := s.landevicesStore.List()
	if err != nil {
		log.Printf("lan-devices: list failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
		return
	}
	out := make([]lanDeviceJSON, 0, len(devices))
	for _, d := range devices {
		out = append(out, lanDeviceJSON{
			IP:           d.IP,
			MAC:          d.MAC,
			Vendor:       d.Vendor,
			Online:       d.Online,
			FirstSeen:    d.FirstSeen.UTC().Format(time.RFC3339),
			LastSeen:     d.LastSeen.UTC().Format(time.RFC3339),
			MissedSweeps: d.MissedSweeps,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleLANDevicesStatus serves the status envelope (FR-038, FR-030).
func (s *Server) handleLANDevicesStatus(w http.ResponseWriter, r *http.Request) {
	if s.landevices == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lan-devices not initialized"})
		return
	}
	st := s.landevices.Status()
	out := lanDevicesStatusJSON{
		Subnet:              st.Subnet,
		Interface:           st.Interface,
		SubnetStatus:        st.SubnetStatus,
		LastSweepDurationMS: st.LastSweepDuration.Milliseconds(),
		LastSweepResponders: st.LastSweepResponders,
		OverrunCount:        st.OverrunCount,
		SelfThrottled:       st.SelfThrottled,
		CurrentCadenceMS:    st.CurrentCadence.Milliseconds(),
		DeviceCount:         st.DeviceCount,
		Disabled:            st.Disabled,
	}
	if !st.LastSweepAt.IsZero() {
		out.LastSweepAt = st.LastSweepAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

// LANDevicesFragmentRow is one row passed to the lan-devices partial template.
type LANDevicesFragmentRow struct {
	IP             string
	MAC            string
	Vendor         string
	Online         bool
	LastSeenLabel  string // pre-formatted relative or absolute time
	FirstSeenLabel string // tooltip / debug
}

// LANDevicesFragmentData is the full payload for the lan-devices HTML partial.
type LANDevicesFragmentData struct {
	HasDevices       bool
	Devices          []LANDevicesFragmentRow
	EmptyMessage     string // shown when HasDevices is false
	NextSweepCountMS int64  // for the empty-state countdown ("first sweep in <N>s")
}

// handleLANDevicesFragment serves the HTMX fragment rendered into /network (FR-037).
func (s *Server) handleLANDevicesFragment(w http.ResponseWriter, r *http.Request) {
	data := s.gatherLANDevicesFragment(time.Now())
	tmpl, ok := s.tmplCache["partials/lan-devices.html"]
	if !ok {
		log.Printf("lan-devices fragment: template not in cache")
		http.Error(w, "template missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "partials/lan-devices.html", data); err != nil {
		log.Printf("lan-devices fragment: render: %v", err)
	}
}

func (s *Server) gatherLANDevicesFragment(now time.Time) LANDevicesFragmentData {
	out := LANDevicesFragmentData{}
	if s.landevicesStore == nil {
		out.EmptyMessage = "LAN devices not initialised"
		return out
	}

	devices, err := s.landevicesStore.List()
	if err != nil {
		log.Printf("lan-devices fragment: list: %v", err)
		out.EmptyMessage = "Unable to load device list"
		return out
	}

	if len(devices) == 0 {
		// Empty state — compute countdown to first sweep based on the
		// orchestrator's current cadence (FR-037 AC-002).
		out.EmptyMessage = "No devices discovered yet — first sweep in"
		if s.landevices != nil {
			cadence := s.landevices.Status().CurrentCadence
			if cadence <= 0 {
				cadence = landevices.BaseCadence
			}
			out.NextSweepCountMS = cadence.Milliseconds()
		} else {
			out.NextSweepCountMS = landevices.BaseCadence.Milliseconds()
		}
		return out
	}

	out.HasDevices = true
	out.Devices = make([]LANDevicesFragmentRow, 0, len(devices))
	for _, d := range devices {
		out.Devices = append(out.Devices, LANDevicesFragmentRow{
			IP:             d.IP,
			MAC:            d.MAC,
			Vendor:         d.Vendor,
			Online:         d.Online,
			LastSeenLabel:  formatLastSeen(now, d.LastSeen),
			FirstSeenLabel: d.FirstSeen.UTC().Format("2006-01-02 15:04:05 UTC"),
		})
	}
	return out
}

// formatLastSeen produces a relative label for entries newer than 24h
// (e.g. "5 min ago", "2 h ago") and an absolute timestamp for older
// (FR-037 AC-003).
func formatLastSeen(now, ts time.Time) string {
	if ts.IsZero() {
		return "never"
	}
	d := now.Sub(ts)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return formatPlural(int(d/time.Minute), "min")
	case d < 24*time.Hour:
		return formatPlural(int(d/time.Hour), "h")
	default:
		return ts.UTC().Format("2006-01-02 15:04 UTC")
	}
}

func formatPlural(n int, unit string) string {
	if n <= 0 {
		n = 1
	}
	return formatInt(n) + " " + unit + " ago"
}

func formatInt(n int) string {
	// tiny helper to avoid pulling fmt for hot-path string concat
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	if err := enc.Encode(body); err != nil {
		log.Printf("writeJSON: encode: %v", err)
	}
}

// SetLANDevices wires the orchestrator + store into the server. The
// orchestrator may be nil for tests that exercise only the device list.
func (s *Server) SetLANDevices(orch *landevices.Orchestrator, st *store.Store) {
	s.landevices = orch
	s.landevicesStore = st
}
