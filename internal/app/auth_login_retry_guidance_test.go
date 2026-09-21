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
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAuthLoginRetryShowsOnlyActionableGuidance(t *testing.T) {
	technicalCause := errors.New("DEKMissing after profiles v1 to v2 migration")
	retryErr := authpkg.NewLoginRetryGuidanceErrorForTest(technicalCause)
	const profileSelector = "retry-profile"
	const corpID = "ding-retry"

	for _, tc := range []struct {
		name  string
		lang  string
		flags map[string]string
		want  string
		stub  func(t *testing.T)
	}{
		{
			name:  "oauth-zh",
			lang:  "zh",
			flags: map[string]string{"profile": profileSelector},
			want:  "请保持 --profile 参数不变，并重新执行 dws auth login",
			stub: func(t *testing.T) {
				t.Helper()
				testseam.Swap(t, &authOAuthLogin, func(provider *authpkg.OAuthProvider, _ context.Context, _ bool) (*authpkg.TokenData, error) {
					if provider.TargetCorpID != corpID {
						t.Errorf("OAuth target corp = %q, want %q", provider.TargetCorpID, corpID)
					}
					return nil, fmt.Errorf("保存 token 失败: %w", retryErr)
				})
			},
		},
		{
			name:  "device-zh",
			lang:  "zh",
			flags: map[string]string{"device": "true", "profile": profileSelector},
			want:  "请保持 --profile 参数不变，并重新执行 dws auth login",
			stub: func(t *testing.T) {
				t.Helper()
				testseam.Swap(t, &authDeviceLogin, func(*authpkg.DeviceFlowProvider, context.Context) (*authpkg.TokenData, error) {
					return nil, fmt.Errorf("保存 token 失败: %w", retryErr)
				})
			},
		},
		{
			// --intl 登录默认输出英文契约：重试提示也必须渲染英文目录条目。
			name:  "oauth-intl-en",
			lang:  "en",
			flags: map[string]string{"profile": profileSelector, "intl": "true"},
			want:  "Please keep the --profile flag unchanged and run dws auth login again",
			stub: func(t *testing.T) {
				t.Helper()
				testseam.Swap(t, &authOAuthLogin, func(provider *authpkg.OAuthProvider, _ context.Context, _ bool) (*authpkg.TokenData, error) {
					if provider.TargetCorpID != corpID {
						t.Errorf("OAuth target corp = %q, want %q", provider.TargetCorpID, corpID)
					}
					return nil, fmt.Errorf("保存 token 失败: %w", retryErr)
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restore := i18n.PushLang(tc.lang)
			t.Cleanup(restore)
			if tc.lang == "en" {
				// 解除环境语言 pin，确保 --intl 的英文 push 真正生效。
				t.Setenv("DWS_LANG", "")
			}
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			testseam.Swap(t, &authLoginInteractiveTerminal, func() bool { return false })
			testseam.Swap(t, &authResolveProfile, func(_ string, selector string) (*authpkg.Profile, error) {
				if selector != profileSelector {
					t.Fatalf("authResolveProfile selector = %q, want %q", selector, profileSelector)
				}
				return &authpkg.Profile{Name: profileSelector, CorpID: corpID, UserID: "retry-user"}, nil
			})
			tc.stub(t)

			_, _, err := authCoverageRunLogin(t, nil, "table", true, tc.flags)
			if err == nil {
				t.Fatal("auth login error = nil")
			}
			if err.Error() != tc.want {
				t.Fatalf("auth login error = %q, want %q", err, tc.want)
			}
			if errors.Is(err, technicalCause) {
				t.Fatalf("auth login error still exposes its technical cause: %v", err)
			}
			var jsonOutput bytes.Buffer
			if printErr := apperrors.PrintJSON(&jsonOutput, err); printErr != nil {
				t.Fatalf("PrintJSON() error = %v", printErr)
			}
			var verboseOutput bytes.Buffer
			if printErr := apperrors.PrintHumanAt(&verboseOutput, err, apperrors.VerbosityVerbose); printErr != nil {
				t.Fatalf("PrintHumanAt() error = %v", printErr)
			}
			var debugOutput bytes.Buffer
			if printErr := apperrors.PrintHumanAt(&debugOutput, err, apperrors.VerbosityDebug); printErr != nil {
				t.Fatalf("PrintHumanAt(debug) error = %v", printErr)
			}
			displays := err.Error() + "\n" + jsonOutput.String() + "\n" + verboseOutput.String() + "\n" + debugOutput.String()
			for _, hidden := range []string{"v1", "v2", "DEK", "token", "配置", "保存", "failed"} {
				if strings.Contains(displays, hidden) {
					t.Fatalf("auth login output exposed %q:\n%s", hidden, displays)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAuthLoginFlowErrorPreservesOrdinaryFailures(t *testing.T) {
	cause := errors.New("network unavailable")
	for _, tc := range []struct {
		name   string
		prefix string
	}{
		{name: "oauth", prefix: "dingtalk login failed"},
		{name: "device", prefix: "device authorization failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := authLoginFlowError(tc.prefix, cause)
			want := tc.prefix + ": " + cause.Error()
			if err.Error() != want {
				t.Fatalf("authLoginFlowError() = %q, want %q", err, want)
			}
			if got := apperrors.ExitCode(err); got != apperrors.ExitCodeAuth {
				t.Fatalf("ExitCode() = %d, want %d", got, apperrors.ExitCodeAuth)
			}
			if _, ok := authpkg.LoginRetryGuidance(err); ok {
				t.Fatalf("ordinary error was misclassified as retry guidance: %v", err)
			}
		})
	}
}
