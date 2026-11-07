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
	"net/http"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestCrossPlatformCoverageAppTokenUserTokenAndClientSecretNamespacesAreIsolated(t *testing.T) {
	clientID := "app-key-1"
	appTokenAccount := appTokenPrefix + clientID
	userTokenAccounts := []string{
		keychain.AccountToken,
		TokenAccountForCorpID("corp-1"),
		TokenAccountForIdentity("corp-1", "user-1"),
	}
	secretAccount := secretAccountKey(clientID)
	for _, userAccount := range userTokenAccounts {
		if appTokenAccount == userAccount || secretAccount == userAccount {
			t.Fatalf("credential namespaces collide: app=%q user=%q secret=%q", appTokenAccount, userAccount, secretAccount)
		}
	}
	if appTokenAccount == secretAccount || !strings.HasPrefix(appTokenAccount, "app-token:") || !strings.HasPrefix(secretAccount, "appsecret:") {
		t.Fatalf("app token and client secret namespaces are not isolated: %q %q", appTokenAccount, secretAccount)
	}
}

func TestCrossPlatformCoverageAppTokenRedirectPolicyIsSameOriginHTTPSOnly(t *testing.T) {
	original, _ := http.NewRequest(http.MethodPost, AppAccessTokenURL, nil)
	same, _ := http.NewRequest(http.MethodPost, "https://api.dingtalk.com/v1.0/oauth2/accessToken2", nil)
	if err := appTokenRedirectPolicy(same, []*http.Request{original}); err != nil {
		t.Fatalf("same-origin app token redirect failed: %v", err)
	}
	for _, target := range []string{
		"https://oapi.dingtalk.com/gettoken",
		"http://api.dingtalk.com/v1.0/oauth2/accessToken",
	} {
		next, _ := http.NewRequest(http.MethodPost, target, nil)
		if err := appTokenRedirectPolicy(next, []*http.Request{original}); err == nil {
			t.Errorf("app token redirect to %s should fail", target)
		}
	}
}
