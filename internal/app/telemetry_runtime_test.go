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

package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/telemetry"
)

// TestEmitTelemetryWiresEvent proves the app-layer hook assembles a correct,
// content-free event and ships it when telemetry is enabled.
func TestEmitTelemetryWiresEvent(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv(telemetry.EnvEnabled, "true")
	t.Setenv(telemetry.EnvURL, srv.URL)
	t.Setenv(envDWSChannel, "openclaw")

	inv := executor.Invocation{
		CanonicalProduct: "doc",
		Tool:             "create",
		// Params carry content; telemetry must NOT read any of it.
		Params: map[string]any{"title": "Q3-Earnings-Report", "doc_id": "doc-secret-123"},
	}

	emitTelemetry("trace-xyz", inv, false, "validation", 123*time.Millisecond)

	if len(body) == 0 {
		t.Fatal("no telemetry was POSTed")
	}
	var ev telemetry.Event
	if err := json.Unmarshal(body, &ev); err != nil {
		t.Fatalf("non-JSON body %q: %v", body, err)
	}

	if ev.TraceID != "trace-xyz" {
		t.Errorf("trace_id=%q", ev.TraceID)
	}
	if ev.Command != "doc" || ev.Subcommand != "create" {
		t.Errorf("command/subcommand=%q/%q", ev.Command, ev.Subcommand)
	}
	if ev.Outcome != "error" || ev.ErrClass != "validation" || ev.ExitCode != 1 {
		t.Errorf("outcome wiring wrong: %+v", ev)
	}
	if ev.DurationMS != 123 {
		t.Errorf("duration_ms=%d, want 123", ev.DurationMS)
	}
	if ev.Channel != "openclaw" {
		t.Errorf("channel=%q, want openclaw", ev.Channel)
	}
	if ev.OS == "" {
		t.Error("os dimension should be set")
	}

	// Privacy boundary: no param content may ever leak into the wire payload.
	raw := string(body)
	for _, secret := range []string{"Q3-Earnings-Report", "doc-secret-123", "title"} {
		if contains(raw, secret) {
			t.Errorf("telemetry payload leaked content %q: %s", secret, raw)
		}
	}
}

// TestEmitTelemetryNoopWhenDisabled proves the hot path is silent when off.
func TestEmitTelemetryNoopWhenDisabled(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv(telemetry.EnvEnabled, "")
	t.Setenv(telemetry.EnvURL, srv.URL)

	emitTelemetry("t", executor.Invocation{CanonicalProduct: "doc", Tool: "get"}, true, "", time.Second)

	if called {
		t.Fatal("telemetry was sent while disabled")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
