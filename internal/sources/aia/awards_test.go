package aia

import (
	"os"
	"strings"
	"testing"

	"github.com/slick-daddy/stash-awards/internal/store"
)

const testPageURL = "https://adultindustryawards.com/test-performer/"

// loadFixtureAwards parses the rendered content out of the stand-in REST post.
// The fixture keeps the entity encoding the real API returns (&#8211; for the en
// dash that separates an award from its film) so the parser is tested against it.
func loadFixtureAwards(t *testing.T) []store.Award {
	t.Helper()
	body, err := os.ReadFile("testdata/post.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	// Reuse the provider's own decoding so the fixture shape is checked too.
	posts, err := decodePosts(body)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	awards, err := ParseAwards(strings.NewReader(posts[0].Content.Rendered), testPageURL)
	if err != nil {
		t.Fatalf("ParseAwards: %v", err)
	}
	return awards
}

func findAward(t *testing.T, awards []store.Award, org, name string, year int) store.Award {
	t.Helper()
	for _, a := range awards {
		if a.Organization == org && a.AwardName == name && a.Year == year {
			return a
		}
	}
	t.Fatalf("no award %q / %q / %d among %d parsed", org, name, year, len(awards))
	return store.Award{}
}

func TestParseAwardsIgnoresBiographyAndPromoItems(t *testing.T) {
	awards := loadFixtureAwards(t)
	for _, a := range awards {
		lower := strings.ToLower(a.AwardName)
		for _, bad := range []string{"twitter", "measurements", "fine scenes", "year born"} {
			if strings.Contains(lower, bad) {
				t.Errorf("non-award item parsed as an award: %+v", a)
			}
		}
	}
	if len(awards) != 10 {
		t.Errorf("got %d awards, want 10: %+v", len(awards), awards)
	}
}

func TestParseAwardsRejectsImplausibleYears(t *testing.T) {
	for _, a := range loadFixtureAwards(t) {
		if a.Year < 1900 {
			t.Errorf("kept an implausible past year: %+v", a)
		}
		if a.Year > 3000 {
			t.Errorf("kept an implausible future year: %+v", a)
		}
	}
}

func TestParseAwardsSeparatesOrganizationFromAwardName(t *testing.T) {
	awards := loadFixtureAwards(t)

	a := findAward(t, awards, "XBIZ", "Adult Site Of The Year", 2015)
	if a.AssociatedMovie != "Performer TestPerformer.com" {
		t.Errorf("detail = %q", a.AssociatedMovie)
	}
	if a.Result != store.ResultWon {
		t.Errorf("result = %q, want won: AIA lists only wins", a.Result)
	}
	if a.Event != "XBIZ 2015" {
		t.Errorf("event = %q", a.Event)
	}
	if a.SourceURL != testPageURL {
		t.Errorf("source url = %q", a.SourceURL)
	}

	findAward(t, awards, "Nightmoves", "Best Boobs", 2016)
}

// "Pornhub Awards Nicest Tits" must not become an organization called "Pornhub"
// and another called "Pornhub Awards".
func TestParseAwardsDropsARedundantAwardsWordFromTheOrganization(t *testing.T) {
	findAward(t, loadFixtureAwards(t), "Pornhub", "Nicest Tits", 2022)
}

// The ADR called for splitting on the first dash, which would truncate award
// names that carry their own qualifier.
func TestParseAwardsSplitsOnTheLastSpacedDash(t *testing.T) {
	awards := loadFixtureAwards(t)

	a := findAward(t, awards, "AVN", "Best Actress – Featurette", 2019)
	if a.AssociatedMovie != "Games We Play, TrenchcoatX" {
		t.Errorf("detail = %q", a.AssociatedMovie)
	}

	b := findAward(t, awards, "AVN", "Best Three-Way Sex Scene – Girl/Girl/Boy", 2019)
	if b.AssociatedMovie != "The Corruption Of Someone, Test Studio" {
		t.Errorf("detail = %q", b.AssociatedMovie)
	}
}

// A hyphen inside a word is not a separator.
func TestParseAwardsKeepsHyphenatedNamesIntact(t *testing.T) {
	awards := loadFixtureAwards(t)
	for _, a := range awards {
		if a.AwardName == "Best Three" {
			t.Fatalf("split a hyphenated award name: %+v", a)
		}
	}
}

// Many entries simply have no separator, and the film runs straight on from the
// award. There is nothing to split, so the whole remainder is the award name.
func TestParseAwardsKeepsUnseparatedLinesWhole(t *testing.T) {
	a := findAward(t, loadFixtureAwards(t), "Fleshbot", "Best Anal Scene Test Movie Three Evil Studio", 2019)
	if a.AssociatedMovie != "" {
		t.Errorf("detail = %q, want empty when there is no separator", a.AssociatedMovie)
	}
}

func TestParseAwardsDecodesEntities(t *testing.T) {
	findAward(t, loadFixtureAwards(t), "Nightmoves", "Best Actress (Fan’s Choice)", 2020)
	a := findAward(t, loadFixtureAwards(t), "AVN", "Best Editing", 2022)
	if a.AssociatedMovie != "Test Movie Four, AGW & Girlfriends" {
		t.Errorf("detail = %q, want the ampersand decoded", a.AssociatedMovie)
	}
}

func TestParseAwardsMarksHallOfFameAsAnInduction(t *testing.T) {
	a := findAward(t, loadFixtureAwards(t), "AVN", "Hall Of Fame (Video Branch)", 2018)
	if a.Result != store.ResultInducted {
		t.Errorf("result = %q, want inducted", a.Result)
	}
	if a.AssociatedMovie != "" {
		t.Errorf("detail = %q, want empty for an induction", a.AssociatedMovie)
	}
}

// A line with a year and a single word has no award name once the organization is
// taken off the front.
func TestParseAwardsSkipsLinesWithNothingAfterTheOrganization(t *testing.T) {
	for _, a := range loadFixtureAwards(t) {
		if a.Organization == "OnlyOneToken" {
			t.Fatalf("kept a line with no award name: %+v", a)
		}
	}
}

func TestSlug(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Angela White", "angela-white"},
		{"Eva Elfie", "eva-elfie"},
		{"  Spaced  Out  ", "spaced-out"},
		{"D'Angelo Smith", "dangelo-smith"},
		{"J.J. Jones", "jj-jones"},
		{"Anna-Maria", "anna-maria"},
		{"Nikki Benz!", "nikki-benz"},
		{"", ""},
		{"???", ""},
	} {
		if got := Slug(tc.in); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
