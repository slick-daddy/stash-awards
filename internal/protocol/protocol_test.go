package protocol

import (
	"encoding/json"
	"testing"
)

func TestArgsAccessors(t *testing.T) {
	// Numbers arrive as float64 from JSON, but Stash task defaultArgs are
	// always strings. Both must work.
	var args Args
	raw := `{"n":30,"nstr":"45","b":true,"bstr":"true","s":"hi","list":["a","b"],"one":"solo"}`
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Fatal(err)
	}

	if got := args.Int("n", -1); got != 30 {
		t.Errorf("Int(n) = %d, want 30", got)
	}
	if got := args.Int("nstr", -1); got != 45 {
		t.Errorf("Int(nstr) = %d, want 45", got)
	}
	if got := args.Int("missing", -1); got != -1 {
		t.Errorf("Int(missing) = %d, want -1", got)
	}
	if !args.Bool("b", false) {
		t.Error("Bool(b) = false, want true")
	}
	if !args.Bool("bstr", false) {
		t.Error("Bool(bstr) = false, want true")
	}
	if !args.Bool("missing", true) {
		t.Error("Bool(missing) should fall back to the default")
	}
	if got := args.String("s"); got != "hi" {
		t.Errorf("String(s) = %q, want \"hi\"", got)
	}
	if got := args.String("missing"); got != "" {
		t.Errorf("String(missing) = %q, want \"\"", got)
	}
	if got := args.StringSlice("list"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("StringSlice(list) = %v, want [a b]", got)
	}
	if got := args.StringSlice("one"); len(got) != 1 || got[0] != "solo" {
		t.Errorf("StringSlice(one) = %v, want [solo]", got)
	}
	if got := args.StringSlice("missing"); got != nil {
		t.Errorf("StringSlice(missing) = %v, want nil", got)
	}
}

func TestInputDecodesStashMessage(t *testing.T) {
	// Shape taken from Stash's pkg/plugin/common.PluginInput as serialised by
	// pkg/plugin/raw.go.
	raw := `{
		"server_connection": {
			"Scheme": "http",
			"Host": "localhost",
			"Port": 9999,
			"SessionCookie": {"Name":"session","Value":"abc"},
			"Dir": "/config",
			"PluginDir": "/config/plugins/awards"
		},
		"args": {"mode": "getAwards", "performer_id": "7"}
	}`

	var in Input
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	if in.ServerConnection.Port != 9999 {
		t.Errorf("Port = %d, want 9999", in.ServerConnection.Port)
	}
	if in.ServerConnection.PluginDir != "/config/plugins/awards" {
		t.Errorf("PluginDir = %q", in.ServerConnection.PluginDir)
	}
	if in.ServerConnection.SessionCookie == nil || in.ServerConnection.SessionCookie.Value != "abc" {
		t.Errorf("SessionCookie = %+v", in.ServerConnection.SessionCookie)
	}
	if in.Args.String("mode") != "getAwards" {
		t.Errorf("mode = %q", in.Args.String("mode"))
	}
}

func TestOutputSetError(t *testing.T) {
	out := Output{Output: "stale"}
	out.SetError(errFake{})
	if out.Error == nil || *out.Error != "boom" {
		t.Fatalf("Error = %v, want \"boom\"", out.Error)
	}
	if out.Output != nil {
		t.Errorf("Output should be cleared on error, got %v", out.Output)
	}
}

type errFake struct{}

func (errFake) Error() string { return "boom" }
