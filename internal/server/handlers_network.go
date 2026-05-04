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

// networkHistoryWindow controls how many recent samples are charted on
// /network. At 5s probe cadence, 720 samples ≈ 60 minutes per target.
const networkHistoryWindow = 720

// networkEventTail caps how many WAN up/down rows are listed on /network.
const networkEventTail = 20

// NetworkPageData is the data passed to the /network template.
type NetworkPageData struct {
	WAN     *wanmonitor.Snapshot
	Targets []NetworkTargetView
	Events  []database.NetEvent
}

// NetworkTargetView is one rendered target panel on /network.
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

	if s.gateway != nil {
		snaps := s.gateway.Snapshots()
		out.Targets = make([]NetworkTargetView, 0, len(snaps))
		for _, snap := range snaps {
			view := NetworkTargetView{Snapshot: snap}
			if snap.Target != "" {
				rows, err := s.db.RecentNetSamples(snap.Target, networkHistoryWindow)
				if err != nil {
					log.Printf("network: recent samples for %s failed: %v", snap.Target, err)
				}
				view.Series, view.MinMs, view.MaxMs, view.AvgMs = computeRTTSeries(rows)
				view.HasData = len(view.Series) > 0
			}
			out.Targets = append(out.Targets, view)
		}
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

