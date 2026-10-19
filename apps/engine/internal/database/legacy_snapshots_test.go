package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestLegacyTransitionRejectsMalformedInputBeforeDatabase(t *testing.T) {
	for _, tc := range []struct {
		account, expected string
		apply             bool
	}{
		{account: "forged"},
		{account: "00000000-0000-4000-8000-000000000001", apply: true},
		{account: "00000000-0000-4000-8000-000000000001", expected: "forged", apply: true},
		{account: "00000000-0000-4000-8000-000000000001", expected: strings.Repeat("a", 64)},
	} {
		if _, err := TransitionLegacySnapshot(context.Background(), "", tc.account, tc.expected, tc.apply); !errors.Is(err, ErrSnapshotTransition) {
			t.Fatal("invalid transition input accepted")
		}
	}
}

func TestLegacyFingerprintCoversWholeRowAndNormalizesTimezones(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 123000000, time.UTC)
	b := legacyBaseline{AccountID: "account", OwnerID: "owner", StartedAt: now, CapturedAt: now.Add(time.Second), StoredAt: now.Add(2 * time.Second), Snapshot: json.RawMessage(`{"mode":"read_only"}`)}
	original, err := b.fingerprint()
	if err != nil || len(original) != 64 {
		t.Fatal("fingerprint failed")
	}
	shifted := b
	shifted.CapturedAt = shifted.CapturedAt.In(time.FixedZone("synthetic", -4*60*60))
	if got, _ := shifted.fingerprint(); got != original {
		t.Fatal("timezone changed the fingerprint")
	}
	for _, mutate := range []func(*legacyBaseline){
		func(b *legacyBaseline) { b.AccountID = "other" },
		func(b *legacyBaseline) { b.OwnerID = "other" },
		func(b *legacyBaseline) { b.StartedAt = b.StartedAt.Add(time.Microsecond) },
		func(b *legacyBaseline) { b.CapturedAt = b.CapturedAt.Add(time.Microsecond) },
		func(b *legacyBaseline) { b.StoredAt = b.StoredAt.Add(time.Microsecond) },
		func(b *legacyBaseline) { b.Snapshot = json.RawMessage(`{"mode":"changed"}`) },
		func(b *legacyBaseline) { b.Comparison = json.RawMessage(`{}`) },
	} {
		changed := b
		mutate(&changed)
		if got, err := changed.fingerprint(); err != nil || got == original {
			t.Fatal("row change was not fingerprinted")
		}
	}
}

// Uses the isolated account/database from the credential lifecycle. No exchange
// access, raw keys or production databases. All failure checks are fail-closed.
func testLegacySnapshotTransition(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, adminURI, runtimeURI, hash, account string) {
	t.Helper()
	identity, err := store.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	const otherAccount = "00000000-0000-4000-8000-000000000090"
	if _, err := admin.ExecContext(ctx, `INSERT INTO exchange_accounts(id,owner_id,exchange,environment,name) VALUES($1,'00000000-0000-4000-8000-000000000001','binance','production','Synthetic foreign account')`, otherAccount); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.ExecContext(ctx, `DELETE FROM exchange_accounts WHERE id=$1`, otherAccount); err != nil {
			t.Error("could not clean foreign test account")
		}
	}()
	for _, target := range []string{otherAccount, "00000000-0000-4000-8000-000000000099"} {
		if _, err := TransitionLegacySnapshot(ctx, adminURI, target, "", false); !errors.Is(err, ErrSnapshotTransition) {
			t.Fatal("foreign or unknown account metadata disclosed")
		}
		if _, err := TransitionLegacySnapshot(ctx, adminURI, target, strings.Repeat("a", 64), true); !errors.Is(err, ErrSnapshotTransition) {
			t.Fatal("foreign or unknown account accepted for transition")
		}
	}
	for _, apply := range []bool{false, true} {
		expected := ""
		if apply {
			expected = strings.Repeat("a", 64)
		}
		if _, err := TransitionLegacySnapshot(ctx, runtimeURI, account, expected, apply); !errors.Is(err, ErrSnapshotTransition) {
			t.Fatal("runtime role accessed the administrative transition")
		}
	}
	current, err := TransitionLegacySnapshot(ctx, adminURI, account, "", false)
	if err != nil || current.Status != "current" || current.Fingerprint != "" {
		t.Fatal("modern baseline was not left alone", err)
	}
	if _, err := TransitionLegacySnapshot(ctx, adminURI, account, strings.Repeat("a", 64), true); err == nil {
		t.Fatal("modern baseline was retired")
	}
	// Presence of an invalid cutoff is corruption, not a pre-cutoff schema.
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET snapshot=jsonb_set(snapshot,'{capturedAtMs}','null'::jsonb) WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if result, err := TransitionLegacySnapshot(ctx, adminURI, account, "", false); err != nil || result.Status != "current" {
		t.Fatal("null cutoff reclassified as legacy")
	}
	if _, err := TransitionLegacySnapshot(ctx, adminURI, account, strings.Repeat("a", 64), true); err == nil {
		t.Fatal("corrupt cutoff was silently retired")
	}
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET snapshot=snapshot-'capturedAtMs' WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	var original legacyBaseline
	if err := admin.QueryRowContext(ctx, `SELECT started_at,captured_at,stored_at,snapshot,comparison FROM exchange_read_snapshots WHERE account_id=$1`, account).Scan(&original.StartedAt, &original.CapturedAt, &original.StoredAt, &original.Snapshot, &original.Comparison); err != nil {
		t.Fatal(err)
	}
	preview, err := TransitionLegacySnapshot(ctx, adminURI, account, "", false)
	if err != nil || preview.Status != "legacy" || len(preview.Fingerprint) != 64 || preview.ArchiveID != "" {
		t.Fatal("legacy preview failed", err)
	}
	uppercase, err := TransitionLegacySnapshot(ctx, adminURI, strings.ToUpper(account), "", false)
	if err != nil || uppercase.Fingerprint != preview.Fingerprint {
		t.Fatal("account UUID casing changed the preview fingerprint")
	}
	var count int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM exchange_read_snapshot_archives`).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview wrote an archive")
	}
	if _, err := store.LatestReadSnapshot(ctx, hash, account); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("legacy snapshot accepted as exchange-clock evidence")
	}
	if err := store.SaveReadSnapshot(ctx, hash, account, syntheticReadSnapshot()); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("legacy baseline was silently replaced")
	}
	// A comparison-only change also invalidates the operator's preview.
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET comparison=NULL WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := TransitionLegacySnapshot(ctx, adminURI, account, preview.Fingerprint, true); err == nil {
		t.Fatal("stale fingerprint applied")
	}
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET comparison=$1 WHERE account_id=$2`, []byte(original.Comparison), account); err != nil {
		t.Fatal(err)
	}
	// Force failure after both copy and deletion. All data/state must roll back.
	beforeRollback, err := store.LatestStreamState(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `CREATE FUNCTION reject_archive_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='account.read_archived' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_archive_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_archive_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := TransitionLegacySnapshot(ctx, adminURI, account, preview.Fingerprint, true); err == nil {
		t.Fatal("failed audit committed the archive")
	}
	if _, err := admin.ExecContext(ctx, `DROP TRIGGER reject_archive_audit ON audit_events; DROP FUNCTION reject_archive_audit()`); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM exchange_read_snapshot_archives`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed transition retained an archive")
	}
	unchanged, err := TransitionLegacySnapshot(ctx, adminURI, account, "", false)
	if err != nil || unchanged.Fingerprint != preview.Fingerprint {
		t.Fatal("failed transition lost or changed the active baseline")
	}
	before, err := store.LatestStreamState(ctx, hash, account)
	if err != nil || before.Connected != beforeRollback.Connected || before.GappedSinceSnapshot != beforeRollback.GappedSinceSnapshot || before.LastEventTime != beforeRollback.LastEventTime {
		t.Fatal("failed transition changed stream state", err)
	}
	type attempt struct {
		result LegacySnapshotTransition
		err    error
	}
	results := make(chan attempt, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := TransitionLegacySnapshot(ctx, adminURI, account, preview.Fingerprint, true)
			results <- attempt{result: result, err: err}
		}()
	}
	workers.Wait()
	close(results)
	statuses := map[string]int{}
	for a := range results {
		if a.err != nil || (a.result.Status == "archived" && a.result.ArchiveID == "") {
			t.Fatal("concurrent transition failed", a.err)
		}
		statuses[a.result.Status]++
	}
	if statuses["archived"] != 1 || statuses["missing"] != 1 {
		t.Fatal("concurrent applies duplicated or lost an archive", statuses)
	}
	var exact bool
	if err := admin.QueryRowContext(ctx, `SELECT started_at=$2 AND captured_at=$3 AND stored_at=$4 AND snapshot=$5::jsonb AND comparison IS NOT DISTINCT FROM $6::jsonb AND owner_id=$7 AND reason='missing_exchange_cutoff' FROM exchange_read_snapshot_archives WHERE account_id=$1`, account, original.StartedAt, original.CapturedAt, original.StoredAt, []byte(original.Snapshot), []byte(original.Comparison), identity.OwnerID).Scan(&exact); err != nil || !exact {
		t.Fatal("archive changed original evidence", err)
	}
	if _, err := store.LatestReadSnapshot(ctx, hash, account); !errors.Is(err, ErrSnapshotMissing) {
		t.Fatal("retired baseline remained active", err)
	}
	if _, err := store.LatestReadComparison(ctx, hash, account); !errors.Is(err, ErrComparisonMissing) {
		t.Fatal("retired comparison remained active", err)
	}
	after, err := store.LatestStreamState(ctx, hash, account)
	if err != nil || !after.GappedSinceSnapshot || after.Connected != before.Connected || after.LastEventTime != before.LastEventTime {
		t.Fatal("archive fabricated continuity or reset the watermark")
	}
	if repeated, err := TransitionLegacySnapshot(ctx, adminURI, account, preview.Fingerprint, true); err != nil || repeated.Status != "missing" || repeated.ArchiveID != "" {
		t.Fatal("repeated apply was not harmless", err)
	}
	// The fresh observation starts explicitly without a comparison baseline.
	fresh := syntheticReadSnapshot()
	fresh.StartedAt = original.StartedAt.Add(time.Second)
	fresh.CapturedAt = original.CapturedAt.Add(time.Second)
	fresh.CapturedAtMs = fresh.CapturedAt.UnixMilli()
	if err := store.SaveReadSnapshot(ctx, hash, account, fresh); err != nil {
		t.Fatal("fresh snapshot blocked after archive", err)
	}
	report, err := store.LatestReadComparison(ctx, hash, account)
	if err != nil || report.Report.Status != "initial_observation" || report.Report.PreviousCapturedAt != nil {
		t.Fatal("archive was reused as a comparison baseline")
	}
	after, err = store.LatestStreamState(ctx, hash, account)
	if err != nil || after.GappedSinceSnapshot || after.LastEventTime != before.LastEventTime {
		t.Fatal("fresh baseline did not reset the gap safely")
	}
	// A new baseline arriving after preview must never be retired by stale apply.
	if _, err := TransitionLegacySnapshot(ctx, adminURI, account, preview.Fingerprint, true); err == nil {
		t.Fatal("stale apply retired the replacement baseline")
	}
	var audits int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE action='account.read_archived' AND resource_id=$1`, account).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("transition audit missing or duplicated")
	}
	// Very old rows may have no comparison and even corrupt financial data.
	// Archive that evidence verbatim; never decode it as a usable baseline.
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET snapshot=jsonb_set(snapshot-'capturedAtMs','{assets,0,walletBalance}','"NaN"'::jsonb),comparison=NULL WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	nullPreview, err := TransitionLegacySnapshot(ctx, adminURI, account, "", false)
	if err != nil || nullPreview.Status != "legacy" {
		t.Fatal("nullable comparison prevented preview", err)
	}
	if _, err := TransitionLegacySnapshot(ctx, adminURI, account, nullPreview.Fingerprint, true); err != nil {
		t.Fatal("nullable comparison prevented preservation", err)
	}
	if err := admin.QueryRowContext(ctx, `SELECT comparison IS NULL AND snapshot->'assets'->0->>'walletBalance'='NaN' AND NOT (snapshot ? 'capturedAtMs') FROM exchange_read_snapshot_archives WHERE account_id=$1 AND captured_at=$2`, account, fresh.CapturedAt).Scan(&exact); err != nil || !exact {
		t.Fatal("archive changed nullable/corrupt legacy evidence", err)
	}
	fresh.StartedAt = fresh.StartedAt.Add(time.Second)
	fresh.CapturedAt = fresh.CapturedAt.Add(time.Second)
	fresh.CapturedAtMs = fresh.CapturedAt.UnixMilli()
	if err := store.SaveReadSnapshot(ctx, hash, account, fresh); err != nil {
		t.Fatal("fresh reading blocked after nullable legacy archive", err)
	}
	// Runtime has no archive privileges. Temporarily grant SELECT in this
	// disposable database only to test forced RLS, then restore no access.
	var role string
	if err := store.db.QueryRowContext(ctx, `SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
		var allowed bool
		if err := store.db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user,'public.exchange_read_snapshot_archives',$1)`, privilege).Scan(&allowed); err != nil || allowed {
			t.Fatal("runtime can access legacy archives")
		}
	}
	quoted := pgx.Identifier{role}.Sanitize()
	if _, err := admin.ExecContext(ctx, `GRANT SELECT ON exchange_read_snapshot_archives TO `+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.ExecContext(ctx, `REVOKE SELECT ON exchange_read_snapshot_archives FROM `+quoted); err != nil {
			t.Error("could not restore archive privileges")
		}
	}()
	for owner, want := range map[string]int{identity.OwnerID: 2, "00000000-0000-4000-8000-000000000001": 0} {
		if err := store.WithOwner(ctx, owner, func(tx *sql.Tx) error {
			var got int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM exchange_read_snapshot_archives`).Scan(&got); err != nil {
				return err
			}
			if got != want {
				return errors.New("cross-owner archive access")
			}
			return nil
		}); err != nil {
			t.Fatal("archive RLS failed", err)
		}
	}
}
