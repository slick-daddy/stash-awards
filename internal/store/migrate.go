package store

import (
	"database/sql"
	"fmt"
)

// migrations are applied in order; the database's user_version records how many
// have run. Never edit an existing entry — append a new one instead, or older
// installations will end up with a different schema than fresh ones.
var migrations = []string{
	`CREATE TABLE awards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		performer_id TEXT NOT NULL,
		source TEXT NOT NULL,
		organization TEXT NOT NULL,
		award_name TEXT NOT NULL,
		category TEXT,
		year INTEGER NOT NULL,
		event TEXT,
		result TEXT NOT NULL DEFAULT 'won',
		source_url TEXT,
		associated_movie TEXT,
		associated_movie_url TEXT,
		associated_movie_year INTEGER,
		last_scraped DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(performer_id, source, organization, award_name, year)
	);

	CREATE INDEX awards_performer ON awards(performer_id);

	CREATE TABLE performer_urls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		performer_id TEXT NOT NULL,
		source TEXT NOT NULL,
		url TEXT NOT NULL,
		resolved_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(performer_id, source)
	);

	CREATE TABLE sync_state (
		performer_id TEXT NOT NULL,
		source TEXT NOT NULL,
		last_synced DATETIME,
		next_sync_after DATETIME,
		error TEXT,
		PRIMARY KEY (performer_id, source)
	);

	CREATE INDEX sync_state_due ON sync_state(next_sync_after);`,
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this plugin understands (%d): upgrade the plugin", version, len(migrations))
	}

	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", i+1, err)
		}
		// PRAGMA does not accept a bound parameter, and i+1 is not user input.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", i+1, err)
		}
	}
	return nil
}
