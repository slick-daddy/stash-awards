package store

import (
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
}
