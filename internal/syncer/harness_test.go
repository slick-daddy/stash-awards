package syncer

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/fetch"
	"github.com/slick-daddy/stash-awards/internal/protocol"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/stashapi"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// fixedNow is the clock every test runs against, so scheduling can be asserted
// exactly.
var fixedNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// fakeProvider stands in for a source. Pages that exist are listed in pages;
// anything else is reported missing, which is how a real source answers an
// unknown performer.
type fakeProvider struct {
	source  store.Source
	guess   string
	pages   map[string][]store.Award
	failURL map[string]error
	matches []sources.Match
	sErr    error

	fetched  []string
	searches int
}

func (f *fakeProvider) prefix() string { return "https://" + string(f.source) + ".test/" }

func (f *fakeProvider) ID() store.Source { return f.source }

func (f *fakeProvider) GuessURL(string) (string, bool) {
	if f.guess == "" {
		return "", false
	}
	return f.guess, true
}

func (f *fakeProvider) RecogniseURL(raw string) (string, bool) {
	if strings.HasPrefix(raw, f.prefix()) {
		return raw, true
	}
	return "", false
}

func (f *fakeProvider) Search(_ context.Context, _ string) ([]sources.Match, error) {
	f.searches++
	return f.matches, f.sErr
}

func (f *fakeProvider) Awards(_ context.Context, pageURL string) ([]store.Award, error) {
	f.fetched = append(f.fetched, pageURL)
	if err, ok := f.failURL[pageURL]; ok {
		return nil, err
	}
	awards, ok := f.pages[pageURL]
	if !ok {
		return nil, fmt.Errorf("no page at %s: %w", pageURL, fetch.ErrNotFound)
	}
	return awards, nil
}

// fakeStash stands in for the Stash GraphQL API.
type fakeStash struct {
	performers []stashapi.Performer
	err        error
	pages      [][]stashapi.Performer
	lookups    int
}

func (f *fakeStash) Performer(_ context.Context, id string) (*stashapi.Performer, error) {
	f.lookups++
	if f.err != nil {
		return nil, f.err
	}
	for i := range f.performers {
		if f.performers[i].ID == id {
			return &f.performers[i], nil
		}
	}
	return nil, fmt.Errorf("performer %s: %w", id, stashapi.ErrNotFound)
}

func (f *fakeStash) Performers(_ context.Context, page, _ int) ([]stashapi.Performer, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	if page < 1 || page > len(f.pages) {
		return nil, len(f.performers), nil
	}
	return f.pages[page-1], len(f.performers), nil
}

// harness bundles a syncer with the doubles behind it.
type harness struct {
	*Syncer
	store  *store.Store
	stash  *fakeStash
	iafd   *fakeProvider
	aia    *fakeProvider
	slept  []time.Duration
	nowVal time.Time
}

func newHarness(t *testing.T, settings config.Settings, performers ...stashapi.Performer) *harness {
	t.Helper()

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	h := &harness{
		store:  st,
		stash:  &fakeStash{performers: performers, pages: [][]stashapi.Performer{performers}},
		iafd:   &fakeProvider{source: store.SourceIAFD, pages: map[string][]store.Award{}},
		aia:    &fakeProvider{source: store.SourceAIA, pages: map[string][]store.Award{}},
		nowVal: fixedNow,
	}
	h.Syncer = New(Deps{
		Store: st,
		Stash: h.stash,
		Providers: map[store.Source]sources.Provider{
			store.SourceIAFD: h.iafd,
			store.SourceAIA:  h.aia,
		},
		Settings: settings,
		Log:      protocol.NewLogTo(&strings.Builder{}),
		Now:      func() time.Time { return h.nowVal },
		Sleep: func(_ context.Context, d time.Duration) error {
			h.slept = append(h.slept, d)
			return nil
		},
	})
	return h
}

// iafdOnly enables just one source, which keeps single-source assertions honest.
func iafdOnly() config.Settings {
	s := config.Default()
	s.AIAEnabled = false
	return s
}

func award(org, name string, year int) store.Award {
	return store.Award{Organization: org, AwardName: name, Year: year, Result: store.ResultWon}
}

func performer(id, name string, urls ...string) stashapi.Performer {
	return stashapi.Performer{ID: id, Name: name, URLs: urls}
}
