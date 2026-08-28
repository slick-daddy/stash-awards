package config

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/slick-daddy/stash-awards/internal/sources/aia"
	"github.com/slick-daddy/stash-awards/internal/sources/iafd"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// fakeSource stands in for the Stash client.
type fakeSource struct {
	settings map[string]interface{}
	err      error
}

func (f fakeSource) PluginSettings(context.Context, string) (map[string]interface{}, error) {
	return f.settings, f.err
}

func TestDefaultsMatchTheSourcesOwnRecommendations(t *testing.T) {
	d := Default()
	if d.IAFDDelayMs != iafd.DefaultDelay {
		t.Errorf("iafd delay = %d, want the provider's own %d", d.IAFDDelayMs, iafd.DefaultDelay)
	}
	if d.AIADelayMs != aia.DefaultDelay {
		t.Errorf("aia delay = %d, want the provider's own %d", d.AIADelayMs, aia.DefaultDelay)
	}
	// Installing the plugin must not start scraping on its own.
	if d.AutoSyncEnabled {
		t.Error("auto-sync defaults to on")
	}
}

// Stash returns nothing at all for a plugin whose settings were never saved.
func TestLoadFallsBackToDefaultsWhenNothingIsSaved(t *testing.T) {
	got, err := Load(context.Background(), fakeSource{settings: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != Default() {
		t.Errorf("settings = %+v, want the defaults", got)
	}
}

// A plugin operation is still worth attempting when only the settings lookup
// failed, so Load hands back usable defaults alongside the error.
func TestLoadReturnsDefaultsAlongsideAnError(t *testing.T) {
	boom := errors.New("stash is down")
	got, err := Load(context.Background(), fakeSource{err: boom})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the underlying failure", err)
	}
	if got != Default() {
		t.Errorf("settings = %+v, want the defaults", got)
	}
}

// A nil source must not panic; the plugin must still hand back the defaults
// rather than refusing to run.
func TestLoadToleratesANilSource(t *testing.T) {
	got, err := Load(context.Background(), nil)
	if err != nil {
		t.Fatalf("err = %v, want nil for a missing source", err)
	}
	if got != Default() {
		t.Errorf("settings = %+v, want the defaults", got)
	}
}

func TestFromMapReadsSavedValues(t *testing.T) {
	got := FromMap(map[string]interface{}{
		KeyAutoSync:         true,
		KeySyncIntervalDays: float64(7),
		KeyIAFDEnabled:      false,
		KeyAIAEnabled:       true,
		KeyIAFDDelayMs:      float64(3000),
		KeyAIADelayMs:       float64(750),
	})
	want := Settings{
		AutoSyncEnabled:  true,
		SyncIntervalDays: 7,
		IAFDEnabled:      false,
		AIAEnabled:       true,
		IAFDDelayMs:      3000,
		AIADelayMs:       750,
	}
	if got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

// Stash's task defaultArgs and some UI paths deliver numbers and booleans as
// strings.
func TestFromMapAcceptsStringValues(t *testing.T) {
	got := FromMap(map[string]interface{}{
		KeyAutoSync:    "true",
		KeyIAFDDelayMs: "2500",
	})
	if !got.AutoSyncEnabled {
		t.Error("string \"true\" did not enable auto-sync")
	}
	if got.IAFDDelayMs != 2500 {
		t.Errorf("iafd delay = %d, want 2500", got.IAFDDelayMs)
	}
}

// A zero delay would hammer the source and a zero interval would re-scrape every
// performer on every run, so out-of-range values are pulled back into range
// rather than obeyed.
func TestFromMapClampsUnusableValues(t *testing.T) {
	got := FromMap(map[string]interface{}{
		KeyIAFDDelayMs:      float64(0),
		KeyAIADelayMs:       float64(-1),
		KeySyncIntervalDays: float64(0),
	})
	if got.IAFDDelayMs != MinDelayMs || got.AIADelayMs != MinDelayMs {
		t.Errorf("delays = %d/%d, want both clamped to %d", got.IAFDDelayMs, got.AIADelayMs, MinDelayMs)
	}
	if got.SyncIntervalDays != MinSyncIntervalDay {
		t.Errorf("interval = %d, want %d", got.SyncIntervalDays, MinSyncIntervalDay)
	}

	huge := FromMap(map[string]interface{}{KeyIAFDDelayMs: float64(1 << 40)})
	if huge.IAFDDelayMs != MaxDelayMs {
		t.Errorf("delay = %d, want %d", huge.IAFDDelayMs, MaxDelayMs)
	}
}

func TestFromMapIgnoresJunk(t *testing.T) {
	got := FromMap(map[string]interface{}{
		KeyIAFDEnabled: "maybe",
		KeyAIADelayMs:  []interface{}{1, 2},
	})
	if got.IAFDEnabled != DefaultIAFDEnabled || got.AIADelayMs != aia.DefaultDelay {
		t.Errorf("settings = %+v, want the defaults preserved", got)
	}
}

func TestBoolValueCoversEveryBranch(t *testing.T) {
	// Direct test of the parser, since FromMap only feeds it the surface
	// paths and the falsy-string fallthrough is otherwise untested.
	cases := []struct {
		name string
		in   interface{}
		def  bool
		want bool
	}{
		{"bool true", true, false, true},
		{"bool false", false, true, false},
		{"string True", "True", false, true},
		{"string 1", "1", false, true},
		{"string false", "false", true, false},
		{"string 0", "0", true, false},
		{"unrecognised string", "yes", false, false},
		{"unrecognised string keeps default true", "yes", true, true},
		{"nil", nil, true, true},
		{"number", 1, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := boolValue(map[string]interface{}{KeyAutoSync: c.in}, KeyAutoSync, c.def); got != c.want {
				t.Errorf("boolValue(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestIntValueCoversEveryBranch(t *testing.T) {
	// "12abc" exercises the non-digit-after-first-character branch: it parses
	// the leading digits and stops, rather than discarding the whole value.
	cases := []struct {
		name string
		in   interface{}
		def  int
		want int
	}{
		{"float64", float64(42), -1, 42},
		{"int literal", 42, -1, 42},
		{"plain string", "42", -1, 42},
		{"string with trailing junk", "12abc", -1, 12},
		{"empty string", "", -1, -1},
		{"non-digit at start", "abc", -1, -1},
		{"nil", nil, 99, 99},
		{"bool", true, 99, 99},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := intValue(map[string]interface{}{KeyIAFDDelayMs: c.in}, KeyIAFDDelayMs, c.def); got != c.want {
				t.Errorf("intValue(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestDelayAndEnabledAreLookedUpBySource(t *testing.T) {
	s := Settings{IAFDEnabled: true, AIAEnabled: false, IAFDDelayMs: 2000, AIADelayMs: 500}

	if got := s.Delay(store.SourceIAFD); got != 2*time.Second {
		t.Errorf("iafd delay = %v, want 2s", got)
	}
	if got := s.Delay(store.SourceAIA); got != 500*time.Millisecond {
		t.Errorf("aia delay = %v, want 500ms", got)
	}
	// An unknown source must not be treated as unlimited.
	if got := s.Delay(store.Source("nonsense")); got < time.Second {
		t.Errorf("unknown source delay = %v, want a cautious value", got)
	}
	if s.Enabled(store.Source("nonsense")) {
		t.Error("an unknown source reported as enabled")
	}

	if got := s.EnabledSources(); len(got) != 1 || got[0] != store.SourceIAFD {
		t.Errorf("enabled sources = %v, want just iafd", got)
	}
}

func TestSyncIntervalIsInDays(t *testing.T) {
	if got := (Settings{SyncIntervalDays: 30}).SyncInterval(); got != 30*24*time.Hour {
		t.Errorf("interval = %v, want 720h", got)
	}
}
