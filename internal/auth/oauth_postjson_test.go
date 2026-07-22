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
	"errors"
	"io"
	"net/http"
	"testing"
)

type postJSONRoundTripFunc func(*http.Request) (*http.Response, error)

func (f postJSONRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type oauthBrokenBody struct{}

func (oauthBrokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (oauthBrokenBody) Close() error             { return nil }

func TestCrossPlatformCoveragePostJSONTruncatedErrorBodyKeepsHTTPStatus(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			provider := &OAuthProvider{httpClient: &http.Client{Transport: postJSONRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: oauthBrokenBody{}, Header: make(http.Header)}, nil
			})}}

			_, err := provider.postJSON(context.Background(), "https://oauth.test/token", map[string]string{"grantType": "refresh_token"})
			var statusErr *HTTPStatusError
			if !errors.As(err, &statusErr) || statusErr.StatusCode != status {
				t.Fatalf("postJSON() error = %v, want HTTPStatusError %d", err, status)
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("HTTP status error should not expose diagnostic body read failure: %v", err)
			}
			if got := ClassifyRefreshFailure(err); got != RefreshFailureTransient {
				t.Fatalf("ClassifyRefreshFailure() = %s, want transient", got)
			}
		})
	}
}

func TestCrossPlatformCoveragePostJSONOKTruncatedBodyIsTransient(t *testing.T) {
	provider := &OAuthProvider{httpClient: &http.Client{Transport: postJSONRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: oauthBrokenBody{}, Header: make(http.Header)}, nil
	})}}

	_, err := provider.postJSON(context.Background(), "https://oauth.test/token", map[string]string{"grantType": "refresh_token"})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("postJSON() error = %v, want io.ErrUnexpectedEOF", err)
	}
	if got := ClassifyRefreshFailure(err); got != RefreshFailureTransient {
		t.Fatalf("ClassifyRefreshFailure() = %s, want transient", got)
	}
}
