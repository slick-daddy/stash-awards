// Package stashapi is a minimal GraphQL client for the Stash server that
// spawned this plugin.
//
// Stash hands every plugin process a ServerConnection describing where the
// server listens and a session cookie that authenticates calls back to it, so
// the plugin needs no credentials of its own.
package stashapi

import (
	"context"
	"fmt"
)

// ConfigurePlugin overwrites the saved settings for one plugin id. Stash
// renders the plugin's settings UI from whatever it has stored, so on a fresh
// install this call is the only way to make the form show the plugin's
// intended defaults instead of empty fields.
//
// Stash's schema has no default-value field on settings, and the form does
// not consult the plugin. Seeding the values here keeps the install UX and
// the runtime on the same page.
//
// Returns the map Stash now has stored for this plugin id.
func (c *Client) ConfigurePlugin(ctx context.Context, pluginID string, settings map[string]interface{}) (map[string]interface{}, error) {
	doc := "mutation ConfigurePlugin($id: ID!, $input: Map!) { configurePlugin(plugin_id: $id, input: $input) }"
	var data struct {
		ConfigurePlugin map[string]interface{} `json:"configurePlugin"`
	}
	if err := c.mutate(ctx, doc, map[string]interface{}{"id": pluginID, "input": settings}, &data); err != nil {
		return nil, fmt.Errorf("configure plugin %s: %w", pluginID, err)
	}
	return data.ConfigurePlugin, nil
}