package iafd

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
	"github.com/slick-daddy/stash-awards/internal/store"
)

// fixtureTransport answers requests for www.iafd.com from testdata, so the
// provider can be exercised end to end without a seam for the base URL and
// without touching the network.
type fixtureTransport struct {
	t        *testing.T
	requests []string
}

func (ft *fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ft.t.Helper()
	if r.URL.Host != Host {
		ft.t.Fatalf("request escaped to %s", r.URL.Host)
	}
	ft.requests = append(ft.requests, r.URL.String())

	rec := httptest.NewRecorder()
	switch {
	case strings.HasPrefix(r.URL.Path, "/ramesearch.asp"):
		writeFixture(ft.t, rec, "testdata/search.html")
	case strings.Contains(r.URL.Path, "perfid="):
		// IAFD answers a pretty performer URL with a redirect to the id= form.
		rec.Header().Set("Location", "/person.rme/id=9d655dea-1397-458e-9416-568e23bc8b9c")
		rec.WriteHeader(http.StatusMovedPermanently)
	case strings.Contains(r.URL.Path, "person.rme"):
		writeFixture(ft.t, rec, "testdata/performer.html")
	default:
		rec.WriteHeader(http.StatusNotFound)
	}

	resp := rec.Result()
	resp.Request = r
	return resp, nil
}

func writeFixture(t *testing.T, rec *httptest.ResponseRecorder, path string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	rec.Header().Set("Content-Type", "text/html; charset=utf-8")
	rec.Write(body)
}

func testProvider(t *testing.T) (*Provider, *fixtureTransport) {
	t.Helper()
	ft := &fixtureTransport{t: t}
	client := fetch.New(fetch.Options{
		HTTPClient: &http.Client{Transport: ft},
		Sleep:      func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
	})
	return New(client), ft
}

func TestProviderIDIsTheStoredSourceValue(t *testing.T) {
	if got := (&Provider{}).ID(); got != store.SourceIAFD {
		t.Errorf("ID = %q, want %q", got, store.SourceIAFD)
	}
}

func TestProviderAwardsStampsTheSource(t *testing.T) {
	p, ft := testProvider(t)

	awards, err := p.Awards(context.Background(), testPageURL)
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	if len(awards) == 0 {
		t.Fatal("no awards parsed")
	}
	for _, a := range awards {
		if a.Source != store.SourceIAFD {
			t.Fatalf("award source = %q, want %q", a.Source, store.SourceIAFD)
		}
	}
	if len(ft.requests) != 1 {
		t.Errorf("made %d requests, want 1", len(ft.requests))
	}
}

// A pretty URL redirects to the canonical one; the award records must point at
// where the data actually came from, not at the address that was asked for.
func TestProviderAwardsRecordsTheCanonicalURLAfterRedirect(t *testing.T) {
	p, _ := testProvider(t)

	pretty := "https://www.iafd.com/person.rme/perfid=testguy/gender=m/test-guy.htm"
	awards, err := p.Awards(context.Background(), pretty)
	if err != nil {
		t.Fatalf("Awards: %v", err)
	}
	if len(awards) == 0 {
		t.Fatal("no awards parsed")
	}
	if awards[0].SourceURL != testPageURL {
		t.Errorf("source url = %q, want the canonical %q", awards[0].SourceURL, testPageURL)
	}
}

func TestProviderAwardsRejectsForeignURLs(t *testing.T) {
	p, ft := testProvider(t)
	if _, err := p.Awards(context.Background(), "https://example.com/person.rme/id=x"); err == nil {
		t.Fatal("Awards accepted a URL from another site")
	}
	if len(ft.requests) != 0 {
		t.Errorf("made %d requests for a rejected URL, want 0", len(ft.requests))
	}
}

func TestProviderSearchReturnsMatches(t *testing.T) {
	p, ft := testProvider(t)

	matches, err := p.Search(context.Background(), "Test Performer")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3", len(matches))
	}
	if len(ft.requests) != 1 || !strings.Contains(ft.requests[0], "searchstring=Test+Performer") {
		t.Errorf("requests = %v", ft.requests)
	}
}

func TestProviderSearchRejectsAnEmptyName(t *testing.T) {
	p, ft := testProvider(t)
	if _, err := p.Search(context.Background(), "   "); err == nil {
		t.Fatal("Search accepted an empty name")
	}
	if len(ft.requests) != 0 {
		t.Errorf("made %d requests for an empty name, want 0", len(ft.requests))
	}
}

func TestProviderAwardsSurfacesAMissingPage(t *testing.T) {
	client := fetch.New(fetch.Options{HTTPClient: &http.Client{Transport: notFoundTransport{}}})
	p := New(client)

	_, err := p.Awards(context.Background(), testPageURL)
	if !errors.Is(err, fetch.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// notFoundTransport answers everything with a 404.
type notFoundTransport struct{}

func (notFoundTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusNotFound)
	resp := rec.Result()
	resp.Request = r
	return resp, nil
}
