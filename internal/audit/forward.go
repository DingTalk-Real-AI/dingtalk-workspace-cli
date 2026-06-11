// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HTTPForwarder ships audit events to an endpoint the DEPLOYING ORGANIZATION
// controls (its own internal audit store) — never a vendor-hardcoded URL.
// It is best-effort: the local FileSink is the durable source of truth, so a
// transient forward failure must never block or fail the user's command. It
// POSTs a single JSON event per call (application/json); batching can be layered
// on later without changing the Sink contract.
type HTTPForwarder struct {
	URL    string
	Token  string // optional bearer; enterprise's own auth to its sink
	Header map[string]string
	Client *http.Client
}

// NewHTTPForwarder builds a forwarder with a short default timeout so auditing
// never stalls a command. Auditing is a side effect, not a gate.
func NewHTTPForwarder(url, token string) *HTTPForwarder {
	return &HTTPForwarder{
		URL:    url,
		Token:  token,
		Client: &http.Client{Timeout: 3 * time.Second},
	}
}

// Emit POSTs e as JSON. A non-2xx or transport error is returned to the caller
// (typically MultiSink, which logs it) but the event is already persisted
// locally, so loss here is recoverable by replaying the file.
func (f *HTTPForwarder) Emit(e *Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.Client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dws-Audit-Schema", SchemaVersion)
	if f.Token != "" {
		req.Header.Set("Authorization", "Bearer "+f.Token)
	}
	for k, v := range f.Header {
		req.Header.Set(k, v)
	}

	resp, err := f.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("audit forward: sink returned %d", resp.StatusCode)
	}
	return nil
}
