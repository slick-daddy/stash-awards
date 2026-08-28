package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// newStore opens a fresh on-disk database in a per-test directory. On-disk rather
// than :memory: so the tests exercise the same WAL configuration as production.
func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenFile(filepath.Join(t.TempDir(), DBFileName))
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), DBFileName)
	for i := 0; i < 2; i++ {
		s, err := OpenFile(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("close %d: %v", i, err)
		}
	}
}

func TestOpenCreatesFileInDirectory(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.SetURL("1", SourceIAFD, "https://example.test/p"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}
}

func TestReplaceAwardsRoundTrips(t *testing.T) {
	s := newStore(t)
	in := []Award{
		{Organization: "AVN Awards", AwardName: "Best New Starlet", Year: 2015, Result: ResultWon,
			SourceURL: "https://www.iafd.com/person.rme/id=abc", AssociatedMovie: "Angela 3"},
		{Organization: "XBIZ Awards", AwardName: "Best Actress", Category: "Feature Movie", Year: 2016,
			Result: ResultNominated, Event: "XBIZ Awards 2016"},
	}
	if err := s.ReplaceAwards("42", SourceIAFD, in); err != nil {
		t.Fatalf("ReplaceAwards: %v", err)
	}

	got, err := s.Awards("42")
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d awards, want 2", len(got))
	}
	// Newest first.
	if got[0].Year != 2016 || got[0].AwardName != "Best Actress" {
		t.Errorf("first award = %d %q, want 2016 Best Actress", got[0].Year, got[0].AwardName)
	}
	if got[0].Category != "Feature Movie" || got[0].Event != "XBIZ Awards 2016" {
		t.Errorf("optional fields lost: %+v", got[0])
	}
	if got[1].AssociatedMovie != "Angela 3" {
		t.Errorf("associated movie = %q, want Angela 3", got[1].AssociatedMovie)
	}
	if got[0].LastScraped == "" {
		t.Error("last_scraped was not defaulted")
	}
	if got[0].PerformerID != "42" || got[0].Source != SourceIAFD {
		t.Errorf("identity columns wrong: %+v", got[0])
	}
}

func TestReplaceAwardsDropsRecordsTheSourceNoLongerLists(t *testing.T) {
	s := newStore(t)
	first := []Award{
		{Organization: "AVN Awards", AwardName: "Best New Starlet", Year: 2015},
		{Organization: "AVN Awards", AwardName: "Retracted Award", Year: 2016},
	}
	if err := s.ReplaceAwards("42", SourceIAFD, first); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	second := []Award{{Organization: "AVN Awards", AwardName: "Best New Starlet", Year: 2015}}
	if err := s.ReplaceAwards("42", SourceIAFD, second); err != nil {
		t.Fatalf("second replace: %v", err)
	}

	got, err := s.AwardsBySource("42", SourceIAFD)
	if err != nil {
		t.Fatalf("AwardsBySource: %v", err)
	}
	if len(got) != 1 || got[0].AwardName != "Best New Starlet" {
		t.Fatalf("got %+v, want only Best New Starlet", got)
	}
	if got[0].Result != ResultWon {
		t.Errorf("result = %q, want the %q default", got[0].Result, ResultWon)
	}
}

func TestReplaceAwardsLeavesOtherSourcesAlone(t *testing.T) {
	s := newStore(t)
	if err := s.ReplaceAwards("42", SourceAIA, []Award{
		{Organization: "AVN Awards", AwardName: "Best New Starlet", Year: 2015},
	}); err != nil {
		t.Fatalf("seed aia: %v", err)
	}
	if err := s.ReplaceAwards("42", SourceIAFD, nil); err != nil {
		t.Fatalf("clear iafd: %v", err)
	}

	got, err := s.AwardsBySource("42", SourceAIA)
	if err != nil {
		t.Fatalf("AwardsBySource: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d aia awards, want 1", len(got))
	}
}

// A source page can list the same organization, award and year twice. That
// collides with the unique constraint, and losing the whole sync over it would be
// worse than keeping the last one seen.
func TestReplaceAwardsToleratesDuplicateKeys(t *testing.T) {
	s := newStore(t)
	err := s.ReplaceAwards("42", SourceIAFD, []Award{
		{Organization: "AVN Awards", AwardName: "Best Sex Scene", Year: 2015, AssociatedMovie: "First"},
		{Organization: "AVN Awards", AwardName: "Best Sex Scene", Year: 2015, AssociatedMovie: "Second"},
	})
	if err != nil {
		t.Fatalf("ReplaceAwards: %v", err)
	}

	got, err := s.Awards("42")
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d awards, want 1", len(got))
	}
	if got[0].AssociatedMovie != "Second" {
		t.Errorf("movie = %q, want the later row (Second)", got[0].AssociatedMovie)
	}
}

func TestURLLookupReportsMissingRow(t *testing.T) {
	s := newStore(t)
	if _, err := s.URL("42", SourceIAFD); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSetURLOverwrites(t *testing.T) {
	s := newStore(t)
	if err := s.SetURL("42", SourceIAFD, "https://old.test"); err != nil {
		t.Fatalf("first SetURL: %v", err)
	}
	if err := s.SetURL("42", SourceIAFD, "https://new.test"); err != nil {
		t.Fatalf("second SetURL: %v", err)
	}

	u, err := s.URL("42", SourceIAFD)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if u.URL != "https://new.test" {
		t.Errorf("url = %q, want https://new.test", u.URL)
	}
	if u.ResolvedAt == "" {
		t.Error("resolved_at is empty")
	}
}

func TestURLsAndDeleteURL(t *testing.T) {
	s := newStore(t)
	if err := s.SetURL("42", SourceIAFD, "https://iafd.test"); err != nil {
		t.Fatalf("SetURL iafd: %v", err)
	}
	if err := s.SetURL("42", SourceAIA, "https://aia.test"); err != nil {
		t.Fatalf("SetURL aia: %v", err)
	}

	all, err := s.URLs("42")
	if err != nil {
		t.Fatalf("URLs: %v", err)
	}
	if len(all) != 2 || all[SourceAIA].URL != "https://aia.test" {
		t.Fatalf("URLs = %+v", all)
	}

	if err := s.DeleteURL("42", SourceIAFD); err != nil {
		t.Fatalf("DeleteURL: %v", err)
	}
	all, err = s.URLs("42")
	if err != nil {
		t.Fatalf("URLs after delete: %v", err)
	}
	if _, ok := all[SourceIAFD]; ok {
		t.Error("iafd url survived the delete")
	}
}

// Deleting a URL that was never set must be a no-op, not an error.
func TestDeleteURLToleratesAMissingRow(t *testing.T) {
	s := newStore(t)
	if err := s.DeleteURL("42", SourceIAFD); err != nil {
		t.Errorf("DeleteURL: %v", err)
	}
}

// An on-disk database opened again must apply the same migrations idempotently
// and not crash. The Open() convenience must also drop the file in the right
// place.
func TestOpenFileRejectsUnrecognisedSchemaVersion(t *testing.T) {
	// Open once to migrate to the latest version, then hand-write a higher
	// PRAGMA user_version to simulate a database produced by a newer plugin.
	path := filepath.Join(t.TempDir(), DBFileName)
	s, err := OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	s.Close()

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA user_version = 999`); err != nil {
		t.Fatalf("bump version: %v", err)
	}
	db.Close()

	if _, err := OpenFile(path); err == nil {
		t.Fatal("OpenFile accepted a database from a newer plugin")
	}
}

// :memory: databases still need WAL-like pragma configuration but cannot use
// a journal file; the dsn builder must pick a different path. It also has
// to add cache=shared so the write and read pools land on the same in-memory
// database, otherwise they would see two different worlds.
func TestOpenFileInMemoryShared(t *testing.T) {
	a, err := OpenFile(":memory:")
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	defer a.Close()

	if err := a.SetURL("42", SourceIAFD, "https://iafd.test"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	b, err := OpenFile(":memory:")
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer b.Close()

	got, err := b.URL("42", SourceIAFD)
	if err != nil {
		t.Fatalf("URL from second handle: %v", err)
	}
	if got.URL != "https://iafd.test" {
		t.Errorf("URL = %q, want it visible across handles (the shared-cache DSN should make this work)", got.URL)
	}
}

// Every method that touches the database must surface the underlying error
// rather than swallowing it. Using a closed Store is the cheapest way to
// force a driver error without mocking the SQLite layer.
func TestMethodsSurfaceErrorsOnClosedStore(t *testing.T) {
	checks := []struct {
		name string
		call func(*Store) error
	}{
		{"ReplaceAwards", func(s *Store) error { return s.ReplaceAwards("42", SourceIAFD, nil) }},
		{"Awards", func(s *Store) error { _, err := s.Awards("42"); return err }},
		{"AwardsBySource", func(s *Store) error { _, err := s.AwardsBySource("42", SourceIAFD); return err }},
		{"ForgetPerformer", func(s *Store) error { return s.ForgetPerformer("42") }},
		{"SetURL", func(s *Store) error { return s.SetURL("42", SourceIAFD, "u") }},
		{"URLs", func(s *Store) error { _, err := s.URLs("42"); return err }},
		{"DeleteURL", func(s *Store) error { return s.DeleteURL("42", SourceIAFD) }},
		{"MarkSynced", func(s *Store) error { return s.MarkSynced("42", SourceIAFD, time.Now()) }},
		{"MarkError", func(s *Store) error { return s.MarkError("42", SourceIAFD, "x", time.Now()) }},
		{"States", func(s *Store) error { _, err := s.States("42"); return err }},
		{"Due", func(s *Store) error { _, err := s.Due(time.Now(), 1); return err }},
		{"DueCount", func(s *Store) error { _, err := s.DueCount(time.Now()); return err }},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			s, err := OpenFile(":memory:")
			if err != nil {
				t.Fatalf("OpenFile: %v", err)
			}
			if err := s.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := c.call(s); err == nil {
				t.Errorf("%s succeeded on a closed store", c.name)
			}
		})
	}
}
