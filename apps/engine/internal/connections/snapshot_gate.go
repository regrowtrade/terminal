package connections

import (
	"sync"

	"regrow/engine/internal/binance"
)

// SnapshotGate serializes a REST snapshot read with the live read-only event
// stream for a single account. While a read is in flight the monitor buffers
// validated events instead of journaling them immediately; when the last
// concurrent read commits, the buffer is drained and journaled through the
// monitor's sink. It only coordinates within this process: durability and
// ordering come from the append-only journal itself, and an event's exchange
// time (not its arrival order) is what later reconciliation uses as boundary.
type SnapshotGate struct {
	mu       sync.Mutex
	accounts map[string]*accountGate
}

type accountGate struct {
	readers int
	pending []binance.UserEvent
	journal func([]binance.UserEvent)
}

// NewSnapshotGate returns an empty gate. It is safe for concurrent use and
// optional: with a nil gate the monitor journals immediately and a snapshot
// read does not buffer events.
func NewSnapshotGate() *SnapshotGate {
	return &SnapshotGate{accounts: make(map[string]*accountGate)}
}

func (g *SnapshotGate) account(account string) *accountGate {
	a := g.accounts[account]
	if a == nil {
		a = &accountGate{}
		g.accounts[account] = a
	}
	return a
}

// setJournal registers the journaling sink for an account. The monitor calls it
// once before streaming starts; buffered events are journaled through it on
// commit, and events outside a read are journaled through it immediately.
func (g *SnapshotGate) setJournal(account string, fn func([]binance.UserEvent)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.account(account).journal = fn
}

// begin marks a snapshot read in progress and returns a commit function that
// ends it. Buffered events are drained (and journaled) only when the last
// concurrent read commits, so events that straddle the boundary are never lost.
func (g *SnapshotGate) begin(account string) func() {
	g.mu.Lock()
	a := g.account(account)
	a.readers++
	g.mu.Unlock()
	return func() {
		g.mu.Lock()
		a.readers--
		var flushed []binance.UserEvent
		if a.readers == 0 {
			flushed = a.pending
			a.pending = nil
		}
		journal := a.journal
		g.mu.Unlock()
		if len(flushed) > 0 && journal != nil {
			journal(flushed)
		}
	}
}

// ingest is called by the monitor for each validated event. It returns true
// when the gate handled the event (buffered while a read is in flight, or
// journaled immediately). It returns false only before a journal sink is
// registered, in which case the caller must journal directly so no event is
// dropped.
func (g *SnapshotGate) ingest(account string, event binance.UserEvent) bool {
	g.mu.Lock()
	a := g.accounts[account]
	if a != nil && a.readers > 0 {
		a.pending = append(a.pending, event)
		g.mu.Unlock()
		return true
	}
	var journal func([]binance.UserEvent)
	if a != nil {
		journal = a.journal
	}
	g.mu.Unlock()
	if journal == nil {
		return false
	}
	journal([]binance.UserEvent{event})
	return true
}
