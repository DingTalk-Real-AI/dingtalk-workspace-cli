package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type HTTPForwarder struct {
	url     string
	token   string
	redact  RedactLevel
	client  *http.Client
	wg      sync.WaitGroup
	report  func(format string, args ...any)
	timeout time.Duration
}

var (
	forwardMarshal         = json.Marshal
	forwardRedactEventJSON = RedactEventJSON
)

func NewHTTPForwarder(url, token string, redact RedactLevel, report func(string, ...any)) *HTTPForwarder {
	if report == nil {
		report = func(string, ...any) {}
	}
	return &HTTPForwarder{
		url:     url,
		token:   token,
		redact:  redact,
		client:  &http.Client{Timeout: 3 * time.Second},
		report:  report,
		timeout: 3 * time.Second,
	}
}

// Forward dispatches the event asynchronously while tracking the goroutine so
// Close can wait for delivery instead of the CLI dropping it on exit.
func (f *HTTPForwarder) Forward(evt Event) {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		f.send(evt)
	}()
}

// Close waits for in-flight forwards to finish, bounded by ctx (and, if ctx has
// no deadline, by a small internal timeout) so shutdown never blocks forever.
func (f *HTTPForwarder) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, f.timeout+2*time.Second)
		defer cancel()
	}

	done := make(chan struct{})
	go func() {
		f.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		f.report("forward flush timed out: %v", ctx.Err())
		return fmt.Errorf("audit: forward flush timed out: %w", ctx.Err())
	}
}

func (f *HTTPForwarder) send(evt Event) {
	var body []byte
	var err error
	if f.redact != RedactNone {
		body, err = forwardRedactEventJSON(evt, f.redact)
	} else {
		body, err = forwardMarshal(evt)
	}
	if err != nil {
		f.report("forward marshal failed: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(body))
	if err != nil {
		f.report("forward build request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		f.report("forward request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		f.report("forward rejected: status %d", resp.StatusCode)
	}
}
