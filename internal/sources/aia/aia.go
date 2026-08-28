package aia

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/slick-daddy/stash-awards/internal/fetch"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/store"
)

const (
	// Host is the only host this provider will fetch from.
	Host = "adultindustryawards.com"
	// BaseURL prefixes every URL this provider builds.
	BaseURL = "https://" + Host
	// DefaultDelay is the minimum spacing between requests, in milliseconds. The
	// site shows no rate limiting, but a plugin has no business hammering it.
	DefaultDelay = 500

	postsEndpoint = BaseURL + "/wp-json/wp/v2/posts"
	// searchLimit caps how many candidates a name search returns. The list is for
	// a person to choose from, so a long tail is noise.
	searchLimit = 10
)

// post is the subset of a WordPress post this plugin reads.
type post struct {
	ID    int    `json:"id"`
	Slug  string `json:"slug"`
	Link  string `json:"link"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
}

// Provider fetches awards from AdultIndustryAwards.
type Provider struct {
	client *fetch.Client
}

// New returns a Provider that fetches through client.
func New(client *fetch.Client) *Provider {
	return &Provider{client: client}
}

// ID implements sources.Provider.
func (p *Provider) ID() store.Source { return store.SourceAIA }

// GuessURL implements sources.Provider. AIA performer pages are addressed by a
// slug derived from the name, so a URL can be proposed without searching.
func (p *Provider) GuessURL(name string) (string, bool) {
	slug := Slug(name)
	if slug == "" {
		return "", false
	}
	return BaseURL + "/" + slug + "/", true
}

// RecogniseURL implements sources.Provider.
func (p *Provider) RecogniseURL(raw string) (string, bool) {
	slug, ok := slugFromURL(raw)
	if !ok {
		return "", false
	}
	return BaseURL + "/" + slug + "/", true
}

// slugFromURL extracts the performer slug from a page URL.
func slugFromURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	if !strings.EqualFold(strings.TrimPrefix(u.Host, "www."), Host) {
		return "", false
	}
	// A performer page is a single path segment. Anything deeper is a category,
	// tag or feed URL rather than a performer.
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(parts) != 1 {
		return "", false
	}
	slug := Slug(parts[0])
	if slug == "" {
		return "", false
	}
	return slug, true
}

// Awards implements sources.Provider.
func (p *Provider) Awards(ctx context.Context, pageURL string) ([]store.Award, error) {
	slug, ok := slugFromURL(pageURL)
	if !ok {
		return nil, fmt.Errorf("not an adultindustryawards performer URL: %q", pageURL)
	}

	posts, err := p.posts(ctx, url.Values{
		"slug":    {slug},
		"_fields": {"id,slug,link,title,content"},
	})
	if err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		// The REST API answers an unmatched slug with an empty array rather than
		// a 404, so the "no such performer" case has to be detected here.
		return nil, fmt.Errorf("no adultindustryawards post with slug %q: %w", slug, fetch.ErrNotFound)
	}

	link := posts[0].Link
	if link == "" {
		link = BaseURL + "/" + posts[0].Slug + "/"
	}
	awards, err := ParseAwards(strings.NewReader(posts[0].Content.Rendered), link)
	if err != nil {
		return nil, err
	}
	for i := range awards {
		awards[i].Source = store.SourceAIA
	}
	return awards, nil
}

// Search implements sources.Provider. The derived slug is tried first because it
// is an exact hit when it works; the site's full-text search is a fallback that
// ranks poorly on its own.
func (p *Provider) Search(ctx context.Context, name string) ([]sources.Match, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("aia search: empty performer name")
	}

	var matches []sources.Match
	seen := map[string]bool{}
	add := func(ps []post) {
		for _, po := range ps {
			link := po.Link
			if link == "" {
				link = BaseURL + "/" + po.Slug + "/"
			}
			if po.Slug == "" || seen[link] {
				continue
			}
			seen[link] = true
			matches = append(matches, sources.Match{
				Name:   clean(html.UnescapeString(po.Title.Rendered)),
				URL:    link,
				Detail: po.Slug,
			})
		}
	}

	if slug := Slug(name); slug != "" {
		exact, err := p.posts(ctx, url.Values{"slug": {slug}, "_fields": {"id,slug,link,title"}})
		if err != nil {
			return nil, err
		}
		add(exact)
	}

	found, err := p.posts(ctx, url.Values{
		"search":   {name},
		"per_page": {fmt.Sprint(searchLimit)},
		"_fields":  {"id,slug,link,title"},
	})
	if err != nil {
		// The slug lookup may already have produced the answer, and a failing
		// full-text search should not throw that away.
		if len(matches) > 0 {
			return matches, nil
		}
		return nil, err
	}
	add(found)

	return rank(name, matches), nil
}

// posts performs one REST query and decodes the array it returns.
func (p *Provider) posts(ctx context.Context, q url.Values) ([]post, error) {
	endpoint := postsEndpoint + "?" + q.Encode()
	resp, err := p.client.Get(ctx, string(store.SourceAIA), endpoint)
	if err != nil {
		return nil, fmt.Errorf("aia request %s: %w", endpoint, err)
	}

	posts, err := decodePosts(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode aia response from %s: %w", endpoint, err)
	}
	return posts, nil
}

// decodePosts reads the array of posts the REST API returns.
func decodePosts(body []byte) ([]post, error) {
	var posts []post
	if err := json.Unmarshal(body, &posts); err != nil {
		return nil, err
	}
	return posts, nil
}
