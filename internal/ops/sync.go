package ops

import (
	"context"
	"fmt"
	"time"

	"github.com/slick-daddy/stash-awards/internal/syncer"
)

// nowFunc is the clock, replaceable in tests.
var nowFunc = time.Now

// SyncPayload is the reply to an explicit sync request.
type SyncPayload struct {
	PerformerID string          `json:"performerId"`
	Results     []syncer.Result `json:"results"`
}

// sync refreshes one performer now. An explicit request means now, so freshness
// is ignored unless the caller says otherwise.
func (rt *runtime) sync(ctx context.Context) (interface{}, error) {
	id, err := rt.performerID()
	if err != nil {
		return nil, err
	}
	force := rt.args.Bool("force", true)

	if rt.args.String("source") != "" {
		source, err := rt.source()
		if err != nil {
			return nil, err
		}
		p, err := rt.stash.Performer(ctx, id)
		if err != nil {
			return nil, err
		}
		res, err := rt.syncer.SyncSource(ctx, p, source, force)
		if err != nil {
			return nil, err
		}
		return SyncPayload{PerformerID: id, Results: []syncer.Result{res}}, nil
	}

	results, err := rt.syncer.SyncPerformerID(ctx, id, force)
	if err != nil {
		return nil, err
	}
	return SyncPayload{PerformerID: id, Results: results}, nil
}

// DuePayload reports how much work is waiting.
type DuePayload struct {
	Count           int  `json:"count"`
	AutoSyncEnabled bool `json:"autoSyncEnabled"`
}

// dueCount reports how many performer/source pairs are ready to be synced. The
// UI uses it to decide whether nudging the backend is worth the round trip.
func (rt *runtime) dueCount() (interface{}, error) {
	count, err := rt.store.DueCount(nowFunc())
	if err != nil {
		return nil, err
	}
	return DuePayload{Count: count, AutoSyncEnabled: rt.settings.AutoSyncEnabled}, nil
}

// BatchPayload is the reply to a batch sync.
type BatchPayload struct {
	syncer.BatchSummary
	// Message explains a run that did nothing, which is otherwise indistinguishable
	// from a run with nothing to do.
	Message string `json:"message,omitempty"`
}

// syncDue works through the performers whose next sync has come around. This is
// the scheduled task, and it is also what the UI nudges: passing
// requireAutoSync=true makes the run obey the auto-sync setting, while a person
// running the task by hand has already said what they want.
func (rt *runtime) syncDue(ctx context.Context) (interface{}, error) {
	if rt.args.Bool("requireAutoSync", false) && !rt.settings.AutoSyncEnabled {
		rt.log.Debug("auto-sync is disabled; nothing to do")
		return BatchPayload{Message: "auto-sync is disabled in the plugin settings"}, nil
	}

	limit := rt.args.Int("limit", syncer.BatchSize)
	summary, err := rt.syncer.SyncDue(ctx, limit)
	if err != nil {
		return nil, err
	}
	return BatchPayload{BatchSummary: summary}, nil
}

// syncAll walks the whole library. It is deliberately a separate, explicit task:
// on a large collection it is a long run against rate-limited sources.
func (rt *runtime) syncAll(ctx context.Context) (interface{}, error) {
	summary, err := rt.syncer.SyncAll(ctx, rt.args.Bool("force", false))
	if err != nil {
		return nil, err
	}
	return BatchPayload{BatchSummary: summary}, nil
}

// forget clears everything stored about a deleted performer. Stash calls this
// through the Performer.Destroy.Post hook, which supplies the id in a hook
// context rather than as a plain argument.
func (rt *runtime) forget() (interface{}, error) {
	id := rt.args.String("performerId")
	if id == "" {
		id = hookPerformerID(rt.args)
	}
	if id == "" {
		return nil, fmt.Errorf("no performer id in the arguments or hook context")
	}

	if err := rt.store.ForgetPerformer(id); err != nil {
		return nil, err
	}
	rt.log.Info("forgot stored awards for deleted performer %s", id)
	return map[string]interface{}{"ok": true, "performerId": id}, nil
}
