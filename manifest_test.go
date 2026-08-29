package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/ops"
)

// manifest mirrors the parts of Stash's plugin configuration schema this plugin
// uses (pkg/plugin/config.go). Stash parses the file strictly, so a field it
// does not know stops the plugin from loading at all; decoding into this struct
// with KnownFields set reproduces that.
type manifest struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
	URL         string `yaml:"url"`
	Interface   string `yaml:"interface"`
	Exec        []string `yaml:"exec"`
	Tasks       []operation `yaml:"tasks"`
	Hooks       []hook      `yaml:"hooks"`
	UI          struct {
		Javascript []string `yaml:"javascript"`
		CSS        []string `yaml:"css"`
	} `yaml:"ui"`
	Settings map[string]struct {
		DisplayName string `yaml:"displayName"`
		Description string `yaml:"description"`
		Type        string `yaml:"type"`
	} `yaml:"settings"`
}

type operation struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	ExecArgs    []string          `yaml:"execArgs"`
	DefaultArgs map[string]string `yaml:"defaultArgs"`
}

type hook struct {
	operation   `yaml:",inline"`
	TriggeredBy []string `yaml:"triggeredBy"`
}

func loadManifest(t *testing.T) manifest {
	t.Helper()
	f, err := os.Open("plugin/awards.yml")
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var m manifest
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("stash would refuse this manifest: %v", err)
	}
	return m
}

func TestManifestDeclaresTheBackendBinary(t *testing.T) {
	m := loadManifest(t)
	if m.Name == "" || m.Description == "" || m.Version == "" || m.URL == "" {
		t.Fatalf("the plugin manager shows all of these: %+v", m)
	}
	// The Go binary speaks the raw protocol; "js" would look for a script.
	if m.Interface != "raw" {
		t.Fatalf("interface = %q, want raw", m.Interface)
	}
	if len(m.Exec) != 1 || m.Exec[0] != "stash-awards" {
		t.Fatalf("exec = %v, want the bare binary name so stash adds .exe itself", m.Exec)
	}
	if len(m.UI.Javascript) != 1 || len(m.UI.CSS) != 1 {
		t.Fatalf("the ui bundle is one script and one stylesheet: %+v", m.UI)
	}
}

// Every mode named in the manifest has to be one Dispatch answers, or the task
// fails only when a user runs it.
func TestManifestModesAreDispatchable(t *testing.T) {
	m := loadManifest(t)
	known := map[string]bool{}
	for _, mode := range []string{
		ops.ModePing, ops.ModeSettings, ops.ModeGetAwards, ops.ModeGetLinks,
		ops.ModeSync, ops.ModeSyncDue, ops.ModeSyncAll, ops.ModeDueCount,
		ops.ModeSearch, ops.ModeLink, ops.ModeUnlink, ops.ModeForget,
	} {
		known[mode] = true
	}

	operations := append([]operation{}, m.Tasks...)
	for _, h := range m.Hooks {
		operations = append(operations, h.operation)
	}
	if len(operations) < 4 {
		t.Fatalf("want the three tasks and the destroy hook, got %d operations", len(operations))
	}

	for _, op := range operations {
		if op.Name == "" || op.Description == "" {
			t.Errorf("every task and hook is a labelled button in the UI: %+v", op)
		}
		mode := op.DefaultArgs["mode"]
		if !known[mode] {
			t.Errorf("%q runs mode %q, which Dispatch does not answer", op.Name, mode)
		}
	}
}

// A performer deleted in Stash has to be forgotten here too, and the hook that
// does it is only called if the manifest asks for the right trigger.
func TestManifestHooksTheDestroyTrigger(t *testing.T) {
	m := loadManifest(t)
	if len(m.Hooks) != 1 {
		t.Fatalf("want one hook, got %d", len(m.Hooks))
	}
	h := m.Hooks[0]
	if h.DefaultArgs["mode"] != ops.ModeForget {
		t.Fatalf("the hook runs mode %q", h.DefaultArgs["mode"])
	}
	if len(h.TriggeredBy) != 1 || h.TriggeredBy[0] != "Performer.Destroy.Post" {
		t.Fatalf("triggeredBy = %v, want Performer.Destroy.Post", h.TriggeredBy)
	}
}

// The settings the manifest declares are the ones the backend reads. A key that
// only exists on one side is a setting the user can change with no effect.
func TestManifestSettingsMatchTheBackend(t *testing.T) {
	m := loadManifest(t)
	want := map[string]string{
		config.KeyAutoSync:         "BOOLEAN",
		config.KeyIAFDEnabled:      "BOOLEAN",
		config.KeyAIAEnabled:       "BOOLEAN",
		config.KeySyncIntervalDays: "NUMBER",
		config.KeyIAFDDelayMs:      "NUMBER",
		config.KeyAIADelayMs:       "NUMBER",
	}

	for key, kind := range want {
		s, ok := m.Settings[key]
		if !ok {
			t.Errorf("the backend reads %q but the manifest never offers it", key)
			continue
		}
		if s.Type != kind {
			t.Errorf("setting %q is declared %q, want %q", key, s.Type, kind)
		}
		if s.DisplayName == "" {
			t.Errorf("setting %q would show as its bare key", key)
		}
		// Stash cannot render a default, so the description has to state it.
		if !strings.Contains(s.Description, "default") {
			t.Errorf("setting %q does not tell the user its default: %q", key, s.Description)
		}
		switch s.Type {
		case "BOOLEAN":
			if !descriptionMatchesBool(s.Description, boolDefault(key)) {
				t.Errorf("setting %q description %q does not match its default %t, so a fresh install will look broken", key, s.Description, boolDefault(key))
			}
		case "NUMBER":
			if !descriptionMatchesInt(s.Description, intDefault(key)) {
				t.Errorf("setting %q description %q does not match its default %d, so a fresh install will look broken", key, s.Description, intDefault(key))
			}
		}
	}

	for key := range m.Settings {
		if _, ok := want[key]; !ok {
			t.Errorf("the manifest offers %q, which the backend ignores", key)
		}
	}
}

func intDefault(key string) int {
	switch key {
	case config.KeySyncIntervalDays:
		return config.DefaultSyncIntervalDays
	case config.KeyIAFDDelayMs:
		return config.DefaultIAFDDelayMs
	case config.KeyAIADelayMs:
		return config.DefaultAIADelayMs
	}
	return 0
}

func boolDefault(key string) bool {
	switch key {
	case config.KeyAutoSync:
		return config.DefaultAutoSync
	case config.KeyIAFDEnabled:
		return config.DefaultIAFDEnabled
	case config.KeyAIAEnabled:
		return config.DefaultAIAEnabled
	}
	return false
}

// descriptionMatchesInt reports whether the prose carries want as a standalone
// number. "uses 30 when" matches 30; "2000 ms" matches 2000. Stray digits that
// happen to spell the value in unrelated words (e.g. "1 of 30 settings") do
// not match, because the prose has to state the value plainly.
func descriptionMatchesInt(desc string, want int) bool {
	wantStr := strconv.Itoa(want)
	for i := 0; i+len(wantStr) <= len(desc); {
		if desc[i:i+len(wantStr)] == wantStr {
			left := i == 0 || !isWordByte(desc[i-1])
			right := i+len(wantStr) == len(desc) || !isWordByte(desc[i+len(wantStr)])
			if left && right {
				return true
			}
		}
		i++
	}
	return false
}

// descriptionMatchesBool accepts the prose the manifest actually uses to
// describe an on-or-off setting.
func descriptionMatchesBool(desc string, want bool) bool {
	low := strings.ToLower(desc)
	if want {
		return strings.Contains(low, "on by default") || strings.Contains(low, "enables this by default")
	}
	return strings.Contains(low, "off by default") || strings.Contains(low, "disables this by default")
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

