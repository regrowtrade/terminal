// Package reconcile turns the read-only user data stream into a bounded,
// deduplicated summary layered on the authoritative REST snapshot. It never
// produces a "live" merged account, infers fills from absence or enables
// execution. The snapshot remains the source of truth; events and the durable
// fill table only annotate it.
package reconcile

import (
	"sort"
	"time"

	"regrow/engine/internal/binance"
)

const (
	ContinuityEstablished = "established"
	ContinuityGapped      = "gapped"
	FillLimit             = 200
)

type Report struct {
	Continuity         string         `json:"continuity"`
	ExecutionReady     bool           `json:"executionReady"`
	SnapshotCapturedAt time.Time      `json:"snapshotCapturedAt"`
	EventsTruncated    bool           `json:"eventsTruncated"`
	TotalEvents        int            `json:"totalEvents"`
	OrderUpdates       int            `json:"orderUpdates"`
	AccountUpdates     int            `json:"accountUpdates"`
	LastEventTime      int64          `json:"lastEventTime"`
	TotalFills         int            `json:"totalFills"`
	FillLimit          int            `json:"fillLimit"`
	FillsTruncated     bool           `json:"fillsTruncated"`
	Fills              []binance.Fill `json:"fills"`
}

// Reconcile summarizes the post-cutoff event stream and the durable,
// deduplicated fills layered on a snapshot. fills is already bounded and
// deduplicated by (orderId, tradeId); fillTotal is the complete count and
// fillsTruncated marks whether the bounded list dropped any. connected reports
// whether the stream has stayed connected since the snapshot was saved; when
// false the report is gapped and fills may be missing.
func Reconcile(snapshot binance.Snapshot, events []binance.UserEvent, fills []binance.Fill, fillTotal int, fillsTruncated bool, connected bool) Report {
	report := Report{
		Continuity:         ContinuityGapped,
		ExecutionReady:     false,
		SnapshotCapturedAt: snapshot.CapturedAt,
		FillLimit:          FillLimit,
		TotalFills:         fillTotal,
		FillsTruncated:     fillsTruncated,
		Fills:              []binance.Fill{},
	}
	if connected {
		report.Continuity = ContinuityEstablished
	}
	sorted := make([]binance.UserEvent, 0, len(events))
	seen := make(map[string]bool, len(events))
	for _, event := range events {
		if !event.Valid() {
			continue
		}
		key := event.DedupKey()
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		sorted = append(sorted, event)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].EventTime != sorted[j].EventTime {
			return sorted[i].EventTime < sorted[j].EventTime
		}
		return sorted[i].TransactionTime < sorted[j].TransactionTime
	})
	for _, event := range sorted {
		report.TotalEvents++
		if event.EventTime > report.LastEventTime {
			report.LastEventTime = event.EventTime
		}
		switch event.Type {
		case binance.EventOrderTradeUpdate:
			report.OrderUpdates++
		case binance.EventAccountUpdate:
			report.AccountUpdates++
		}
	}
	if fills == nil {
		fills = []binance.Fill{}
	}
	if len(fills) > FillLimit {
		fills = fills[:FillLimit]
		report.FillsTruncated = true
	}
	report.Fills = fills
	return report
}
