// Package ops maps a plugin operation name onto the work it performs.
//
// Stash runs this binary once per operation, so every mode has to set up its own
// database handle, HTTP client and Stash connection, do its work and exit.
package ops

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/fetch"
	"github.com/slick-daddy/stash-awards/internal/protocol"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/sources/aia"
	"github.com/slick-daddy/stash-awards/internal/sources/iafd"
	"github.com/slick-daddy/stash-awards/internal/stashapi"
	"github.com/slick-daddy/stash-awards/internal/store"
	"github.com/slick-daddy/stash-awards/internal/syncer"
)

// Operation modes, named by the "mode" argument in the plugin YAML and by the UI.
const (
	ModePing      = "ping"
	ModeSettings  = "getSettings"
	ModeGetAwards = "getAwards"
	ModeGetLinks  = "getLinks"
	ModeSync      = "sync"
	ModeSyncDue   = "syncDue"
	ModeSyncAll   = "syncAll"
	ModeDueCount  = "dueCount"
	ModeSearch    = "searchSource"
	ModeLink      = "linkSource"
	ModeUnlink    = "unlinkSource"
	ModeForget    = "forgetPerformer"
)

// Timeouts. A plugin process that hangs would sit in Stash's job list forever,
// so every mode is bounded; a batch run gets far longer than a UI request.
const (
	interactiveTimeout = 10 * time.Minute
	taskTimeout        = 24 * time.Hour
)

// runtime is everything one operation needs, assembled from the plugin input.
type runtime struct {
	log      *protocol.Log
	in       protocol.Input
	args     protocol.Args
	store    *store.Store
	stash    *stashapi.Client
	settings config.Settings
	syncer   *syncer.Syncer
}

// Dispatch runs the operation named by the "mode" argument.
func Dispatch(log *protocol.Log, in protocol.Input) (interface{}, error) {
	mode := in.Args.String("mode")
	if mode == "" {
		return nil, fmt.Errorf("no mode argument supplied")
	}

	if mode == ModePing {
		// Lets the UI confirm the backend binary is installed and runnable.
		return map[string]interface{}{"ok": true, "pluginDir": in.ServerConnection.PluginDir}, nil
	}

	timeout := interactiveTimeout
	switch mode {
	case ModeSyncDue, ModeSyncAll:
		timeout = taskTimeout
	}

	// Stash stops a task by killing the process; catching the signal lets an
	// interrupted batch run leave the schedule coherent instead of half-written.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rt, err := newRuntime(ctx, log, in)
	if err != nil {
		return nil, err
	}
	defer rt.store.Close()

	return rt.run(ctx, mode)
}

// run performs one mode against a prepared runtime.
func (rt *runtime) run(ctx context.Context, mode string) (interface{}, error) {
	switch mode {
	case ModeSettings:
		return rt.settings, nil
	case ModeGetAwards:
		return rt.getAwards(ctx)
	case ModeGetLinks:
		return rt.getLinks()
	case ModeSync:
		return rt.sync(ctx)
	case ModeSyncDue:
		return rt.syncDue(ctx)
	case ModeSyncAll:
		return rt.syncAll(ctx)
	case ModeDueCount:
		return rt.dueCount()
	case ModeSearch:
		return rt.search(ctx)
	case ModeLink:
		return rt.link(ctx)
	case ModeUnlink:
		return rt.unlink()
	case ModeForget:
		return rt.forget()
	default:
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
}

// newRuntime opens the database and builds the clients this operation needs.
func newRuntime(ctx context.Context, log *protocol.Log, in protocol.Input) (*runtime, error) {
	if in.ServerConnection.PluginDir == "" {
		return nil, fmt.Errorf("stash did not supply a plugin directory, so there is nowhere to keep the database")
	}

	db, err := store.Open(in.ServerConnection.PluginDir)
	if err != nil {
		return nil, err
	}

	stash := stashapi.New(in.ServerConnection)

	// A settings lookup that fails is not worth abandoning the operation for: the
	// defaults are usable, and saying so is more helpful than refusing to run.
	// EnsureDefaults also seeds Stash on a fresh install, so the user-visible
	// settings form and the runtime both show the intended values.
	settings, err := config.EnsureDefaults(ctx, stash, stash)
	if err != nil {
		log.Warn("could not read or seed plugin settings, using defaults: %v", err)
	}

	client := fetch.New(fetch.Options{})
	providers := map[store.Source]sources.Provider{
		store.SourceIAFD: iafd.New(client),
		store.SourceAIA:  aia.New(client),
	}
	for source := range providers {
		client.SetDelay(string(source), settings.Delay(source))
	}

	return &runtime{
		log:      log,
		in:       in,
		args:     in.Args,
		store:    db,
		stash:    stash,
		settings: settings,
		syncer: syncer.New(syncer.Deps{
			Store:     db,
			Stash:     stash,
			Providers: providers,
			Settings:  settings,
			Log:       log,
		}),
	}, nil
}

// performerID reads the performer this operation is about.
func (rt *runtime) performerID() (string, error) {
	id := rt.args.String("performerId")
	if id == "" {
		return "", fmt.Errorf("no performerId argument supplied")
	}
	return id, nil
}

// source reads and validates the source this operation is about.
func (rt *runtime) source() (store.Source, error) {
	s := store.Source(rt.args.String("source"))
	if !s.Valid() {
		return "", fmt.Errorf("unknown source %q", rt.args.String("source"))
	}
	return s, nil
}
