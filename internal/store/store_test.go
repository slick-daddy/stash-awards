package store

import (
	"errors"
	"path/filepath"
	"testing"
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
