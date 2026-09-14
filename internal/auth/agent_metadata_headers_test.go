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

package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type agentMetadataHeaderRoundTripFunc func(*http.Request) (*http.Response, error)

func (f agentMetadataHeaderRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCrossPlatformCoverageOAuthRequestsExcludeAgentMetadataHeaders(t *testing.T) {
	t.Setenv("DWS_AGENT_VER", "1.2.3-test")
	t.Setenv("DWS_AGENT_EXT", `{"umt":"test-umt","miniwua":"test-wua","ua":"test-agent"}`)

	assertExcluded := func(t *testing.T, header http.Header) {
		t.Helper()
		for _, name := range []string{"x-dws-agent-ver", "x-dws-agent-ext"} {
			if values := header.Values(name); len(values) != 0 {
				t.Fatalf("OAuth request header %q = %q, want absent", name, values)
			}
		}
	}

	newCaptureClient := func(responseBody string) (*http.Client, <-chan http.Header) {
		seen := make(chan http.Header, 1)
		client := &http.Client{Transport: agentMetadataHeaderRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			seen <- req.Header.Clone()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Request:    req,
			}, nil
		})}
		return client, seen
	}

	t.Run("JSON token request", func(t *testing.T) {
		client, seen := newCaptureClient(`{}`)
		provider := &OAuthProvider{httpClient: client}
		if _, err := provider.postJSON(context.Background(), "https://oauth.test/token", map[string]string{
			"code":      "test-code",
			"grantType": "authorization_code",
		}); err != nil {
			t.Fatalf("postJSON() error = %v", err)
		}
		assertExcluded(t, <-seen)
	})

	t.Run("device code request", func(t *testing.T) {
		client, seen := newCaptureClient(`{"success":true,"result":{"deviceCode":"test-device-code","userCode":"TEST-CODE","verificationUri":"https://example.test/device","expiresIn":900,"interval":1}}`)
		provider := NewDeviceFlowProvider(t.TempDir(), newDeviceFlowTestLogger())
		provider.clientID = "test-client-id"
		provider.httpClient = client
		provider.SetBaseURL("https://oauth.test")
		if _, err := provider.requestDeviceCode(context.Background()); err != nil {
			t.Fatalf("requestDeviceCode() error = %v", err)
		}
		assertExcluded(t, <-seen)
	})
}
