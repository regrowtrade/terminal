// Package reconcile turns the read-only user data stream into a bounded,
// deduplicated summary layered on the authoritative REST snapshot. It never
// produces a "live" merged account, infers fills from absence or enables
// execution. The snapshot remains the source of truth; events only annotate it.
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

// Fill is a single TRADE execution observed in the stream, deduplicated by
// (orderId, tradeId). It is not a reconciled ledger: events missed during a
// disconnection are never replayed by the exchange.
type Fill struct {
	OrderID         string `json:"orderId"`
	TradeID         string `json:"tradeId"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	PositionSide    string `json:"positionSide"`
	Price           string `json:"price"`
	Quantity        string `json:"quantity"`
	Commission      string `json:"commission"`
	CommissionAsset string `json:"commissionAsset"`
	EventTime       int64  `json:"eventTime"`
	TransactionTime int64  `json:"transactionTime"`
}

type Report struct {
	Continuity         string    `json:"continuity"`
	ExecutionReady     bool      `json:"executionReady"`
	SnapshotCapturedAt time.Time `json:"snapshotCapturedAt"`
	EventsTruncated    bool      `json:"eventsTruncated"`
	TotalEvents        int       `json:"totalEvents"`
	OrderUpdates       int       `json:"orderUpdates"`
	AccountUpdates     int       `json:"accountUpdates"`
	LastEventTime      int64     `json:"lastEventTime"`
	TotalFills         int       `json:"totalFills"`
	FillLimit          int       `json:"fillLimit"`
	FillsTruncated     bool      `json:"fillsTruncated"`
	Fills              []Fill    `json:"fills"`
}

// Reconcile summarizes the normalized, deduplicated event stream relative to a
// snapshot. connected reports whether the stream has stayed connected since the
// snapshot was saved; when false the report is gapped and fills may be missing.
func Reconcile(snapshot binance.Snapshot, events []binance.UserEvent, connected bool) Report {
	report := Report{
		Continuity:         ContinuityGapped,
		ExecutionReady:     false,
		SnapshotCapturedAt: snapshot.CapturedAt,
		FillLimit:          FillLimit,
		Fills:              []Fill{},
	}
	if connected {
		report.Continuity = ContinuityEstablished
	}
	if len(events) == 0 {
		return report
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
	fillSeen := make(map[string]bool)
	for _, event := range sorted {
		report.TotalEvents++
		if event.EventTime > report.LastEventTime {
			report.LastEventTime = event.EventTime
		}
		switch event.Type {
		case binance.EventOrderTradeUpdate:
			report.OrderUpdates++
			o := event.OrderUpdate
			if o == nil || o.ExecutionType != "TRADE" || o.TradeID == "0" {
				continue
			}
			key := o.OrderID + ":" + o.TradeID
			if fillSeen[key] {
				continue
			}
			fillSeen[key] = true
			report.TotalFills++
			if len(report.Fills) >= FillLimit {
				report.FillsTruncated = true
				continue
			}
			report.Fills = append(report.Fills, Fill{
				OrderID: o.OrderID, TradeID: o.TradeID, Symbol: o.Symbol,
				Side: o.Side, PositionSide: o.PositionSide, Price: o.LastFilledPrice,
				Quantity: o.LastFilledQty, Commission: o.Commission, CommissionAsset: o.CommissionAsset,
				EventTime: event.EventTime, TransactionTime: event.TransactionTime,
			})
		case binance.EventAccountUpdate:
			report.AccountUpdates++
		}
	}
	return report
}
