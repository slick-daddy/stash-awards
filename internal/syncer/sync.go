package syncer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/slick-daddy/stash-awards/internal/fetch"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/stashapi"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// SyncPerformerID syncs every enabled source for one performer, looking the
// performer up in Stash first.
func (s *Syncer) SyncPerformerID(ctx context.Context, performerID string, force bool) ([]Result, error) {
	p, err := s.stash.Performer(ctx, performerID)
	if err != nil {
		return nil, err
	}
	return s.SyncPerformer(ctx, p, force)
}

// SyncPerformer syncs every enabled source for p.
func (s *Syncer) SyncPerformer(ctx context.Context, p *stashapi.Performer, force bool) ([]Result, error) {
	var out []Result
	for _, source := range s.settings.EnabledSources() {
		res, err := s.SyncSource(ctx, p, source, force)
		if err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}

// SyncSource syncs one source for one performer. A source that cannot be read is
// reported in the Result; the error return is reserved for failures that make
// continuing pointless, such as the database being unusable.
func (s *Syncer) SyncSource(ctx context.Context, p *stashapi.Performer, source store.Source, force bool) (Result, error) {
	res := Result{PerformerID: p.ID, Source: source}

	provider, ok := s.Provider(source)
	if !ok {
		res.Status = StatusDisabled
		res.Message = "source is disabled in the plugin settings"
		return res, nil
	}

	if !force {
		fresh, err := s.isFresh(p.ID, source)
		if err != nil {
			return res, err
		}
		if fresh {
			return s.skipped(p.ID, source)
		}
	}

	pageURL, origin, err := s.knownURL(p, source, provider)
	if err != nil {
		return res, err
	}

	if pageURL == "" {
		// A name-derived URL is worth one request: on AIA it is usually right, and
		// it costs less than a search. IAFD pages are keyed by an opaque id, so its
		// provider declines to guess and this is skipped.
		if guess, ok := provider.GuessURL(p.Name); ok {
			awards, notFound, err := readAwards(ctx, provider, guess)
			switch {
			case err == nil:
				return s.record(p.ID, source, guess, OriginGuessed, awards)
			case !notFound:
				return s.recordFailure(p.ID, source, guess, OriginGuessed, err)
			default:
				s.log.Debug("no %s page at the guessed URL %s; searching by name", source, guess)
			}
		}

		matches, err := provider.Search(ctx, p.Name)
		if err != nil {
			return s.recordFailure(p.ID, source, "", "", fmt.Errorf("search for %q: %w", p.Name, err))
		}
		pick, ok := bestMatch(p, matches)
		if !ok {
			return s.recordNoPage(p.ID, source, matches)
		}
		pageURL, origin = pick.URL, OriginSearch
	}

	awards, _, err := readAwards(ctx, provider, pageURL)
	if err != nil {
		return s.recordFailure(p.ID, source, pageURL, origin, err)
	}
	return s.record(p.ID, source, pageURL, origin, awards)
}

// Link stores a page URL chosen by the user and syncs it immediately, so the
// choice is confirmed by real data rather than accepted on faith.
func (s *Syncer) Link(ctx context.Context, performerID string, source store.Source, rawURL string) (Result, error) {
	res := Result{PerformerID: performerID, Source: source}

	provider, ok := s.providers[source]
	if !ok {
		return res, fmt.Errorf("unknown source %q", source)
	}
	canonical, ok := provider.RecogniseURL(rawURL)
	if !ok {
		return res, fmt.Errorf("%q is not a %s performer URL", rawURL, source.Label())
	}
	if err := s.store.SetURL(performerID, source, canonical); err != nil {
		return res, err
	}

	p, err := s.stash.Performer(ctx, performerID)
	if err != nil {
		return res, err
	}
	return s.SyncSource(ctx, p, source, true)
}

// Unlink forgets a source for one performer: the URL, the awards and the sync
// schedule. Leaving the URL behind would let the next sync resurrect the link
// the user just rejected.
func (s *Syncer) Unlink(performerID string, source store.Source) error {
	return s.store.ForgetSource(performerID, source)
}

// Search offers candidate pages for a performer without storing anything.
func (s *Syncer) Search(ctx context.Context, source store.Source, name string) ([]sources.Match, error) {
	provider, ok := s.providers[source]
	if !ok {
		return nil, fmt.Errorf("unknown source %q", source)
	}
	return provider.Search(ctx, name)
}

// readAwards fetches one page. The second return reports that the page does not
// exist, which the caller treats differently from a page that could not be read.
func readAwards(ctx context.Context, provider sources.Provider, pageURL string) ([]store.Award, bool, error) {
	awards, err := provider.Awards(ctx, pageURL)
	if err != nil {
		return nil, errors.Is(err, fetch.ErrNotFound), err
	}
	return awards, false, nil
}

// knownURL returns a page URL already known for this performer: one stored here
// before, or one the Stash performer record already carries.
func (s *Syncer) knownURL(p *stashapi.Performer, source store.Source, provider sources.Provider) (string, Origin, error) {
	stored, err := s.store.URL(p.ID, source)
	switch {
	case err == nil && stored.URL != "":
		return stored.URL, OriginStored, nil
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return "", "", err
	}

	// Step one of performer matching: the performer may already link to the
	// source, in which case there is nothing to work out.
	for _, raw := range p.URLs {
		canonical, ok := provider.RecogniseURL(raw)
		if !ok {
			continue
		}
		// Remember it so later syncs skip this scan, and so the UI can show where
		// the link came from.
		if err := s.store.SetURL(p.ID, source, canonical); err != nil {
			return "", "", err
		}
		return canonical, OriginPerformer, nil
	}
	return "", "", nil
}

// bestMatch picks the one candidate that is certainly the performer. Anything
// less than an exact name or alias match is left to a person to decide, because
// a wrong link silently attributes someone else's awards.
func bestMatch(p *stashapi.Performer, matches []sources.Match) (sources.Match, bool) {
	if len(matches) == 0 {
		return sources.Match{}, false
	}

	wanted := []string{p.Name}
	wanted = append(wanted, p.Aliases...)
	for _, name := range wanted {
		norm := normaliseName(name)
		if norm == "" {
			continue
		}
		for _, m := range matches {
			if normaliseName(m.Name) == norm {
				return m, true
			}
		}
	}
	return sources.Match{}, false
}

// nameList renders candidate names for a log line.
func nameList(matches []sources.Match) string {
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m.Name)
	}
	return strings.Join(names, ", ")
}
