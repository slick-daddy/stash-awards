package iafd

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/slick-daddy/stash-awards/internal/store"
)

const testPageURL = "https://www.iafd.com/person.rme/id=9d655dea-1397-458e-9416-568e23bc8b9c"

func loadAwards(t *testing.T) []store.Award {
	t.Helper()
	f, err := os.Open("testdata/performer.html")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	awards, err := ParseAwards(f, testPageURL)
	if err != nil {
		t.Fatalf("ParseAwards: %v", err)
	}
	return awards
}

// find returns the single award matching org/name/year, failing if it is absent.
func find(t *testing.T, awards []store.Award, org, name string, year int) store.Award {
	t.Helper()
	for _, a := range awards {
		if a.Organization == org && a.AwardName == name && a.Year == year {
			return a
		}
	}
	t.Fatalf("no award %q / %q / %d in %d parsed awards", org, name, year, len(awards))
	return store.Award{}
}

func TestParseAwardsCarriesOrganizationAndYearForward(t *testing.T) {
	awards := loadAwards(t)

	a := find(t, awards, "AVN Awards", "Best Porn Star Website", 2015)
	if a.Result != store.ResultNominated {
		t.Errorf("result = %q, want nominated", a.Result)
	}
	if a.Event != "AVN Awards 2015" {
		t.Errorf("event = %q, want AVN Awards 2015", a.Event)
	}
	if a.SourceURL != testPageURL {
		t.Errorf("source url = %q, want the page URL", a.SourceURL)
	}
	if a.AssociatedMovie != "" {
		t.Errorf("associated movie = %q, want empty for a personal award", a.AssociatedMovie)
	}

	// A later year divider under the same heading applies to following entries.
	find(t, awards, "AVN Awards", "Best All-Girl Group Sex Scene", 2016)
}

// The award name can contain colons, so only the first one separates the result
// label from the name.
func TestParseAwardsSplitsOnlyTheFirstColon(t *testing.T) {
	awards := loadAwards(t)
	a := find(t, awards, "AVN Awards", "Best Three-Way Sex Scene: G/G/B", 2015)
	if a.Result != store.ResultNominated {
		t.Errorf("result = %q, want nominated", a.Result)
	}
	find(t, awards, "AVN Awards", "Best Director: Non-Feature", 2016)
}

func TestParseAwardsReadsMovieLinkAndYear(t *testing.T) {
	awards := loadAwards(t)
	a := find(t, awards, "AVN Awards", "Best All-Girl Group Sex Scene", 2016)

	if a.Result != store.ResultWon {
		t.Errorf("result = %q, want won", a.Result)
	}
	if a.AssociatedMovie != "Test Movie Two" {
		t.Errorf("movie = %q", a.AssociatedMovie)
	}
	if a.AssociatedMovieYear != 2015 {
		t.Errorf("movie year = %d, want 2015", a.AssociatedMovieYear)
	}
	want := "https://www.iafd.com/title.rme/id=bc17b724-205f-4573-8b10-7ceb629809e4"
	if a.AssociatedMovieURL != want {
		t.Errorf("movie url = %q, want %q", a.AssociatedMovieURL, want)
	}
}

// Movie links are absolute on a performer page but the ADR described them as
// relative, so both forms have to resolve.
func TestParseAwardsResolvesRelativeMovieLinks(t *testing.T) {
	awards := loadAwards(t)
	a := find(t, awards, "XRCO Awards", "Relative Movie Link", 2021)
	want := "https://www.iafd.com/title.rme/id=3d508ec3-d307-4d04-a114-906ed6d8e16f"
	if a.AssociatedMovieURL != want {
		t.Errorf("movie url = %q, want %q", a.AssociatedMovieURL, want)
	}
	if a.AssociatedMovieYear != 2020 {
		t.Errorf("movie year = %d, want 2020", a.AssociatedMovieYear)
	}
}

func TestParseAwardsReadsInductions(t *testing.T) {
	awards := loadAwards(t)
	a := find(t, awards, "XRCO Awards", "Hall of Fame (Video Branch)", 2021)
	if a.Result != store.ResultInducted {
		t.Errorf("result = %q, want inducted", a.Result)
	}
}

// Two organizations that differ only by a parenthesised division must stay apart,
// or their awards would merge under one heading.
func TestParseAwardsKeepsDivisionsSeparate(t *testing.T) {
	awards := loadAwards(t)
	straight := find(t, awards, "Fleshbot Awards (Straight)", "Hottest Cougar", 2019)
	if straight.Result != store.ResultWon {
		t.Errorf("result = %q, want won", straight.Result)
	}
	find(t, awards, "Fleshbot Awards (Trans)", "Hottest Newcomer", 2019)
}

func TestParseAwardsTreatsUnlabelledBoldEntriesAsWins(t *testing.T) {
	awards := loadAwards(t)
	if a := find(t, awards, "XRCO Awards", "Best Unlabelled Bold Entry", 2021); a.Result != store.ResultWon {
		t.Errorf("bold entry result = %q, want won", a.Result)
	}
	if a := find(t, awards, "XRCO Awards", "Unlabelled Plain Entry", 2021); a.Result != store.ResultNominated {
		t.Errorf("plain entry result = %q, want nominated", a.Result)
	}
}

func TestParseAwardsSkipsUnattributableAndEmptyEntries(t *testing.T) {
	for _, a := range loadAwards(t) {
		if a.Year == 0 {
			t.Errorf("award %q has no year: %+v", a.AwardName, a)
		}
		if a.AwardName == "" {
			t.Errorf("award with empty name: %+v", a)
		}
		if strings.Contains(a.AwardName, "No Year Divider") {
			t.Errorf("entry before the first year divider was kept: %+v", a)
		}
	}
}

func TestParseAwardsIgnoresOtherTabs(t *testing.T) {
	for _, a := range loadAwards(t) {
		if a.Organization == "Birthday" || a.Organization == "Measurements" {
			t.Fatalf("bio tab content leaked into awards: %+v", a)
		}
	}
}

func TestParseAwardsWithoutPanelReturnsNothing(t *testing.T) {
	awards, err := ParseAwards(strings.NewReader("<html><body><p>no awards here</p></body></html>"), testPageURL)
	if err != nil {
		t.Fatalf("ParseAwards: %v", err)
	}
	if len(awards) != 0 {
		t.Errorf("got %d awards, want 0", len(awards))
	}
}

// resolve covers three reachable paths: empty href, unparsable href, and
// an absolute href that should pass through unchanged.
func TestResolve(t *testing.T) {
	base, _ := url.Parse("https://www.iafd.com/search/")
	cases := []struct {
		name string
		base *url.URL
		href string
		want string
	}{
		{"empty href", base, "", ""},
		{"unparsable href", base, "://bad", "://bad"},
		{"absolute href", base, "https://other.test/x", "https://other.test/x"},
		{"relative href resolved against base", base, "/person/abc", "https://www.iafd.com/person/abc"},
		{"nil base means return the href as parsed", nil, "/relative", "/relative"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolve(c.base, c.href); got != c.want {
				t.Errorf("resolve = %q, want %q", got, c.want)
			}
		})
	}
}

// RecogniseURL must accept the canonical id= form and reject URLs that are
// not on the IAFD host.
func TestRecogniseURLHandlesHostVariants(t *testing.T) {
	p := &Provider{}
	for _, in := range []string{
		"https://www.iafd.com/person.rme/id=abc",
		"https://iafd.com/person.rme/id=abc",
	} {
		if _, ok := p.RecogniseURL(in); !ok {
			t.Errorf("RecogniseURL(%q) = false, want true", in)
		}
	}
	if _, ok := p.RecogniseURL("https://example.com/person.rme/id=abc"); ok {
		t.Error("RecogniseURL accepted a URL from another site")
	}
	if _, ok := p.RecogniseURL("https://www.iafd.com/title.rme/id=abc"); ok {
		t.Error("RecogniseURL accepted a non-person URL on the iafd host")
	}
}
