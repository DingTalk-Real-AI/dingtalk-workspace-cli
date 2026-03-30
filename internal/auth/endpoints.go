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
	"os"
	"path/filepath"
	"sync"
)

const (
	// AuthorizeURL is the DingTalk OAuth authorization page.
	AuthorizeURL = "https://login.dingtalk.com/oauth2/auth"

	// UserAccessTokenURL exchanges an authorization code for user tokens.
	UserAccessTokenURL = "https://api.dingtalk.com/v1.0/oauth2/userAccessToken"

	// UserInfoURL fetches the authenticated user's profile.
	UserInfoURL = "https://api.dingtalk.com/v1.0/contact/users/me"

	// DefaultClientID is the CLI's built-in OAuth client ID (DingTalk AppKey).
	// TODO: Replace <YOUR_CLIENT_ID> with your actual DingTalk AppKey before building.
	DefaultClientID = "<YOUR_CLIENT_ID>"

	// DefaultClientSecret is the CLI's built-in OAuth client secret (DingTalk AppSecret).
	// TODO: Replace <YOUR_CLIENT_SECRET> with your actual DingTalk AppSecret before building.
	DefaultClientSecret = "<YOUR_CLIENT_SECRET>"

	// CallbackPath is the localhost callback endpoint for OAuth redirect.
	CallbackPath = "/callback"

	// DefaultScopes are the OAuth scopes requested by the CLI.
	DefaultScopes = "openid corpid"

	// Device Authorization Grant (RFC 8628) endpoints.

	// DefaultDeviceBaseURL is the login server base URL for device flow.
	DefaultDeviceBaseURL = "https://login.dingtalk.com"

	// DeviceCodePath requests a device_code and user_code.
	DeviceCodePath = "/oauth2/device/code.json"

	// DeviceTokenPath polls for authorization completion.
	DeviceTokenPath = "/oauth2/device/token.json"

	// DeviceGrantType is the grant_type value defined by RFC 8628.
	DeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	LogoutURL         = "https://login.dingtalk.com/oauth2/logout"
	LogoutContinueURL = "https://login.dingtalk.com"
)

// Runtime overrides set via CLI flags (--client-id, --client-secret).
// These take highest priority over environment variables and defaults.
var (
	clientMu            sync.RWMutex
	runtimeClientID     string
	runtimeClientSecret string
)

// SetClientID allows runtime override of the client ID (e.g., from CLI flags).
func SetClientID(id string) {
	clientMu.Lock()
	defer clientMu.Unlock()
	runtimeClientID = id
}

// SetClientSecret allows runtime override of the client secret (e.g., from CLI flags).
func SetClientSecret(secret string) {
	clientMu.Lock()
	defer clientMu.Unlock()
	runtimeClientSecret = secret
}

// ClientID returns the OAuth client ID with priority:
// 1. Runtime override (CLI flag --client-id)
// 2. Persisted app config (from previous login)
// 3. Environment variable (DWS_CLIENT_ID)
// 4. Default hardcoded value
func ClientID() string {
	clientMu.RLock()
	override := runtimeClientID
	clientMu.RUnlock()
	if override != "" {
		return override
	}
	// Try loading from persisted app config
	if id, _ := ResolveAppCredentials(getDefaultConfigDir()); id != "" {
		return id
	}
	if v := os.Getenv("DWS_CLIENT_ID"); v != "" {
		return v
	}
	return DefaultClientID
}

// ClientSecret returns the OAuth client secret with priority:
// 1. Runtime override (CLI flag --client-secret)
// 2. Persisted app config (from previous login, stored in keychain)
// 3. Environment variable (DWS_CLIENT_SECRET)
// 4. Default hardcoded value
func ClientSecret() string {
	clientMu.RLock()
	override := runtimeClientSecret
	clientMu.RUnlock()
	if override != "" {
		return override
	}
	// Try loading from persisted app config (secret is in keychain)
	if _, secret := ResolveAppCredentials(getDefaultConfigDir()); secret != "" {
		return secret
	}
	if v := os.Getenv("DWS_CLIENT_SECRET"); v != "" {
		return v
	}
	return DefaultClientSecret
}

// getRuntimeCredentials returns the runtime-override credentials if set.
// Returns empty strings if no runtime overrides were provided.
func getRuntimeCredentials() (clientID, clientSecret string) {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return runtimeClientID, runtimeClientSecret
}

// getEnvClientID returns the environment variable client ID if set.
func getEnvClientID() string {
	return os.Getenv("DWS_CLIENT_ID")
}

// getDefaultConfigDir returns the default configuration directory.
// Priority: DWS_CONFIG_DIR env var > ~/.dws
func getDefaultConfigDir() string {
	if envDir := os.Getenv("DWS_CONFIG_DIR"); envDir != "" {
		return envDir
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".dws"
	}
	return filepath.Join(homeDir, ".dws")
}
