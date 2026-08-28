// Package store persists award data in a SQLite database that lives beside the
// plugin binary. Keeping it out of Stash's own database avoids any chance of
// interfering with Stash migrations.
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go driver: no CGO, so release binaries cross-compile
)

// DBFileName is the database file created inside the plugin directory.
const DBFileName = "awards.db"

// Store owns the database connections. Writes are funnelled through a
// single-connection pool so SQLite never has to arbitrate between two writers;
// reads use their own pool and run concurrently thanks to WAL mode.
type Store struct {
	write *sql.DB
	read  *sql.DB
}

// Open opens (creating it if needed) the database in dir.
func Open(dir string) (*Store, error) {
	return OpenFile(filepath.Join(dir, DBFileName))
}

// OpenFile opens the database at path. Pass ":memory:" for a throwaway database.
func OpenFile(path string) (*Store, error) {
	dsn := dsnFor(path)

	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open write pool: %w", err)
	}
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)

	if err := migrate(write); err != nil {
		write.Close()
		return nil, err
	}

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		write.Close()
		return nil, fmt.Errorf("open read pool: %w", err)
	}
	read.SetMaxOpenConns(5)
	read.SetMaxIdleConns(5)

	return &Store{write: write, read: read}, nil
}

// dsnFor builds the connection string. An in-memory database has to be shared
// between the two pools, otherwise each pool would see a different database.
func dsnFor(path string) string {
	// busy_timeout lets a blocked reader wait for a checkpoint instead of
	// failing immediately with SQLITE_BUSY.
	const pragmas = "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if path == ":memory:" {
		return "file::memory:?cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	return "file:" + filepath.ToSlash(path) + pragmas
}

// Close releases both connection pools.
func (s *Store) Close() error {
	readErr := s.read.Close()
	if err := s.write.Close(); err != nil {
		return err
	}
	return readErr
}
