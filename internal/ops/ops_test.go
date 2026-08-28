package ops

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/protocol"
	"github.com/slick-daddy/stash-awards/internal/store"
)

func TestDispatchRejectsMissingMode(t *testing.T) {
	stub := newStashStub(t)
	if _, err := stub.dispatch(t.TempDir(), protocol.Args{}); err == nil {
		t.Fatal("an input with no mode should be an error")
	}
}

func TestDispatchRejectsUnknownMode(t *testing.T) {
	stub := newStashStub(t)
	_, err := stub.dispatch(t.TempDir(), protocol.Args{"mode": "invent"})
	if err == nil || !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("want an unknown mode error, got %v", err)
	}
}

// Stash supplies the plugin directory; without it there is nowhere to keep the
// database, and guessing a location would scatter files.
func TestDispatchRequiresPluginDir(t *testing.T) {
	stub := newStashStub(t)
	sc := stub.connection("")
	_, err := Dispatch(protocol.NewLogTo(io.Discard), protocol.Input{
		ServerConnection: sc,
		Args:             protocol.Args{"mode": ModeGetLinks, "performerId": "1"},
	})
	if err == nil || !strings.Contains(err.Error(), "plugin directory") {
		t.Fatalf("want a missing plugin directory error, got %v", err)
	}
}

// ping is answered without opening the database or calling Stash, because its
// job is to prove the binary itself runs.
func TestPingAnswersWithoutDatabase(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	out, err := stub.dispatch(dir, protocol.Args{"mode": ModePing})
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("ping returned %T", out)
	}
	if m["ok"] != true || m["pluginDir"] != dir {
		t.Fatalf("ping returned %v", m)
	}
}

// The settings a fresh install runs with come from Go, not the YAML, so the
// resolved settings have to be readable by the UI.
func TestGetSettingsLayersSavedValuesOverDefaults(t *testing.T) {
	stub := newStashStub(t)
	stub.settings = map[string]interface{}{
		config.KeyAutoSync:    true,
		config.KeyAIAEnabled:  false,
		config.KeyIAFDDelayMs: float64(5000),
	}

	out, err := stub.dispatch(t.TempDir(), protocol.Args{"mode": ModeSettings})
	if err != nil {
		t.Fatalf("getSettings: %v", err)
	}
	got, ok := out.(config.Settings)
	if !ok {
		t.Fatalf("getSettings returned %T", out)
	}
	if !got.AutoSyncEnabled || got.AIAEnabled || got.IAFDDelayMs != 5000 {
		t.Fatalf("saved values were not applied: %+v", got)
	}
	if !got.IAFDEnabled || got.SyncIntervalDays != config.DefaultSyncIntervalDays {
		t.Fatalf("untouched settings should keep their defaults: %+v", got)
	}
}

// sync=false is the read-only path the UI uses when it only wants what is
// already stored.
func TestGetAwardsReadsStoredDataWithoutSyncing(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	stub.performers["7"] = stubPerformer{ID: "7", Name: "Test Performer"}

	withStore(t, dir, func(db *store.Store) {
		if err := db.SetURL("7", store.SourceIAFD, "https://iafd.test/page"); err != nil {
			t.Fatalf("set url: %v", err)
		}
		awards := []store.Award{award(store.SourceIAFD, "Best Actress", 2021), award(store.SourceIAFD, "Best Scene", 2019)}
		if err := db.ReplaceAwards("7", store.SourceIAFD, awards); err != nil {
			t.Fatalf("replace awards: %v", err)
		}
		if err := db.MarkSynced("7", store.SourceIAFD, time.Now().Add(24*time.Hour)); err != nil {
			t.Fatalf("mark synced: %v", err)
		}
	})

	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeGetAwards, "performerId": "7", "sync": false})
	if err != nil {
		t.Fatalf("getAwards: %v", err)
	}
	payload, ok := out.(AwardsPayload)
	if !ok {
		t.Fatalf("getAwards returned %T", out)
	}
	if payload.PerformerName != "Test Performer" || payload.Total != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Warning != "" || payload.Synced != nil {
		t.Fatalf("sync=false should neither sync nor warn: %+v", payload)
	}
	if len(payload.Sources) != len(store.Sources) {
		t.Fatalf("want one view per source, got %d", len(payload.Sources))
	}

	iafd := payload.Sources[0]
	if iafd.Source != store.SourceIAFD || iafd.Label != "IAFD" || !iafd.Enabled {
		t.Fatalf("unexpected iafd view: %+v", iafd)
	}
	if iafd.URL != "https://iafd.test/page" || iafd.LastSynced == "" || iafd.Count != 2 || len(iafd.Awards) != 2 {
		t.Fatalf("stored iafd data missing from the view: %+v", iafd)
	}
	// Newest first, so the UI does not have to sort.
	if iafd.Awards[0].Year != 2021 {
		t.Fatalf("awards are out of order: %+v", iafd.Awards)
	}

	// A source with nothing stored is still reported, so the UI can say so
	// rather than silently dropping a tab.
	if aia := payload.Sources[1]; aia.Source != store.SourceAIA || aia.Count != 0 || aia.URL != "" {
		t.Fatalf("unexpected aia view: %+v", aia)
	}
}

// Data already on disk is worth showing even when Stash cannot be reached, so
// the failure is reported alongside it rather than instead of it.
func TestGetAwardsWarnsButStillReturnsStoredAwards(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	withStore(t, dir, func(db *store.Store) {
		if err := db.ReplaceAwards("7", store.SourceAIA, []store.Award{award(store.SourceAIA, "Hall of Fame", 2020)}); err != nil {
			t.Fatalf("replace awards: %v", err)
		}
	})
	stub.status = 500

	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeGetAwards, "performerId": "7"})
	if err != nil {
		t.Fatalf("getAwards should not fail when stash is down: %v", err)
	}
	payload := out.(AwardsPayload)
	if payload.Warning == "" {
		t.Fatal("want a warning describing the stash failure")
	}
	if payload.Total != 1 {
		t.Fatalf("stored awards were dropped: %+v", payload)
	}
}

// When every source is fresh, sync=true reports a skip and avoids the
// network entirely. AIA is disabled so the test does not scrape it.
func TestGetAwardsSkipsWhenEverySourceIsFresh(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	stub.performers["7"] = stubPerformer{ID: "7", Name: "Test Performer"}
	stub.settings = map[string]interface{}{config.KeyAIAEnabled: false}
	withStore(t, dir, func(db *store.Store) {
		if err := db.ReplaceAwards("7", store.SourceIAFD, []store.Award{award(store.SourceIAFD, "Best Actress", 2021)}); err != nil {
			t.Fatalf("replace awards: %v", err)
		}
		// A future NextSyncAfter makes the data "fresh", so SyncPerformer
		// should report StatusSkipped rather than scrape.
		if err := db.MarkSynced("7", store.SourceIAFD, time.Now().Add(24*time.Hour)); err != nil {
			t.Fatalf("mark synced: %v", err)
		}
	})

	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeGetAwards, "performerId": "7", "sync": true, "force": false})
	if err != nil {
		t.Fatalf("getAwards: %v", err)
	}
	payload := out.(AwardsPayload)
	if payload.Total != 1 {
		t.Fatalf("stored awards were dropped: %+v", payload)
	}
	if payload.Warning != "" {
		t.Errorf("unexpected warning: %q", payload.Warning)
	}
	// Skipped is the status that proves no scrape happened.
	if len(payload.Synced) == 0 || payload.Synced[0].Status != "skipped" {
		t.Errorf("expected a skipped sync, got %+v", payload.Synced)
	}
}

// A performer id is the one argument every performer operation needs.
func TestPerformerOperationsRequireAnID(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	for _, mode := range []string{ModeGetAwards, ModeGetLinks, ModeSync, ModeUnlink} {
		_, err := stub.dispatch(dir, protocol.Args{"mode": mode})
		if err == nil || !strings.Contains(err.Error(), "performerId") {
			t.Fatalf("%s without a performer id returned %v", mode, err)
		}
	}
}

// An unknown source has to be refused before it reaches a query or a provider
// lookup, since it can only come from a malformed request.
func TestSourceOperationsRejectAnUnknownSource(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	for _, mode := range []string{ModeSearch, ModeLink, ModeUnlink} {
		args := protocol.Args{"mode": mode, "performerId": "7", "source": "wikipedia", "url": "https://iafd.test/x"}
		_, err := stub.dispatch(dir, args)
		if err == nil || !strings.Contains(err.Error(), "unknown source") {
			t.Fatalf("%s with a bogus source returned %v", mode, err)
		}
	}
}

// The link editor needs the URLs and the sync state, but not the award rows.
func TestGetLinksReportsURLsWithoutAwardRows(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	withStore(t, dir, func(db *store.Store) {
		if err := db.SetURL("7", store.SourceAIA, "https://aia.test/p/test"); err != nil {
			t.Fatalf("set url: %v", err)
		}
		if err := db.ReplaceAwards("7", store.SourceAIA, []store.Award{award(store.SourceAIA, "Best Actress", 2022)}); err != nil {
			t.Fatalf("replace awards: %v", err)
		}
		if err := db.MarkError("7", store.SourceIAFD, "page not found", time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("mark error: %v", err)
		}
	})

	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeGetLinks, "performerId": "7"})
	if err != nil {
		t.Fatalf("getLinks: %v", err)
	}
	payload, ok := out.(LinksPayload)
	if !ok {
		t.Fatalf("getLinks returned %T", out)
	}

	bySource := map[store.Source]SourceView{}
	for _, v := range payload.Sources {
		bySource[v.Source] = v
	}
	aia := bySource[store.SourceAIA]
	if aia.URL != "https://aia.test/p/test" || aia.URLResolvedAt == "" {
		t.Fatalf("unexpected aia link: %+v", aia)
	}
	if aia.Count != 1 || aia.Awards != nil {
		t.Fatalf("getLinks should count awards but not carry them: %+v", aia)
	}
	// A stored failure is what the UI shows to explain an empty tab.
	if iafd := bySource[store.SourceIAFD]; iafd.Error != "page not found" || iafd.LastSynced != "" {
		t.Fatalf("unexpected iafd link: %+v", iafd)
	}
}

// Unlinking has to clear the URL as well as the awards, or the next sync would
// resurrect the link the user just rejected.
func TestUnlinkClearsOneSourceOnly(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	withStore(t, dir, func(db *store.Store) {
		for _, source := range store.Sources {
			if err := db.SetURL("7", source, "https://"+string(source)+".test/page"); err != nil {
				t.Fatalf("set url: %v", err)
			}
			if err := db.ReplaceAwards("7", source, []store.Award{award(source, "Best Actress", 2021)}); err != nil {
				t.Fatalf("replace awards: %v", err)
			}
			if err := db.MarkSynced("7", source, time.Now().Add(24*time.Hour)); err != nil {
				t.Fatalf("mark synced: %v", err)
			}
		}
	})

	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeUnlink, "performerId": "7", "source": string(store.SourceIAFD)})
	if err != nil {
		t.Fatalf("unlinkSource: %v", err)
	}
	if m, ok := out.(map[string]interface{}); !ok || m["ok"] != true {
		t.Fatalf("unlinkSource returned %v", out)
	}

	withStore(t, dir, func(db *store.Store) {
		if _, err := db.URL("7", store.SourceIAFD); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("iafd url survived the unlink: %v", err)
		}
		if _, err := db.State("7", store.SourceIAFD); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("iafd schedule survived the unlink: %v", err)
		}
		awards, err := db.AwardsBySource("7", store.SourceIAFD)
		if err != nil || len(awards) != 0 {
			t.Fatalf("iafd awards survived the unlink: %d %v", len(awards), err)
		}
		if awards, err := db.AwardsBySource("7", store.SourceAIA); err != nil || len(awards) != 1 {
			t.Fatalf("the other source should be untouched: %d %v", len(awards), err)
		}
	})
}

// The UI reads the due count to decide whether nudging the backend is worth a
// round trip, so it needs the auto-sync setting in the same reply.
func TestDueCountReportsWaitingWorkAndTheAutoSyncSetting(t *testing.T) {
	stub := newStashStub(t)
	stub.settings = map[string]interface{}{config.KeyAutoSync: true}
	dir := t.TempDir()
	withStore(t, dir, func(db *store.Store) {
		if err := db.MarkSynced("7", store.SourceIAFD, time.Now().Add(-time.Hour)); err != nil {
			t.Fatalf("mark synced: %v", err)
		}
		if err := db.MarkSynced("7", store.SourceAIA, time.Now().Add(24*time.Hour)); err != nil {
			t.Fatalf("mark synced: %v", err)
		}
	})

	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeDueCount})
	if err != nil {
		t.Fatalf("dueCount: %v", err)
	}
	payload, ok := out.(DuePayload)
	if !ok {
		t.Fatalf("dueCount returned %T", out)
	}
	if payload.Count != 1 {
		t.Fatalf("want the one overdue pair, got %d", payload.Count)
	}
	if !payload.AutoSyncEnabled {
		t.Fatal("the auto-sync setting was not reported")
	}
}

// The UI's opportunistic nudge passes requireAutoSync, so a user who left
// auto-sync off never has network traffic started on their behalf. A run that
// declines has to say so: an empty summary looks the same as having no work.
func TestSyncDueObeysTheAutoSyncSettingWhenAsked(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	withStore(t, dir, func(db *store.Store) {
		if err := db.MarkSynced("7", store.SourceIAFD, time.Now().Add(-time.Hour)); err != nil {
			t.Fatalf("mark synced: %v", err)
		}
	})

	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeSyncDue, "requireAutoSync": true})
	if err != nil {
		t.Fatalf("syncDue: %v", err)
	}
	payload, ok := out.(BatchPayload)
	if !ok {
		t.Fatalf("syncDue returned %T", out)
	}
	if payload.Message == "" {
		t.Fatal("want an explanation for the run that did nothing")
	}
	if payload.Performers != 0 || len(payload.Results) != 0 {
		t.Fatalf("nothing should have been synced: %+v", payload)
	}
}

// Stash calls forgetPerformer through a hook, which carries the id in a hook
// context rather than as a plain argument.
func TestForgetPerformerReadsTheHookContext(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	withStore(t, dir, func(db *store.Store) {
		for _, source := range store.Sources {
			if err := db.SetURL("7", source, "https://"+string(source)+".test/page"); err != nil {
				t.Fatalf("set url: %v", err)
			}
			if err := db.ReplaceAwards("7", source, []store.Award{award(source, "Best Actress", 2021)}); err != nil {
				t.Fatalf("replace awards: %v", err)
			}
		}
		if err := db.ReplaceAwards("8", store.SourceIAFD, []store.Award{award(store.SourceIAFD, "Best Scene", 2018)}); err != nil {
			t.Fatalf("replace awards: %v", err)
		}
	})

	// Stash sends the performer id as a JSON number in the hook context.
	args := protocol.Args{"mode": ModeForget, "hookContext": map[string]interface{}{
		"id": float64(7), "type": "Performer.Destroy.Post",
	}}
	out, err := stub.dispatch(dir, args)
	if err != nil {
		t.Fatalf("forgetPerformer: %v", err)
	}
	if m, ok := out.(map[string]interface{}); !ok || m["performerId"] != "7" {
		t.Fatalf("forgetPerformer returned %v", out)
	}

	withStore(t, dir, func(db *store.Store) {
		awards, err := db.Awards("7")
		if err != nil || len(awards) != 0 {
			t.Fatalf("deleted performer still has %d awards: %v", len(awards), err)
		}
		urls, err := db.URLs("7")
		if err != nil || len(urls) != 0 {
			t.Fatalf("deleted performer still has %d urls: %v", len(urls), err)
		}
		if awards, err := db.Awards("8"); err != nil || len(awards) != 1 {
			t.Fatalf("another performer's awards were removed: %d %v", len(awards), err)
		}
	})
}

func TestForgetPerformerNeedsAnID(t *testing.T) {
	stub := newStashStub(t)
	_, err := stub.dispatch(t.TempDir(), protocol.Args{"mode": ModeForget})
	if err == nil || !strings.Contains(err.Error(), "hook context") {
		t.Fatalf("want an error naming the hook context, got %v", err)
	}
}

func TestHookPerformerID(t *testing.T) {
	cases := []struct {
		name string
		args protocol.Args
		want string
	}{
		{"number id", protocol.Args{"hookContext": map[string]interface{}{"id": float64(42)}}, "42"},
		{"string id", protocol.Args{"hookContext": map[string]interface{}{"id": "42"}}, "42"},
		{
			"id only in the mutation input",
			protocol.Args{"hookContext": map[string]interface{}{
				"input": map[string]interface{}{"id": "42"},
			}},
			"42",
		},
		{
			"numeric id in the mutation input",
			protocol.Args{"hookContext": map[string]interface{}{
				"input": map[string]interface{}{"id": float64(42)},
			}},
			"42",
		},
		{"no hook context", protocol.Args{}, ""},
		{"hook context of the wrong shape", protocol.Args{"hookContext": "42"}, ""},
		{"no id anywhere", protocol.Args{"hookContext": map[string]interface{}{"type": "Performer.Destroy.Post"}}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hookPerformerID(tc.args); got != tc.want {
				t.Fatalf("hookPerformerID = %q, want %q", got, tc.want)
			}
		})
	}
}

// Sync with an unknown source must fail validation before any network work.
func TestSyncWithUnknownSourceIsRejected(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	stub.performers["7"] = stubPerformer{ID: "7", Name: "Angela"}
	if _, err := stub.dispatch(dir, protocol.Args{
		"mode": ModeSync, "performerId": "7", "source": "wikipedia",
	}); err == nil || !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("want an unknown source error, got %v", err)
	}
}

// search needs either a name or a performer id; supplying neither is a 400.
func TestSearchRequiresNameOrPerformerID(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	if _, err := stub.dispatch(dir, protocol.Args{
		"mode": ModeSearch, "source": string(store.SourceAIA),
	}); err == nil || !strings.Contains(err.Error(), "name or a performerId") {
		t.Fatalf("want a missing-argument error, got %v", err)
	}
}

// search with a name argument must use the argument verbatim and skip the
// Stash performer lookup. The Stash stub is left with a working handler, so
// if the dispatcher were to consult Stash the path would succeed there too;
// the test instead inspects the reply to confirm the supplied name made it
// through.
func TestSearchUsesNameArgument(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()

	// Stash performer "7" exists, but a "name" argument must take precedence
	// over the performerId-based lookup.
	stub.performers["7"] = stubPerformer{ID: "7", Name: "Different Name"}

	out, err := stub.dispatch(dir, protocol.Args{
		"mode": ModeSearch, "source": string(store.SourceAIA),
		"performerId": "7", "name": "Angela",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	payload, ok := out.(SearchPayload)
	if !ok {
		t.Fatalf("Search returned %T", out)
	}
	if payload.Name != "Angela" {
		t.Errorf("name = %q, want the explicit argument Angela", payload.Name)
	}
}

// search with a performer id but no name must fall back to looking the
// performer up in Stash to get the name. Stash is configured to fail, so
// the fall-back path surfaces a Stash error before the provider is ever
// consulted.
func TestSearchFallsBackToStashWhenNameIsMissing(t *testing.T) {
	stub := newStashStub(t)
	stub.status = http.StatusInternalServerError
	dir := t.TempDir()

	_, err := stub.dispatch(dir, protocol.Args{
		"mode": ModeSearch, "source": string(store.SourceAIA), "performerId": "7",
	})
	if err == nil {
		t.Fatal("Search succeeded despite Stash being down")
	}
	// The fall-back path goes rt.stash.Performer first, so a 500 there
	// produces an error that mentions the HTTP status.
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want a Stash 500 surfaced through the fall-back path", err)
	}
}

// link rejects an empty URL before going to the network.
func TestLinkRejectsEmptyURL(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	if _, err := stub.dispatch(dir, protocol.Args{
		"mode": ModeLink, "performerId": "7", "source": string(store.SourceAIA), "url": "  ",
	}); err == nil || !strings.Contains(err.Error(), "url") {
		t.Fatalf("want a missing-url error, got %v", err)
	}
}

// link requires a performer id like the other performer-scoped modes.
func TestLinkRejectsMissingPerformerID(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	if _, err := stub.dispatch(dir, protocol.Args{
		"mode": ModeLink, "source": string(store.SourceAIA), "url": "https://aia.test/x",
	}); err == nil || !strings.Contains(err.Error(), "performerId") {
		t.Fatalf("want a missing-id error, got %v", err)
	}
}

// link with an unrecognised URL must fail before reaching the network.
func TestLinkRejectsUnrecognisedURL(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	stub.performers["7"] = stubPerformer{ID: "7", Name: "Angela"}
	if _, err := stub.dispatch(dir, protocol.Args{
		"mode": ModeLink, "performerId": "7", "source": string(store.SourceAIA),
		"url": "https://example.com/whoever",
	}); err == nil || !strings.Contains(err.Error(), "not a") {
		t.Fatalf("want an unrecognised-url error, got %v", err)
	}
}

// syncDue with no work to do must still return a payload (not nil).
func TestSyncDueWithNoWorkReturnsAnEmptySummary(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeSyncDue, "limit": 1})
	if err != nil {
		t.Fatalf("syncDue: %v", err)
	}
	payload, ok := out.(BatchPayload)
	if !ok {
		t.Fatalf("syncDue returned %T", out)
	}
	if payload.Performers != 0 || len(payload.Results) != 0 {
		t.Errorf("expected an empty run: %+v", payload)
	}
}

// syncAll walks the library. With an empty store it is a no-op.
func TestSyncAllOnAnEmptyStoreIsANoOp(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeSyncAll})
	if err != nil {
		t.Fatalf("syncAll: %v", err)
	}
	payload, ok := out.(BatchPayload)
	if !ok {
		t.Fatalf("syncAll returned %T", out)
	}
	if payload.Performers != 0 {
		t.Errorf("expected no performers, got %+v", payload)
	}
}

// forget with a plain argument (no hook context) uses the argument.
func TestForgetPerformerReadsThePlainArgument(t *testing.T) {
	stub := newStashStub(t)
	dir := t.TempDir()
	withStore(t, dir, func(db *store.Store) {
		if err := db.SetURL("7", store.SourceIAFD, "https://iafd.test/page"); err != nil {
			t.Fatalf("set url: %v", err)
		}
	})
	out, err := stub.dispatch(dir, protocol.Args{"mode": ModeForget, "performerId": "7"})
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if m, ok := out.(map[string]interface{}); !ok || m["performerId"] != "7" {
		t.Fatalf("forget returned %v", out)
	}
	withStore(t, dir, func(db *store.Store) {
		if _, err := db.URL("7", store.SourceIAFD); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("forget did not clear the url: %v", err)
		}
	})
}
