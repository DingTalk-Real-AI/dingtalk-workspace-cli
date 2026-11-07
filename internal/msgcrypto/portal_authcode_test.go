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

package msgcrypto

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

func TestCrossPlatformCoveragePortalAuthCodePostsVendorAndCorpOnly(t *testing.T) {
	var got auth.VendorAuthCodeInput
	p := &PortalAuthCode{
		ConfigDir:  t.TempDir(),
		Vendor:     VendorSafeChat,
		CLIVersion: "1.2.3",
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{
				AccessToken: "user-token",
				ClientID:    "dws-client",
				CorpID:      "ding_login",
				LoginRegion: "",
			}, nil
		},
		fetch: func(_ context.Context, in auth.VendorAuthCodeInput) (*auth.VendorAuthCodeResult, error) {
			got = in
			return &auth.VendorAuthCodeResult{AuthCode: "tmp-code", ExpiresIn: 120}, nil
		},
	}

	code, err := p.AuthCodeForCorp(context.Background(), "ding_target")
	if err != nil || code != "tmp-code" {
		t.Fatalf("AuthCodeForCorp() = %q, %v", code, err)
	}
	if got.Vendor != VendorSafeChat || got.CorpID != "ding_target" {
		t.Fatalf("body vendor/corpId = %q/%q", got.Vendor, got.CorpID)
	}
	if got.AccessToken != "user-token" || got.ClientID != "dws-client" || got.CLIVersion != "1.2.3" {
		t.Fatalf("headers token/client/version = %q/%q/%q", got.AccessToken, got.ClientID, got.CLIVersion)
	}
}

func TestCrossPlatformCoveragePortalAuthCodeRetriesOnceOnTokenInvalidAfterRefresh(t *testing.T) {
	var calls atomic.Int32
	p := &PortalAuthCode{
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{AccessToken: "stale", ClientID: "dws-client", CorpID: "ding"}, nil
		},
		refresh: func(context.Context, string, string) (string, error) {
			return "fresh", nil
		},
		fetch: func(_ context.Context, in auth.VendorAuthCodeInput) (*auth.VendorAuthCodeResult, error) {
			n := calls.Add(1)
			if in.AccessToken == "stale" {
				return nil, &auth.VendorAuthCodeError{Code: auth.VendorAuthCodeTokenInvalid, Message: "expired"}
			}
			if in.AccessToken != "fresh" {
				t.Fatalf("retry token = %q, want fresh", in.AccessToken)
			}
			if n != 2 {
				t.Fatalf("fetch calls = %d, want 2", n)
			}
			return &auth.VendorAuthCodeResult{AuthCode: "new-code", ExpiresIn: 120}, nil
		},
	}

	code, err := p.AuthCodeForCorp(context.Background(), "ding")
	if err != nil || code != "new-code" {
		t.Fatalf("AuthCodeForCorp() = %q, %v", code, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("fetch called %d times, want 2", calls.Load())
	}
}

func TestCrossPlatformCoveragePortalAuthCodeDoesNotRetryOrgMismatch(t *testing.T) {
	var calls atomic.Int32
	want := &auth.VendorAuthCodeError{Code: auth.VendorAuthCodeOrgMismatch, Message: "mismatch"}
	p := &PortalAuthCode{
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{AccessToken: "tok", ClientID: "dws-client", CorpID: "ding"}, nil
		},
		fetch: func(context.Context, auth.VendorAuthCodeInput) (*auth.VendorAuthCodeResult, error) {
			calls.Add(1)
			return nil, want
		},
	}

	_, err := p.AuthCodeForCorp(context.Background(), "other")
	if !errors.Is(err, want) {
		t.Fatalf("AuthCodeForCorp() = %v, want %v", err, want)
	}
	if calls.Load() != 1 {
		t.Fatalf("fetch called %d times, want 1", calls.Load())
	}
}

func TestCrossPlatformCoveragePortalAuthCodeRequiresCorpID(t *testing.T) {
	p := &PortalAuthCode{
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{AccessToken: "tok", ClientID: "id"}, nil
		},
	}
	if _, err := p.AuthCodeForCorp(context.Background(), "  "); !errors.Is(err, ErrNoCorpID) {
		t.Fatalf("AuthCodeForCorp() = %v, want ErrNoCorpID", err)
	}
}

func TestCrossPlatformCoveragePortalAuthCodeRemainingEdges(t *testing.T) {
	constructed := NewPortalAuthCode(t.TempDir(), "1.2.3")
	if constructed.Vendor != VendorSafeChat || constructed.CLIVersion != "1.2.3" || constructed.clientID == nil || constructed.snapshot == nil || constructed.refresh == nil || constructed.fetch == nil {
		t.Fatalf("constructed portal provider = %#v", constructed)
	}
	_, _ = constructed.snapshot(context.Background(), constructed.ConfigDir)
	_, _ = constructed.refresh(context.Background(), constructed.ConfigDir, "rejected")

	wantErr := errors.New("snapshot failed")
	p := &PortalAuthCode{snapshot: func(context.Context, string) (*auth.TokenData, error) {
		return nil, wantErr
	}}
	if _, err := p.AuthCode(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("AuthCode snapshot err = %v, want %v", err, wantErr)
	}

	p = &PortalAuthCode{
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{AccessToken: "tok", ClientID: "client", CorpID: "corp"}, nil
		},
		fetch: func(context.Context, auth.VendorAuthCodeInput) (*auth.VendorAuthCodeResult, error) {
			return &auth.VendorAuthCodeResult{AuthCode: "auth-code"}, nil
		},
	}
	if code, err := p.AuthCode(context.Background()); err != nil || code != "auth-code" {
		t.Fatalf("AuthCode success = %q, %v", code, err)
	}

	p = &PortalAuthCode{}
	if _, err := p.AuthCodeForCorp(context.Background(), "corp"); err == nil || !containsError(err, "snapshot loader") {
		t.Fatalf("missing snapshot loader err = %v", err)
	}

	p = &PortalAuthCode{snapshot: func(context.Context, string) (*auth.TokenData, error) { return nil, nil }}
	if _, err := p.AuthCodeForCorp(context.Background(), "corp"); err == nil || !containsError(err, "empty snapshot") {
		t.Fatalf("empty snapshot err = %v", err)
	}

	p = &PortalAuthCode{snapshot: func(context.Context, string) (*auth.TokenData, error) {
		return &auth.TokenData{AccessToken: " ", CorpID: "corp"}, nil
	}}
	if _, err := p.AuthCodeForCorp(context.Background(), "corp"); err == nil || !containsError(err, "access token is empty") {
		t.Fatalf("empty access token err = %v", err)
	}

	p = &PortalAuthCode{snapshot: func(context.Context, string) (*auth.TokenData, error) {
		return &auth.TokenData{AccessToken: "tok", ClientID: "client"}, nil
	}}
	if _, err := p.AuthCodeForCorp(context.Background(), "corp"); err == nil || !containsError(err, "fetcher") {
		t.Fatalf("missing fetch err = %v", err)
	}

	var got auth.VendorAuthCodeInput
	p = &PortalAuthCode{
		Vendor: "",
		clientID: func() string {
			return "fallback-client"
		},
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{AccessToken: "tok", LoginRegion: string(auth.LoginRegionInternational)}, nil
		},
		fetch: func(_ context.Context, in auth.VendorAuthCodeInput) (*auth.VendorAuthCodeResult, error) {
			got = in
			return &auth.VendorAuthCodeResult{AuthCode: "code"}, nil
		},
	}
	if code, err := p.AuthCodeForCorp(context.Background(), " corp "); err != nil || code != "code" {
		t.Fatalf("fallback fetch = %q, %v", code, err)
	}
	if got.Vendor != VendorSafeChat || got.ClientID != "fallback-client" || got.CorpID != "corp" || got.LoginRegion != auth.LoginRegionInternational {
		t.Fatalf("fallback input = %#v", got)
	}

	var calls atomic.Int32
	p = &PortalAuthCode{
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{AccessToken: "tok", ClientID: "client", CorpID: "corp"}, nil
		},
		fetch: func(context.Context, auth.VendorAuthCodeInput) (*auth.VendorAuthCodeResult, error) {
			if calls.Add(1) == 1 {
				return nil, &auth.HTTPStatusError{StatusCode: http.StatusTooManyRequests}
			}
			return &auth.VendorAuthCodeResult{AuthCode: "retry-code"}, nil
		},
	}
	if code, err := p.AuthCodeForCorp(context.Background(), "corp"); err != nil || code != "retry-code" || calls.Load() != 2 {
		t.Fatalf("retryable status = %q, %v, calls=%d", code, err, calls.Load())
	}
	if retryableVendorAuthCode(errors.New("plain")) {
		t.Fatal("plain errors must not be retryable")
	}

	p = &PortalAuthCode{
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{AccessToken: "stale", ClientID: "client", CorpID: "corp"}, nil
		},
		fetch: func(context.Context, auth.VendorAuthCodeInput) (*auth.VendorAuthCodeResult, error) {
			return nil, &auth.VendorAuthCodeError{Code: auth.VendorAuthCodeTokenInvalid}
		},
	}
	if _, err := p.AuthCodeForCorp(context.Background(), "corp"); err == nil || !containsError(err, auth.VendorAuthCodeTokenInvalid) {
		t.Fatalf("missing refresh err = %v", err)
	}

	p.refresh = func(context.Context, string, string) (string, error) { return "stale", nil }
	if _, err := p.AuthCodeForCorp(context.Background(), "corp"); err == nil || !containsError(err, auth.VendorAuthCodeTokenInvalid) {
		t.Fatalf("unchanged refresh token err = %v", err)
	}
}

func containsError(err error, text string) bool {
	return err != nil && strings.Contains(err.Error(), text)
}
