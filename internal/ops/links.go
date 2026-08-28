package ops

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/slick-daddy/stash-awards/internal/protocol"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/store"
	"github.com/slick-daddy/stash-awards/internal/syncer"
)

// SearchPayload offers candidate pages for a performer.
type SearchPayload struct {
	Source  store.Source    `json:"source"`
	Name    string          `json:"name"`
	Matches []sources.Match `json:"matches"`
}

// search looks for candidate pages on one source. The name defaults to the Stash
// performer's own, which is what the "find this performer" button sends.
func (rt *runtime) search(ctx context.Context) (interface{}, error) {
	source, err := rt.source()
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(rt.args.String("name"))
	if name == "" {
		id := rt.args.String("performerId")
		if id == "" {
			return nil, fmt.Errorf("supply either a name or a performerId to search for")
		}
		p, err := rt.stash.Performer(ctx, id)
		if err != nil {
			return nil, err
		}
		name = p.Name
	}

	matches, err := rt.syncer.Search(ctx, source, name)
	if err != nil {
		return nil, err
	}
	return SearchPayload{Source: source, Name: name, Matches: matches}, nil
}

// link records the page a user chose and syncs it straight away, so the choice is
// confirmed by real data rather than accepted on faith.
func (rt *runtime) link(ctx context.Context) (interface{}, error) {
	id, err := rt.performerID()
	if err != nil {
		return nil, err
	}
	source, err := rt.source()
	if err != nil {
		return nil, err
	}
	rawURL := strings.TrimSpace(rt.args.String("url"))
	if rawURL == "" {
		return nil, fmt.Errorf("no url argument supplied")
	}

	res, err := rt.syncer.Link(ctx, id, source, rawURL)
	if err != nil {
		return nil, err
	}
	return SyncPayload{PerformerID: id, Results: []syncer.Result{res}}, nil
}

// unlink forgets one source for one performer.
func (rt *runtime) unlink() (interface{}, error) {
	id, err := rt.performerID()
	if err != nil {
		return nil, err
	}
	source, err := rt.source()
	if err != nil {
		return nil, err
	}

	if err := rt.syncer.Unlink(id, source); err != nil {
		return nil, err
	}
	rt.log.Info("unlinked %s for performer %s", source, id)
	return map[string]interface{}{"ok": true, "performerId": id, "source": source}, nil
}

// hookPerformerID pulls the performer id out of the hook context Stash adds to a
// hook operation's arguments. Stash sends the id as a number, while everything
// else in this plugin keeps performer ids as the strings GraphQL uses.
func hookPerformerID(args protocol.Args) string {
	hc, ok := args["hookContext"].(map[string]interface{})
	if !ok {
		return ""
	}

	switch t := hc["id"].(type) {
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case string:
		return t
	}

	// Older payloads and some mutations carry the id only inside the mutation
	// input.
	if input, ok := hc["input"].(map[string]interface{}); ok {
		if s, ok := input["id"].(string); ok {
			return s
		}
		if f, ok := input["id"].(float64); ok {
			return strconv.FormatInt(int64(f), 10)
		}
	}
	return ""
}
