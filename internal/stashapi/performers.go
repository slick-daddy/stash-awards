package stashapi

import (
	"context"
	"fmt"
)

// Performer is the subset of a Stash performer this plugin reads. The urls
// field is where an existing IAFD or AIA link is found, which is the first step
// of performer matching.
type Performer struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	URLs    []string `json:"urls"`
	Aliases []string `json:"alias_list"`
}

// performerFields is the selection set used by every performer query.
const performerFields = "id name urls alias_list"

// Performer looks up one performer by id.
func (c *Client) Performer(ctx context.Context, id string) (*Performer, error) {
	doc := "query FindPerformer($id: ID!) { findPerformer(id: $id) { " + performerFields + " } }"

	var data struct {
		FindPerformer *Performer `json:"findPerformer"`
	}
	if err := c.query(ctx, doc, map[string]interface{}{"id": id}, &data); err != nil {
		return nil, err
	}
	if data.FindPerformer == nil {
		return nil, fmt.Errorf("performer %s: %w", id, ErrNotFound)
	}
	return data.FindPerformer, nil
}

// Performers returns one page of performers along with the total count, sorted
// by id so that paging stays stable while a long sync runs.
func (c *Client) Performers(ctx context.Context, page, perPage int) ([]Performer, int, error) {
	if page < 1 {
		page = 1
	}
	doc := "query AllPerformers($page: Int!, $perPage: Int!) {" +
		" findPerformers(filter: {page: $page, per_page: $perPage, sort: \"id\", direction: ASC})" +
		" { count performers { " + performerFields + " } } }"

	var data struct {
		FindPerformers struct {
			Count      int         `json:"count"`
			Performers []Performer `json:"performers"`
		} `json:"findPerformers"`
	}
	vars := map[string]interface{}{"page": page, "perPage": perPage}
	if err := c.query(ctx, doc, vars, &data); err != nil {
		return nil, 0, err
	}
	return data.FindPerformers.Performers, data.FindPerformers.Count, nil
}

// PluginSettings returns the saved settings for one plugin id. Stash keeps
// these in its own config file, and returns no entry at all until the user
// changes something, so an absent plugin is not an error.
func (c *Client) PluginSettings(ctx context.Context, pluginID string) (map[string]interface{}, error) {
	doc := "query PluginSettings($include: [ID!]) { configuration { plugins(include: $include) } }"

	var data struct {
		Configuration struct {
			Plugins map[string]map[string]interface{} `json:"plugins"`
		} `json:"configuration"`
	}
	vars := map[string]interface{}{"include": []string{pluginID}}
	if err := c.query(ctx, doc, vars, &data); err != nil {
		return nil, err
	}
	settings := data.Configuration.Plugins[pluginID]
	if settings == nil {
		settings = map[string]interface{}{}
	}
	return settings, nil
}
