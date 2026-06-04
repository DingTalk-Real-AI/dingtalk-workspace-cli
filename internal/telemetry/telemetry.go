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

package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
)

// Environment variables that drive telemetry. All default OFF: the CLI emits
// nothing unless the operator opts in, and the destination is operator-set.
const (
	// EnvEnabled turns ops telemetry on ("true"/"1"). Independent of DWS_AUDIT_*.
	EnvEnabled = "DWS_TELEMETRY_ENABLED"
	// EnvURL is the ingest endpoint that receives one JSON Event per POST.
	// Empty disables forwarding even when EnvEnabled is set.
	EnvURL = "DWS_TELEMETRY_URL"
	// EnvToken is an optional bearer for the ingest endpoint.
	EnvToken = "DWS_TELEMETRY_TOKEN"
	// EnvTimeoutMS bounds how long a single POST may block command exit.
	EnvTimeoutMS = "DWS_TELEMETRY_TIMEOUT_MS"
)

// defaultTimeout caps how long telemetry may delay command exit. Telemetry is a
// side effect, never a gate: a slow or dead sink must not punish the user.
const defaultTimeout = 1500 * time.Millisecond

func init() {
	for _, it := range []configmeta.ConfigItem{
		{Name: EnvEnabled, Category: configmeta.CategoryDebug, Description: "Enable anonymous ops telemetry (dimensions only, no content/identity; off by default)", Example: "true"},
		{Name: EnvURL, Category: configmeta.CategoryDebug, Description: "Telemetry ingest endpoint (one JSON event POSTed per command)", Example: "https://telemetry.example.com/dws"},
		{Name: EnvToken, Category: configmeta.CategoryDebug, Description: "Bearer token for the telemetry endpoint (optional)", Sensitive: true, Example: "xxxxx"},
		{Name: EnvTimeoutMS, Category: configmeta.CategoryDebug, Description: "Per-POST timeout cap in milliseconds (default 1500)", Example: "1500"},
	} {
		configmeta.Register(it)
	}
}

// Enabled reports whether telemetry should run. It requires BOTH the opt-in
// switch and a destination — neither alone does anything.
func Enabled() bool {
	return truthy(os.Getenv(EnvEnabled)) && strings.TrimSpace(os.Getenv(EnvURL)) != ""
}

// Forwarder ships events to the operator-configured endpoint. Best-effort: a
// transport error or non-2xx is returned for logging but never blocks beyond
// the timeout, and the command's own result is unaffected.
type Forwarder struct {
	URL    string
	Token  string
	Client *http.Client
}

// NewForwarderFromEnv builds a Forwarder from the env, or returns nil when
// telemetry is disabled. A nil *Forwarder's Emit is a safe no-op.
func NewForwarderFromEnv() *Forwarder {
	if !Enabled() {
		return nil
	}
	return &Forwarder{
		URL:    strings.TrimSpace(os.Getenv(EnvURL)),
		Token:  strings.TrimSpace(os.Getenv(EnvToken)),
		Client: &http.Client{Timeout: timeoutFromEnv()},
	}
}

// Emit POSTs e as a single JSON object. A nil receiver is a no-op so callers
// never need a guard. Errors are returned (best-effort) but the bounded client
// timeout guarantees command exit is never delayed past the configured cap.
func (f *Forwarder) Emit(e *Event) error {
	if f == nil || e == nil {
		return nil
	}
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
	req.Header.Set("X-Dws-Telemetry-Schema", SchemaVersion)
	if f.Token != "" {
		req.Header.Set("Authorization", "Bearer "+f.Token)
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telemetry: sink returned %d", resp.StatusCode)
	}
	return nil
}

func timeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(EnvTimeoutMS))
	if raw == "" {
		return defaultTimeout
	}
	var ms int
	if _, err := fmt.Sscanf(raw, "%d", &ms); err != nil || ms <= 0 {
		return defaultTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
