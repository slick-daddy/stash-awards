package stashapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/slick-daddy/stash-awards/internal/protocol"
)

// recorder is a stand-in Stash GraphQL endpoint. It records what the client
// sent and replies with whatever the test supplies.
type recorder struct {
	t *testing.T
	// reply is written verbatim as the response body.
	reply string
	// status, when set, replaces a 200.
	status int

	requests []map[string]interface{}
	cookies  []string
	paths    []string
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.t.Helper()
	r.paths = append(r.paths, req.URL.Path)
	if c, err := req.Cookie("session"); err == nil {
		r.cookies = append(r.cookies, c.Value)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.t.Fatalf("decode request: %v", err)
	}
	r.requests = append(r.requests, body)

	w.Header().Set("Content-Type", "application/json")
	if r.status != 0 {
		w.WriteHeader(r.status)
	}
	w.Write([]byte(r.reply))
}

func testClient(t *testing.T, rec *recorder) *Client {
	t.Helper()
	rec.t = t
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	// The client builds its own endpoint from the connection details, so the test
	// server's address is fed back in through the same path Stash uses.
	u := strings.TrimPrefix(srv.URL, "http://")
	host, port, ok := strings.Cut(u, ":")
	if !ok {
		t.Fatalf("unexpected test server URL %q", srv.URL)
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parse port %q: %v", port, err)
	}
	return New(protocol.ServerConnection{
		Scheme:        "http",
		Host:          host,
		Port:          portNum,
		SessionCookie: &http.Cookie{Name: "session", Value: "test-session"},
	})
}

func TestEndpointReplacesTheWildcardBindAddress(t *testing.T) {
	for _, tc := range []struct {
		in   protocol.ServerConnection
		want string
	}{
		{protocol.ServerConnection{Scheme: "http", Host: "0.0.0.0", Port: 9999}, "http://localhost:9999/graphql"},
		{protocol.ServerConnection{Scheme: "http", Host: "", Port: 9999}, "http://localhost:9999/graphql"},
		{protocol.ServerConnection{Scheme: "https", Host: "127.0.0.1", Port: 443}, "https://127.0.0.1:443/graphql"},
		{protocol.ServerConnection{Scheme: "", Host: "stash.local", Port: 0}, "http://stash.local:9999/graphql"},
		{protocol.ServerConnection{Scheme: "http", Host: "::1", Port: 9999}, "http://[::1]:9999/graphql"},
	} {
		if got := Endpoint(tc.in); got != tc.want {
			t.Errorf("Endpoint(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPerformerSendsTheSessionCookie(t *testing.T) {
	rec := &recorder{reply: `{"data":{"findPerformer":{"id":"7","name":"Test Performer",
		"urls":["https://www.iafd.com/person.rme/id=abc"],"alias_list":["Testy"]}}}`}
	c := testClient(t, rec)

	p, err := c.Performer(context.Background(), "7")
	if err != nil {
		t.Fatalf("Performer: %v", err)
	}
	if p.Name != "Test Performer" || len(p.URLs) != 1 || len(p.Aliases) != 1 {
		t.Errorf("performer = %+v", p)
	}
	if len(rec.cookies) != 1 || rec.cookies[0] != "test-session" {
		t.Errorf("cookies = %v, want the session cookie", rec.cookies)
	}
	if rec.paths[0] != "/graphql" {
		t.Errorf("path = %q, want /graphql", rec.paths[0])
	}
	if got := rec.requests[0]["variables"].(map[string]interface{})["id"]; got != "7" {
		t.Errorf("id variable = %v, want 7", got)
	}
}

// Stash answers an unknown id with a null record and no error, which has to read
// as "no such performer" rather than as an empty performer.
func TestPerformerReportsNullAsNotFound(t *testing.T) {
	c := testClient(t, &recorder{reply: `{"data":{"findPerformer":null}}`})

	_, err := c.Performer(context.Background(), "404")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// GraphQL reports failures inside a 200 response.
func TestQuerySurfacesGraphQLErrors(t *testing.T) {
	c := testClient(t, &recorder{reply: `{"data":null,"errors":[{"message":"boom"},{"message":"and again"}]}`})

	_, err := c.Performer(context.Background(), "7")
	if err == nil {
		t.Fatal("Performer succeeded despite a GraphQL error")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "and again") {
		t.Errorf("err = %v, want both messages", err)
	}
}

func TestQueryReportsAnHTTPFailure(t *testing.T) {
	c := testClient(t, &recorder{status: http.StatusUnauthorized, reply: `unauthorized`})

	if _, err := c.Performer(context.Background(), "7"); err == nil {
		t.Fatal("Performer succeeded on a 401")
	}
}

func TestPerformersPagesAndReturnsTheTotal(t *testing.T) {
	rec := &recorder{reply: `{"data":{"findPerformers":{"count":42,"performers":[
		{"id":"1","name":"One","urls":[],"alias_list":[]},
		{"id":"2","name":"Two","urls":[],"alias_list":[]}]}}}`}
	c := testClient(t, rec)

	performers, total, err := c.Performers(context.Background(), 0, 50)
	if err != nil {
		t.Fatalf("Performers: %v", err)
	}
	if total != 42 || len(performers) != 2 {
		t.Errorf("got %d of %d performers", len(performers), total)
	}
	// A page below 1 is meaningless to Stash, so it is normalised.
	vars := rec.requests[0]["variables"].(map[string]interface{})
	if vars["page"] != float64(1) {
		t.Errorf("page = %v, want 1", vars["page"])
	}
}

func TestPluginSettingsUnwrapsThePluginEntry(t *testing.T) {
	rec := &recorder{reply: `{"data":{"configuration":{"plugins":{"awards":{"iafdEnabled":true,"syncIntervalDays":30}}}}}`}
	c := testClient(t, rec)

	got, err := c.PluginSettings(context.Background(), "awards")
	if err != nil {
		t.Fatalf("PluginSettings: %v", err)
	}
	if got["iafdEnabled"] != true || got["syncIntervalDays"] != float64(30) {
		t.Errorf("settings = %v", got)
	}
}

// Stash returns no entry for a plugin whose settings the user has never changed.
func TestPluginSettingsTreatsAnAbsentPluginAsEmpty(t *testing.T) {
	c := testClient(t, &recorder{reply: `{"data":{"configuration":{"plugins":{}}}}`})

	got, err := c.PluginSettings(context.Background(), "awards")
	if err != nil {
		t.Fatalf("PluginSettings: %v", err)
	}
	if got == nil {
		t.Fatal("settings map is nil, want an empty map callers can read from")
	}
	if len(got) != 0 {
		t.Errorf("settings = %v, want empty", got)
	}
}
