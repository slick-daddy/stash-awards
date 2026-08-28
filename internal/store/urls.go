package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a lookup has no matching row.
var ErrNotFound = errors.New("not found")

// SetURL records the resolved page for a performer on a source, replacing any
// URL already stored for that pair.
func (s *Store) SetURL(performerID string, source Source, url string) error {
	_, err := s.write.Exec(`
		INSERT INTO performer_urls (performer_id, source, url, resolved_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(performer_id, source) DO UPDATE SET
			url = excluded.url,
			resolved_at = excluded.resolved_at`,
		performerID, source, url, FormatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("save performer url: %w", err)
	}
	return nil
}

// URL returns the cached page for a performer on a source, or ErrNotFound.
func (s *Store) URL(performerID string, source Source) (PerformerURL, error) {
	var u PerformerURL
	var resolvedAt sql.NullString
	err := s.read.QueryRow(`
		SELECT performer_id, source, url, resolved_at
		FROM performer_urls WHERE performer_id = ? AND source = ?`, performerID, source).
		Scan(&u.PerformerID, &u.Source, &u.URL, &resolvedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PerformerURL{}, ErrNotFound
	}
	if err != nil {
		return PerformerURL{}, fmt.Errorf("query performer url: %w", err)
	}
	u.ResolvedAt = str(resolvedAt)
	return u, nil
}

// URLs returns every cached page for a performer, keyed by source.
func (s *Store) URLs(performerID string) (map[Source]PerformerURL, error) {
	rows, err := s.read.Query(`
		SELECT performer_id, source, url, resolved_at
		FROM performer_urls WHERE performer_id = ?`, performerID)
	if err != nil {
		return nil, fmt.Errorf("query performer urls: %w", err)
	}
	defer rows.Close()

	out := map[Source]PerformerURL{}
	for rows.Next() {
		var u PerformerURL
		var resolvedAt sql.NullString
		if err := rows.Scan(&u.PerformerID, &u.Source, &u.URL, &resolvedAt); err != nil {
			return nil, fmt.Errorf("scan performer url: %w", err)
		}
		u.ResolvedAt = str(resolvedAt)
		out[u.Source] = u
	}
	return out, rows.Err()
}

// DeleteURL forgets the cached page for a performer on a source, so the next sync
// resolves it again. Used when a user unlinks a wrongly matched performer.
func (s *Store) DeleteURL(performerID string, source Source) error {
	if _, err := s.write.Exec(`DELETE FROM performer_urls WHERE performer_id = ? AND source = ?`, performerID, source); err != nil {
		return fmt.Errorf("delete performer url: %w", err)
	}
	return nil
}
