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
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
)

// Environment variables that drive telemetry.
//
// Posture depends on the build (see the defaultURL build-time var and Enabled):
//   - Open-source build: OFF; the operator must opt in with EnvEnabled + EnvURL.
//   - Downstream build with a baked-in default endpoint: ON by default, so a
//     fleet reports to the operator's ingest out of the box; users opt out with
//     EnvDisabled.
const (
	// EnvEnabled explicitly turns telemetry on/off ("true"/"1" or "false"/"0"),
	// overriding the build posture either way.
	EnvEnabled = "DWS_TELEMETRY_ENABLED"
	// EnvDisabled is a hard opt-out. When truthy it disables telemetry no matter
	// what the build default or EnvEnabled says.
	EnvDisabled = "DWS_TELEMETRY_DISABLED"
	// EnvURL is the ingest endpoint that receives one JSON Event per POST. It
	// overrides the build-time default endpoint when set.
	EnvURL = "DWS_TELEMETRY_URL"
	// EnvToken is an optional bearer for the ingest endpoint. Overrides the
	// build-time default token when set.
	EnvToken = "DWS_TELEMETRY_TOKEN"
	// EnvTimeoutMS bounds how long a single POST may block command exit.
	EnvTimeoutMS = "DWS_TELEMETRY_TIMEOUT_MS"
	// EnvFile is the lightest sink: when set, each event is appended as one JSON
	// line to this local file instead of being POSTed — no server, no network.
	// Ideal for local/per-machine stability monitoring. Takes precedence over URL.
	EnvFile = "DWS_TELEMETRY_FILE"
)

// Build-time defaults, empty in the open-source build so telemetry stays opt-in
// and OFF. A downstream distribution may inject these via -ldflags to ship
// telemetry on-by-default to its own ingest, e.g.:
//
//	go build -ldflags "\
//	  -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/telemetry.defaultURL=https://<fc-host>/dws \
//	  -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/telemetry.defaultToken=<token>" ./cmd
//
// The public repo never hardcodes a real endpoint; only a downstream build does.
// This keeps "code is open source" and "data lands in the operator's own sink"
// decoupled.
var (
	defaultURL   string
	defaultToken string
)

// defaultTimeout caps how long telemetry may delay command exit. Telemetry is a
// side effect, never a gate: a slow or dead sink must not punish the user.
const defaultTimeout = 1500 * time.Millisecond

func init() {
	for _, it := range []configmeta.ConfigItem{
		{Name: EnvEnabled, Category: configmeta.CategoryDebug, Description: "Explicitly enable/disable ops telemetry (overrides the build default)", Example: "true"},
		{Name: EnvDisabled, Category: configmeta.CategoryDebug, Description: "Hard opt-out of ops telemetry (wins over everything)", Example: "true"},
		{Name: EnvURL, Category: configmeta.CategoryDebug, Description: "Telemetry ingest endpoint (overrides the build-time default; one JSON event POSTed per invocation)", Example: "https://telemetry.example.com/dws"},
		{Name: EnvToken, Category: configmeta.CategoryDebug, Description: "Bearer auth for the telemetry endpoint (optional)", Sensitive: true, Example: "xxxxx"},
		{Name: EnvTimeoutMS, Category: configmeta.CategoryDebug, Description: "Per-report timeout cap in milliseconds (default 1500)", Example: "1500"},
		{Name: EnvFile, Category: configmeta.CategoryDebug, Description: "Local file sink: append each event as one JSON line here instead of POSTing (no server). Takes precedence over URL", Example: "~/.dws/telemetry.jsonl"},
	} {
		configmeta.Register(it)
	}
}

// resolvedFile returns the local file sink path (env only). When set, events are
// appended to this file instead of POSTed — the lightest, server-less sink.
func resolvedFile() string {
	return expandHome(strings.TrimSpace(os.Getenv(EnvFile)))
}

// expandHome resolves a leading ~ to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// resolvedURL returns the effective ingest endpoint: the env override if set,
// otherwise the build-time default (empty in the open-source build).
func resolvedURL() string {
	if u := strings.TrimSpace(os.Getenv(EnvURL)); u != "" {
		return u
	}
	return strings.TrimSpace(defaultURL)
}

// resolvedToken returns the effective bearer token: env override, else default.
func resolvedToken() string {
	if t := strings.TrimSpace(os.Getenv(EnvToken)); t != "" {
		return t
	}
	return strings.TrimSpace(defaultToken)
}

// Enabled reports whether telemetry should run.
//
//   - EnvDisabled (hard opt-out) always wins.
//   - With no destination (no env URL and no baked-in default) nothing is sent.
//   - EnvEnabled, when set, is an explicit operator override either way.
//   - Otherwise: ON only when a default endpoint is baked into the build
//     (downstream distribution). A bare env URL in the open-source build stays
//     opt-in (off until EnvEnabled is also set).
func Enabled() bool {
	if truthy(os.Getenv(EnvDisabled)) {
		return false
	}
	if resolvedURL() == "" && resolvedFile() == "" {
		return false
	}
	if v := strings.TrimSpace(os.Getenv(EnvEnabled)); v != "" {
		return truthy(v)
	}
	// On when a default endpoint is baked into the build (downstream distribution)
	// or a local file sink is explicitly set (user opted into local monitoring).
	return strings.TrimSpace(defaultURL) != "" || resolvedFile() != ""
}

// noticeText is the one-time disclosure shown when telemetry is active. Keep it
// short, factual, and actionable (how to opt out).
const noticeText = "ℹ️  dws reports anonymous operational telemetry (command, outcome, latency, version — no content, no identity) to help monitor stability. Opt out anytime with DWS_TELEMETRY_DISABLED=true. Details: docs/telemetry.md"

// ShowNoticeOnce prints the telemetry disclosure to stderr the first time
// telemetry is active on this machine, then writes a marker so it never repeats.
// Best-effort: any filesystem error silently skips — telemetry, including its
// disclosure, must never disrupt the command.
func ShowNoticeOnce(configDir string) {
	if strings.TrimSpace(configDir) == "" {
		return
	}
	marker := filepath.Join(configDir, ".telemetry_notice_shown")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, noticeText)
	_ = os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Forwarder ships events to the configured endpoint. Best-effort: a transport
// error or non-2xx is returned for logging but never blocks beyond the timeout,
// and the command's own result is unaffected.
type Forwarder struct {
	URL    string
	Token  string
	File   string // local file sink; when set, append JSONL instead of POSTing
	Client *http.Client
}

// NewForwarderFromEnv builds a Forwarder using the effective URL/token, or
// returns nil when telemetry is disabled. A nil *Forwarder's Emit is a safe
// no-op.
func NewForwarderFromEnv() *Forwarder {
	if !Enabled() {
		return nil
	}
	return &Forwarder{
		URL:    resolvedURL(),
		Token:  resolvedToken(),
		File:   resolvedFile(),
		Client: &http.Client{Timeout: timeoutFromEnv()},
	}
}

// Emit ships e as a single JSON object. With a file sink configured it appends
// one JSON line locally (no network); otherwise it POSTs to the URL. A nil
// receiver is a no-op so callers never need a guard. Errors are returned
// (best-effort) but never block command exit past the configured timeout.
func (f *Forwarder) Emit(e *Event) error {
	if f == nil || e == nil {
		return nil
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	// Local file sink (lightest path): append one JSON line, no network.
	if f.File != "" {
		fh, openErr := os.OpenFile(f.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if openErr != nil {
			return openErr
		}
		defer fh.Close()
		_, writeErr := fh.Write(append(data, '\n'))
		return writeErr
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
