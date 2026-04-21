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

// oauth_helpers.go holds the cross-cutting OAuth utilities that did not fit
// into oauth_exchange.go / oauth_refresh.go / oauth_errors.go /
// oauth_callback.go: the authorisation-URL builder, the shared HTTP POST
// wrapper used by both exchange and refresh, and the small MCP REST clients
// (/cli/cliAuthEnabled, /cli/superAdmin, /cli/sendCliAuthApply, /cli/clientId)
// together with their retry wrapper and response envelopes.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

func (p *OAuthProvider) postJSON(ctx context.Context, endpoint string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	ApplyChannelHeader(req)

	client := p.httpClient
	if client == nil {
		client = oauthHTTPClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBody(data, 200))
	}
	return data, nil
}

func buildAuthURL(clientID, redirectURI string) string {
	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {DefaultScopes},
		"prompt":        {"consent"},
	}
	return AuthorizeURL + "?" + params.Encode()
}

// SuperAdmin represents a corp super admin.
type SuperAdmin struct {
	StaffID string `json:"staffId"`
	Name    string `json:"name"`
}

// SuperAdminResponse represents the response from /cli/superAdmin API.
type SuperAdminResponse struct {
	Success   bool         `json:"success"`
	ErrorCode string       `json:"errorCode,omitempty"`
	ErrorMsg  string       `json:"errorMsg,omitempty"`
	Result    []SuperAdmin `json:"result"`
}

// SendApplyResponse represents the response from /cli/sendCliAuthApply API.
type SendApplyResponse struct {
	Success   bool   `json:"success"`
	ErrorCode string `json:"errorCode,omitempty"`
	ErrorMsg  string `json:"errorMsg,omitempty"`
	Result    bool   `json:"result"`
}

// mcpRequestMaxRetries is the maximum number of attempts for MCP API calls
// (e.g. /cli/cliAuthEnabled, /cli/clientId, /cli/superAdmin, /cli/sendCliAuthApply)
// to tolerate transient network errors before propagating the failure.
const mcpRequestMaxRetries = 3

// CheckCLIAuthEnabled checks if CLI authorization is enabled for the current corp.
// It retries up to mcpRequestMaxRetries times on transient errors to avoid
// false negatives caused by momentary network issues.
func (p *OAuthProvider) CheckCLIAuthEnabled(ctx context.Context, accessToken string) (*CLIAuthStatus, error) {
	var lastErr error
	for attempt := 0; attempt < mcpRequestMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		status, err := p.doCheckCLIAuthEnabled(ctx, accessToken)
		if err == nil {
			return status, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("check CLI auth status failed after %d attempts: %w", mcpRequestMaxRetries, lastErr)
}

func (p *OAuthProvider) doCheckCLIAuthEnabled(ctx context.Context, accessToken string) (*CLIAuthStatus, error) {
	url := GetMCPBaseURL() + CLIAuthEnabledPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-user-access-token", accessToken)
	ApplyChannelHeader(req)

	client := p.httpClient
	if client == nil {
		client = oauthHTTPClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var status CLIAuthStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &status, nil
}

// GetSuperAdmins fetches the list of corp super admins.
// It retries up to mcpRequestMaxRetries times on transient errors.
func GetSuperAdmins(ctx context.Context, accessToken string) (*SuperAdminResponse, error) {
	var lastErr error
	for attempt := 0; attempt < mcpRequestMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		result, err := doGetSuperAdmins(ctx, accessToken)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("get super admins failed after %d attempts: %w", mcpRequestMaxRetries, lastErr)
}

func doGetSuperAdmins(ctx context.Context, accessToken string) (*SuperAdminResponse, error) {
	url := GetMCPBaseURL() + SuperAdminPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-user-access-token", accessToken)
	ApplyChannelHeader(req)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result SuperAdminResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &result, nil
}

// SendCliAuthApply sends a CLI auth apply request to the specified admin.
// It retries up to mcpRequestMaxRetries times on transient errors.
func SendCliAuthApply(ctx context.Context, accessToken, adminStaffID string) (*SendApplyResponse, error) {
	var lastErr error
	for attempt := 0; attempt < mcpRequestMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		result, err := doSendCliAuthApply(ctx, accessToken, adminStaffID)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("send CLI auth apply failed after %d attempts: %w", mcpRequestMaxRetries, lastErr)
}

func doSendCliAuthApply(ctx context.Context, accessToken, adminStaffID string) (*SendApplyResponse, error) {
	url := GetMCPBaseURL() + SendCliAuthApplyPath + "?adminStaffId=" + adminStaffID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-user-access-token", accessToken)
	ApplyChannelHeader(req)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result SendApplyResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &result, nil
}

// ClientIDResponse represents the response from /cli/clientId API.
type ClientIDResponse struct {
	Success   bool   `json:"success"`
	ErrorCode string `json:"errorCode,omitempty"`
	ErrorMsg  string `json:"errorMsg,omitempty"`
	Result    string `json:"result"`
}

// FetchClientIDFromMCP fetches the CLI client ID from MCP server.
// This is used when no client ID is provided via flags, config, or env vars.
// It retries up to mcpRequestMaxRetries times on transient errors.
func FetchClientIDFromMCP(ctx context.Context) (string, error) {
	var lastErr error
	for attempt := 0; attempt < mcpRequestMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		id, err := doFetchClientIDFromMCP(ctx)
		if err == nil {
			return id, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("fetch client ID failed after %d attempts: %w", mcpRequestMaxRetries, lastErr)
}

func doFetchClientIDFromMCP(ctx context.Context) (string, error) {
	url := GetMCPBaseURL() + ClientIDPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	ApplyChannelHeader(req)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var result ClientIDResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("%s: %s", result.ErrorCode, result.ErrorMsg)
	}
	return result.Result, nil
}
