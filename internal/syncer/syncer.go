// Package syncer turns a Stash performer into stored awards: it works out which
// page on each source belongs to the performer, scrapes it, and records when the
// performer should be looked at again.
package syncer

import (
	"context"
	"strings"
	"time"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/protocol"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/stashapi"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// BatchSize is how many performers one pass of a batch sync handles before
// pausing, and the page size used when walking the whole library.
const BatchSize = 50

// BatchPause is the gap between batches. Sustained scraping is the thing most
// likely to get a user blocked, so the run deliberately idles.
const BatchPause = 10 * time.Second

// RetryInterval is how long a performer waits after a failure. It is shorter
// than the sync interval because most failures are transient, but long enough
// that a source that is down is not hammered.
const RetryInterval = 6 * time.Hour

// Status is the outcome of syncing one performer against one source.
type Status string

const (
	// StatusSynced means awards were fetched and stored.
	StatusSynced Status = "synced"
	// StatusSkipped means the stored data is still fresh.
	StatusSkipped Status = "skipped"
	// StatusDisabled means the source is turned off in the settings.
	StatusDisabled Status = "disabled"
	// StatusAmbiguous means a search found candidates but none of them was a
	// confident match, so a person has to choose.
	StatusAmbiguous Status = "ambiguous"
	// StatusUnresolved means the source appears not to have this performer.
	StatusUnresolved Status = "unresolved"
	// StatusFailed means the source could not be read.
	StatusFailed Status = "failed"
)

// Origin records how a page URL was arrived at, which is worth showing to a user
// deciding whether to trust a link.
type Origin string

const (
	// OriginStored is a URL previously resolved or set by hand.
	OriginStored Origin = "stored"
	// OriginPerformer is a URL already present on the Stash performer.
	OriginPerformer Origin = "performer"
	// OriginGuessed is a URL derived from the performer's name.
	OriginGuessed Origin = "guessed"
	// OriginSearch is a URL taken from a name search.
	OriginSearch Origin = "search"
)

// Result describes one performer/source sync. It is returned to the UI as JSON,
// so a failure is a value here rather than an error.
type Result struct {
	PerformerID string          `json:"performerId"`
	Source      store.Source    `json:"source"`
	Status      Status          `json:"status"`
	URL         string          `json:"url,omitempty"`
	Origin      Origin          `json:"origin,omitempty"`
	Awards      int             `json:"awards"`
	Message     string          `json:"message,omitempty"`
	Candidates  []sources.Match `json:"candidates,omitempty"`
}

// stashClient is the part of the Stash API the syncer needs.
type stashClient interface {
	Performer(ctx context.Context, id string) (*stashapi.Performer, error)
	Performers(ctx context.Context, page, perPage int) ([]stashapi.Performer, int, error)
}

// Syncer coordinates the store, the sources and the Stash server.
type Syncer struct {
	store     *store.Store
	stash     stashClient
	providers map[store.Source]sources.Provider
	settings  config.Settings
	log       *protocol.Log

	// now and sleep are injectable so that batching and scheduling can be
	// tested without waiting for real time to pass.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

// Deps are the collaborators a Syncer needs.
type Deps struct {
	Store     *store.Store
	Stash     stashClient
	Providers map[store.Source]sources.Provider
	Settings  config.Settings
	Log       *protocol.Log

	// Now defaults to time.Now.
	Now func() time.Time
	// Sleep defaults to a context-aware sleep.
	Sleep func(ctx context.Context, d time.Duration) error
}

// New returns a Syncer built from deps.
func New(deps Deps) *Syncer {
	s := &Syncer{
		store:     deps.Store,
		stash:     deps.Stash,
		providers: deps.Providers,
		settings:  deps.Settings,
		log:       deps.Log,
		now:       deps.Now,
		sleep:     deps.Sleep,
	}
	if s.log == nil {
		s.log = protocol.NewLog()
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.sleep == nil {
		s.sleep = sleepCtx
	}
	return s
}

// sleepCtx waits for d unless ctx ends first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Settings returns the settings this Syncer runs with.
func (s *Syncer) Settings() config.Settings { return s.settings }

// Provider returns the provider for source, if the source is known and enabled.
func (s *Syncer) Provider(source store.Source) (sources.Provider, bool) {
	if !s.settings.Enabled(source) {
		return nil, false
	}
	p, ok := s.providers[source]
	return p, ok
}

// normaliseName reduces a name to the form used when comparing a search result
// against the performer being looked for. Punctuation varies between Stash and
// the sources ("D'Angelo" against "DAngelo"), so it is dropped rather than
// treated as a difference; a hyphen becomes a space, since that is what it
// stands in for in a name.
func normaliseName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
