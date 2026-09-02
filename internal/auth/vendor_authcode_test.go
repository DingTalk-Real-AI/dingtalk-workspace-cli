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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageFetchVendorAuthCodeSuccessEnvelope(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != MCPVendorAuthCodePath {
			t.Fatalf("path = %q, want %s", r.URL.Path, MCPVendorAuthCodePath)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("x-user-access-token"); got != "user-token" {
			t.Fatalf("x-user-access-token = %q", got)
		}
		if got := r.Header.Get("x-dws-client-id"); got != "dws-client" {
			t.Fatalf("x-dws-client-id = %q", got)
		}
		if got := r.Header.Get("x-dws-cli-version"); got != "1.2.3" {
			t.Fatalf("x-dws-cli-version = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, `{"authCode":"tmp-code","expiresIn":120}`)
	}))
	defer srv.Close()

	got, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "user-token",
		ClientID:    "dws-client",
		CLIVersion:  "1.2.3",
		Vendor:      "SafeChat",
		CorpID:      "dingxxxxxxxxxxxx",
		HTTPClient:  srv.Client(),
		BaseURL:     srv.URL,
	})
	if err != nil {
		t.Fatalf("FetchVendorAuthCode() = %v", err)
	}
	if got.AuthCode != "tmp-code" || got.ExpiresIn != 120 {
		t.Fatalf("result = %+v", got)
	}
	if gotBody["vendor"] != "safechat" || gotBody["corpId"] != "dingxxxxxxxxxxxx" {
		t.Fatalf("posted body = %v", gotBody)
	}
	if _, ok := gotBody["redirectURI"]; ok {
		t.Fatalf("posted redirectURI, body = %v", gotBody)
	}
	if _, ok := gotBody["domain"]; ok {
		t.Fatalf("posted domain, body = %v", gotBody)
	}
}

func TestCrossPlatformCoverageFetchVendorAuthCodeParsesAlways200ServiceResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":false,"errorCode":"VENDOR_NOT_ENABLED","errorMsg":"not installed"}`)
	}))
	defer srv.Close()

	_, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "user-token",
		ClientID:    "dws-client",
		Vendor:      "safechat",
		CorpID:      "dingxxxxxxxxxxxx",
		HTTPClient:  srv.Client(),
		BaseURL:     srv.URL,
	})
	var verr *VendorAuthCodeError
	if !errors.As(err, &verr) || verr.Code != VendorAuthCodeVendorNotEnabled {
		t.Fatalf("error = %v, want VENDOR_NOT_ENABLED", err)
	}
	if verr.Retryable() {
		t.Fatal("VENDOR_NOT_ENABLED must not be retryable")
	}
}

func TestCrossPlatformCoverageFetchVendorAuthCodeRequiresLocalFields(t *testing.T) {
	_, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{Vendor: "safechat", CorpID: "ding"})
	var verr *VendorAuthCodeError
	if !errors.As(err, &verr) || verr.Code != VendorAuthCodeParamError {
		t.Fatalf("error = %v, want PARAM_ERROR", err)
	}
}

func TestCrossPlatformCoverageFetchVendorAuthCodeKeepsHTTPStatusOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "oops", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "user-token",
		ClientID:    "dws-client",
		Vendor:      "safechat",
		CorpID:      "dingxxxxxxxxxxxx",
		HTTPClient:  srv.Client(),
		BaseURL:     srv.URL,
	})
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("error = %v, want HTTP 502", err)
	}
}

func TestCrossPlatformCoverageParseVendorAuthCodeResponseDefaultsExpiresIn(t *testing.T) {
	got, err := parseVendorAuthCodeResponse([]byte(`{"authCode":"x"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ExpiresIn != DefaultVendorAuthCodeExpiresIn {
		t.Fatalf("expiresIn = %d, want %d", got.ExpiresIn, DefaultVendorAuthCodeExpiresIn)
	}
}

func TestCrossPlatformCoverageVendorAuthCodeErrorRetryable(t *testing.T) {
	for _, code := range []string{VendorAuthCodeTokenInvalid, VendorAuthCodeRateLimited, VendorAuthCodeInternalError} {
		if !(&VendorAuthCodeError{Code: code}).Retryable() {
			t.Fatalf("%s should be retryable", code)
		}
	}
	if (&VendorAuthCodeError{Code: VendorAuthCodeOrgMismatch}).Retryable() {
		t.Fatal("ORG_MISMATCH must not be retryable")
	}
}

func TestCrossPlatformCoverageFetchVendorAuthCodeDoesNotSendBlankCLIVersionHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-dws-cli-version"); got != "" {
			t.Fatalf("x-dws-cli-version = %q, want empty", got)
		}
		_, _ = io.WriteString(w, `{"authCode":"tmp-code","expiresIn":90}`)
	}))
	defer srv.Close()

	got, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "user-token",
		ClientID:    "dws-client",
		Vendor:      "safechat",
		CorpID:      "dingxxxxxxxxxxxx",
		HTTPClient:  srv.Client(),
		BaseURL:     srv.URL,
	})
	if err != nil {
		t.Fatalf("FetchVendorAuthCode() = %v", err)
	}
	if got.ExpiresIn != 90 {
		t.Fatalf("expiresIn = %d, want 90 from response", got.ExpiresIn)
	}
	if strings.Contains(got.AuthCode, "redirect") {
		t.Fatalf("unexpected code %q", got.AuthCode)
	}
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) { return 0, fmt.Errorf("read failed") }
func (failingReadCloser) Close() error             { return nil }

type vendorRoundTripFunc func(*http.Request) (*http.Response, error)

func (f vendorRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCrossPlatformCoverageVendorAuthCodeRemainingEdges(t *testing.T) {
	if got := (*VendorAuthCodeError)(nil).Error(); got != "vendorAuthCode failed" {
		t.Fatalf("nil error string = %q", got)
	}
	if (*VendorAuthCodeError)(nil).Retryable() {
		t.Fatal("nil vendorAuthCode error should not be retryable")
	}
	if got := (&VendorAuthCodeError{Code: VendorAuthCodeInternalError}).Error(); got != "vendorAuthCode INTERNAL_ERROR" {
		t.Fatalf("code-only error string = %q", got)
	}
	if _, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "token",
		ClientID:    "client",
		Vendor:      "safechat",
		CorpID:      "corp",
		BaseURL:     "://bad",
	}); err == nil || !strings.Contains(err.Error(), "creating vendorAuthCode request") {
		t.Fatalf("bad request URL err = %v", err)
	}
	if _, err := parseVendorAuthCodeResponse([]byte("{")); err == nil || !strings.Contains(err.Error(), "parsing vendorAuthCode response") {
		t.Fatalf("parse error = %v", err)
	}
	if _, err := parseVendorAuthCodeResponse([]byte(`{"success":false,"errorMsg":"failed"}`)); err == nil || !strings.Contains(err.Error(), VendorAuthCodeInternalError) {
		t.Fatalf("missing-code service error = %v", err)
	}
	if _, err := parseVendorAuthCodeResponse([]byte(`{"authCode":" "}`)); err == nil || !strings.Contains(err.Error(), "missing authCode") {
		t.Fatalf("missing authCode error = %v", err)
	}

	readFailClient := &http.Client{Transport: vendorRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingReadCloser{}, Header: http.Header{}, Request: req}, nil
	})}
	if _, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "token",
		ClientID:    "client",
		Vendor:      "safechat",
		CorpID:      "corp",
		HTTPClient:  readFailClient,
		BaseURL:     "https://portal.example.test",
	}); err == nil || !strings.Contains(err.Error(), "reading vendorAuthCode response") {
		t.Fatalf("read error = %v", err)
	}

	sendFailClient := &http.Client{Transport: vendorRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("send failed")
	})}
	if _, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "token",
		ClientID:    "client",
		Vendor:      "safechat",
		CorpID:      "corp",
		HTTPClient:  sendFailClient,
		BaseURL:     "https://portal.example.test",
	}); err == nil || !strings.Contains(err.Error(), "sending vendorAuthCode request") {
		t.Fatalf("send error = %v", err)
	}

	non200ReadFailClient := &http.Client{Transport: vendorRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: failingReadCloser{}, Header: http.Header{}, Request: req}, nil
	})}
	var statusErr *HTTPStatusError
	_, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "token",
		ClientID:    "client",
		Vendor:      "safechat",
		CorpID:      "corp",
		HTTPClient:  non200ReadFailClient,
		BaseURL:     "https://portal.example.test",
	})
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("non-200 read error = %v", err)
	}

	defaultBaseClient := &http.Client{Transport: vendorRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, MCPVendorAuthCodePath) {
			t.Fatalf("default base path = %s", req.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"authCode":"code"}`)), Header: http.Header{}, Request: req}, nil
	})}
	if _, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "token",
		ClientID:    "client",
		Vendor:      "safechat",
		CorpID:      "corp",
		HTTPClient:  defaultBaseClient,
	}); err != nil {
		t.Fatalf("default base fetch = %v", err)
	}

	oldClient := oauthHTTPClient
	oauthHTTPClient = defaultBaseClient
	t.Cleanup(func() { oauthHTTPClient = oldClient })
	if _, err := FetchVendorAuthCode(context.Background(), VendorAuthCodeInput{
		AccessToken: "token",
		ClientID:    "client",
		Vendor:      "safechat",
		CorpID:      "corp",
		BaseURL:     "https://portal.example.test",
	}); err != nil {
		t.Fatalf("default http client fetch = %v", err)
	}
}
