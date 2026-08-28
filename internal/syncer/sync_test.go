package syncer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/stashapi"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// Step one of performer matching: a link already on the Stash performer is the
// cheapest and most trustworthy answer, and must not trigger a search.
func TestSyncUsesTheURLAlreadyOnThePerformer(t *testing.T) {
	page := "https://iafd.test/person/angela"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", "https://twitter.com/x", page))
	h.iafd.pages[page] = []store.Award{award("AVN", "Best Actress", 2019)}

	results, err := h.SyncPerformerID(context.Background(), "1", false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one per enabled source", results)
	}
	res := results[0]
	if res.Status != StatusSynced || res.Origin != OriginPerformer || res.Awards != 1 {
		t.Errorf("result = %+v", res)
	}
	if h.iafd.searches != 0 {
		t.Errorf("searched %d times despite a usable URL", h.iafd.searches)
	}

	// The URL is remembered so later syncs skip the scan.
	stored, err := h.store.URL("1", store.SourceIAFD)
	if err != nil || stored.URL != page {
		t.Errorf("stored URL = %+v, %v; want %s", stored, err, page)
	}
	awards, err := h.store.AwardsBySource("1", store.SourceIAFD)
	if err != nil || len(awards) != 1 {
		t.Errorf("stored %d awards, %v; want 1", len(awards), err)
	}
	// The next sync is a full interval away.
	state, err := h.store.State("1", store.SourceIAFD)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	next, err := store.ParseTime(state.NextSyncAfter)
	if err != nil {
		t.Fatalf("parse next: %v", err)
	}
	if want := fixedNow.Add(h.Settings().SyncInterval()); !next.Equal(want) {
		t.Errorf("next sync = %v, want %v", next, want)
	}
}

// A URL stored here was either resolved before or set by hand, so it outranks
// whatever the performer record happens to carry.
func TestSyncPrefersTheStoredURL(t *testing.T) {
	stored := "https://iafd.test/person/chosen"
	onPerformer := "https://iafd.test/person/other"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", onPerformer))
	h.iafd.pages[stored] = []store.Award{award("AVN", "Best Actress", 2019)}
	h.iafd.pages[onPerformer] = []store.Award{award("XBIZ", "Wrong Person", 2019)}
	if err := h.store.SetURL("1", store.SourceIAFD, stored); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	results, err := h.SyncPerformerID(context.Background(), "1", false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if results[0].URL != stored || results[0].Origin != OriginStored {
		t.Errorf("result = %+v, want the stored URL", results[0])
	}
}

// A name-derived URL costs one request; when the page is not there the search is
// the real answer, and the failed guess must not be recorded as an error.
func TestSyncFallsBackFromAMissingGuessToASearch(t *testing.T) {
	found := "https://aia.test/angela-test/"
	settings := iafdOnly()
	settings.IAFDEnabled = false
	settings.AIAEnabled = true

	h := newHarness(t, settings, performer("1", "Angela Test"))
	h.aia.guess = "https://aia.test/angela-test-wrong/"
	h.aia.matches = []sources.Match{
		{Name: "Someone Else", URL: "https://aia.test/someone-else/"},
		{Name: "Angela Test", URL: found},
	}
	h.aia.pages[found] = []store.Award{award("AVN", "Best Actress", 2019)}

	results, err := h.SyncPerformerID(context.Background(), "1", false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	res := results[0]
	if res.Status != StatusSynced || res.Origin != OriginSearch || res.URL != found {
		t.Errorf("result = %+v", res)
	}
	if h.aia.searches != 1 {
		t.Errorf("searched %d times, want 1", h.aia.searches)
	}
	state, err := h.store.State("1", store.SourceAIA)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.Error != "" {
		t.Errorf("recorded an error for a guess that simply missed: %q", state.Error)
	}
}

// A wrong link silently attributes someone else's awards, so anything short of
// an exact name match is left to a person.
func TestSyncStopsAtAnAmbiguousSearch(t *testing.T) {
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test"))
	h.iafd.matches = []sources.Match{
		{Name: "Angela Testing", URL: "https://iafd.test/person/a"},
		{Name: "Angie Test", URL: "https://iafd.test/person/b"},
	}

	results, err := h.SyncPerformerID(context.Background(), "1", false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	res := results[0]
	if res.Status != StatusAmbiguous || len(res.Candidates) != 2 {
		t.Errorf("result = %+v, want the candidates handed back", res)
	}
	if len(h.iafd.fetched) != 0 {
		t.Errorf("fetched %v despite having no confident match", h.iafd.fetched)
	}
	if _, err := h.store.URL("1", store.SourceIAFD); !errors.Is(err, store.ErrNotFound) {
		t.Error("stored a URL that nobody confirmed")
	}
	// Re-running must not repeat the same fruitless search straight away.
	assertNextSync(t, h, store.SourceIAFD, h.Settings().SyncInterval())
}

func TestSyncReportsUnresolvedWhenTheSourceHasNobody(t *testing.T) {
	h := newHarness(t, iafdOnly(), performer("1", "Nobody At All"))

	results, err := h.SyncPerformerID(context.Background(), "1", false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if results[0].Status != StatusUnresolved {
		t.Errorf("result = %+v, want unresolved", results[0])
	}
	assertNextSync(t, h, store.SourceIAFD, h.Settings().SyncInterval())
}

// An alias is how the same performer appears under a different name on a source.
func TestSyncMatchesOnAnAlias(t *testing.T) {
	page := "https://iafd.test/person/aka"
	h := newHarness(t, iafdOnly(), stashapi.Performer{
		ID: "1", Name: "Angela Test", Aliases: []string{"Testy P"},
	})
	h.iafd.matches = []sources.Match{{Name: "Testy P", URL: page}}
	h.iafd.pages[page] = []store.Award{award("AVN", "Best Actress", 2019)}

	results, err := h.SyncPerformerID(context.Background(), "1", false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if results[0].Status != StatusSynced || results[0].URL != page {
		t.Errorf("result = %+v, want the alias match", results[0])
	}
}

// Most failures are transient, so a retry is scheduled sooner than a normal sync
// but not immediately.
func TestSyncSchedulesAShorterRetryAfterAFailure(t *testing.T) {
	page := "https://iafd.test/person/angela"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", page))
	h.iafd.failURL = map[string]error{page: errors.New("cloudflare said no")}

	results, err := h.SyncPerformerID(context.Background(), "1", false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if results[0].Status != StatusFailed || results[0].Message == "" {
		t.Errorf("result = %+v, want a described failure", results[0])
	}
	state, err := h.store.State("1", store.SourceIAFD)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.Error == "" {
		t.Error("failure was not recorded")
	}
	assertNextSync(t, h, store.SourceIAFD, RetryInterval)
}

func TestSyncSkipsFreshDataUnlessForced(t *testing.T) {
	page := "https://iafd.test/person/angela"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", page))
	h.iafd.pages[page] = []store.Award{award("AVN", "Best Actress", 2019)}

	if _, err := h.SyncPerformerID(context.Background(), "1", false); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	results, err := h.SyncPerformerID(context.Background(), "1", false)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if results[0].Status != StatusSkipped || results[0].Awards != 1 {
		t.Errorf("result = %+v, want a skip that still reports the stored count", results[0])
	}
	if len(h.iafd.fetched) != 1 {
		t.Errorf("fetched %d times, want the second sync to skip the network", len(h.iafd.fetched))
	}

	forced, err := h.SyncPerformerID(context.Background(), "1", true)
	if err != nil {
		t.Fatalf("forced sync: %v", err)
	}
	if forced[0].Status != StatusSynced || len(h.iafd.fetched) != 2 {
		t.Errorf("force did not re-fetch: %+v", forced[0])
	}
}

func TestSyncReportsADisabledSource(t *testing.T) {
	settings := iafdOnly()
	settings.IAFDEnabled = false
	settings.AIAEnabled = true
	h := newHarness(t, settings, performer("1", "Angela Test"))

	res, err := h.SyncSource(context.Background(), &h.stash.performers[0], store.SourceIAFD, false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != StatusDisabled {
		t.Errorf("result = %+v, want disabled", res)
	}
	if len(h.iafd.fetched) != 0 || h.iafd.searches != 0 {
		t.Error("a disabled source was contacted")
	}
}

// assertNextSync checks the scheduled retry is at now+want.
func assertNextSync(t *testing.T, h *harness, source store.Source, want time.Duration) {
	t.Helper()
	state, err := h.store.State("1", source)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	next, err := store.ParseTime(state.NextSyncAfter)
	if err != nil {
		t.Fatalf("parse next sync %q: %v", state.NextSyncAfter, err)
	}
	if expected := fixedNow.Add(want); !next.Equal(expected) {
		t.Errorf("next sync = %v, want %v", next, expected)
	}
}

func TestNormaliseNameIgnoresPunctuationAndSpacing(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"D'Angelo Smith", "DAngelo  Smith"},
		{"J.J. Jones", "JJ Jones"},
		{" Anna-Maria ", "Anna Maria"},
	} {
		if normaliseName(tc.a) != normaliseName(tc.b) {
			t.Errorf("normaliseName(%q) = %q, normaliseName(%q) = %q; want equal",
				tc.a, normaliseName(tc.a), tc.b, normaliseName(tc.b))
		}
	}
	if normaliseName("Angela Test") == normaliseName("Angela Testing") {
		t.Error("distinct names normalised to the same value")
	}
}

// Search passes through to the provider; an unknown source surfaces the
// same error the provider would, before the provider is even consulted.
func TestSearchDelegatesToTheProvider(t *testing.T) {
	h := newHarness(t, iafdOnly())
	h.iafd.matches = []sources.Match{{Name: "Angela", URL: "https://iafd.test/person/a"}}

	matches, err := h.Search(context.Background(), store.SourceIAFD, "Angela")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(matches) != 1 || matches[0].Name != "Angela" {
		t.Errorf("matches = %+v", matches)
	}
}

func TestSearchRejectsAnUnknownSource(t *testing.T) {
	h := newHarness(t, iafdOnly())
	if _, err := h.Search(context.Background(), store.Source("bogus"), "Angela"); err == nil {
		t.Fatal("Search accepted an unknown source")
	}
}

func TestProviderReportsEnabledAndKnown(t *testing.T) {
	h := newHarness(t, iafdOnly())
	if _, ok := h.Provider(store.SourceIAFD); !ok {
		t.Error("IAFD provider should be known and enabled")
	}
	if _, ok := h.Provider(store.Source("nope")); ok {
		t.Error("an unknown source reported a provider")
	}
}

func TestProviderRejectsADisabledSource(t *testing.T) {
	settings := iafdOnly()
	settings.IAFDEnabled = false
	h := newHarness(t, settings)
	if _, ok := h.Provider(store.SourceIAFD); ok {
		t.Error("a disabled source reported a provider")
	}
}

// sleepCtx is the production default and must be exercised directly.
func TestSleepCtxRespectsZeroAndCancellation(t *testing.T) {
	if err := sleepCtx(context.Background(), 0); err != nil {
		t.Errorf("zero duration err = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled ctx err = %v, want context.Canceled", err)
	}
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Errorf("positive duration err = %v, want nil", err)
	}
}

// New must apply sensible defaults when the caller does not supply them.
func TestNewAppliesDefaults(t *testing.T) {
	s := New(Deps{Log: nil})
	if s.log == nil {
		t.Error("Log defaulted to nil")
	}
	if s.now == nil {
		t.Error("Now defaulted to nil")
	}
	if s.sleep == nil {
		t.Error("Sleep defaulted to nil")
	}
}

// A sync with a forced re-fetch must clear the freshness check and re-read
// the page.
func TestSyncSourceForcesARefetch(t *testing.T) {
	page := "https://iafd.test/person/angela"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", page))
	h.iafd.pages[page] = []store.Award{award("AVN", "Best Actress", 2019)}

	if _, err := h.SyncPerformerID(context.Background(), "1", false); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	// Re-same call must skip the network.
	if _, err := h.SyncPerformerID(context.Background(), "1", false); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(h.iafd.fetched) != 1 {
		t.Fatalf("fetched %d times, want 1 (the second sync should skip)", len(h.iafd.fetched))
	}
	if _, err := h.SyncPerformerID(context.Background(), "1", true); err != nil {
		t.Fatalf("forced sync: %v", err)
	}
	if len(h.iafd.fetched) != 2 {
		t.Errorf("forced sync fetched %d times, want 2", len(h.iafd.fetched))
	}
}

// isFresh on a missing performer must return false rather than an error: the
// "no row yet" case is the common path on a fresh install.
func TestIsFreshReportsMissingAsStale(t *testing.T) {
	h := newHarness(t, iafdOnly())
	fresh, err := h.isFresh("nobody", store.SourceIAFD)
	if err != nil {
		t.Fatalf("isFresh: %v", err)
	}
	if fresh {
		t.Error("a missing row was reported as fresh")
	}
}

// skipped must still surface the stored count and URL so the UI has something
// to show without making another request.
func TestSkippedReportsStoredCountsAndURL(t *testing.T) {
	page := "https://iafd.test/person/angela"
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", page))
	if err := h.store.SetURL("1", store.SourceIAFD, page); err != nil {
		t.Fatalf("SetURL: %v", err)
	}
	if err := h.store.ReplaceAwards("1", store.SourceIAFD, []store.Award{award("AVN", "Best Actress", 2019)}); err != nil {
		t.Fatalf("ReplaceAwards: %v", err)
	}

	res, err := h.skipped("1", store.SourceIAFD)
	if err != nil {
		t.Fatalf("skipped: %v", err)
	}
	if res.URL != page || res.Awards != 1 {
		t.Errorf("skipped = %+v, want url+count from the store", res)
	}
}

// The retry interval is bounded by the package RetryInterval. SyncIntervalDays
// has a floor of one day, so the configured interval is always at least as
// long as RetryInterval, and the cap always wins.
func TestRetryIntervalHonoursThePackageCap(t *testing.T) {
	h := newHarness(t, iafdOnly())
	h.settings.SyncIntervalDays = 1
	if got := h.retryInterval(); got != RetryInterval {
		t.Errorf("retry = %v, want RetryInterval %v", got, RetryInterval)
	}
	h.settings.SyncIntervalDays = 365 * 10
	if got := h.retryInterval(); got != RetryInterval {
		t.Errorf("retry = %v, want RetryInterval %v", got, RetryInterval)
	}
}

// SyncPerformerID surfaces stash lookup failures rather than swallowing them,
// so the UI can show a real error rather than silently skipping.
func TestSyncPerformerIDSurfacesStashErrors(t *testing.T) {
	h := newHarness(t, iafdOnly())
	h.stash.err = errors.New("graphql is down")
	if _, err := h.SyncPerformerID(context.Background(), "1", false); err == nil {
		t.Fatal("SyncPerformerID hid a stash error")
	}
}

// SyncPerformer returns one result per enabled source.
func TestSyncPerformerProducesOneResultPerEnabledSource(t *testing.T) {
	settings := config.Default()
	h := newHarness(t, settings, performer("1", "Angela Test",
		"https://iafd.test/person/angela", "https://aia.test/angela-test/"))
	h.iafd.pages["https://iafd.test/person/angela"] = []store.Award{award("AVN", "A", 2019)}
	h.aia.pages["https://aia.test/angela-test/"] = []store.Award{award("XBIZ", "B", 2020)}

	p := &h.stash.performers[0]
	results, err := h.SyncPerformer(context.Background(), p, false)
	if err != nil {
		t.Fatalf("SyncPerformer: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want one per enabled source", len(results))
	}
}

// A successful guess at the source's canonical URL is recorded as OriginGuessed.
func TestSyncUsesAGuessedURLWhenItWorks(t *testing.T) {
	settings := iafdOnly()
	settings.IAFDEnabled = false
	settings.AIAEnabled = true
	h := newHarness(t, settings, performer("1", "Angela Test"))
	guess := "https://aia.test/angela-test/"
	h.aia.guess = guess
	h.aia.pages[guess] = []store.Award{award("AVN", "Best Actress", 2019)}

	res, err := h.SyncSource(context.Background(), &h.stash.performers[0], store.SourceAIA, false)
	if err != nil {
		t.Fatalf("SyncSource: %v", err)
	}
	if res.Status != StatusSynced || res.Origin != OriginGuessed {
		t.Errorf("result = %+v, want guessed origin", res)
	}
	if h.aia.searches != 0 {
		t.Errorf("a working guess should not trigger a search, but searched %d times", h.aia.searches)
	}
}

// A guess that returns a non-NotFound error is treated as a failure rather
// than silently falling through to a search.
func TestSyncReportsAGuessFailure(t *testing.T) {
	settings := iafdOnly()
	settings.IAFDEnabled = false
	settings.AIAEnabled = true
	h := newHarness(t, settings, performer("1", "Angela Test"))
	guess := "https://aia.test/angela-test/"
	h.aia.guess = guess
	h.aia.failURL = map[string]error{guess: errors.New("upstream 503")}

	res, err := h.SyncSource(context.Background(), &h.stash.performers[0], store.SourceAIA, false)
	if err != nil {
		t.Fatalf("SyncSource: %v", err)
	}
	if res.Status != StatusFailed {
		t.Errorf("result = %+v, want a failure", res)
	}
	if h.aia.searches != 0 {
		t.Errorf("a failed guess should not fall through to a search, but searched %d times", h.aia.searches)
	}
}

// A search error is reported as a failure with the search context attached.
func TestSyncReportsASearchError(t *testing.T) {
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test"))
	h.iafd.sErr = errors.New("rate limit exceeded")

	res, err := h.SyncSource(context.Background(), &h.stash.performers[0], store.SourceIAFD, false)
	if err != nil {
		t.Fatalf("SyncSource: %v", err)
	}
	if res.Status != StatusFailed {
		t.Errorf("result = %+v, want a failure", res)
	}
	if !strings.Contains(res.Message, "rate limit exceeded") {
		t.Errorf("result = %+v, want the search error surfaced", res)
	}
}

// An empty NextSyncAfter must be treated as stale: a row with no schedule is
// not "up to date" and is eligible for re-sync.
func TestIsFreshTreatsEmptyScheduleAsStale(t *testing.T) {
	h := newHarness(t, iafdOnly(), performer("1", "Angela Test", "https://iafd.test/person/a"))
	// The default for NextSyncAfter is NULL, which reads as the empty
	// string. After MarkError the column is still NULL.
	if err := h.store.MarkError("1", store.SourceIAFD, "previous failure", fixedNow.Add(-time.Hour)); err != nil {
		t.Fatalf("mark: %v", err)
	}
	fresh, err := h.isFresh("1", store.SourceIAFD)
	if err != nil {
		t.Fatalf("isFresh: %v", err)
	}
	if fresh {
		t.Error("an unscheduled row was reported as fresh")
	}
}
