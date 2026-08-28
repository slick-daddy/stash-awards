// Package iafd reads award data from the Internet Adult Film Database.
package iafd

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/slick-daddy/stash-awards/internal/fetch"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/store"
)

const (
	// Host is the only host this provider will fetch from.
	Host = "www.iafd.com"
	// BaseURL prefixes every URL this provider builds.
	BaseURL = "https://" + Host
	// DefaultDelay is the minimum spacing between IAFD requests. The site rate
	// limits rapid requests, and 2s has proved comfortable.
	DefaultDelay = 2000
)

// Provider fetches awards from IAFD.
type Provider struct {
	client *fetch.Client
}

// New returns a Provider that fetches through client.
func New(client *fetch.Client) *Provider {
	return &Provider{client: client}
}

// ID implements sources.Provider.
func (p *Provider) ID() store.Source { return store.SourceIAFD }

// GuessURL implements sources.Provider. IAFD performer URLs embed an opaque UUID
// or a perfid/gender pair, neither of which can be derived from a name, so this
// provider always searches.
func (p *Provider) GuessURL(string) (string, bool) { return "", false }

// RecogniseURL implements sources.Provider.
func (p *Provider) RecogniseURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Host != "" && !strings.EqualFold(strings.TrimPrefix(u.Host, "www."), "iafd.com") {
		return "", false
	}
	if canonical, ok := canonicalPersonURL(raw); ok {
		return canonical, true
	}
	// A "pretty" performer URL (person.rme/perfid=…/gender=f/name.htm) has no
	// UUID to canonicalise, but IAFD redirects it to the id= form, so it is still
	// a usable starting point.
	if u.Host != "" && strings.Contains(strings.ToLower(u.Path), "person.rme") {
		return u.String(), true
	}
	return "", false
}

// canonicalPersonURL rewrites any URL carrying a performer UUID into the single
// canonical form, so the same performer is never cached under two URLs.
func canonicalPersonURL(raw string) (string, bool) {
	m := personLinkRe.FindStringSubmatch(raw)
	if m == nil {
		return "", false
	}
	return BaseURL + "/person.rme/id=" + strings.ToLower(m[1]), true
}

// SearchURL is the comprehensive-search address for a name.
func SearchURL(name string) string {
	q := url.Values{}
	q.Set("searchtype", "comprehensive")
	q.Set("searchstring", name)
	return BaseURL + "/ramesearch.asp?" + q.Encode()
}

// Search implements sources.Provider.
func (p *Provider) Search(ctx context.Context, name string) ([]sources.Match, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("iafd search: empty performer name")
	}
	pageURL := SearchURL(name)
	resp, err := p.client.Get(ctx, string(store.SourceIAFD), pageURL)
	if err != nil {
		return nil, fmt.Errorf("iafd search %q: %w", name, err)
	}
	return ParseSearch(bytes.NewReader(resp.Body), resp.URL)
}

// Awards implements sources.Provider.
func (p *Provider) Awards(ctx context.Context, pageURL string) ([]store.Award, error) {
	canonical, ok := p.RecogniseURL(pageURL)
	if !ok {
		return nil, fmt.Errorf("not an iafd performer URL: %q", pageURL)
	}
	resp, err := p.client.Get(ctx, string(store.SourceIAFD), canonical)
	if err != nil {
		return nil, fmt.Errorf("iafd fetch %s: %w", canonical, err)
	}
	// resp.URL is where the response really came from, so a pretty URL that
	// redirected is recorded against its canonical address.
	sourceURL := resp.URL
	if c, ok := canonicalPersonURL(sourceURL); ok {
		sourceURL = c
	}
	awards, err := ParseAwards(bytes.NewReader(resp.Body), sourceURL)
	if err != nil {
		return nil, err
	}
	for i := range awards {
		awards[i].Source = store.SourceIAFD
	}
	return awards, nil
}
