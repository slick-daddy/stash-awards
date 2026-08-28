package aia

import (
	"sort"
	"strings"

	"github.com/slick-daddy/stash-awards/internal/sources"
)

// rank reorders search results so the likeliest performer comes first. The site's
// full-text search matches page bodies as well as titles, so a query for one
// performer readily returns pages that merely mention them; without reordering,
// the intended performer can sit well down the list.
//
// The order is stable within a score, which keeps the site's own relevance as the
// tie-breaker.
func rank(query string, matches []sources.Match) []sources.Match {
	want := normalise(query)
	sort.SliceStable(matches, func(i, j int) bool {
		return score(want, matches[i]) < score(want, matches[j])
	})
	return matches
}

// score is a rank, so lower is better.
func score(want string, m sources.Match) int {
	name := normalise(m.Name)
	switch {
	case name == want:
		return 0
	case m.Detail == Slug(want):
		// The slug lookup hit, even though the title differs in punctuation.
		return 1
	case strings.HasPrefix(name, want):
		return 2
	case strings.Contains(name, want):
		return 3
	case sharesAnyWord(name, want):
		return 4
	default:
		return 5
	}
}

// normalise reduces a name to lowercase words separated by single spaces, so
// punctuation and spacing differences do not affect comparison.
func normalise(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.NewReplacer(
		"'", "", "’", "", ".", "", "-", " ", "_", " ",
	).Replace(s))), " ")
}

func sharesAnyWord(a, b string) bool {
	words := map[string]bool{}
	for _, w := range strings.Fields(a) {
		words[w] = true
	}
	for _, w := range strings.Fields(b) {
		if words[w] {
			return true
		}
	}
	return false
}
