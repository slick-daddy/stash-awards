// Package protocol implements Stash's "raw" plugin interface: the JSON message
// that Stash writes to the plugin's stdin, the JSON result the plugin writes to
// stdout, and the control-character log encoding Stash reads from stderr.
//
// It deliberately duplicates the wire format rather than importing
// github.com/stashapp/stash/pkg/plugin/common, so that this plugin stays a
// small standalone module instead of depending on the whole Stash server.
//
// The format is defined by Stash in pkg/plugin/common/msg.go and
// pkg/logger/plugin.go.
package protocol

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ServerConnection describes how to reach the Stash server that spawned us.
type ServerConnection struct {
	Scheme string `json:"Scheme"`
	Host   string `json:"Host"`
	Port   int    `json:"Port"`

	// SessionCookie authenticates GraphQL calls back to Stash.
	SessionCookie *http.Cookie `json:"SessionCookie"`

	// Dir is the directory holding the Stash server's config file.
	Dir string `json:"Dir"`

	// PluginDir is the directory holding this plugin's YAML config. The
	// plugin database lives here.
	PluginDir string `json:"PluginDir"`
}

// Input is the JSON document Stash writes to the plugin's stdin.
type Input struct {
	ServerConnection ServerConnection `json:"server_connection"`
	Args             Args             `json:"args"`
}

// Output is the JSON document Stash reads from the plugin's stdout. Exactly one
// of Error and Output is expected to be set.
type Output struct {
	Error  *string     `json:"error"`
	Output interface{} `json:"output"`
}

// SetError records err as the operation's failure.
func (o *Output) SetError(err error) {
	s := err.Error()
	o.Error = &s
	o.Output = nil
}

// Args holds the operation arguments. Values arrive as generic JSON, so numbers
// are float64 and everything may be absent; the accessors below normalise that.
type Args map[string]interface{}

// String returns the value for key as a string, or "" if absent or not a string.
func (a Args) String(key string) string {
	s, _ := a[key].(string)
	return s
}

// Int returns the value for key as an int. JSON numbers decode to float64, and
// Stash's task defaultArgs are always strings, so both are accepted. Returns
// def when the key is absent or cannot be interpreted as a number.
func (a Args) Int(key string, def int) int {
	v, ok := a[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
	case string:
		var i int
		if _, err := fmt.Sscanf(t, "%d", &i); err == nil {
			return i
		}
	}
	return def
}

// Bool returns the value for key as a bool, accepting both real booleans and
// the strings "true"/"false" that Stash's task defaultArgs produce. Returns def
// when the key is absent or cannot be interpreted.
func (a Args) Bool(key string, def bool) bool {
	v, ok := a[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
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

// StringSlice returns the value for key as a []string, accepting a JSON array
// of strings or a single string. Returns nil when the key is absent.
func (a Args) StringSlice(key string) []string {
	v, ok := a[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []interface{}:
		ret := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				ret = append(ret, s)
			}
		}
		return ret
	case string:
		return []string{t}
	}
	return nil
}
