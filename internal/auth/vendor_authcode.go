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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	headerUserAccessToken = "x-user-access-token"
	headerDWSClientID     = "x-dws-client-id"
	headerDWSCLIVersion   = "x-dws-cli-version"

	// VendorAuthCode default / documented portal expiresIn, used only when
	// the success body omits a positive value. Callers must still prefer
	// the response field.
	DefaultVendorAuthCodeExpiresIn = 120

	VendorAuthCodeParamError        = "PARAM_ERROR"
	VendorAuthCodeVendorUnsupported = "VENDOR_UNSUPPORTED"
	VendorAuthCodeTokenInvalid      = "TOKEN_INVALID"
	VendorAuthCodeOrgMismatch       = "ORG_MISMATCH"
	VendorAuthCodeUserNotInOrg      = "USER_NOT_IN_ORG"
	VendorAuthCodeVendorNotEnabled  = "VENDOR_NOT_ENABLED"
	VendorAuthCodeRateLimited       = "RATE_LIMITED"
	VendorAuthCodeInternalError     = "INTERNAL_ERROR"
)

// VendorAuthCodeInput is a POST /oauth2/vendorAuthCode call. The body is
// only vendor + corpId; redirectURI and domain must not be sent.
type VendorAuthCodeInput struct {
	AccessToken string
	ClientID    string
	CLIVersion  string
	LoginRegion LoginRegion
	Vendor      string
	CorpID      string
	HTTPClient  *http.Client
	// BaseURL overrides MCPBaseURLForLoginRegion. Tests use it; production
	// callers leave it empty.
	BaseURL string
}

// VendorAuthCodeResult is the success VO from portal.
type VendorAuthCodeResult struct {
	AuthCode  string
	ExpiresIn int
}

// VendorAuthCodeError is a portal business error carried in an HTTP 200
// ServiceResult body (same envelope as /oauth2/getToken).
type VendorAuthCodeError struct {
	Code    string
	Message string
}

func (e *VendorAuthCodeError) Error() string {
	if e == nil {
		return "vendorAuthCode failed"
	}
	if strings.TrimSpace(e.Message) != "" {
		return fmt.Sprintf("vendorAuthCode %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("vendorAuthCode %s", e.Code)
}

// Retryable reports whether DWS should retry this portal error once.
func (e *VendorAuthCodeError) Retryable() bool {
	if e == nil {
		return false
	}
	switch e.Code {
	case VendorAuthCodeTokenInvalid, VendorAuthCodeRateLimited, VendorAuthCodeInternalError:
		return true
	default:
		return false
	}
}

// FetchVendorAuthCode POSTs {vendor, corpId} to /oauth2/vendorAuthCode.
// HTTP is expected to be 200; errors are read from body.errorCode.
func FetchVendorAuthCode(ctx context.Context, in VendorAuthCodeInput) (*VendorAuthCodeResult, error) {
	vendor := strings.ToLower(strings.TrimSpace(in.Vendor))
	corpID := strings.TrimSpace(in.CorpID)
	token := strings.TrimSpace(in.AccessToken)
	clientID := strings.TrimSpace(in.ClientID)
	if token == "" || clientID == "" || vendor == "" || corpID == "" {
		return nil, &VendorAuthCodeError{
			Code:    VendorAuthCodeParamError,
			Message: "token, clientId, vendor and corpId are required",
		}
	}

	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if base == "" {
		base = strings.TrimRight(MCPBaseURLForLoginRegion(in.LoginRegion), "/")
	}
	endpoint := base + MCPVendorAuthCodePath

	payload, err := json.Marshal(struct {
		Vendor string `json:"vendor"`
		CorpID string `json:"corpId"`
	}{Vendor: vendor, CorpID: corpID})
	if err != nil {
		return nil, fmt.Errorf("marshaling vendorAuthCode request: %w", err)
	}

	req, err := oauthNewRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating vendorAuthCode request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerUserAccessToken, token)
	req.Header.Set(headerDWSClientID, clientID)
	if ver := strings.TrimSpace(in.CLIVersion); ver != "" {
		req.Header.Set(headerDWSCLIVersion, ver)
	}
	applyEditionEnterpriseCredentialHeaders(req)

	client := in.HTTPClient
	if client == nil {
		client = oauthHTTPClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending vendorAuthCode request: %w", err)
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
	if resp.StatusCode != http.StatusOK {
		if readErr != nil {
			data = nil
		}
		return nil, &HTTPStatusError{
			StatusCode:   resp.StatusCode,
			responseBody: truncateBody(data, 200),
		}
	}
	if readErr != nil {
		return nil, fmt.Errorf("reading vendorAuthCode response: %w", readErr)
	}
	return parseVendorAuthCodeResponse(data)
}

func parseVendorAuthCodeResponse(body []byte) (*VendorAuthCodeResult, error) {
	var resp struct {
		AuthCode  string `json:"authCode"`
		ExpiresIn int    `json:"expiresIn"`
		Success   *bool  `json:"success"`
		ErrorCode string `json:"errorCode"`
		ErrorMsg  string `json:"errorMsg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing vendorAuthCode response: %w", err)
	}
	if resp.ErrorCode != "" || resp.ErrorMsg != "" || (resp.Success != nil && !*resp.Success) {
		code := strings.TrimSpace(resp.ErrorCode)
		if code == "" {
			code = VendorAuthCodeInternalError
		}
		return nil, &VendorAuthCodeError{Code: code, Message: resp.ErrorMsg}
	}
	authCode := strings.TrimSpace(resp.AuthCode)
	if authCode == "" {
		return nil, fmt.Errorf("vendorAuthCode response missing authCode")
	}
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = DefaultVendorAuthCodeExpiresIn
	}
	return &VendorAuthCodeResult{AuthCode: authCode, ExpiresIn: expiresIn}, nil
}
