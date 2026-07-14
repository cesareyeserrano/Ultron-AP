package server

import (
	"log"
	"math"
	"net/http"
	"sort"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/network/wanmonitor"
)

// networkEventTail caps how many WAN up/down rows are listed on /network.
const networkEventTail = 20

// NetworkPageData is the data passed to the /network template.
type NetworkPageData struct {
	WAN    *wanmonitor.Snapshot
	Events []database.NetEvent
}

// NetworkTargetView is one rendered target sparkline tile (used by the
// dashboard charts area).
type NetworkTargetView struct {
	Snapshot *gatewayprobe.Snapshot // current state (live)
	Series   []float64              // last N RTT samples in ms (NaN = failed)
	HasData  bool
	MinMs    float64
	MaxMs    float64
	AvgMs    float64
}

func (s *Server) handleNetworkPage(w http.ResponseWriter, r *http.Request) {
	dd := s.gatherNetworkPageData()
	s.render(w, r, "network.html", "Network", "network", dd)
}

func (s *Server) gatherNetworkPageData() NetworkPageData {
	out := NetworkPageData{}

	if s.wan != nil {
		snap := s.wan.Snapshot()
		out.WAN = &snap
	}

	if s.db != nil {
		events, err := s.db.RecentNetEvents(networkEventTail)
		if err != nil {
			log.Printf("network: recent events failed: %v", err)
		} else {
			out.Events = events
		}
	}

	return out
}

// gatherNetworkTargetViews builds per-target sparkline panels for the
// dashboard charts area. historyPoints sizes the RTT history window to
// match the dashboard timeline selector (probes share the 5s cadence).
func (s *Server) gatherNetworkTargetViews(historyPoints int) []NetworkTargetView {
	if s.gateway == nil {
		return nil
	}
	if historyPoints < 12 {
		historyPoints = 12
	}
	snaps := s.gateway.Snapshots()
	views := make([]NetworkTargetView, 0, len(snaps))
	for _, snap := range snaps {
		view := NetworkTargetView{Snapshot: snap}
		if snap.Target != "" && s.db != nil {
			rows, err := s.db.RecentNetSamples(snap.Target, historyPoints)
			if err != nil {
				log.Printf("network: recent samples for %s failed: %v", snap.Target, err)
			}
			view.Series, view.MinMs, view.MaxMs, view.AvgMs = computeRTTSeries(rows)
			view.HasData = len(view.Series) > 0
		}
		views = append(views, view)
	}
	return views
}

// computeRTTSeries reverses the (newest-first) DB rows into time-ascending
// order suitable for sparklineSVG, and returns min / max / mean across
// successful samples only. Failed samples are emitted as NaN so the
// sparkline helper renders gaps rather than zero values that distort scale.
func computeRTTSeries(rows []database.NetSample) (series []float64, minMs, maxMs, avgMs float64) {
	if len(rows) == 0 {
		return nil, 0, 0, 0
	}
	// rows are newest-first (ts DESC); reverse for chart left-to-right.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].TS.Before(rows[j].TS) })

	series = make([]float64, len(rows))
	hasValid := false
	minMs = math.Inf(1)
	maxMs = math.Inf(-1)
	var sum float64
	var count int
	for i, r := range rows {
		if r.RTTMs == nil {
			series[i] = math.NaN()
			continue
		}
		v := *r.RTTMs
		series[i] = v
		hasValid = true
		if v < minMs {
			minMs = v
		}
		if v > maxMs {
			maxMs = v
		}
		sum += v
		count++
	}
	if !hasValid {
		return series, 0, 0, 0
	}
	avgMs = sum / float64(count)
	return series, minMs, maxMs, avgMs
}
