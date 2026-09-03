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
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

func TestPortalAuthCodePostsVendorAndCorpOnly(t *testing.T) {
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

func TestPortalAuthCodeRetriesOnceOnTokenInvalidAfterRefresh(t *testing.T) {
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

func TestPortalAuthCodeDoesNotRetryOrgMismatch(t *testing.T) {
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

func TestPortalAuthCodeRequiresCorpID(t *testing.T) {
	p := &PortalAuthCode{
		snapshot: func(context.Context, string) (*auth.TokenData, error) {
			return &auth.TokenData{AccessToken: "tok", ClientID: "id"}, nil
		},
	}
	if _, err := p.AuthCodeForCorp(context.Background(), "  "); !errors.Is(err, ErrNoCorpID) {
		t.Fatalf("AuthCodeForCorp() = %v, want ErrNoCorpID", err)
	}
}
