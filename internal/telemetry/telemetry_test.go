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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnabledRequiresBothSwitchAndURL(t *testing.T) {
	cases := []struct {
		name, enabled, url string
		want               bool
	}{
		{"both set", "true", "https://x.example/dws", true},
		{"only switch", "true", "", false},
		{"only url", "", "https://x.example/dws", false},
		{"neither", "", "", false},
		{"falsey switch", "0", "https://x.example/dws", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvEnabled, c.enabled)
			t.Setenv(EnvURL, c.url)
			if got := Enabled(); got != c.want {
				t.Fatalf("Enabled()=%v, want %v", got, c.want)
			}
		})
	}
}

func TestEnabledWithBakedInDefaultEndpoint(t *testing.T) {
	// Simulate a downstream build that injected a default endpoint via -ldflags.
	orig := defaultURL
	defaultURL = "https://fleet.example/dws"
	t.Cleanup(func() { defaultURL = orig })

	cases := []struct {
		name, enabled, disabled, url string
		want                         bool
	}{
		{"default on (no env)", "", "", "", true},
		{"hard opt-out wins", "", "true", "", false},
		{"hard opt-out beats explicit enable", "true", "true", "", false},
		{"explicit disable via enabled=false", "false", "", "", false},
		{"explicit enable", "true", "", "", true},
		{"env url overrides default, still on", "", "", "https://other.example/dws", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvEnabled, c.enabled)
			t.Setenv(EnvDisabled, c.disabled)
			t.Setenv(EnvURL, c.url)
			if got := Enabled(); got != c.want {
				t.Fatalf("Enabled()=%v, want %v", got, c.want)
			}
		})
	}
}

func TestFileSinkAppendsAndEnables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.jsonl")
	t.Setenv(EnvEnabled, "")
	t.Setenv(EnvURL, "")
	t.Setenv(EnvFile, path)

	// A file sink alone is a destination -> enabled (local monitoring opt-in).
	if !Enabled() {
		t.Fatal("Enabled()=false, want true when DWS_TELEMETRY_FILE is set")
	}
	fwd := NewForwarderFromEnv()
	if fwd == nil {
		t.Fatal("expected a forwarder when file sink is set")
	}
	if fwd.File != path {
		t.Fatalf("forwarder File=%q, want %q", fwd.File, path)
	}

	// Two events -> two JSON lines, no network.
	for _, oc := range []string{"ok", "error"} {
		ev := New(time.Unix(0, 0), "t")
		ev.Command = "doc"
		ev.Outcome = oc
		if err := fwd.Emit(ev); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sink file: %v", err)
	}
	lines := 0
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Fatalf("line not JSON: %q (%v)", l, err)
		}
		lines++
	}
	if lines != 2 {
		t.Fatalf("file has %d JSON lines, want 2", lines)
	}

	// Hard opt-out still wins over a file sink.
	t.Setenv(EnvDisabled, "true")
	if Enabled() {
		t.Fatal("Enabled()=true with DWS_TELEMETRY_DISABLED set, want false")
	}
}

func TestNewForwarderFromEnvNilWhenDisabled(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	t.Setenv(EnvURL, "")
	if f := NewForwarderFromEnv(); f != nil {
		t.Fatalf("expected nil forwarder when disabled, got %+v", f)
	}
	// Emit on a nil forwarder must be a safe no-op.
	var nilFwd *Forwarder
	if err := nilFwd.Emit(New(time.Unix(0, 0), "t")); err != nil {
		t.Fatalf("nil Emit should be no-op, got %v", err)
	}
}

func TestForwarderEmitPostsJSON(t *testing.T) {
	var gotBody []byte
	var gotAuth, gotSchema string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotAuth = r.Header.Get("Authorization")
		gotSchema = r.Header.Get("X-Dws-Telemetry-Schema")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv(EnvEnabled, "true")
	t.Setenv(EnvURL, srv.URL)
	t.Setenv(EnvToken, "secret-token")

	fwd := NewForwarderFromEnv()
	if fwd == nil {
		t.Fatal("expected a forwarder when enabled")
	}

	ev := New(time.Unix(1700000000, 0).UTC(), "trace-123")
	ev.CLIVersion = "1.2.3"
	ev.Command = "doc"
	ev.Subcommand = "create"
	ev.Outcome = "ok"
	ev.DurationMS = 42

	if err := fwd.Emit(ev); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization=%q, want Bearer secret-token", gotAuth)
	}
	if gotSchema != SchemaVersion {
		t.Errorf("schema header=%q, want %q", gotSchema, SchemaVersion)
	}

	var decoded Event
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("server got non-JSON body %q: %v", gotBody, err)
	}
	if decoded.TraceID != "trace-123" || decoded.Command != "doc" ||
		decoded.Subcommand != "create" || decoded.Outcome != "ok" || decoded.DurationMS != 42 {
		t.Errorf("decoded event mismatch: %+v", decoded)
	}
}

func TestForwarderEmitReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv(EnvEnabled, "true")
	t.Setenv(EnvURL, srv.URL)
	fwd := NewForwarderFromEnv()
	if err := fwd.Emit(New(time.Unix(0, 0), "t")); err == nil {
		t.Fatal("expected error on 500 response")
	}
}
