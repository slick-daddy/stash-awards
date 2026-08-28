package syncer

import (
	"context"
	"fmt"

	"github.com/slick-daddy/stash-awards/internal/stashapi"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// BatchSummary counts the outcomes of a batch run.
type BatchSummary struct {
	Performers int            `json:"performers"`
	Synced     int            `json:"synced"`
	Skipped    int            `json:"skipped"`
	Failed     int            `json:"failed"`
	Unresolved int            `json:"unresolved"`
	Ambiguous  int            `json:"ambiguous"`
	Results    []Result       `json:"results,omitempty"`
	Sources    []store.Source `json:"sources,omitempty"`
}

// add folds one result into the summary.
func (b *BatchSummary) add(res Result) {
	switch res.Status {
	case StatusSynced:
		b.Synced++
	case StatusSkipped:
		b.Skipped++
	case StatusFailed:
		b.Failed++
	case StatusUnresolved:
		b.Unresolved++
	case StatusAmbiguous:
		b.Ambiguous++
	}
}

// SyncDue syncs the performer/source pairs whose next sync has come around. This
// is what the scheduled task runs, so nothing here searches the whole library.
func (s *Syncer) SyncDue(ctx context.Context, limit int) (BatchSummary, error) {
	if limit <= 0 {
		limit = BatchSize
	}

	due, err := s.store.Due(s.now(), limit)
	if err != nil {
		return BatchSummary{}, err
	}

	summary := BatchSummary{Sources: s.settings.EnabledSources()}
	if len(due) == 0 {
		s.log.Info("nothing due to sync")
		return summary, nil
	}
	s.log.Info("syncing %d due performer/source pair(s)", len(due))

	seen := map[string]bool{}
	for i, state := range due {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		s.log.Progress(float64(i) / float64(len(due)))

		if !s.settings.Enabled(state.Source) {
			continue
		}
		p, err := s.stash.Performer(ctx, state.PerformerID)
		if err != nil {
			// A performer deleted between the query and now is not a sync failure,
			// and the hook that cleans up after deletion may simply not have run.
			s.log.Warn("skipping performer %s: %v", state.PerformerID, err)
			continue
		}

		res, err := s.SyncSource(ctx, p, state.Source, false)
		if err != nil {
			return summary, err
		}
		summary.add(res)
		summary.Results = append(summary.Results, res)
		if !seen[state.PerformerID] {
			seen[state.PerformerID] = true
			summary.Performers++
		}
	}

	s.log.Progress(1)
	return summary, nil
}

// SyncAll walks every performer in the library. Fresh performers are skipped
// without a request, so a run that is interrupted can simply be started again.
func (s *Syncer) SyncAll(ctx context.Context, force bool) (BatchSummary, error) {
	summary := BatchSummary{Sources: s.settings.EnabledSources()}
	if len(summary.Sources) == 0 {
		return summary, fmt.Errorf("no sources are enabled")
	}

	total := 0
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return summary, err
		}

		performers, count, err := s.stash.Performers(ctx, page, BatchSize)
		if err != nil {
			return summary, err
		}
		if page == 1 {
			total = count
			s.log.Info("syncing awards for %d performer(s)", total)
		}
		if len(performers) == 0 {
			break
		}

		if err := s.syncBatch(ctx, performers, force, &summary); err != nil {
			return summary, err
		}
		if total > 0 {
			s.log.Progress(float64(summary.Performers) / float64(total))
		}

		if len(performers) < BatchSize {
			break
		}
		// Pausing between batches keeps a whole-library sync from looking like a
		// sustained crawl, which is what gets a scraper blocked.
		s.log.Debug("pausing %s before the next batch", BatchPause)
		if err := s.sleep(ctx, BatchPause); err != nil {
			return summary, err
		}
	}

	s.log.Progress(1)
	s.log.Info("sync finished: %d synced, %d skipped, %d failed, %d without a page",
		summary.Synced, summary.Skipped, summary.Failed, summary.Unresolved+summary.Ambiguous)
	return summary, nil
}

// syncBatch syncs one page of performers.
func (s *Syncer) syncBatch(ctx context.Context, performers []stashapi.Performer, force bool, summary *BatchSummary) error {
	for i := range performers {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := &performers[i]

		results, err := s.SyncPerformer(ctx, p, force)
		if err != nil {
			return err
		}
		for _, res := range results {
			summary.add(res)
		}
		summary.Performers++
	}
	return nil
}
