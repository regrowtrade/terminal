package connections

import (
	"sync"
	"testing"

	"regrow/engine/internal/binance"
)

func accountUpdate(eventTime int64) binance.UserEvent {
	return binance.UserEvent{
		Type: binance.EventAccountUpdate, EventTime: eventTime, TransactionTime: eventTime,
		AccountUpdate: &binance.AccountUpdate{Reason: "ORDER"},
	}
}

type journalSpy struct {
	mu     sync.Mutex
	events []binance.UserEvent
}

func (s *journalSpy) record(events []binance.UserEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
}

func (s *journalSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *journalSpy) times() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int64, len(s.events))
	for i, e := range s.events {
		out[i] = e.EventTime
	}
	return out
}

func TestGateJournalsImmediatelyOutsideRead(t *testing.T) {
	gate := NewSnapshotGate()
	spy := &journalSpy{}
	gate.setJournal("account-1", spy.record)
	if !gate.ingest("account-1", accountUpdate(1)) {
		t.Fatal("ingest refused to journal outside a read")
	}
	if spy.count() != 1 || spy.times()[0] != 1 {
		t.Fatal("event was not journaled immediately")
	}
}

func TestGateBuffersDuringReadAndFlushesOnCommit(t *testing.T) {
	gate := NewSnapshotGate()
	spy := &journalSpy{}
	gate.setJournal("account-1", spy.record)
	commit := gate.begin("account-1")
	if !gate.ingest("account-1", accountUpdate(1)) || !gate.ingest("account-1", accountUpdate(2)) {
		t.Fatal("ingest did not buffer during read")
	}
	if spy.count() != 0 {
		t.Fatal("events leaked to journal before commit")
	}
	commit()
	if got := spy.times(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("buffered events were not flushed in order: %v", got)
	}
}

func TestGateNestedReadsFlushOnceOnLastCommit(t *testing.T) {
	gate := NewSnapshotGate()
	spy := &journalSpy{}
	gate.setJournal("account-1", spy.record)
	first := gate.begin("account-1")
	second := gate.begin("account-1")
	gate.ingest("account-1", accountUpdate(1))
	first()
	if spy.count() != 0 {
		t.Fatal("events flushed before the last read committed")
	}
	gate.ingest("account-1", accountUpdate(2))
	second()
	if got := spy.times(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("nested reads did not drain once on last commit: %v", got)
	}
}

func TestGateIngestReturnsFalseWithoutJournal(t *testing.T) {
	gate := NewSnapshotGate()
	if gate.ingest("account-1", accountUpdate(1)) {
		t.Fatal("ingest claimed to handle an event with no journal sink")
	}
}

func TestGateDoesNotClaimBufferedEventsWithoutJournal(t *testing.T) {
	gate := NewSnapshotGate()
	commit := gate.begin("account-1")
	if gate.ingest("account-1", accountUpdate(1)) {
		t.Fatal("event without a journaling sink did not fall back to the caller")
	}
	commit()
}

func TestGateCommitIsIdempotent(t *testing.T) {
	gate := NewSnapshotGate()
	spy := &journalSpy{}
	gate.setJournal("account-1", spy.record)
	commit := gate.begin("account-1")
	gate.ingest("account-1", accountUpdate(1))
	commit()
	commit()
	next := gate.begin("account-1")
	gate.ingest("account-1", accountUpdate(2))
	if spy.count() != 1 {
		t.Fatal("duplicate commit corrupted the next read's buffer")
	}
	next()
	if spy.count() != 2 {
		t.Fatal("next read did not drain its event")
	}
}

func TestGateDrainsWithinDatabaseBatchLimit(t *testing.T) {
	gate := NewSnapshotGate()
	spy := &journalSpy{}
	var sizes []int
	gate.setJournal("account-1", func(events []binance.UserEvent) {
		sizes = append(sizes, len(events))
		spy.record(events)
	})
	commit := gate.begin("account-1")
	for i := range 2001 {
		gate.ingest("account-1", accountUpdate(int64(i+1)))
	}
	commit()
	if len(sizes) != 3 || sizes[0] != 1000 || sizes[1] != 1000 || sizes[2] != 1 || spy.count() != 2001 {
		t.Fatal("buffer exceeded the durable batch limit or dropped events", sizes)
	}
	for i, eventTime := range spy.times() {
		if eventTime != int64(i+1) {
			t.Fatal("batch split reordered events")
		}
	}
}

func TestGateIsolationAcrossAccounts(t *testing.T) {
	gate := NewSnapshotGate()
	a := &journalSpy{}
	b := &journalSpy{}
	gate.setJournal("account-a", a.record)
	gate.setJournal("account-b", b.record)
	commitA := gate.begin("account-a")
	gate.ingest("account-a", accountUpdate(1))
	gate.ingest("account-b", accountUpdate(2)) // not buffered: different account
	commitA()
	if a.count() != 1 || a.times()[0] != 1 {
		t.Fatal("account-a did not buffer and flush its own event")
	}
	if b.count() != 1 || b.times()[0] != 2 {
		t.Fatal("account-b event was wrongly buffered by account-a's read")
	}
}

func TestGateConcurrentIngestLosesNothing(t *testing.T) {
	gate := NewSnapshotGate()
	spy := &journalSpy{}
	gate.setJournal("account-1", spy.record)
	commit := gate.begin("account-1")
	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			gate.ingest("account-1", accountUpdate(int64(i)))
		}(i)
	}
	wg.Wait()
	commit()
	if spy.count() != n {
		t.Fatalf("lost events under concurrency: got %d want %d", spy.count(), n)
	}
}
