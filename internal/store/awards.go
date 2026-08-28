package store

import (
	"database/sql"
	"fmt"
	"time"
)

// timeLayout is how every timestamp this package writes is stored. It is UTC and
// fixed-width, so string comparison in SQL is also chronological comparison.
const timeLayout = "2006-01-02T15:04:05Z"

// FormatTime renders t in the layout used by the database.
func FormatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// ParseTime reads a timestamp written by FormatTime.
func ParseTime(s string) (time.Time, error) { return time.Parse(timeLayout, s) }

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func str(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// ReplaceAwards swaps out every award this performer has from source, in one
// transaction. A source that legitimately drops a record should stop showing it,
// which an insert-only merge could never do.
func (s *Store) ReplaceAwards(performerID string, source Source, awards []Award) error {
	tx, err := s.write.Begin()
	if err != nil {
		return fmt.Errorf("begin award replace: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM awards WHERE performer_id = ? AND source = ?`, performerID, source); err != nil {
		return fmt.Errorf("clear awards: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO awards (
			performer_id, source, organization, award_name, category, year,
			event, result, source_url, associated_movie, last_scraped
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(performer_id, source, organization, award_name, year) DO UPDATE SET
			category = excluded.category,
			event = excluded.event,
			result = excluded.result,
			source_url = excluded.source_url,
			associated_movie = excluded.associated_movie,
			last_scraped = excluded.last_scraped`)
	if err != nil {
		return fmt.Errorf("prepare award insert: %w", err)
	}
	defer stmt.Close()

	scraped := FormatTime(time.Now())
	for _, a := range awards {
		result := a.Result
		if result == "" {
			result = ResultWon
		}
		lastScraped := a.LastScraped
		if lastScraped == "" {
			lastScraped = scraped
		}
		// Two rows from the same page can collide on the unique key (an org
		// running the same award twice in a year); the upsert keeps the later
		// one rather than aborting the whole sync.
		if _, err := stmt.Exec(performerID, source, a.Organization, a.AwardName,
			nullString(a.Category), a.Year, nullString(a.Event), result,
			nullString(a.SourceURL), nullString(a.AssociatedMovie), lastScraped); err != nil {
			return fmt.Errorf("insert award %q %d: %w", a.AwardName, a.Year, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit award replace: %w", err)
	}
	return nil
}

const awardColumns = `id, performer_id, source, organization, award_name, category,
	year, event, result, source_url, associated_movie, last_scraped`

// awardOrder puts the newest awards first, then groups deterministically so the
// UI never reorders rows between identical requests.
const awardOrder = `ORDER BY year DESC, organization ASC, award_name ASC, id ASC`

// Awards returns every award for a performer across all sources.
func (s *Store) Awards(performerID string) ([]Award, error) {
	rows, err := s.read.Query(`SELECT `+awardColumns+` FROM awards WHERE performer_id = ? `+awardOrder, performerID)
	if err != nil {
		return nil, fmt.Errorf("query awards: %w", err)
	}
	return scanAwards(rows)
}

// AwardsBySource returns a performer's awards from one source only.
func (s *Store) AwardsBySource(performerID string, source Source) ([]Award, error) {
	rows, err := s.read.Query(`SELECT `+awardColumns+` FROM awards WHERE performer_id = ? AND source = ? `+awardOrder, performerID, source)
	if err != nil {
		return nil, fmt.Errorf("query awards: %w", err)
	}
	return scanAwards(rows)
}

func scanAwards(rows *sql.Rows) ([]Award, error) {
	defer rows.Close()
	var out []Award
	for rows.Next() {
		var a Award
		var category, event, sourceURL, movie sql.NullString
		if err := rows.Scan(&a.ID, &a.PerformerID, &a.Source, &a.Organization, &a.AwardName,
			&category, &a.Year, &event, &a.Result, &sourceURL, &movie, &a.LastScraped); err != nil {
			return nil, fmt.Errorf("scan award: %w", err)
		}
		a.Category, a.Event, a.SourceURL, a.AssociatedMovie = str(category), str(event), str(sourceURL), str(movie)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ForgetPerformer removes everything stored about a performer. Called when Stash
// reports the performer was deleted, so the database does not accumulate rows
// pointing at IDs that no longer exist.
func (s *Store) ForgetPerformer(performerID string) error {
	tx, err := s.write.Begin()
	if err != nil {
		return fmt.Errorf("begin forget: %w", err)
	}
	defer tx.Rollback()

	for _, table := range []string{"awards", "performer_urls", "sync_state"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE performer_id = ?`, performerID); err != nil {
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit forget: %w", err)
	}
	return nil
}
