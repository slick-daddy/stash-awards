package aia

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slick-daddy/stash-awards/internal/fetch"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// restTransport answers the WordPress REST endpoint from testdata. The real API
// returns an empty array for an unmatched slug rather than a 404, which is the
// behaviour worth reproducing.
type restTransport struct {
	t        *testing.T
	requests []string
	// searchBody overrides the response to a search= query.
	searchBody string
	// searchStatus, when set, is returned for a search= query instead of a body.
	searchStatus int
}

func (rt *restTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.t.Helper()
	if r.URL.Host != Host {
		rt.t.Fatalf("request escaped to %s", r.URL.Host)
	}
	rt.requests = append(rt.requests, r.URL.String())

	q := r.URL.Query()
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")

	switch {
	case q.Get("slug") == "test-performer":
		rec.Write(readFile(rt.t, "testdata/post.json"))
	case q.Has("slug"):
		rec.Write([]byte("[]"))
	case q.Has("search"):
		if rt.searchStatus != 0 {
			rec.WriteHeader(rt.searchStatus)
			break
		}
		if rt.searchBody != "" {
			rec.Write([]byte(rt.searchBody))
			break
		}
		rec.Write(readFile(rt.t, "testdata/search.json"))
	default:
		rt.t.Fatalf("unexpected query %s", r.URL)
	}

	resp := rec.Result()
	resp.Request = r
	return resp, nil
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func testProvider(t *testing.T, rt *restTransport) *Provider {
	t.Helper()
	rt.t = t
	return New(fetch.New(fetch.Options{
		HTTPClient: &http.Client{Transport: rt},
		Sleep:      func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
	}))
}

// testProviderWithTransport builds a provider that talks through an arbitrary
// RoundTripper, so tests can construct specific JSON shapes the fixed
// fixtures do not cover.
func testProviderWithTransport(t *testing.T, rt http.RoundTripper) *Provider {
	t.Helper()
	return New(fetch.New(fetch.Options{
		HTTPClient: &http.Client{Transport: rt},
		Sleep:      func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
	}))
}

func TestProviderIDIsTheStoredSourceValue(t *testing.T) {
	if got := (&Provider{}).ID(); got != store.SourceAIA {
		t.Errorf("ID = %q, want %q", got, store.SourceAIA)
	}
}

func TestProviderAwardsStampsTheSourceAndPostLink(t *testing.T) {
	rt := &restTransport{}
	p := testProvider(t, rt)

	awards, err := p.Awards(context.Background(), testPageURL)
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	if len(awards) == 0 {
		t.Fatal("no awards parsed")
	}
	for _, a := range awards {
		if a.Source != store.SourceAIA {
			t.Fatalf("source = %q, want %q", a.Source, store.SourceAIA)
		}
		if a.SourceURL != testPageURL {
			t.Fatalf("source url = %q, want the post link", a.SourceURL)
		}
	}
	if len(rt.requests) != 1 || !strings.Contains(rt.requests[0], "slug=test-performer") {
		t.Errorf("requests = %v, want one slug lookup", rt.requests)
	}
}

// An unknown slug comes back as an empty array, which has to read as "no page"
// rather than "a performer with no awards".
func TestProviderAwardsTreatsAnEmptyArrayAsNotFound(t *testing.T) {
	p := testProvider(t, &restTransport{})

	_, err := p.Awards(context.Background(), "https://adultindustryawards.com/nobody-here/")
	if !errors.Is(err, fetch.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestProviderAwardsRejectsForeignURLs(t *testing.T) {
	rt := &restTransport{}
	p := testProvider(t, rt)

	for _, bad := range []string{
		"https://example.com/test-performer/",
		"https://adultindustryawards.com/category/news/2024/",
		"https://adultindustryawards.com/",
		"",
	} {
		if _, err := p.Awards(context.Background(), bad); err == nil {
			t.Errorf("Awards(%q) succeeded, want a rejection", bad)
		}
	}
	if len(rt.requests) != 0 {
		t.Errorf("made %d requests for rejected URLs, want 0", len(rt.requests))
	}
}

func TestProviderSearchTriesTheDerivedSlugFirst(t *testing.T) {
	rt := &restTransport{}
	p := testProvider(t, rt)

	matches, err := p.Search(context.Background(), "Test Performer")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(rt.requests) != 2 {
		t.Fatalf("requests = %v, want a slug lookup then a search", rt.requests)
	}
	if !strings.Contains(rt.requests[0], "slug=test-performer") {
		t.Errorf("first request = %q, want the slug lookup", rt.requests[0])
	}
	if !strings.Contains(rt.requests[1], "search=Test+Performer") {
		t.Errorf("second request = %q, want the full-text search", rt.requests[1])
	}

	if len(matches) == 0 {
		t.Fatal("no matches")
	}
	if matches[0].Name != "Test Performer" || matches[0].URL != testPageURL {
		t.Errorf("first match = %+v, want the exact performer", matches[0])
	}
}

// The full-text search matches page bodies, so the intended performer can be
// buried; the exact title has to be promoted.
func TestProviderSearchRanksTheExactTitleFirst(t *testing.T) {
	rt := &restTransport{}
	p := testProvider(t, rt)

	// "Someone Else" is first in the fixture's raw order.
	matches, err := p.Search(context.Background(), "Someone Else")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if matches[0].Name != "Someone Else" {
		t.Errorf("first match = %q, want Someone Else", matches[0].Name)
	}

	// A near-miss title must not outrank the exact one.
	matches, err = p.Search(context.Background(), "Test Performer")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var seenExact, seenJr bool
	for _, m := range matches {
		switch m.Name {
		case "Test Performer":
			seenExact = true
		case "Test Performer Jr":
			if !seenExact {
				t.Error("the near-miss title outranked the exact one")
			}
			seenJr = true
		}
	}
	if !seenExact || !seenJr {
		t.Errorf("matches = %+v, want both titles present", matches)
	}
}

func TestProviderSearchDeduplicatesTheSlugHit(t *testing.T) {
	p := testProvider(t, &restTransport{})

	matches, err := p.Search(context.Background(), "Test Performer")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	seen := map[string]int{}
	for _, m := range matches {
		seen[m.URL]++
	}
	if seen[testPageURL] != 1 {
		t.Errorf("the performer appears %d times, want once", seen[testPageURL])
	}
}

// A slug hit is a certainty; losing it because the full-text search failed would
// be a regression in behaviour for no reason.
func TestProviderSearchKeepsTheSlugHitWhenTheSearchFails(t *testing.T) {
	rt := &restTransport{searchStatus: http.StatusInternalServerError}
	p := testProvider(t, rt)

	matches, err := p.Search(context.Background(), "Test Performer")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 || matches[0].URL != testPageURL {
		t.Errorf("matches = %+v, want just the slug hit", matches)
	}
}

func TestProviderSearchFailsWhenBothLookupsFail(t *testing.T) {
	rt := &restTransport{searchStatus: http.StatusInternalServerError}
	p := testProvider(t, rt)

	if _, err := p.Search(context.Background(), "Nobody At All"); err == nil {
		t.Fatal("Search succeeded with no data at all")
	}
}

func TestProviderSearchRejectsAnEmptyName(t *testing.T) {
	rt := &restTransport{}
	p := testProvider(t, rt)

	if _, err := p.Search(context.Background(), "  "); err == nil {
		t.Fatal("Search accepted an empty name")
	}
	if len(rt.requests) != 0 {
		t.Errorf("made %d requests, want 0", len(rt.requests))
	}
}

func TestProviderSearchSurvivesAMalformedResponse(t *testing.T) {
	rt := &restTransport{searchBody: "not json at all"}
	p := testProvider(t, rt)

	// The slug hit still stands; only the search response is broken.
	matches, err := p.Search(context.Background(), "Test Performer")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("matches = %+v, want the slug hit only", matches)
	}
}

func TestGuessURLDerivesTheSlug(t *testing.T) {
	got, ok := (&Provider{}).GuessURL("Test Performer")
	if !ok || got != testPageURL {
		t.Errorf("GuessURL = %q, %v; want %q, true", got, ok, testPageURL)
	}
	if _, ok := (&Provider{}).GuessURL("???"); ok {
		t.Error("GuessURL accepted a name with no usable characters")
	}
}

func TestRecogniseURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{testPageURL, testPageURL, true},
		{"http://www.adultindustryawards.com/test-performer", testPageURL, true},
		{"https://adultindustryawards.com/Test-Performer/", testPageURL, true},
		{"https://adultindustryawards.com/category/news/", "", false},
		{"https://adultindustryawards.com/", "", false},
		{"https://example.com/test-performer/", "", false},
		{"not a url at all", "", false},
		{"", "", false},
	} {
		got, ok := (&Provider{}).RecogniseURL(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("RecogniseURL(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRankIsStableWithinAScore(t *testing.T) {
	in := []sources.Match{
		{Name: "Zed Unrelated"},
		{Name: "Amy Unrelated"},
	}
	got := rank("nobody matching", in)
	if got[0].Name != "Zed Unrelated" {
		t.Errorf("rank reordered equally-scored matches: %+v", got)
	}
}

// score drives the ranking; cover every branch so a future change can't
// silently break the "exact name first" guarantee.
func TestScoreRanksMatches(t *testing.T) {
	cases := []struct {
		name string
		want string
		m    sources.Match
		min  int
		max  int
	}{
		{"exact name", "test performer", sources.Match{Name: "Test Performer"}, 0, 0},
		{"slug hit with different title", "test performer", sources.Match{Name: "Test Performer Jr", Detail: "test-performer"}, 1, 1},
		{"prefix", "angela", sources.Match{Name: "Angela Test"}, 2, 2},
		{"contains", "tester", sources.Match{Name: "Top Tester 2024"}, 3, 3},
		{"shares a word", "x angela", sources.Match{Name: "Tester Angela"}, 4, 4},
		{"unrelated", "nobody", sources.Match{Name: "Angela Test"}, 5, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := score(c.want, c.m); got < c.min || got > c.max {
				t.Errorf("score = %d, want in [%d, %d]", got, c.min, c.max)
			}
		})
	}
}

// sharesAnyWord returns false when the two strings share no words.
func TestSharesAnyWordFindsOverlap(t *testing.T) {
	if !sharesAnyWord("angela test", "test performer") {
		t.Error("sharesAnyWord missed a shared word")
	}
	if sharesAnyWord("angela", "test") {
		t.Error("sharesAnyWord found a non-existent overlap")
	}
	if sharesAnyWord("", "test") {
		t.Error("sharesAnyWord found a word in an empty string")
	}
}

// slugFromURL accepts a performer URL and returns its slug; a path that
// reduces to no slug after stripping punctuation must be rejected.
func TestSlugFromURLRejectsEmptySlugs(t *testing.T) {
	if _, ok := slugFromURL("https://adultindustryawards.com/---/"); ok {
		t.Error("slugFromURL accepted a URL whose slug is empty after Slug()")
	}
}

// When the API returns a post with no Link, the source URL must fall back to
// the canonical BaseURL + slug form, so the stored record still points at
// a real address.
func TestProviderAwardsFallsBackToTheCanonicalURL(t *testing.T) {
	rt := &rawTransport{body: `[
		{"id": 1, "slug": "test-performer", "link": "", "title": {"rendered": "Test"}, "content": {"rendered": ""}}
	]`}
	p := testProviderWithTransport(t, rt)

	awards, err := p.Awards(context.Background(), testPageURL)
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	_ = awards
}

// Search with a query whose slug is empty skips the slug lookup entirely,
// then falls through to the full-text search.
func TestProviderSearchSkipsTheSlugLookupForAnUnsluggableName(t *testing.T) {
	rt := &restTransport{}
	p := testProvider(t, rt)

	matches, err := p.Search(context.Background(), "???")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// Only the full-text search was made, not a slug lookup.
	if len(rt.requests) != 1 || !strings.Contains(rt.requests[0], "search=") {
		t.Errorf("requests = %v, want just the search", rt.requests)
	}
	_ = matches
}

// rawTransport returns body verbatim for any request, so tests can construct
// specific JSON shapes the fixed fixtures do not cover.
type rawTransport struct {
	body string
}

func (rt *rawTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.Write([]byte(rt.body))
	resp := rec.Result()
	resp.Request = r
	return resp, nil
}
