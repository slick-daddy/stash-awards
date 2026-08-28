package ops

import (
	"context"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/store"
	"github.com/slick-daddy/stash-awards/internal/syncer"
)

// SourceView is what the UI shows for one source: the link it is reading, when
// it was last read, and the awards found there.
type SourceView struct {
	Source        store.Source  `json:"source"`
	Label         string        `json:"label"`
	Enabled       bool          `json:"enabled"`
	URL           string        `json:"url,omitempty"`
	URLResolvedAt string        `json:"urlResolvedAt,omitempty"`
	LastSynced    string        `json:"lastSynced,omitempty"`
	NextSyncAfter string        `json:"nextSyncAfter,omitempty"`
	Error         string        `json:"error,omitempty"`
	Count         int           `json:"count"`
	Awards        []store.Award `json:"awards,omitempty"`
}

// AwardsPayload is the reply the awards page renders.
type AwardsPayload struct {
	PerformerID   string          `json:"performerId"`
	PerformerName string          `json:"performerName,omitempty"`
	Settings      config.Settings `json:"settings"`
	Sources       []SourceView    `json:"sources"`
	Total         int             `json:"total"`
	Synced        []syncer.Result `json:"synced,omitempty"`
	Warning       string          `json:"warning,omitempty"`
}

// getAwards returns everything stored for a performer, optionally bringing stale
// sources up to date first.
//
// The UI calls this when the awards page opens, which is where the "sync when it
// is stale" behaviour lives: passing sync=false reads the database only.
func (rt *runtime) getAwards(ctx context.Context) (interface{}, error) {
	id, err := rt.performerID()
	if err != nil {
		return nil, err
	}

	payload := AwardsPayload{PerformerID: id, Settings: rt.settings}

	// One lookup serves both the page title and the sync below.
	p, err := rt.stash.Performer(ctx, id)
	if err != nil {
		// Stored awards are still worth showing when Stash cannot be reached.
		rt.log.Warn("could not read performer %s from stash: %v", id, err)
		payload.Warning = err.Error()
	} else {
		payload.PerformerName = p.Name
	}

	if p != nil && rt.args.Bool("sync", true) {
		results, err := rt.syncer.SyncPerformer(ctx, p, rt.args.Bool("force", false))
		if err != nil {
			// A sync failure must not hide data that is already stored.
			rt.log.Warn("sync for performer %s stopped early: %v", id, err)
			payload.Warning = err.Error()
		}
		payload.Synced = results
	}

	views, total, err := rt.sourceViews(id, true)
	if err != nil {
		return nil, err
	}
	payload.Sources = views
	payload.Total = total
	return payload, nil
}

// LinksPayload describes which page each source is linked to, without the awards
// themselves. This is what the link-editing UI reads.
type LinksPayload struct {
	PerformerID string          `json:"performerId"`
	Settings    config.Settings `json:"settings"`
	Sources     []SourceView    `json:"sources"`
}

// getLinks returns the stored URL and sync state for each source.
func (rt *runtime) getLinks() (interface{}, error) {
	id, err := rt.performerID()
	if err != nil {
		return nil, err
	}
	views, _, err := rt.sourceViews(id, false)
	if err != nil {
		return nil, err
	}
	return LinksPayload{PerformerID: id, Settings: rt.settings, Sources: views}, nil
}

// sourceViews builds one view per known source. Disabled sources are included so
// the UI can explain their absence rather than silently dropping a tab.
func (rt *runtime) sourceViews(performerID string, withAwards bool) ([]SourceView, int, error) {
	urls, err := rt.store.URLs(performerID)
	if err != nil {
		return nil, 0, err
	}
	states, err := rt.store.States(performerID)
	if err != nil {
		return nil, 0, err
	}

	total := 0
	views := make([]SourceView, 0, len(store.Sources))
	for _, source := range store.Sources {
		view := SourceView{
			Source:  source,
			Label:   source.Label(),
			Enabled: rt.settings.Enabled(source),
		}
		if u, ok := urls[source]; ok {
			view.URL = u.URL
			view.URLResolvedAt = u.ResolvedAt
		}
		if st, ok := states[source]; ok {
			view.LastSynced = st.LastSynced
			view.NextSyncAfter = st.NextSyncAfter
			view.Error = st.Error
		}

		awards, err := rt.store.AwardsBySource(performerID, source)
		if err != nil {
			return nil, 0, err
		}
		view.Count = len(awards)
		if withAwards {
			view.Awards = awards
		}
		total += len(awards)
		views = append(views, view)
	}
	return views, total, nil
}
