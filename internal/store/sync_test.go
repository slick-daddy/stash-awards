package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestMarkSyncedThenDue(t *testing.T) {
	s := newStore(t)
	now := time.Now()

	if err := s.MarkSynced("42", SourceIAFD, now.Add(7*24*time.Hour)); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}

	st, err := s.State("42", SourceIAFD)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.Error != "" {
		t.Errorf("error = %q, want empty", st.Error)
	}
	if st.LastSynced == "" || st.NextSyncAfter == "" {
		t.Fatalf("timestamps not recorded: %+v", st)
	}

	// Not due yet.
	due, err := s.Due(now, 50)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("got %d due, want 0", len(due))
	}

	// Due once the interval has elapsed.
	due, err = s.Due(now.Add(8*24*time.Hour), 50)
	if err != nil {
		t.Fatalf("Due later: %v", err)
	}
	if len(due) != 1 || due[0].Source != SourceIAFD {
		t.Fatalf("got %+v, want one iafd entry", due)
	}
}

func TestMarkSyncedClearsPreviousError(t *testing.T) {
	s := newStore(t)
	now := time.Now()

	if err := s.MarkError("42", SourceAIA, "boom", now.Add(time.Hour)); err != nil {
		t.Fatalf("MarkError: %v", err)
	}
	st, err := s.State("42", SourceAIA)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.Error != "boom" {
		t.Fatalf("error = %q, want boom", st.Error)
	}
	if st.LastSynced != "" {
		t.Errorf("last_synced = %q, want it left unset after a failure", st.LastSynced)
	}

	if err := s.MarkSynced("42", SourceAIA, now.Add(24*time.Hour)); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}
	st, err = s.State("42", SourceAIA)
	if err != nil {
		t.Fatalf("State after success: %v", err)
	}
	if st.Error != "" {
		t.Errorf("error = %q, want it cleared", st.Error)
	}
}

// A transient failure must not exclude the performer from auto-sync forever; it is
// rescheduled sooner instead.
func TestErroredPerformerIsStillEventuallyDue(t *testing.T) {
	s := newStore(t)
	now := time.Now()

	if err := s.MarkError("42", SourceIAFD, "connection reset", now.Add(time.Hour)); err != nil {
		t.Fatalf("MarkError: %v", err)
	}

	due, err := s.Due(now.Add(2*time.Hour), 50)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("got %d due, want the errored pair back", len(due))
	}
	if due[0].Error != "connection reset" {
		t.Errorf("error = %q, want it preserved for display", due[0].Error)
	}
}

func TestMarkErrorDefaultsBlankMessage(t *testing.T) {
	s := newStore(t)
	if err := s.MarkError("42", SourceIAFD, "", time.Now()); err != nil {
		t.Fatalf("MarkError: %v", err)
	}
	st, err := s.State("42", SourceIAFD)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.Error == "" {
		t.Error("blank message stored as NULL, which would read as success")
	}
}

func TestDueOrdersOldestFirstAndRespectsLimit(t *testing.T) {
	s := newStore(t)
	base := time.Now()

	if err := s.MarkSynced("new", SourceIAFD, base.Add(-time.Hour)); err != nil {
		t.Fatalf("MarkSynced new: %v", err)
	}
	if err := s.MarkSynced("old", SourceIAFD, base.Add(-72*time.Hour)); err != nil {
		t.Fatalf("MarkSynced old: %v", err)
	}

	due, err := s.Due(base, 1)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 || due[0].PerformerID != "old" {
		t.Fatalf("got %+v, want the oldest entry only", due)
	}

	n, err := s.DueCount(base)
	if err != nil {
		t.Fatalf("DueCount: %v", err)
	}
	if n != 2 {
		t.Errorf("DueCount = %d, want 2", n)
	}
}

func TestDueWithNonPositiveLimitReturnsNothing(t *testing.T) {
	s := newStore(t)
	if err := s.MarkSynced("42", SourceIAFD, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}
	due, err := s.Due(time.Now(), 0)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("got %d due, want 0", len(due))
	}
}

func TestStatesForPerformer(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	if err := s.MarkSynced("42", SourceIAFD, now.Add(time.Hour)); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}
	if err := s.MarkError("42", SourceAIA, "not found", now.Add(time.Hour)); err != nil {
		t.Fatalf("MarkError: %v", err)
	}

	states, err := s.States("42")
	if err != nil {
		t.Fatalf("States: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("got %d states, want 2", len(states))
	}
	if states[SourceAIA].Error != "not found" {
		t.Errorf("aia error = %q", states[SourceAIA].Error)
	}
}

func TestStateReportsMissingRow(t *testing.T) {
	s := newStore(t)
	if _, err := s.State("42", SourceIAFD); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestForgetPerformerClearsEveryTable(t *testing.T) {
	s := newStore(t)
	if err := s.ReplaceAwards("42", SourceIAFD, []Award{
		{Organization: "AVN Awards", AwardName: "Best New Starlet", Year: 2015},
	}); err != nil {
		t.Fatalf("ReplaceAwards: %v", err)
	}
	if err := s.SetURL("42", SourceIAFD, "https://iafd.test"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}
	if err := s.MarkSynced("42", SourceIAFD, time.Now()); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}
	// A second performer must survive.
	if err := s.SetURL("99", SourceIAFD, "https://other.test"); err != nil {
		t.Fatalf("SetURL other: %v", err)
	}

	if err := s.ForgetPerformer("42"); err != nil {
		t.Fatalf("ForgetPerformer: %v", err)
	}

	awards, err := s.Awards("42")
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	if len(awards) != 0 {
		t.Errorf("got %d awards, want 0", len(awards))
	}
	if _, err := s.URL("42", SourceIAFD); !errors.Is(err, ErrNotFound) {
		t.Errorf("url err = %v, want ErrNotFound", err)
	}
	if _, err := s.State("42", SourceIAFD); !errors.Is(err, ErrNotFound) {
		t.Errorf("state err = %v, want ErrNotFound", err)
	}
	if _, err := s.URL("99", SourceIAFD); err != nil {
		t.Errorf("other performer was affected: %v", err)
	}
}

func TestFormatTimeSortsChronologically(t *testing.T) {
	earlier := FormatTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	later := FormatTime(time.Date(2026, 11, 2, 3, 4, 5, 0, time.UTC))
	if !(earlier < later) {
		t.Fatalf("%q should sort before %q", earlier, later)
	}
	got, err := ParseTime(earlier)
	if err != nil {
		t.Fatalf("ParseTime: %v", err)
	}
	if got.Year() != 2026 || got.Month() != time.January {
		t.Errorf("round trip = %v", got)
	}
}

func TestSourceValidation(t *testing.T) {
	if !SourceIAFD.Valid() || !SourceAIA.Valid() {
		t.Error("known sources reported invalid")
	}
	if Source("evil'; DROP TABLE awards; --").Valid() {
		t.Error("unknown source reported valid")
	}
	if SourceAIA.Label() != "AdultIndustryAwards" {
		t.Errorf("aia label = %q", SourceAIA.Label())
	}
	// An unknown source falls back to its raw string rather than blank.
	if got := Source("x").Label(); got != "x" {
		t.Errorf("unknown label = %q, want the raw string", got)
	}
}

// ForgetSource clears awards, URLs and sync state for one source but must
// leave a different source's records alone.
func TestForgetSourceClearsOnlyThatSource(t *testing.T) {
	s := newStore(t)
	if err := s.ReplaceAwards("42", SourceIAFD, []Award{
		{Organization: "AVN", AwardName: "Best New Starlet", Year: 2015},
	}); err != nil {
		t.Fatalf("iafd awards: %v", err)
	}
	if err := s.ReplaceAwards("42", SourceAIA, []Award{
		{Organization: "XBIZ", AwardName: "Best Actress", Year: 2016},
	}); err != nil {
		t.Fatalf("aia awards: %v", err)
	}
	if err := s.SetURL("42", SourceIAFD, "https://iafd.test"); err != nil {
		t.Fatalf("SetURL iafd: %v", err)
	}
	if err := s.SetURL("42", SourceAIA, "https://aia.test"); err != nil {
		t.Fatalf("SetURL aia: %v", err)
	}
	if err := s.MarkSynced("42", SourceIAFD, time.Now()); err != nil {
		t.Fatalf("MarkSynced iafd: %v", err)
	}

	if err := s.ForgetSource("42", SourceIAFD); err != nil {
		t.Fatalf("ForgetSource: %v", err)
	}

	if got, _ := s.AwardsBySource("42", SourceIAFD); len(got) != 0 {
		t.Errorf("iafd awards survived: %v", got)
	}
	if _, err := s.URL("42", SourceIAFD); !errors.Is(err, ErrNotFound) {
		t.Errorf("iafd url survived: %v", err)
	}
	if _, err := s.State("42", SourceIAFD); !errors.Is(err, ErrNotFound) {
		t.Errorf("iafd sync state survived: %v", err)
	}
	if got, _ := s.AwardsBySource("42", SourceAIA); len(got) != 1 {
		t.Errorf("aia awards lost: %v", got)
	}
	if _, err := s.URL("42", SourceAIA); err != nil {
		t.Errorf("aia url lost: %v", err)
	}
}

// ForgetSource must reject an unknown source before touching the database.
func TestForgetSourceRejectsUnknownSource(t *testing.T) {
	s := newStore(t)
	if err := s.ForgetSource("42", Source("evil")); err == nil {
		t.Fatal("ForgetSource accepted an unknown source")
	}
}

// ForgetPerformer on a performer that has no rows must succeed and touch
// nothing it shouldn't.
func TestForgetPerformerOnEmptyDatabaseIsANoOp(t *testing.T) {
	s := newStore(t)
	if err := s.ForgetPerformer("42"); err != nil {
		t.Errorf("ForgetPerformer: %v", err)
	}
}

func TestNullHelpersHandleNullValues(t *testing.T) {
	if got := num(sql.NullInt64{}); got != 0 {
		t.Errorf("num(invalid) = %d, want 0", got)
	}
	if got := num(sql.NullInt64{Int64: 7, Valid: true}); got != 7 {
		t.Errorf("num(valid) = %d, want 7", got)
	}
	if got := nullInt(0); got != nil {
		t.Errorf("nullInt(0) = %v, want nil", got)
	}
	if got := nullInt(5); got != 5 {
		t.Errorf("nullInt(5) = %v, want 5", got)
	}
}

// Awards and AwardsBySource return an empty slice for a performer with no
// rows rather than nil; the JSON serialisation is the same, but the contract
// matters to any consumer that distinguishes.
func TestAwardsReturnsEmptyForUnknownPerformer(t *testing.T) {
	s := newStore(t)
	got, err := s.Awards("nobody")
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want 0 awards", got)
	}
	got, err = s.AwardsBySource("nobody", SourceIAFD)
	if err != nil {
		t.Fatalf("AwardsBySource: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want 0 awards", got)
	}
}

// ReplaceAwards keeps a caller-supplied LastScraped rather than overwriting
// it with the current time. The convenience for tests is the main reason the
// field is exposed.
func TestReplaceAwardsPreservesCallerSuppliedLastScraped(t *testing.T) {
	s := newStore(t)
	stamp := "2020-01-02T03:04:05Z"
	if err := s.ReplaceAwards("42", SourceIAFD, []Award{
		{Organization: "AVN", AwardName: "Best New Starlet", Year: 2015, LastScraped: stamp},
	}); err != nil {
		t.Fatalf("ReplaceAwards: %v", err)
	}
	got, err := s.Awards("42")
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	if got[0].LastScraped != stamp {
		t.Errorf("last_scraped = %q, want the caller-supplied %q", got[0].LastScraped, stamp)
	}
}
