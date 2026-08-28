// Package sources describes what every award source must provide, so the sync
// engine can treat IAFD and AdultIndustryAwards identically.
package sources

import (
	"context"

	"github.com/slick-daddy/stash-awards/internal/store"
)

// Match is one candidate performer page returned by a search.
type Match struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	// Detail is free-form context that helps a person pick the right match —
	// aliases, years active, a title count.
	Detail string `json:"detail,omitempty"`
	// ImageURL is a thumbnail, when the source offers one.
	ImageURL string `json:"imageUrl,omitempty"`
}

// Provider is one award source.
type Provider interface {
	// ID is the value stored in the source column.
	ID() store.Source

	// Search returns candidate pages for a performer name, best guess first.
	Search(ctx context.Context, name string) ([]Match, error)

	// Awards fetches and parses every award on a performer's page. The returned
	// records carry no performer ID or source; the caller stamps those.
	Awards(ctx context.Context, pageURL string) ([]store.Award, error)

	// GuessURL derives a page URL from a performer name without making a
	// request, when the source's URLs are predictable. It returns false when the
	// source offers no such shortcut.
	GuessURL(name string) (string, bool)

	// RecogniseURL reports whether raw is a performer page on this source and,
	// if so, returns it in canonical form. Used both to read URLs off a Stash
	// performer record and to validate a URL a user typed in.
	RecogniseURL(raw string) (string, bool)
}
