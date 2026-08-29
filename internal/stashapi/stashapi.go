// Package stashapi is a minimal GraphQL client for the Stash server that
// spawned this plugin.
//
// Stash hands every plugin process a ServerConnection describing where the
// server listens and a session cookie that authenticates calls back to it, so
// the plugin needs no credentials of its own.
package stashapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/slick-daddy/stash-awards/internal/protocol"
)

// ErrNotFound reports that Stash returned null for a record that was asked for
// by id.
var ErrNotFound = errors.New("not found in stash")

// maxBodyBytes caps a GraphQL response. A performer page is a few kilobytes; a
// reply this large means something is wrong rather than large.
const maxBodyBytes = 32 << 20

// defaultPort is the port Stash listens on when the connection details omit one.
const defaultPort = 9999

// Client talks GraphQL to one Stash server.
type Client struct {
	endpoint string
	cookie   *http.Cookie
	http     *http.Client
}

// New returns a Client for the server described by sc.
//
// Certificate verification is left on. A Stash server configured with a
// self-signed certificate therefore needs that certificate in the machine's
// trust store for the plugin to reach it.
func New(sc protocol.ServerConnection) *Client {
	return NewWithHTTPClient(sc, nil)
}

// NewWithHTTPClient returns a Client that issues requests through hc. A nil hc
// gets a default client.
func NewWithHTTPClient(sc protocol.ServerConnection, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{endpoint: Endpoint(sc), cookie: sc.SessionCookie, http: hc}
}

// Endpoint builds the GraphQL URL for a server connection.
func Endpoint(sc protocol.ServerConnection) string {
	scheme := strings.ToLower(sc.Scheme)
	if scheme != "https" {
		scheme = "http"
	}

	host := strings.TrimSpace(sc.Host)
	switch host {
	// Stash reports the address it binds to, and the default bind address is a
	// wildcard. A wildcard is not a destination, so it has to become a real one.
	case "", "0.0.0.0", "::", "[::]":
		host = "localhost"
	}

	port := sc.Port
	if port <= 0 {
		port = defaultPort
	}

	u := url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/graphql",
	}
	return u.String()
}

// gqlError is one entry of a GraphQL response's errors array.
type gqlError struct {
	Message string `json:"message"`
}

// query runs one GraphQL query and decodes the data object into out.
func (c *Client) query(ctx context.Context, doc string, vars map[string]interface{}, out interface{}) error {
	return c.run(ctx, doc, vars, out)
}

// mutate runs one GraphQL mutation and decodes the data object into out.
// The doc is expected to start with the "mutation" keyword.
func (c *Client) mutate(ctx context.Context, doc string, vars map[string]interface{}, out interface{}) error {
	return c.run(ctx, doc, vars, out)
}

// run sends a GraphQL operation. The doc carries the operation keyword
// (query / mutation), so this function does not need to know which one it is.
func (c *Client) run(ctx context.Context, doc string, vars map[string]interface{}, out interface{}) error {
	payload, err := json.Marshal(map[string]interface{}{"query": doc, "variables": vars})
	if err != nil {
		return fmt.Errorf("encode graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build graphql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("graphql request to %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("read graphql response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stash graphql returned %s", resp.Status)
	}

	// GraphQL reports operation failures inside a 200 response, so the errors
	// array has to be checked even on success.
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode graphql response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("stash graphql: %s", strings.Join(msgs, "; "))
	}
	if out == nil || len(envelope.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decode graphql data: %w", err)
	}
	return nil
}
