package iafd

import (
	"os"
	"strings"
	"testing"
)

const searchPageURL = "https://www.iafd.com/ramesearch.asp?searchstring=test+performer&searchtype=comprehensive"

func TestParseSearchReturnsPerformerRowsOnly(t *testing.T) {
	f, err := os.Open("testdata/search.html")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	matches, err := ParseSearch(f, searchPageURL)
	if err != nil {
		t.Fatalf("ParseSearch: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3 (two female, one male): %+v", len(matches), matches)
	}

	first := matches[0]
	if first.Name != "Test Performer" {
		t.Errorf("name = %q", first.Name)
	}
	if first.URL != testPageURL {
		t.Errorf("url = %q, want the canonical id form %q", first.URL, testPageURL)
	}
	if !strings.Contains(first.Detail, "2003") || !strings.Contains(first.Detail, "Testy P") {
		t.Errorf("detail = %q, want the active years and the alias", first.Detail)
	}
	if first.ImageURL == "" {
		t.Error("thumbnail not captured")
	}

	// A row with no alias still reports its active years and nothing more.
	if got := matches[1].Detail; got != "2010–2019" {
		t.Errorf("second detail = %q, want just the years", got)
	}
	// Relative image sources resolve against the results page.
	if !strings.HasPrefix(matches[1].ImageURL, "https://www.iafd.com/") {
		t.Errorf("second thumbnail = %q, want it resolved", matches[1].ImageURL)
	}
}

func TestParseSearchLowercasesUUIDsSoOneRowIsOneMatch(t *testing.T) {
	f, err := os.Open("testdata/search.html")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	matches, err := ParseSearch(f, searchPageURL)
	if err != nil {
		t.Fatalf("ParseSearch: %v", err)
	}
	want := "https://www.iafd.com/person.rme/id=ab12cd34-1397-458e-9416-568e23bc8b9c"
	if matches[1].URL != want {
		t.Errorf("url = %q, want %q", matches[1].URL, want)
	}
}

// Some rows link to the "pretty" performer URL, which has no UUID to canonicalise.
// Dropping those would silently lose search results.
func TestParseSearchKeepsPrettyPerformerURLs(t *testing.T) {
	f, err := os.Open("testdata/search.html")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	matches, err := ParseSearch(f, searchPageURL)
	if err != nil {
		t.Fatalf("ParseSearch: %v", err)
	}
	last := matches[len(matches)-1]
	if last.Name != "Test Guy" {
		t.Fatalf("last match = %q, want Test Guy", last.Name)
	}
	want := "https://www.iafd.com/person.rme/perfid=testguy/gender=m/test-guy.htm"
	if last.URL != want {
		t.Errorf("url = %q, want %q", last.URL, want)
	}
}

func TestSearchURLEncodesTheName(t *testing.T) {
	got := SearchURL("Test Performer")
	if !strings.Contains(got, "searchtype=comprehensive") {
		t.Errorf("SearchURL = %q, missing the search type", got)
	}
	if !strings.Contains(got, "searchstring=Test+Performer") {
		t.Errorf("SearchURL = %q, name not encoded", got)
	}
}

func TestRecogniseURL(t *testing.T) {
	canonical := "https://www.iafd.com/person.rme/id=9d655dea-1397-458e-9416-568e23bc8b9c"
	pretty := "https://www.iafd.com/person.rme/perfid=testguy/gender=m/test-guy.htm"

	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{canonical, canonical, true},
		{"http://iafd.com/person.rme/id=9D655DEA-1397-458E-9416-568E23BC8B9C", canonical, true},
		{"https://www.iafd.com/person.rme/id=9d655dea-1397-458e-9416-568e23bc8b9c/awards", canonical, true},
		{pretty, pretty, true},
		{"https://www.iafd.com/title.rme/id=bc17b724-205f-4573-8b10-7ceb629809e4", "", false},
		{"https://example.com/person.rme/id=9d655dea-1397-458e-9416-568e23bc8b9c", "", false},
		{"https://twitter.com/someone", "", false},
		{"", "", false},
		{"   ", "", false},
	} {
		got, ok := (&Provider{}).RecogniseURL(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("RecogniseURL(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestGuessURLIsUnsupported(t *testing.T) {
	if _, ok := (&Provider{}).GuessURL("Test Performer"); ok {
		t.Error("IAFD URLs cannot be derived from a name, but GuessURL claimed otherwise")
	}
}
