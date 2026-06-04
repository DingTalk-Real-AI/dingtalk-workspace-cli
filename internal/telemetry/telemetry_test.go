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
