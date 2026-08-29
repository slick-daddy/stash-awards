package ops

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/slick-daddy/stash-awards/internal/config"
	"github.com/slick-daddy/stash-awards/internal/protocol"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// stubPerformer is what the fake Stash server knows about one performer. The
// field names are Stash's, because this is what goes on the wire.
type stubPerformer struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	URLs    []string `json:"urls"`
	Aliases []string `json:"alias_list"`
}

// stashStub answers the GraphQL queries this plugin sends. Dispatch builds its
// own Stash client out of the plugin input, so being the server is the only way
// to control what an operation sees.
type stashStub struct {
	t          *testing.T
	performers map[string]stubPerformer
	settings   map[string]interface{}

	// configuredSettings records what the plugin wrote back via the
	// configurePlugin mutation, so a test can assert that the seeding path
	// actually fired and with the values it expected.
	configuredSettings []map[string]interface{}
	configuredErr     error

	// status, when set, makes every request fail with that HTTP status.
	status int

	server *httptest.Server
}

func newStashStub(t *testing.T) *stashStub {
	t.Helper()
	s := &stashStub{
		t:          t,
		performers: map[string]stubPerformer{},
		settings:   map[string]interface{}{},
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.server.Close)
	return s
}

func (s *stashStub) handle(w http.ResponseWriter, r *http.Request) {
	if s.status != 0 {
		http.Error(w, "stash is unwell", s.status)
		return
	}

	var body struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.t.Errorf("decode graphql request: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	switch {
	case strings.Contains(body.Query, "findPerformer("):
		id, _ := body.Variables["id"].(string)
		p, ok := s.performers[id]
		if !ok {
			// Stash answers a missing performer with a null, not an error.
			writeData(w, map[string]interface{}{"findPerformer": nil})
			return
		}
		writeData(w, map[string]interface{}{"findPerformer": p})

	case strings.Contains(body.Query, "findPerformers("):
		out := make([]stubPerformer, 0, len(s.performers))
		for _, p := range s.performers {
			out = append(out, p)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		writeData(w, map[string]interface{}{"findPerformers": map[string]interface{}{
			"count":      len(out),
			"performers": out,
		}})

	case strings.Contains(body.Query, "plugins("):
		writeData(w, map[string]interface{}{"configuration": map[string]interface{}{
			"plugins": map[string]interface{}{config.PluginID: s.settings},
		}})

	case strings.Contains(body.Query, "configurePlugin("):
		if s.configuredErr != nil {
			http.Error(w, s.configuredErr.Error(), http.StatusInternalServerError)
			return
		}
		input, _ := body.Variables["input"].(map[string]interface{})
		s.configuredSettings = append(s.configuredSettings, input)
		s.settings = input
		writeData(w, map[string]interface{}{"configurePlugin": input})

	default:
		s.t.Errorf("unexpected query %q", body.Query)
		http.Error(w, "unexpected query", http.StatusBadRequest)
	}
}

func writeData(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// connection points a plugin input at the stub server, keeping the database in
// dir the way Stash keeps it beside the plugin's YAML.
func (s *stashStub) connection(dir string) protocol.ServerConnection {
	u, err := url.Parse(s.server.URL)
	if err != nil {
		s.t.Fatalf("parse stub url %q: %v", s.server.URL, err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		s.t.Fatalf("stub port %q: %v", u.Port(), err)
	}
	return protocol.ServerConnection{
		Scheme:    u.Scheme,
		Host:      u.Hostname(),
		Port:      port,
		Dir:       dir,
		PluginDir: dir,
	}
}

// dispatch runs one operation the way Stash would: a fresh process reading the
// input, opening the database in dir and exiting.
func (s *stashStub) dispatch(dir string, args protocol.Args) (interface{}, error) {
	s.t.Helper()
	return Dispatch(protocol.NewLogTo(io.Discard), protocol.Input{
		ServerConnection: s.connection(dir),
		Args:             args,
	})
}

// withStore opens the plugin database directly and hands it to fn. Tests use it
// both to arrange stored data without running a sync and to inspect what an
// operation left behind. The handle is closed before returning, because Dispatch
// opens the same file.
func withStore(t *testing.T, dir string, fn func(*store.Store)) {
	t.Helper()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store in %s: %v", dir, err)
	}
	defer db.Close()
	fn(db)
}

// award returns a filled-in award for source, so tests assert on data they can
// recognise rather than zero values.
func award(source store.Source, name string, year int) store.Award {
	return store.Award{
		Source:       source,
		Organization: "AVN",
		AwardName:    name,
		Category:     "Best Actress",
		Year:         year,
		Result:       store.ResultWon,
		SourceURL:    "https://" + string(source) + ".test/page",
	}
}
