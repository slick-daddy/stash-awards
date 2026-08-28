package syncer

import (
	"errors"
	"time"

	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// isFresh reports whether the stored data for this performer/source is still
// within its sync interval.
func (s *Syncer) isFresh(performerID string, source store.Source) (bool, error) {
	st, err := s.store.State(performerID, source)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if st.NextSyncAfter == "" {
		return false, nil
	}
	next, err := store.ParseTime(st.NextSyncAfter)
	if err != nil {
		// A timestamp that cannot be read is no reason to refuse to sync.
		return false, nil
	}
	return next.After(s.now()), nil
}

// skipped describes a performer/source left alone because its data is fresh. The
// stored URL and count are filled in so the caller has something to show.
func (s *Syncer) skipped(performerID string, source store.Source) (Result, error) {
	res := Result{PerformerID: performerID, Source: source, Status: StatusSkipped, Message: "already up to date"}

	stored, err := s.store.URL(performerID, source)
	switch {
	case err == nil:
		res.URL = stored.URL
		res.Origin = OriginStored
	case !errors.Is(err, store.ErrNotFound):
		return res, err
	}

	awards, err := s.store.AwardsBySource(performerID, source)
	if err != nil {
		return res, err
	}
	res.Awards = len(awards)
	return res, nil
}

// record stores a successful scrape and schedules the next one.
func (s *Syncer) record(performerID string, source store.Source, pageURL string, origin Origin, awards []store.Award) (Result, error) {
	res := Result{
		PerformerID: performerID,
		Source:      source,
		Status:      StatusSynced,
		URL:         pageURL,
		Origin:      origin,
		Awards:      len(awards),
	}

	if err := s.store.SetURL(performerID, source, pageURL); err != nil {
		return res, err
	}
	if err := s.store.ReplaceAwards(performerID, source, awards); err != nil {
		return res, err
	}
	if err := s.store.MarkSynced(performerID, source, s.now().Add(s.settings.SyncInterval())); err != nil {
		return res, err
	}

	s.log.Info("%s: %d award(s) for performer %s from %s", source, len(awards), performerID, pageURL)
	return res, nil
}

// recordFailure notes that a source could not be read and schedules a retry.
// Failures are usually transient, so the retry is sooner than a normal sync but
// far enough out that a source that is down is not hammered.
func (s *Syncer) recordFailure(performerID string, source store.Source, pageURL string, origin Origin, cause error) (Result, error) {
	res := Result{
		PerformerID: performerID,
		Source:      source,
		Status:      StatusFailed,
		URL:         pageURL,
		Origin:      origin,
		Message:     cause.Error(),
	}

	retry := s.now().Add(s.retryInterval())
	if err := s.store.MarkError(performerID, source, cause.Error(), retry); err != nil {
		return res, err
	}

	s.log.Warn("%s: performer %s failed: %v", source, performerID, cause)
	return res, nil
}

// recordNoPage notes that the source has no page for this performer, or that it
// has several and none of them is a certain match.
//
// Neither case is a failure of the plugin, but both are held off for a full sync
// interval: re-running the same fruitless search on every batch run would spend
// the user's rate-limit budget on a question already answered.
func (s *Syncer) recordNoPage(performerID string, source store.Source, matches []sources.Match) (Result, error) {
	res := Result{PerformerID: performerID, Source: source, Candidates: matches}
	if len(matches) > 0 {
		res.Status = StatusAmbiguous
		res.Message = "several possible pages; choose one to link"
		s.log.Info("%s: performer %s needs a choice between: %s", source, performerID, nameList(matches))
	} else {
		res.Status = StatusUnresolved
		res.Message = "no page found for this performer"
		s.log.Info("%s: no page found for performer %s", source, performerID)
	}

	if err := s.store.MarkError(performerID, source, res.Message, s.now().Add(s.settings.SyncInterval())); err != nil {
		return res, err
	}
	return res, nil
}

// retryInterval is how long to wait after a failure, never longer than a normal
// sync interval.
func (s *Syncer) retryInterval() time.Duration {
	interval := s.settings.SyncInterval()
	if interval < RetryInterval {
		return interval
	}
	return RetryInterval
}
