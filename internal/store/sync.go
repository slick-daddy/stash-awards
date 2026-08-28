package store

import (
	"database/sql"
	"fmt"
	"time"
)

// MarkSynced records a successful sync and schedules the next one.
func (s *Store) MarkSynced(performerID string, source Source, next time.Time) error {
	now := time.Now()
	_, err := s.write.Exec(`
		INSERT INTO sync_state (performer_id, source, last_synced, next_sync_after, error)
		VALUES (?, ?, ?, ?, NULL)
		ON CONFLICT(performer_id, source) DO UPDATE SET
			last_synced = excluded.last_synced,
			next_sync_after = excluded.next_sync_after,
			error = NULL`,
		performerID, source, FormatTime(now), FormatTime(next))
	if err != nil {
		return fmt.Errorf("record sync success: %w", err)
	}
	return nil
}

// MarkError records a failed sync. last_synced is deliberately left alone so the
// stored data still shows when it was actually fetched.
//
// retryAfter is when the performer becomes eligible again. The ADR proposed
// excluding errored rows from the due query instead, but that would drop a
// performer out of auto-sync permanently after one transient network failure, so
// failures are rescheduled rather than parked.
func (s *Store) MarkError(performerID string, source Source, message string, retryAfter time.Time) error {
	if message == "" {
		message = "unknown error"
	}
	_, err := s.write.Exec(`
		INSERT INTO sync_state (performer_id, source, next_sync_after, error)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(performer_id, source) DO UPDATE SET
			next_sync_after = excluded.next_sync_after,
			error = excluded.error`,
		performerID, source, FormatTime(retryAfter), message)
	if err != nil {
		return fmt.Errorf("record sync error: %w", err)
	}
	return nil
}

// State returns the sync state for a performer and source, or ErrNotFound when
// the pair has never been synced.
func (s *Store) State(performerID string, source Source) (SyncState, error) {
	row := s.read.QueryRow(`
		SELECT performer_id, source, last_synced, next_sync_after, error
		FROM sync_state WHERE performer_id = ? AND source = ?`, performerID, source)
	st, err := scanState(row)
	if err == sql.ErrNoRows {
		return SyncState{}, ErrNotFound
	}
	return st, err
}

// States returns every sync state for a performer, keyed by source.
func (s *Store) States(performerID string) (map[Source]SyncState, error) {
	rows, err := s.read.Query(`
		SELECT performer_id, source, last_synced, next_sync_after, error
		FROM sync_state WHERE performer_id = ?`, performerID)
	if err != nil {
		return nil, fmt.Errorf("query sync states: %w", err)
	}
	defer rows.Close()

	out := map[Source]SyncState{}
	for rows.Next() {
		st, err := scanState(rows)
		if err != nil {
			return nil, err
		}
		out[st.Source] = st
	}
	return out, rows.Err()
}

// Due returns up to limit performer/source pairs whose next sync is not in the
// future. Ordered oldest-first so a backlog drains fairly.
func (s *Store) Due(before time.Time, limit int) ([]SyncState, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.read.Query(`
		SELECT performer_id, source, last_synced, next_sync_after, error
		FROM sync_state
		WHERE next_sync_after IS NOT NULL AND next_sync_after <= ?
		ORDER BY next_sync_after ASC
		LIMIT ?`, FormatTime(before), limit)
	if err != nil {
		return nil, fmt.Errorf("query due syncs: %w", err)
	}
	defer rows.Close()

	var out []SyncState
	for rows.Next() {
		st, err := scanState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DueCount reports how many performer/source pairs are waiting to be synced.
func (s *Store) DueCount(before time.Time) (int, error) {
	var n int
	err := s.read.QueryRow(`
		SELECT COUNT(*) FROM sync_state
		WHERE next_sync_after IS NOT NULL AND next_sync_after <= ?`, FormatTime(before)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count due syncs: %w", err)
	}
	return n, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanState(sc scanner) (SyncState, error) {
	var st SyncState
	var lastSynced, nextSync, errMsg sql.NullString
	if err := sc.Scan(&st.PerformerID, &st.Source, &lastSynced, &nextSync, &errMsg); err != nil {
		if err == sql.ErrNoRows {
			return SyncState{}, err
		}
		return SyncState{}, fmt.Errorf("scan sync state: %w", err)
	}
	st.LastSynced, st.NextSyncAfter, st.Error = str(lastSynced), str(nextSync), str(errMsg)
	return st, nil
}
