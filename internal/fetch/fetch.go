// Package fetch performs the plugin's outbound HTTP requests. Every request goes
// through a per-source gate that serialises requests and spaces them apart, and
// transient failures are retried with exponential backoff.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// DefaultUserAgent identifies the plugin. IAFD blocks requests without a
// User-Agent, and an honest one is better than impersonating a browser.
const DefaultUserAgent = "stash-awards/1.0 (+https://github.com/slick-daddy/stash-awards)"

// maxBodyBytes caps how much of a response is read. IAFD performer pages are
// around 600 KB; anything far past that is not a page this plugin can use.
const maxBodyBytes = 8 << 20

// ErrNotFound reports a 404. Callers treat it as "this performer has no page
// here", which is an ordinary outcome rather than a failure to retry.
var ErrNotFound = errors.New("page not found")

// StatusError is returned for a response this client will not retry.
type StatusError struct {
	StatusCode int
	URL        string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s returned HTTP %d", e.URL, e.StatusCode)
}

// Options configures a Client. The zero value of each field falls back to a
// sensible default, so callers only set what they care about.
type Options struct {
	UserAgent   string
	Timeout     time.Duration
	MaxRetries  int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration

	// HTTPClient overrides the transport, for tests.
	HTTPClient *http.Client
	// Sleep overrides how the client waits, for tests.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Client issues rate-limited HTTP GETs.
type Client struct {
	http        *http.Client
	userAgent   string
	maxRetries  int
	baseBackoff time.Duration
	maxBackoff  time.Duration
	sleep       func(ctx context.Context, d time.Duration) error

	mu    sync.Mutex
	gates map[string]*gate
}

// New builds a Client from opts.
func New(opts Options) *Client {
	c := &Client{
		http:        opts.HTTPClient,
		userAgent:   opts.UserAgent,
		maxRetries:  opts.MaxRetries,
		baseBackoff: opts.BaseBackoff,
		maxBackoff:  opts.MaxBackoff,
		sleep:       opts.Sleep,
		gates:       map[string]*gate{},
	}
	if c.http == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		c.http = &http.Client{Timeout: timeout}
	}
	if c.userAgent == "" {
		c.userAgent = DefaultUserAgent
	}
	if c.maxRetries <= 0 {
		c.maxRetries = 3
	}
	if c.baseBackoff <= 0 {
		c.baseBackoff = time.Second
	}
	if c.maxBackoff <= 0 {
		c.maxBackoff = 30 * time.Second
	}
	if c.sleep == nil {
		c.sleep = sleepCtx
	}
	return c
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// SetDelay sets the minimum spacing between requests to source. Requests to a
// source are never issued concurrently, so this is also the effective rate.
func (c *Client) SetDelay(source string, d time.Duration) {
	// gateFor releases c.mu before the gate lock is taken: a request in flight
	// holds its gate for the whole request, and blocking c.mu behind that would
	// stall every other source too.
	c.gateFor(source).setDelay(d)
}

func (c *Client) gateFor(source string) *gate {
	c.mu.Lock()
	defer c.mu.Unlock()
	g, ok := c.gates[source]
	if !ok {
		g = &gate{}
		c.gates[source] = g
	}
	return g
}

// Response is the outcome of a successful GET.
type Response struct {
	Body []byte
	// URL is the address the response actually came from, after redirects. IAFD
	// redirects pretty performer URLs to their canonical id= form, and the
	// canonical one is what gets cached.
	URL         string
	ContentType string
}

// Get fetches url, spacing and retrying according to source's configuration.
func (c *Client) Get(ctx context.Context, source, url string) (*Response, error) {
	g := c.gateFor(source)

	var lastErr error
	backoff := c.baseBackoff
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := backoff
			if retry, ok := retryAfter(lastErr); ok {
				wait = retry
			}
			if wait > c.maxBackoff {
				wait = c.maxBackoff
			}
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			backoff *= 2
		}

		resp, err := c.attempt(ctx, g, url)
		if err == nil {
			return resp, nil
		}
		if !retryable(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("giving up on %s after %d attempts: %w", url, c.maxRetries+1, lastErr)
}

// attempt performs one request, holding the source gate for its duration so no
// two requests to the same source overlap.
func (c *Client) attempt(ctx context.Context, g *gate, url string) (*Response, error) {
	release, err := g.enter(ctx, c.sleep)
	if err != nil {
		return nil, err
	}
	defer release()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &transientError{err: err}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s: %w", url, ErrNotFound)
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return nil, &transientError{
			err:   &StatusError{StatusCode: resp.StatusCode, URL: url},
			after: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	case resp.StatusCode >= 400:
		return nil, &StatusError{StatusCode: resp.StatusCode, URL: url}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, &transientError{err: fmt.Errorf("read body of %s: %w", url, err)}
	}
	if len(body) > maxBodyBytes {
		return nil, fmt.Errorf("%s returned more than %d bytes", url, maxBodyBytes)
	}

	final := url
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	return &Response{Body: body, URL: final, ContentType: resp.Header.Get("Content-Type")}, nil
}

// transientError marks a failure worth retrying.
type transientError struct {
	err   error
	after time.Duration
}

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

func retryable(err error) bool {
	var t *transientError
	return errors.As(err, &t)
}

// retryAfter reports a server-requested delay, when the last failure carried one.
func retryAfter(err error) (time.Duration, bool) {
	var t *transientError
	if errors.As(err, &t) && t.after > 0 {
		return t.after, true
	}
	return 0, false
}

// parseRetryAfter understands the delay-seconds form of the Retry-After header.
// The HTTP-date form is ignored; falling back to plain backoff is harmless.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
