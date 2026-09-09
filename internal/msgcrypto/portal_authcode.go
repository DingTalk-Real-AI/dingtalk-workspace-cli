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
	"fmt"
	"net/http"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

// VendorSafeChat is the first vendorAuthCode vendor.
const VendorSafeChat = "safechat"

// PortalAuthCode mints a one-shot 免登 authCode from portal
// POST /oauth2/vendorAuthCode. It does not cache the code: goProxy spends it
// immediately. Do not wrap this in CachedAuthCode.
type PortalAuthCode struct {
	ConfigDir  string
	Vendor     string
	CLIVersion string
	HTTPClient *http.Client

	clientID func() string
	snapshot func(ctx context.Context, configDir string) (*auth.TokenData, error)
	refresh  func(ctx context.Context, configDir, rejected string) (string, error)
	fetch    func(ctx context.Context, in auth.VendorAuthCodeInput) (*auth.VendorAuthCodeResult, error)
}

// NewPortalAuthCode returns a provider that talks to portal with the current
// login. cliVersion is sent as x-dws-cli-version; leave empty only when the
// caller cannot know the CLI version.
func NewPortalAuthCode(configDir, cliVersion string) *PortalAuthCode {
	return &PortalAuthCode{
		ConfigDir:  configDir,
		Vendor:     VendorSafeChat,
		CLIVersion: cliVersion,
		clientID:   auth.ClientID,
		snapshot: func(ctx context.Context, dir string) (*auth.TokenData, error) {
			return auth.NewOAuthProvider(dir, nil).GetTokenSnapshot(ctx)
		},
		refresh: func(ctx context.Context, dir, rejected string) (string, error) {
			return auth.NewOAuthProvider(dir, nil).ForceRefreshRejectedToken(ctx, rejected)
		},
		fetch: auth.FetchVendorAuthCode,
	}
}

// AuthCode mints a code for the logged-in organization. Prefer AuthCodeForCorp
// when goProxy already has a corpID.
func (p *PortalAuthCode) AuthCode(ctx context.Context) (string, error) {
	snap, err := p.loadSnapshot(ctx)
	if err != nil {
		return "", err
	}
	return p.AuthCodeForCorp(ctx, snap.CorpID)
}

// AuthCodeForCorp mints a one-shot code for corpID. The request body is only
// vendor + corpId.
func (p *PortalAuthCode) AuthCodeForCorp(ctx context.Context, corpID string) (string, error) {
	corpID = strings.TrimSpace(corpID)
	if corpID == "" {
		return "", ErrNoCorpID
	}
	snap, err := p.loadSnapshot(ctx)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(snap.AccessToken)
	if token == "" {
		return "", fmt.Errorf("msgcrypto: access token is empty")
	}

	result, err := p.fetchOnce(ctx, snap, token, corpID)
	if err == nil {
		return result.AuthCode, nil
	}

	var verr *auth.VendorAuthCodeError
	if errors.As(err, &verr) && verr.Code == auth.VendorAuthCodeTokenInvalid {
		refreshed, rerr := p.doRefresh(ctx, token)
		if rerr == nil && refreshed != "" && refreshed != token {
			result, err = p.fetchOnce(ctx, snap, refreshed, corpID)
			if err == nil {
				return result.AuthCode, nil
			}
		}
		return "", err
	}
	if retryableVendorAuthCode(err) {
		result, err = p.fetchOnce(ctx, snap, token, corpID)
		if err == nil {
			return result.AuthCode, nil
		}
	}
	return "", err
}

func (p *PortalAuthCode) loadSnapshot(ctx context.Context) (*auth.TokenData, error) {
	if p.snapshot == nil {
		return nil, fmt.Errorf("msgcrypto: portal authCode snapshot loader is not configured")
	}
	snap, err := p.snapshot(ctx, p.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("msgcrypto: load access token: %w", err)
	}
	if snap == nil {
		return nil, fmt.Errorf("msgcrypto: load access token: empty snapshot")
	}
	return snap, nil
}

func (p *PortalAuthCode) fetchOnce(ctx context.Context, snap *auth.TokenData, token, corpID string) (*auth.VendorAuthCodeResult, error) {
	fetch := p.fetch
	if fetch == nil {
		return nil, fmt.Errorf("msgcrypto: portal authCode fetcher is not configured")
	}
	vendor := strings.TrimSpace(p.Vendor)
	if vendor == "" {
		vendor = VendorSafeChat
	}
	clientID := strings.TrimSpace(snap.ClientID)
	if clientID == "" && p.clientID != nil {
		clientID = strings.TrimSpace(p.clientID())
	}
	return fetch(ctx, auth.VendorAuthCodeInput{
		AccessToken: token,
		ClientID:    clientID,
		CLIVersion:  p.CLIVersion,
		LoginRegion: auth.LoginRegion(strings.TrimSpace(snap.LoginRegion)),
		Vendor:      vendor,
		CorpID:      corpID,
		HTTPClient:  p.HTTPClient,
	})
}

func (p *PortalAuthCode) doRefresh(ctx context.Context, rejected string) (string, error) {
	if p.refresh == nil {
		return "", fmt.Errorf("msgcrypto: token refresh is not configured")
	}
	return p.refresh(ctx, p.ConfigDir, rejected)
}

func retryableVendorAuthCode(err error) bool {
	var verr *auth.VendorAuthCodeError
	if errors.As(err, &verr) {
		return verr.Retryable()
	}
	var statusErr *auth.HTTPStatusError
	if errors.As(err, &statusErr) && statusErr != nil {
		return statusErr.StatusCode == http.StatusTooManyRequests ||
			statusErr.StatusCode >= http.StatusInternalServerError
	}
	return false
}
