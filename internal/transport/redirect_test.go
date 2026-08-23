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

package transport

import (
	"net/http"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSafeRedirectPolicyAgentMetadataHeaders(t *testing.T) {
	t.Parallel()

	newRequest := func(t *testing.T, rawURL string) *http.Request {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(HeaderAgentVersion, "1.2.3-test")
		req.Header.Set(HeaderAgentExt, `{"ua":"test-agent-value"}`)
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("x-user-access-token", "test-token")
		return req
	}

	assertHeaders := func(t *testing.T, request *http.Request, wantExt bool) {
		t.Helper()
		if got := request.Header.Get(HeaderAgentVersion); got != "1.2.3-test" {
			t.Fatalf("agent version = %q, want retained", got)
		}
		gotExt := request.Header.Get(HeaderAgentExt)
		if wantExt && gotExt == "" {
			t.Fatal("agent extension was removed")
		}
		if !wantExt && gotExt != "" {
			t.Fatalf("agent extension leaked across origins: %q", gotExt)
		}
		for _, key := range []string{"Authorization", "x-user-access-token"} {
			got := request.Header.Get(key)
			if wantExt && got == "" {
				t.Fatalf("same-origin redirect removed %s", key)
			}
			if !wantExt && got != "" {
				t.Fatalf("credential %s leaked across origins", key)
			}
		}
	}

	t.Run("same origin retains version and extension", func(t *testing.T) {
		t.Parallel()
		previous := newRequest(t, "https://api.example.test/start")
		redirected := newRequest(t, "https://api.example.test/next")

		if err := safeRedirectPolicy(redirected, []*http.Request{previous}); err != nil {
			t.Fatal(err)
		}
		assertHeaders(t, redirected, true)
	})

	t.Run("cross host strips extension and retains version", func(t *testing.T) {
		t.Parallel()
		previous := newRequest(t, "https://api.example.test/start")
		redirected := newRequest(t, "https://cdn.example.test/asset")

		if err := safeRedirectPolicy(redirected, []*http.Request{previous}); err != nil {
			t.Fatal(err)
		}
		assertHeaders(t, redirected, false)
	})

	t.Run("scheme downgrade on same host strips extension", func(t *testing.T) {
		t.Parallel()
		previous := newRequest(t, "https://api.example.test/start")
		redirected := newRequest(t, "http://api.example.test/next")

		if err := safeRedirectPolicy(redirected, []*http.Request{previous}); err != nil {
			t.Fatal(err)
		}
		assertHeaders(t, redirected, false)
	})

	t.Run("extension remains stripped after returning to initial origin", func(t *testing.T) {
		t.Parallel()
		initial := newRequest(t, "https://api.example.test/start")
		crossOrigin := newRequest(t, "https://cdn.example.test/asset")
		redirectedBack := newRequest(t, "https://api.example.test/final")

		if err := safeRedirectPolicy(redirectedBack, []*http.Request{initial, crossOrigin}); err != nil {
			t.Fatal(err)
		}
		assertHeaders(t, redirectedBack, false)
	})

	t.Run("extension remains stripped on later cross-origin hop", func(t *testing.T) {
		t.Parallel()
		initial := newRequest(t, "https://api.example.test/start")
		crossOrigin := newRequest(t, "https://cdn.example.test/asset")
		redirected := newRequest(t, "https://cdn.example.test/final")

		if err := safeRedirectPolicy(redirected, []*http.Request{initial, crossOrigin}); err != nil {
			t.Fatal(err)
		}
		assertHeaders(t, redirected, false)
	})

	t.Run("initial request is unchanged", func(t *testing.T) {
		t.Parallel()
		request := newRequest(t, "https://api.example.test/start")

		if err := safeRedirectPolicy(request, nil); err != nil {
			t.Fatal(err)
		}
		assertHeaders(t, request, true)
	})

	t.Run("redirect limit is enforced", func(t *testing.T) {
		t.Parallel()
		request := newRequest(t, "https://api.example.test/final")
		via := make([]*http.Request, 10)
		for index := range via {
			via[index] = newRequest(t, "https://api.example.test/"+strings.Repeat("x", index+1))
		}

		if err := safeRedirectPolicy(request, via); err == nil {
			t.Fatal("safeRedirectPolicy() error = nil, want redirect limit error")
		}
	})
}
