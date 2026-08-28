package syncer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/stashapi"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// The scheduled task must only touch what is actually due; the whole-library
// walk is a separate, explicit action.
func TestSyncDueOnlyTouchesDuePairs(t *testing.T) {
	duePage := "https://iafd.test/person/due"
	freshPage := "https://iafd.test/person/fresh"
	h := newHarness(t, iafdOnly(),
		performer("1", "Due Performer", duePage),
		performer("2", "Fresh Performer", freshPage),
	)
	h.iafd.pages[duePage] = []store.Award{award("AVN", "Best Actress", 2019)}
	h.iafd.pages[freshPage] = []store.Award{award("AVN", "Best Actor", 2019)}

	// One performer came due an hour ago, the other is not due for a week.
	if err := h.store.MarkSynced("1", store.SourceIAFD, fixedNow.Add(-time.Hour)); err != nil {
		t.Fatalf("mark due: %v", err)
	}
	if err := h.store.MarkSynced("2", store.SourceIAFD, fixedNow.Add(7*24*time.Hour)); err != nil {
		t.Fatalf("mark fresh: %v", err)
	}

	summary, err := h.SyncDue(context.Background(), 50)
	if err != nil {
		t.Fatalf("SyncDue: %v", err)
	}
	if summary.Synced != 1 || summary.Performers != 1 {
		t.Errorf("summary = %+v, want exactly one performer synced", summary)
	}
	if len(h.iafd.fetched) != 1 || h.iafd.fetched[0] != duePage {
		t.Errorf("fetched %v, want only the due performer's page", h.iafd.fetched)
	}
}

// A performer deleted between the due query and the sync is not a failure: the
// cleanup hook may simply not have run yet.
func TestSyncDueSkipsAPerformerStashNoLongerHas(t *testing.T) {
	h := newHarness(t, iafdOnly())
	if err := h.store.MarkSynced("999", store.SourceIAFD, fixedNow.Add(-time.Hour)); err != nil {
		t.Fatalf("mark due: %v", err)
	}

	summary, err := h.SyncDue(context.Background(), 50)
	if err != nil {
		t.Fatalf("SyncDue: %v", err)
	}
	if summary.Performers != 0 || summary.Failed != 0 {
		t.Errorf("summary = %+v, want the missing performer quietly skipped", summary)
	}
}

func TestSyncDueReportsNothingToDo(t *testing.T) {
	h := newHarness(t, iafdOnly())

	summary, err := h.SyncDue(context.Background(), 0)
	if err != nil {
		t.Fatalf("SyncDue: %v", err)
	}
	if summary.Performers != 0 || len(summary.Results) != 0 {
		t.Errorf("summary = %+v, want an empty run", summary)
	}
}

// A whole-library sync pauses between batches, because sustained scraping is
// what gets a user blocked.
func TestSyncAllPagesTheLibraryAndPausesBetweenBatches(t *testing.T) {
	var all []stashapi.Performer
	for i := 0; i < BatchSize+5; i++ {
		page := fmt.Sprintf("https://iafd.test/person/%d", i)
		all = append(all, performer(fmt.Sprint(i), fmt.Sprintf("Performer %d", i), page))
	}

	h := newHarness(t, iafdOnly(), all...)
	h.stash.pages = [][]stashapi.Performer{all[:BatchSize], all[BatchSize:]}
	for i := range all {
		h.iafd.pages[all[i].URLs[0]] = []store.Award{award("AVN", "Best Actress", 2000+i%20)}
	}

	summary, err := h.SyncAll(context.Background(), false)
	if err != nil {
		t.Fatalf("SyncAll: %v", err)
	}
	if summary.Performers != len(all) || summary.Synced != len(all) {
		t.Errorf("summary = %+v, want every performer synced", summary)
	}
	if len(h.slept) != 1 || h.slept[0] != BatchPause {
		t.Errorf("pauses = %v, want one pause of %s between the two batches", h.slept, BatchPause)
	}
}

// An interrupted run must be resumable: a second pass costs no requests.
func TestSyncAllSkipsWhatIsAlreadyFresh(t *testing.T) {
	page := "https://iafd.test/person/angela"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", page))
	h.iafd.pages[page] = []store.Award{award("AVN", "Best Actress", 2019)}

	if _, err := h.SyncAll(context.Background(), false); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	summary, err := h.SyncAll(context.Background(), false)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if summary.Skipped != 1 || summary.Synced != 0 {
		t.Errorf("summary = %+v, want the second pass to skip", summary)
	}
	if len(h.iafd.fetched) != 1 {
		t.Errorf("fetched %d times across two passes, want 1", len(h.iafd.fetched))
	}
}

func TestSyncAllRefusesToRunWithNoSources(t *testing.T) {
	settings := iafdOnly()
	settings.IAFDEnabled = false
	h := newHarness(t, settings, performer("1", "Angela Test"))

	if _, err := h.SyncAll(context.Background(), false); err == nil {
		t.Fatal("SyncAll succeeded with every source disabled")
	}
}

func TestSyncAllStopsWhenCancelled(t *testing.T) {
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := h.SyncAll(ctx, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// Linking is the user correcting the plugin, so the URL is verified by fetching
// it rather than taken on trust.
func TestLinkStoresTheURLAndSyncsImmediately(t *testing.T) {
	page := "https://iafd.test/person/chosen"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test"))
	h.iafd.pages[page] = []store.Award{award("AVN", "Best Actress", 2019)}

	res, err := h.Link(context.Background(), "1", store.SourceIAFD, page)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if res.Status != StatusSynced || res.Awards != 1 {
		t.Errorf("result = %+v", res)
	}
	stored, err := h.store.URL("1", store.SourceIAFD)
	if err != nil || stored.URL != page {
		t.Errorf("stored URL = %+v, %v", stored, err)
	}
}

func TestLinkRejectsAURLFromAnotherSite(t *testing.T) {
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test"))

	if _, err := h.Link(context.Background(), "1", store.SourceIAFD, "https://example.com/whoever"); err == nil {
		t.Fatal("Link accepted a URL from another site")
	}
	if _, err := h.store.URL("1", store.SourceIAFD); !errors.Is(err, store.ErrNotFound) {
		t.Error("a rejected URL was stored anyway")
	}
}

// Unlinking has to clear the URL as well as the awards, or the next sync would
// resurrect the link the user just rejected.
func TestUnlinkForgetsTheSourceEntirely(t *testing.T) {
	page := "https://iafd.test/person/angela"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", page))
	h.iafd.pages[page] = []store.Award{award("AVN", "Best Actress", 2019)}
	if _, err := h.SyncPerformerID(context.Background(), "1", false); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if err := h.Unlink("1", store.SourceIAFD); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if _, err := h.store.URL("1", store.SourceIAFD); !errors.Is(err, store.ErrNotFound) {
		t.Error("the URL survived an unlink")
	}
	awards, err := h.store.AwardsBySource("1", store.SourceIAFD)
	if err != nil || len(awards) != 0 {
		t.Errorf("awards = %d, %v; want none", len(awards), err)
	}
	if _, err := h.store.State("1", store.SourceIAFD); !errors.Is(err, store.ErrNotFound) {
		t.Error("the sync schedule survived an unlink")
	}
}

// Unlinking one source must not disturb the other.
func TestUnlinkLeavesTheOtherSourceAlone(t *testing.T) {
	h := newHarness(t, config.Default(), performer("1", "Angela Test",
		"https://iafd.test/person/angela", "https://aia.test/angela-test/"))
	h.iafd.pages["https://iafd.test/person/angela"] = []store.Award{award("AVN", "A", 2019)}
	h.aia.pages["https://aia.test/angela-test/"] = []store.Award{award("XBIZ", "B", 2020)}
	if _, err := h.SyncPerformerID(context.Background(), "1", false); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if err := h.Unlink("1", store.SourceIAFD); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	awards, err := h.store.AwardsBySource("1", store.SourceAIA)
	if err != nil || len(awards) != 1 {
		t.Errorf("aia awards = %d, %v; want 1 left alone", len(awards), err)
	}
}
