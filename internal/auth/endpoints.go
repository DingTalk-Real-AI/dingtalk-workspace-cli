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
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:         "DWS_CLIENT_ID",
		Category:     configmeta.CategoryAuth,
		Description:  "OAuth AppKey (DingTalk 应用凭证)",
		DefaultValue: "(内置)",
		Sensitive:    true,
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:         "DWS_CLIENT_SECRET",
		Category:     configmeta.CategoryAuth,
		Description:  "OAuth AppSecret (DingTalk 应用凭证)",
		DefaultValue: "(内置)",
		Sensitive:    true,
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "DWS_CHANNEL",
		Category:    configmeta.CategoryExternal,
		Description: "第三方渠道编码 (channelCode)，仅透传到上游，如 Qoderwork",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        "CLAW_TYPE",
		Category:    configmeta.CategoryExternal,
		Description: "本地宿主类型，用于控制 host-owned PAT 授权/回调契约",
	})
}

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

	// Terminal API base URL for developer settings page.
	DefaultTerminalBaseURL = "https://open-dev.dingtalk.com"
	// DevicePollPath is the device flow polling path (used with MCP base URL).
	DevicePollPath = "/cli/oauth/device/poll"

	// DeveloperSettingsPath is the path to the organization developer settings page.
	DeveloperSettingsPath = "/fe/old#/developerSettings"

	LogoutURL         = "https://login.dingtalk.com/oauth2/logout"
	LogoutContinueURL = "https://login.dingtalk.com"

	// MCP API endpoints for CLI authorization management.
	DefaultMCPBaseURL    = "https://mcp.dingtalk.com"
	CLIAuthEnabledPath   = "/cli/cliAuthEnabled"
	SuperAdminPath       = "/cli/superAdmin"
	SendCliAuthApplyPath = "/cli/sendCliAuthApply"
	ClientIDPath         = "/cli/clientId"

	// MCP OAuth endpoints (used when clientId is fetched from MCP).
	MCPOAuthTokenPath   = "/oauth2/getToken"
	MCPRefreshTokenPath = "/oauth2/refreshToken"
	MCPRevokeTokenPath  = "/oauth2/revokeToken"
)

// GetTerminalBaseURL returns the terminal base URL with priority:
// 1. ~/.dws/terminal_url file content (for pre-release environment)
// 2. Default value (https://open-dev.dingtalk.com)
func GetTerminalBaseURL() string {
	return config.GetTerminalBaseURL()
}

// GetDeveloperSettingsURL returns the full URL to the organization developer
// settings page, derived from the terminal base URL.
func GetDeveloperSettingsURL() string {
	return config.GetDeveloperSettingsURL()
}

// GetMCPBaseURL returns the MCP base URL with priority:
// 1. ~/.dws/mcp_url file content (for pre-release environment)
// 2. Default value (https://mcp.dingtalk.com)
func GetMCPBaseURL() string {
	mcpURLPath := filepath.Join(getDefaultConfigDir(), "mcp_url")
	if data, err := os.ReadFile(mcpURLPath); err == nil {
		if url := strings.TrimSpace(string(data)); url != "" {
			return url
		}
	}
	return DefaultMCPBaseURL
}

// Runtime overrides set via CLI flags (--client-id, --client-secret).
// These take highest priority over environment variables and defaults.
var (
	clientMu            sync.RWMutex
	runtimeClientID     string
	runtimeClientSecret string
	// clientIDFromMCP indicates whether the clientID was fetched from MCP server.
	// When true, MCP OAuth endpoints should be used instead of direct DingTalk API.
	clientIDFromMCP bool
)

// SetClientIDFromMCP sets the clientID fetched from MCP server and marks it as MCP-sourced.
func SetClientIDFromMCP(id string) {
	clientMu.Lock()
	defer clientMu.Unlock()
	runtimeClientID = id
	clientIDFromMCP = true
}

// IsClientIDFromMCP returns true if the current clientID was fetched from MCP server.
func IsClientIDFromMCP() bool {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return clientIDFromMCP || edition.Get().AuthClientFromMCP
}

// GetUserAccessTokenURL returns the appropriate token exchange URL.
// Uses MCP endpoint when clientID is from MCP, otherwise uses direct DingTalk API.
func GetUserAccessTokenURL() string {
	if IsClientIDFromMCP() {
		return GetMCPBaseURL() + MCPOAuthTokenPath
	}
	return UserAccessTokenURL
}

// GetRefreshTokenURL returns the appropriate token refresh URL.
// Uses MCP endpoint when clientID is from MCP, otherwise uses direct DingTalk API.
func GetRefreshTokenURL() string {
	if IsClientIDFromMCP() {
		return GetMCPBaseURL() + MCPRefreshTokenPath
	}
	return UserAccessTokenURL // DingTalk uses same endpoint for refresh
}

// GetRevokeTokenURL returns the token revocation URL (MCP only).
// Returns empty string if not using MCP mode.
func GetRevokeTokenURL() string {
	if IsClientIDFromMCP() {
		return GetMCPBaseURL() + MCPRevokeTokenPath
	}
	return "" // Direct mode doesn't have revoke endpoint
}

// resolveCredentialSource determines the source of the current credentials.
// Returns one of: "flag", "env", "app", "default".
// This is used to track where credentials came from for token refresh.
func resolveCredentialSource() string {
	clientMu.RLock()
	hasRuntimeOverride := runtimeClientID != "" || runtimeClientSecret != ""
	clientMu.RUnlock()

	if hasRuntimeOverride {
		return "flag"
	}
	// Check if loaded from app config
	if id, _ := ResolveAppCredentials(getDefaultConfigDir()); id != "" {
		return "app"
	}
	if os.Getenv("DWS_CLIENT_ID") != "" || os.Getenv("DWS_CLIENT_SECRET") != "" {
		return "env"
	}
	return "default"
}

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
// 4. Default hardcoded value (if not a placeholder)
// Returns empty string if no valid client ID is available.
// Note: MCP server fetch (priority 4 in the full flow) is handled in OAuthProvider.Login()
func ClientID() string {
	clientMu.RLock()
	override := runtimeClientID
	clientMu.RUnlock()
	if override != "" {
		return override
	}
	if id := edition.Get().AuthClientID; id != "" {
		return id
	}
	// Try loading from persisted app config
	if id, _ := ResolveAppCredentials(getDefaultConfigDir()); id != "" {
		return id
	}
	if v := os.Getenv("DWS_CLIENT_ID"); v != "" {
		return v
	}
	// Only return default if it's not a placeholder
	if !strings.HasPrefix(DefaultClientID, "<") {
		return DefaultClientID
	}
	return ""
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

// HasValidClientSecret returns true if a valid client secret is available.
// A valid secret is one that is not a placeholder (e.g., <YOUR_CLIENT_SECRET>).
func HasValidClientSecret() bool {
	secret := ClientSecret()
	return secret != "" && !strings.HasPrefix(secret, "<")
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
