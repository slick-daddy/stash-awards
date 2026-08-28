package fetch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testClient builds a Client whose waits are recorded instead of slept, so the
// retry and spacing logic can be asserted without real delays.
func testClient(t *testing.T, opts Options) (*Client, *[]time.Duration) {
	t.Helper()
	var mu sync.Mutex
	var waits []time.Duration
	opts.Sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
		return ctx.Err()
	}
	return New(opts), &waits
}

func TestGetSendsUserAgentAndReturnsBody(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()

	c, _ := testClient(t, Options{})
	resp, err := c.Get(context.Background(), "iafd", srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(resp.Body) != "<html>ok</html>" {
		t.Errorf("body = %q", resp.Body)
	}
	if gotUA != DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, DefaultUserAgent)
	}
	if resp.ContentType == "" {
		t.Error("content type not reported")
	}
}

func TestGetReportsFinalURLAfterRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/canonical", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("page"))
	})
	mux.HandleFunc("/pretty", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/canonical", http.StatusMovedPermanently)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, _ := testClient(t, Options{})
	resp, err := c.Get(context.Background(), "iafd", srv.URL+"/pretty")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.URL != srv.URL+"/canonical" {
		t.Errorf("final URL = %q, want the redirect target", resp.URL)
	}
}

func TestGetTreats404AsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c, waits := testClient(t, Options{})
	_, err := c.Get(context.Background(), "aia", srv.URL)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if len(*waits) != 0 {
		t.Errorf("retried a 404 %d times", len(*waits))
	}
}

func TestGetDoesNotRetryClientErrors(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c, _ := testClient(t, Options{})
	_, err := c.Get(context.Background(), "iafd", srv.URL)
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusForbidden {
		t.Fatalf("err = %v, want a 403 StatusError", err)
	}
	if calls != 1 {
		t.Errorf("made %d requests, want 1", calls)
	}
}

func TestGetRetriesServerErrorsWithDoublingBackoff(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c, waits := testClient(t, Options{BaseBackoff: time.Second, MaxBackoff: 30 * time.Second})
	resp, err := c.Get(context.Background(), "iafd", srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(resp.Body) != "recovered" {
		t.Errorf("body = %q", resp.Body)
	}
	if len(*waits) != 2 {
		t.Fatalf("waits = %v, want two backoffs", *waits)
	}
	if (*waits)[0] != time.Second || (*waits)[1] != 2*time.Second {
		t.Errorf("waits = %v, want 1s then 2s", *waits)
	}
}

func TestGetGivesUpAfterMaxRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, _ := testClient(t, Options{MaxRetries: 2})
	if _, err := c.Get(context.Background(), "iafd", srv.URL); err == nil {
		t.Fatal("Get succeeded, want failure")
	}
	if calls != 3 {
		t.Errorf("made %d requests, want 1 initial + 2 retries", calls)
	}
}

func TestBackoffIsCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c, waits := testClient(t, Options{MaxRetries: 4, BaseBackoff: 10 * time.Second, MaxBackoff: 30 * time.Second})
	c.Get(context.Background(), "iafd", srv.URL)

	for i, w := range *waits {
		if w > 30*time.Second {
			t.Errorf("wait %d = %v, exceeds the 30s cap", i, w)
		}
	}
	if len(*waits) != 4 {
		t.Fatalf("waits = %v, want 4", *waits)
	}
	if (*waits)[3] != 30*time.Second {
		t.Errorf("last wait = %v, want it clamped to 30s", (*waits)[3])
	}
}

func TestRetryAfterHeaderOverridesBackoff(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, waits := testClient(t, Options{BaseBackoff: time.Second})
	if _, err := c.Get(context.Background(), "iafd", srv.URL); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(*waits) != 1 || (*waits)[0] != 7*time.Second {
		t.Errorf("waits = %v, want the server's 7s", *waits)
	}
}

func TestGetHonoursCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c, _ := testClient(t, Options{})
	if _, err := c.Get(ctx, "iafd", srv.URL); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// One source must never have two requests in flight at once, no matter how many
// goroutines ask for one.
func TestRequestsToOneSourceDoNotOverlap(t *testing.T) {
	var inFlight, maxInFlight int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if n <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, _ := testClient(t, Options{})
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), "iafd", srv.URL); err != nil {
				t.Errorf("Get: %v", err)
			}
		}()
	}
	wg.Wait()

	if maxInFlight != 1 {
		t.Errorf("peak concurrency = %d, want 1", maxInFlight)
	}
}

// Different sources are independent, so they may overlap.
func TestSeparateSourcesRunConcurrently(t *testing.T) {
	release := make(chan struct{})
	arrived := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		<-release
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, _ := testClient(t, Options{})
	for _, source := range []string{"iafd", "aia"} {
		go c.Get(context.Background(), source, srv.URL)
	}

	timeout := time.After(2 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-arrived:
		case <-timeout:
			t.Fatalf("only %d of 2 sources reached the server; they are sharing a gate", i)
		}
	}
	close(release)
}

func TestSetDelaySpacesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, waits := testClient(t, Options{})
	c.SetDelay("iafd", 2*time.Second)

	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), "iafd", srv.URL); err != nil {
			t.Fatalf("Get %d: %v", i, err)
		}
	}

	// The first request has nothing to wait behind; the second does.
	if len(*waits) != 1 {
		t.Fatalf("waits = %v, want exactly one spacing wait", *waits)
	}
	if (*waits)[0] <= 0 || (*waits)[0] > 2*time.Second {
		t.Errorf("wait = %v, want a positive value no greater than the 2s delay", (*waits)[0])
	}
}

func TestGateWaitForMeasuresFromLastRequest(t *testing.T) {
	now := time.Now()
	g := &gate{delay: 2 * time.Second, last: now.Add(-1500 * time.Millisecond)}
	if got := g.waitFor(now); got != 500*time.Millisecond {
		t.Errorf("waitFor = %v, want 500ms", got)
	}
	g.last = now.Add(-3 * time.Second)
	if got := g.waitFor(now); got != 0 {
		t.Errorf("waitFor = %v, want 0 once the delay has elapsed", got)
	}
	if got := (&gate{delay: time.Second}).waitFor(now); got != 0 {
		t.Errorf("first request waited %v, want 0", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"Wed, 21 Oct 2026 07:28:00 GMT", 0},
	} {
		if got := parseRetryAfter(tc.in); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// sleepCtx is the production default and must be exercised directly: every
// other test injects a fake sleep and never reaches it.
func TestSleepCtxRespectsZeroAndCancellation(t *testing.T) {
	if err := sleepCtx(context.Background(), 0); err != nil {
		t.Errorf("zero duration err = %v, want nil", err)
	}
	if err := sleepCtx(context.Background(), -time.Second); err != nil {
		t.Errorf("negative duration err = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled ctx err = %v, want context.Canceled", err)
	}
	// The happy path: a tiny positive duration must return nil after the
	// timer fires, not block forever.
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Errorf("positive duration err = %v, want nil", err)
	}
}

// errors.Unwrap is only invoked by errors.Is/As when the chain has at least
// two layers; the bare StatusError path doesn't need it, but the wrapped one
// inside transientError does.
func TestTransientErrorUnwraps(t *testing.T) {
	base := errors.New("network down")
	tr := &transientError{err: base}
	if !errors.Is(tr, base) {
		t.Error("errors.Is did not see the wrapped base error")
	}
	if tr.Unwrap() != base {
		t.Error("Unwrap did not return the base error")
	}
}

// When the context is cancelled while the gate is being entered (after the
// sleep succeeded), enter must surface ctx.Err() rather than handing back a
// release function the caller will then use.
func TestGateEnterSurfacesCancelledContextAfterSleep(t *testing.T) {
	g := &gate{delay: time.Second, last: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sleep := func(context.Context, time.Duration) error { return nil }
	if _, err := g.enter(ctx, sleep); !errors.Is(err, context.Canceled) {
		t.Errorf("enter err = %v, want context.Canceled", err)
	}
}

// When the sleep itself returns a non-ctx error, enter must propagate it and
// release the gate lock.
func TestGateEnterSurfacesSleepError(t *testing.T) {
	g := &gate{delay: time.Second, last: time.Now()}
	boom := errors.New("scheduler failed")
	sleep := func(context.Context, time.Duration) error { return boom }
	if _, err := g.enter(context.Background(), sleep); !errors.Is(err, boom) {
		t.Errorf("enter err = %v, want the sleep error", err)
	}
	// The lock must have been released, so a second enter that does not
	// need to wait (delay 0) can take it.
	g.delay = 0
	release, err := g.enter(context.Background(), sleep)
	if err != nil {
		t.Fatalf("second enter err = %v", err)
	}
	release()
}

// The size cap is a guard, not a retryable condition; the request must fail
// with a plain error so the source code can treat it as a bad page.
func TestGetRejectsOversizedResponse(t *testing.T) {
	// maxBodyBytes+2 puts us over by one byte after the LimitReader.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte{'a'}, maxBodyBytes+2))
	}))
	defer srv.Close()

	c, _ := testClient(t, Options{})
	_, err := c.Get(context.Background(), "iafd", srv.URL)
	if err == nil {
		t.Fatal("Get accepted an oversized body")
	}
	if !strings.Contains(err.Error(), "more than") {
		t.Errorf("err = %v, want a size-cap message", err)
	}
}

// Some response writers don't populate resp.Request; the client must fall
// back to the requested URL in that case.
func TestGetFallsBackToRequestedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Returning 200 with a no-Request transport simulates the edge case.
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// Replace the transport with one that does not set Request on the
	// response, forcing attempt to use the original URL.
	noReqClient := &http.Client{Transport: noRequestTransport{}}
	c := New(Options{HTTPClient: noReqClient, Sleep: func(context.Context, time.Duration) error { return nil }})

	resp, err := c.Get(context.Background(), "iafd", srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.URL != srv.URL {
		t.Errorf("URL = %q, want the requested %q", resp.URL, srv.URL)
	}
}

type noRequestTransport struct{}

func (noRequestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	w.Write([]byte("ok"))
	resp := w.Result()
	resp.Request = nil
	return resp, nil
}

// A transport error (DNS, reset connection) must come back as a transient
// error so the retry loop kicks in.
func TestGetWrapsTransportErrorsAsTransient(t *testing.T) {
	boom := errors.New("dial tcp: connection refused")
	tr := errorTransport{err: boom}
	c := New(Options{
		HTTPClient: &http.Client{Transport: tr},
		Sleep:      func(context.Context, time.Duration) error { return nil },
		MaxRetries: 1,
	})
	_, err := c.Get(context.Background(), "iafd", "http://example.invalid/")
	if err == nil {
		t.Fatal("Get succeeded, want the wrapped transport error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the transport error", err)
	}
	if !strings.Contains(err.Error(), "giving up") {
		t.Errorf("err = %v, want a giving-up message", err)
	}
}

// If the backoff sleep itself fails, Get must surface that failure rather
// than swallow it and try another request.
func TestGetSurfacesBackoffSleepError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	boom := errors.New("interrupted")
	c := New(Options{
		MaxRetries: 2,
		Sleep:      func(context.Context, time.Duration) error { return boom },
	})
	_, err := c.Get(context.Background(), "iafd", srv.URL)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the sleep error", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("made %d requests, want only the first (the sleep aborted the retry)", n)
	}
}

// A body that errors mid-read must surface as a transient error, not as a
// successful empty response, so the retry loop can have another go.
func TestGetTreatsReadErrorAsTransient(t *testing.T) {
	boom := errors.New("connection reset by peer")
	c := New(Options{
		HTTPClient: &http.Client{Transport: readErrorTransport{err: boom}},
		Sleep:      func(context.Context, time.Duration) error { return nil },
		MaxRetries: 1,
	})
	_, err := c.Get(context.Background(), "iafd", "http://example.invalid/")
	if err == nil {
		t.Fatal("Get succeeded, want a transient read error")
	}
	var se *StatusError
	if errors.As(err, &se) {
		t.Errorf("err = %v, want a transient (non-StatusError) failure", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the read error", err)
	}
}

// readErrorTransport returns a 200 with a body that errors on every read.
type readErrorTransport struct{ err error }

func (t readErrorTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(errorReader{err: t.err}),
		Request:    r,
		Header:     http.Header{},
	}, nil
}

type errorReader struct{ err error }

func (e errorReader) Read(_ []byte) (int, error) { return 0, e.err }

type errorTransport struct{ err error }

func (e errorTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, e.err
}

// New must respect a caller-supplied Timeout rather than silently overriding
// it with the package default.
func TestNewAppliesTimeoutWhenHTTPClientUnset(t *testing.T) {
	c := New(Options{Timeout: 5 * time.Second})
	if c.http.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.http.Timeout)
	}
}

// http.NewRequestWithContext can fail for a URL Go's URL parser cannot read.
// The client must surface that as a plain error rather than a transient one,
// since no amount of retrying will fix a malformed URL.
func TestGetRejectsMalformedURL(t *testing.T) {
	c, _ := testClient(t, Options{})
	_, err := c.Get(context.Background(), "iafd", "://no-scheme")
	if err == nil {
		t.Fatal("Get accepted a malformed URL")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("err = %v, want a build-request error", err)
	}
}
