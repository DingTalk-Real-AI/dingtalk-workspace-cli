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
	"net/url"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/config"
)

func (p *OAuthProvider) exchangeCode(ctx context.Context, code string) (*TokenData, error) {
	body := map[string]string{
		"clientId":     ClientID(),
		"clientSecret": ClientSecret(),
		"code":         code,
		"grantType":    "authorization_code",
	}
	resp, err := p.postJSON(ctx, UserAccessTokenURL, body)
	if err != nil {
		return nil, err
	}
	return p.parseTokenResponse(resp)
}

func (p *OAuthProvider) refreshWithRefreshToken(ctx context.Context, data *TokenData) (*TokenData, error) {
	body := map[string]string{
		"clientId":     ClientID(),
		"clientSecret": ClientSecret(),
		"refreshToken": data.RefreshToken,
		"grantType":    "refresh_token",
	}
	resp, err := p.postJSON(ctx, UserAccessTokenURL, body)
	if err != nil {
		return nil, err
	}
	updated, err := p.parseTokenResponse(resp)
	if err != nil {
		return nil, err
	}
	updated.PersistentCode = data.PersistentCode
	updated.CorpID = data.CorpID
	updated.UserID = data.UserID
	updated.UserName = data.UserName
	updated.CorpName = data.CorpName

	if err := SaveTokenData(p.configDir, updated); err != nil {
		return nil, fmt.Errorf("保存刷新后的 token 失败（旧 refresh_token 已失效，请重新登录）: %w", err)
	}
	return updated, nil
}

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

func (p *OAuthProvider) parseTokenResponse(body []byte) (*TokenData, error) {
	var resp struct {
		AccessToken    string `json:"accessToken"`
		RefreshToken   string `json:"refreshToken"`
		PersistentCode string `json:"persistentCode"`
		ExpiresIn      int64  `json:"expiresIn"`
		CorpID         string `json:"corpId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	if resp.AccessToken == "" {
		return nil, fmt.Errorf("token response missing accessToken")
	}

	now := time.Now()
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		// 默认 2 小时有效期（钉钉 access_token 标准有效期）
		expiresIn = config.DefaultAccessTokenExpiry
	}
	data := &TokenData{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second),
		RefreshExpAt: now.Add(config.DefaultRefreshTokenLifetime),
		CorpID:       resp.CorpID,
	}
	if resp.PersistentCode != "" {
		data.PersistentCode = resp.PersistentCode
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

const successHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>钉钉 CLI</title>
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#f5f5f5}
.card{background:#fff;border-radius:12px;padding:40px;text-align:center;box-shadow:0 2px 12px rgba(0,0,0,.08)}
h1{color:#1677ff;margin:0 0 8px}p{color:#666;margin:0}</style></head>
<body><div class="card"><h1>✅ 授权成功</h1><p>请返回终端继续操作。此页面可以关闭。</p></div></body></html>`
