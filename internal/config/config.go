// Package config holds the plugin's settings and their defaults.
//
// Stash keeps plugin settings in its own configuration file and renders the
// plugin settings UI from that file with no notion of a default value. Every
// default therefore lives here, and on a fresh install this package also
// pushes those defaults back into Stash so the form and the runtime agree.
package config

import (
	"context"
	"time"

	"github.com/slick-daddy/stash-awards/internal/sources/aia"
	"github.com/slick-daddy/stash-awards/internal/sources/iafd"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// PluginID is the plugin's id, matching the YAML filename and the id Stash uses
// to address it.
const PluginID = "awards"

// Setting keys as declared in the plugin YAML.
const (
	KeyAutoSync         = "autoSyncEnabled"
	KeySyncIntervalDays = "syncIntervalDays"
	KeyIAFDEnabled      = "iafdEnabled"
	KeyAIAEnabled       = "aiaEnabled"
	KeyIAFDDelayMs      = "iafdDelayMs"
	KeyAIADelayMs       = "aiaDelayMs"
)

// Defaults. Auto-sync is off so that installing the plugin never starts network
// traffic the user did not ask for. The two delays are sourced from the provider
// packages because that is where the rate-limit reasoning lives; config just
// re-exports the values so a manifest test can pin the whole set in one place.
const (
	DefaultAutoSync         = false
	DefaultSyncIntervalDays = 30
	DefaultIAFDEnabled      = true
	DefaultAIAEnabled       = true
	DefaultIAFDDelayMs      = iafd.DefaultDelay
	DefaultAIADelayMs       = aia.DefaultDelay
)

// Bounds keep a mistyped setting from becoming a problem for the sources: a
// delay of zero would hammer them, and an interval of zero would re-scrape
// every performer on every run.
const (
	MinDelayMs         = 100
	MaxDelayMs         = 600000
	MinSyncIntervalDay = 1
	MaxSyncIntervalDay = 3650
)

// Settings is the resolved configuration for one plugin invocation.
type Settings struct {
	AutoSyncEnabled  bool `json:"autoSyncEnabled"`
	SyncIntervalDays int  `json:"syncIntervalDays"`
	IAFDEnabled      bool `json:"iafdEnabled"`
	AIAEnabled       bool `json:"aiaEnabled"`
	IAFDDelayMs      int  `json:"iafdDelayMs"`
	AIADelayMs       int  `json:"aiaDelayMs"`
}

// Default returns the settings a fresh install runs with.
func Default() Settings {
	return Settings{
		AutoSyncEnabled:  DefaultAutoSync,
		SyncIntervalDays: DefaultSyncIntervalDays,
		IAFDEnabled:      DefaultIAFDEnabled,
		AIAEnabled:       DefaultAIAEnabled,
		IAFDDelayMs:      DefaultIAFDDelayMs,
		AIADelayMs:       DefaultAIADelayMs,
	}
}

// settingsSource is the part of the Stash client this package needs, named so
// that tests can supply their own.
type settingsSource interface {
	PluginSettings(ctx context.Context, pluginID string) (map[string]interface{}, error)
}

// settingsWriter is the part of the Stash client this package uses to seed
// defaults back into Stash's saved config. Defined here so tests can stub it.
type settingsWriter interface {
	ConfigurePlugin(ctx context.Context, pluginID string, settings map[string]interface{}) (map[string]interface{}, error)
}

// Load reads the saved settings from Stash and layers them over the defaults. A
// failure to reach Stash returns the defaults along with the error, so a caller
// that would rather continue than abort can.
func Load(ctx context.Context, src settingsSource) (Settings, error) {
	s := Default()
	if src == nil {
		return s, nil
	}
	raw, err := src.PluginSettings(ctx, PluginID)
	if err != nil {
		return s, err
	}
	return FromMap(raw), nil
}

// EnsureDefaults reads the saved settings and, when Stash has never recorded
// any (the install-time empty map), writes the plugin's defaults back through
// the Stash GraphQL API. The plugin's settings UI is rendered from whatever
// Stash has stored, so without this step a fresh install shows empty fields
// while the plugin itself applies the defaults at runtime; the user sees a
// mismatch.
//
// A non-empty saved map is left alone: the user has already touched the form
// and overwriting their choices would be hostile. A failed write is reported
// alongside the resolved defaults so the caller can keep running, since the
// runtime path is still correct without the seed.
func EnsureDefaults(ctx context.Context, src settingsSource, dst settingsWriter) (Settings, error) {
	s := Default()
	if src == nil {
		return s, nil
	}
	raw, err := src.PluginSettings(ctx, PluginID)
	if err != nil {
		return s, err
	}
	if len(raw) > 0 {
		return FromMap(raw), nil
	}
	if dst == nil {
		return s, nil
	}
	if _, err := dst.ConfigurePlugin(ctx, PluginID, s.asMap()); err != nil {
		return s, err
	}
	return s, nil
}

// asMap is the JSON shape Stash's configurePlugin mutation expects.
func (s Settings) asMap() map[string]interface{} {
	return map[string]interface{}{
		KeyAutoSync:         s.AutoSyncEnabled,
		KeySyncIntervalDays: s.SyncIntervalDays,
		KeyIAFDEnabled:      s.IAFDEnabled,
		KeyAIAEnabled:       s.AIAEnabled,
		KeyIAFDDelayMs:      s.IAFDDelayMs,
		KeyAIADelayMs:       s.AIADelayMs,
	}
}

// FromMap layers raw settings over the defaults, ignoring anything absent or of
// an unusable type.
func FromMap(raw map[string]interface{}) Settings {
	s := Default()
	s.AutoSyncEnabled = boolValue(raw, KeyAutoSync, s.AutoSyncEnabled)
	s.IAFDEnabled = boolValue(raw, KeyIAFDEnabled, s.IAFDEnabled)
	s.AIAEnabled = boolValue(raw, KeyAIAEnabled, s.AIAEnabled)
	s.SyncIntervalDays = clamp(intValue(raw, KeySyncIntervalDays, s.SyncIntervalDays), MinSyncIntervalDay, MaxSyncIntervalDay)
	s.IAFDDelayMs = clamp(intValue(raw, KeyIAFDDelayMs, s.IAFDDelayMs), MinDelayMs, MaxDelayMs)
	s.AIADelayMs = clamp(intValue(raw, KeyAIADelayMs, s.AIADelayMs), MinDelayMs, MaxDelayMs)
	return s
}

// Delay returns the minimum spacing between requests to source.
func (s Settings) Delay(source store.Source) time.Duration {
	switch source {
	case store.SourceIAFD:
		return time.Duration(s.IAFDDelayMs) * time.Millisecond
	case store.SourceAIA:
		return time.Duration(s.AIADelayMs) * time.Millisecond
	}
	return time.Duration(MaxDelayMs) * time.Millisecond
}

// Enabled reports whether source should be searched, scraped and shown.
func (s Settings) Enabled(source store.Source) bool {
	switch source {
	case store.SourceIAFD:
		return s.IAFDEnabled
	case store.SourceAIA:
		return s.AIAEnabled
	}
	return false
}

// EnabledSources lists the enabled sources in display order.
func (s Settings) EnabledSources() []store.Source {
	var out []store.Source
	for _, src := range []store.Source{store.SourceIAFD, store.SourceAIA} {
		if s.Enabled(src) {
			out = append(out, src)
		}
	}
	return out
}

// SyncInterval is the gap between automatic syncs of one performer.
func (s Settings) SyncInterval() time.Duration {
	return time.Duration(s.SyncIntervalDays) * 24 * time.Hour
}

// boolValue reads a boolean setting. Stash stores what the UI sent, and a
// checkbox rendered from a string default arrives as a string.
func boolValue(raw map[string]interface{}, key string, def bool) bool {
	switch t := raw[key].(type) {
	case bool:
		return t
	case string:
		switch t {
		case "true", "True", "1":
			return true
		case "false", "False", "0":
			return false
		}
	}
	return def
}

// intValue reads a numeric setting. JSON numbers decode to float64.
func intValue(raw map[string]interface{}, key string, def int) int {
	switch t := raw[key].(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n := 0
		for i, r := range t {
			if r < '0' || r > '9' {
				if i == 0 {
					return def
				}
				return n
			}
			n = n*10 + int(r-'0')
		}
		if t == "" {
			return def
		}
		return n
	}
	return def
}

// clamp holds n inside [lo, hi].
func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
