package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrSnapshotTransition = errors.New("no se pudo confirmar la transición; verifica permisos, migraciones y la huella del snapshot")

// LegacySnapshotTransition contains operational metadata only, never the
// snapshot's financial data. Missing means no active baseline, not archive loss.
type LegacySnapshotTransition struct {
	Status      string     `json:"status"`
	CapturedAt  *time.Time `json:"capturedAt,omitempty"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	ArchiveID   string     `json:"archiveId,omitempty"`
}

type legacyBaseline struct {
	AccountID  string          `json:"accountId"`
	OwnerID    string          `json:"ownerId"`
	StartedAt  time.Time       `json:"startedAt"`
	CapturedAt time.Time       `json:"capturedAt"`
	StoredAt   time.Time       `json:"storedAt"`
	Snapshot   json.RawMessage `json:"snapshot"`
	Comparison json.RawMessage `json:"comparison"`
}

func (b legacyBaseline) fingerprint() (string, error) {
	b.StartedAt = b.StartedAt.UTC()
	b.CapturedAt = b.CapturedAt.UTC()
	b.StoredAt = b.StoredAt.UTC()
	data, err := json.Marshal(b)
	if err != nil {
		return "", ErrSnapshotTransition
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// TransitionLegacySnapshot is administrative only. Preview is read-only; apply
// requires its exact fingerprint and atomically preserves the entire old row
// before retiring the active baseline. No exchange calls or clock fabrication.
// Runtime and administrative writers take the same personal-owner lock.
func TransitionLegacySnapshot(ctx context.Context, uri, account, expected string, apply bool) (LegacySnapshotTransition, error) {
	if !ownerUUID.MatchString(account) || (apply && !sessionHash.MatchString(expected)) || (!apply && expected != "") {
		return LegacySnapshotTransition{}, ErrSnapshotTransition
	}
	account = strings.ToLower(account)
	db, err := connect(uri)
	if err != nil {
		return LegacySnapshotTransition{}, ErrSnapshotTransition
	}
	defer db.Close()
	var allowed bool
	if err := db.QueryRowContext(ctx, `SELECT has_table_privilege(current_user, 'public.exchange_read_snapshots', 'DELETE') AND has_table_privilege(current_user, 'public.exchange_read_snapshot_archives', 'INSERT')`).Scan(&allowed); err != nil || !allowed {
		return LegacySnapshotTransition{}, ErrSnapshotTransition
	}
	store := &Store{db: db}
	var result LegacySnapshotTransition
	err = store.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts WHERE id=$1 AND owner_id=$2 AND exchange='binance' AND environment='production')`, account, owner).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		b := legacyBaseline{AccountID: account, OwnerID: owner}
		var snapshot, comparison []byte
		err := tx.QueryRowContext(ctx, `SELECT started_at,captured_at,stored_at,snapshot,comparison FROM exchange_read_snapshots WHERE account_id=$1 AND owner_id=$2 FOR UPDATE`, account, owner).Scan(&b.StartedAt, &b.CapturedAt, &b.StoredAt, &snapshot, &comparison)
		if errors.Is(err, sql.ErrNoRows) {
			result.Status = "missing"
			return nil
		}
		if err != nil {
			return err
		}
		b.Snapshot, b.Comparison = snapshot, comparison
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(b.Snapshot, &fields); err != nil || fields == nil {
			return ErrSnapshotTransition
		}
		capturedAt := b.CapturedAt.UTC()
		result.CapturedAt = &capturedAt
		// Presence, not a zero-value decode: null/invalid/new cutoffs are not
		// silently reclassified as legacy data and retired.
		if _, present := fields["capturedAtMs"]; present {
			if apply {
				return ErrSnapshotTransition
			}
			result.Status = "current"
			return nil
		}
		result.Status = "legacy"
		result.Fingerprint, err = b.fingerprint()
		if err != nil || !apply {
			return err
		}
		if result.Fingerprint != expected {
			return ErrSnapshotTransition
		}
		// Copy directly inside PostgreSQL: preserve JSON, nullable comparison,
		// timestamps and owner exactly, including any evidence of corruption.
		if err := tx.QueryRowContext(ctx, `INSERT INTO exchange_read_snapshot_archives(account_id,owner_id,started_at,captured_at,stored_at,snapshot,comparison,reason) SELECT account_id,owner_id,started_at,captured_at,stored_at,snapshot,comparison,'missing_exchange_cutoff' FROM exchange_read_snapshots WHERE account_id=$1 AND owner_id=$2 RETURNING id::text`, account, owner).Scan(&result.ArchiveID); err != nil {
			return err
		}
		deleted, err := tx.ExecContext(ctx, `DELETE FROM exchange_read_snapshots WHERE account_id=$1 AND owner_id=$2`, account, owner)
		if err != nil {
			return err
		}
		if n, err := deleted.RowsAffected(); err != nil || n != 1 {
			return ErrSnapshotTransition
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO binance_stream_state(account_id,owner_id,connected,last_event_time,gapped_since_snapshot) VALUES($1,$2,false,0,true) ON CONFLICT(account_id) DO UPDATE SET gapped_since_snapshot=true,updated_at=clock_timestamp() WHERE binance_stream_state.owner_id=EXCLUDED.owner_id`, account, owner); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,'account.read_archived',$2,gen_random_uuid(),jsonb_build_object('archiveId',$3::text,'reason','missing_exchange_cutoff'))`, owner, account, result.ArchiveID); err != nil {
			return err
		}
		result.Status = "archived"
		return nil
	})
	if err != nil {
		return LegacySnapshotTransition{}, ErrSnapshotTransition
	}
	return result, nil
}
