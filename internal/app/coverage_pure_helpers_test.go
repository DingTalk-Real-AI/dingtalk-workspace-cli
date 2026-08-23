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
	"context"
	"errors"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageTokenResolutionErrorBranches(t *testing.T) {
	if err := tokenResolutionError(nil); err != nil {
		t.Fatalf("nil error must pass through: %v", err)
	}
	if err := tokenResolutionError(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("context cancellation must pass through unwrapped: %v", err)
	}
	if err := tokenResolutionError(context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline exceeded must pass through unwrapped: %v", err)
	}
	if err := tokenResolutionError(authpkg.ErrTokenDataNotFound); err == nil ||
		!strings.Contains(err.Error(), "dws auth login") {
		t.Fatalf("not-found must become a login guidance error: %v", err)
	}
	generic := errors.New("keychain locked")
	if err := tokenResolutionError(generic); err == nil ||
		!strings.Contains(err.Error(), "resolve access token") || !errors.Is(err, generic) {
		t.Fatalf("generic error must wrap with resolve access token: %v", err)
	}
}

func TestCrossPlatformCoverageProfileSwitchCellsSameCorp(t *testing.T) {
	profile := authpkg.Profile{CorpID: "corp-1", CorpName: "钉钉", UserName: "Alice", UserID: "u1"}
	cfg := &authpkg.ProfilesConfig{Profiles: []authpkg.Profile{
		profile,
		{CorpID: "corp-1", CorpName: "钉钉", UserName: "Bob", UserID: "u2"},
	}}
	org, _ := profileSwitchProfileCells(profile, cfg)
	if !strings.Contains(org, " / Alice") {
		t.Fatalf("same-corp label must disambiguate with user name: %q", org)
	}

	noName := authpkg.Profile{CorpID: "corp-1", CorpName: "钉钉", UserID: "u1"}
	org, _ = profileSwitchProfileCells(noName, cfg)
	if !strings.Contains(org, " / u1") {
		t.Fatalf("blank user name must fall back to user id: %q", org)
	}

	blankUser := authpkg.Profile{CorpID: "corp-1", CorpName: "钉钉"}
	org, _ = profileSwitchProfileCells(blankUser, cfg)
	if strings.Contains(org, " / ") {
		t.Fatalf("blank user name and id must not append a suffix: %q", org)
	}

	org, _ = profileSwitchProfileCells(profile, nil)
	if strings.Contains(org, " / ") {
		t.Fatalf("nil config must not append a suffix: %q", org)
	}
}

func TestCrossPlatformCoverageAuthRefreshRetryHelpers(t *testing.T) {
	var nilErr *authRefreshFailureError
	if got := nilErr.Unwrap(); got != nil {
		t.Fatalf("nil receiver Unwrap = %v, want nil", got)
	}
	rejection := errors.New("rejected")
	refresh := errors.New("refresh boom")
	wrapped := (&authRefreshFailureError{rejection: rejection, refresh: refresh}).Unwrap()
	if len(wrapped) != 2 || !errors.Is(wrapped[0], rejection) || !errors.Is(wrapped[1], refresh) {
		t.Fatalf("Unwrap must surface both causes: %v", wrapped)
	}

	if ctx := withAuthRetrying(nil); ctx == nil || !IsAuthRetrying(ctx) {
		t.Fatal("withAuthRetrying(nil) must produce a retry-marked background context")
	}

	var nilRunner *runtimeRunner
	if nilRunner.managesRuntimeOAuth(false) {
		t.Fatal("nil runner must not manage runtime OAuth")
	}
	if (&runtimeRunner{}).managesRuntimeOAuth(true) {
		t.Fatal("plugin-owned auth must opt out of runtime OAuth management")
	}
	if !(&runtimeRunner{}).managesRuntimeOAuth(false) {
		t.Fatal("nil global flags must manage runtime OAuth")
	}
	if !(&runtimeRunner{globalFlags: &GlobalFlags{}}).managesRuntimeOAuth(false) {
		t.Fatal("blank token must manage runtime OAuth")
	}
	if (&runtimeRunner{globalFlags: &GlobalFlags{Token: "tok"}}).managesRuntimeOAuth(false) {
		t.Fatal("explicit token must not manage runtime OAuth")
	}
}

func TestCrossPlatformCoverageForceRefreshRejectedGuards(t *testing.T) {
	if _, err := ForceRefreshAccessToken(context.Background(), "  "); err == nil ||
		!strings.Contains(err.Error(), "config directory is empty") {
		t.Fatalf("blank config dir error = %v", err)
	}
	testseam.Swap(t, &loadRefreshTokenData, func(string) (*authpkg.TokenData, error) { return nil, nil })
	if _, err := ForceRefreshAccessToken(context.Background(), t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "stored access token is empty") {
		t.Fatalf("nil token data must report stored access token is empty: %v", err)
	}
	if _, err := forceRefreshRejectedAccessToken(context.Background(), "  ", "tok"); err == nil ||
		!strings.Contains(err.Error(), "config directory is empty") {
		t.Fatalf("blank config dir error = %v", err)
	}
	if _, err := forceRefreshRejectedAccessToken(context.Background(), t.TempDir(), " "); err == nil ||
		!strings.Contains(err.Error(), "rejected access token is empty") {
		t.Fatalf("blank rejected token error = %v", err)
	}
}

func TestCrossPlatformCoverageGetCachedRuntimeTokenSeam(t *testing.T) {
	token, err := getCachedRuntimeToken(context.Background())
	if err != nil && strings.TrimSpace(token) != "" {
		t.Fatalf("failed token resolution must not return a token: %q / %v", token, err)
	}
}
