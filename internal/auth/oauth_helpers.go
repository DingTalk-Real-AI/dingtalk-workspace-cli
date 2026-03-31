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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/config"
)

func (p *OAuthProvider) exchangeCode(ctx context.Context, code string) (*TokenData, error) {
	// Use MCP mode if clientID is from MCP server
	if IsClientIDFromMCP() {
		return p.exchangeCodeViaMCP(ctx, code)
	}
	// Direct mode with client secret
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

// exchangeCodeViaMCP exchanges auth code for token via MCP proxy.
// This is used when client secret is not available (server-side secret management).
func (p *OAuthProvider) exchangeCodeViaMCP(ctx context.Context, code string) (*TokenData, error) {
	url := GetMCPBaseURL() + MCPOAuthTokenPath
	body := map[string]string{
		"clientId":  ClientID(),
		"authCode":  code,
		"grantType": "authorization_code",
	}
	resp, err := p.postJSON(ctx, url, body)
	if err != nil {
		return nil, err
	}
	return p.parseMCPTokenResponse(resp)
}

func (p *OAuthProvider) refreshWithRefreshToken(ctx context.Context, data *TokenData) (*TokenData, error) {
	// Use MCP mode if clientID is from MCP server
	if IsClientIDFromMCP() {
		return p.refreshViaMCP(ctx, data)
	}
	// Direct mode with client secret
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

// refreshViaMCP refreshes token via MCP proxy.
func (p *OAuthProvider) refreshViaMCP(ctx context.Context, data *TokenData) (*TokenData, error) {
	url := GetMCPBaseURL() + MCPRefreshTokenPath
	body := map[string]string{
		"clientId":     ClientID(),
		"refreshToken": data.RefreshToken,
		"grantType":    "refresh_token",
	}
	resp, err := p.postJSON(ctx, url, body)
	if err != nil {
		return nil, err
	}
	updated, err := p.parseMCPTokenResponse(resp)
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

// parseMCPTokenResponse parses token response from MCP proxy.
// MCP OAuth response format: {"accessToken": "...", "refreshToken": "...", "expiresIn": 7200, "corpId": "..."}
func (p *OAuthProvider) parseMCPTokenResponse(body []byte) (*TokenData, error) {
	var resp struct {
		AccessToken    string `json:"accessToken"`
		RefreshToken   string `json:"refreshToken"`
		PersistentCode string `json:"persistentCode"`
		ExpiresIn      int64  `json:"expiresIn"`
		CorpID         string `json:"corpId"`
		// Error fields (when request fails)
		ErrorCode string `json:"errorCode,omitempty"`
		ErrorMsg  string `json:"errorMsg,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing MCP token response: %w (body: %s)", err, string(body))
	}
	// Check for error response
	if resp.ErrorCode != "" || resp.ErrorMsg != "" {
		return nil, fmt.Errorf("MCP token exchange failed: %s - %s", resp.ErrorCode, resp.ErrorMsg)
	}
	if resp.AccessToken == "" {
		return nil, fmt.Errorf("MCP token response missing accessToken (body: %s)", string(body))
	}

	now := time.Now()
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
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

const notEnabledHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>钉钉 CLI</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;
  display:flex;justify-content:center;align-items:center;min-height:100vh;background:#fff;padding:20px}
.container{text-align:center;max-width:400px;width:100%}
.lock-icon{width:80px;height:100px;margin:0 auto 32px;position:relative}
.lock-body{width:60px;height:50px;background:linear-gradient(135deg,#1890ff 0%,#69c0ff 100%);
  border-radius:8px;position:absolute;bottom:0;left:50%;transform:translateX(-50%);
  box-shadow:0 8px 24px rgba(24,144,255,0.3)}
.lock-body::before{content:"";position:absolute;width:12px;height:12px;background:#fff;
  border-radius:50%;top:50%;left:50%;transform:translate(-50%,-50%)}
.lock-body::after{content:"";position:absolute;width:4px;height:12px;background:#fff;
  top:50%;left:50%;transform:translateX(-50%);margin-top:4px;border-radius:2px}
.lock-shackle{width:36px;height:36px;border:6px solid #1890ff;border-bottom:none;
  border-radius:20px 20px 0 0;position:absolute;top:8px;left:50%;transform:translateX(-50%)}
h1{font-size:22px;font-weight:600;color:#181c1f;margin-bottom:16px;line-height:1.4}
p{font-size:15px;color:#5f6368;line-height:1.6;margin-bottom:24px}
.form-group{text-align:left;margin-bottom:24px}
.form-label{font-size:14px;color:#181c1f;margin-bottom:8px;display:block}
.select-wrapper{position:relative}
select{width:100%;padding:12px 40px 12px 16px;font-size:15px;border:1px solid #d9d9d9;
  border-radius:8px;background:#fff;appearance:none;cursor:pointer;color:#181c1f}
select:focus{outline:none;border-color:#1890ff;box-shadow:0 0 0 2px rgba(24,144,255,0.1)}
.select-arrow{position:absolute;right:16px;top:50%;transform:translateY(-50%);pointer-events:none;
  border:5px solid transparent;border-top:6px solid #5f6368}
.btn{display:block;width:100%;padding:14px 24px;background:linear-gradient(135deg,#1890ff 0%,#40a9ff 100%);
  color:#fff;font-size:16px;font-weight:500;border-radius:24px;border:none;cursor:pointer;
  box-shadow:0 4px 16px rgba(24,144,255,0.3);transition:all 0.2s}
.btn:hover{transform:translateY(-1px);box-shadow:0 6px 20px rgba(24,144,255,0.4)}
.btn:disabled{background:#d9d9d9;box-shadow:none;cursor:not-allowed;transform:none}
.link{color:#1890ff;font-size:14px;text-decoration:none;margin-top:16px;display:inline-block}
.link:hover{text-decoration:underline}
.success-msg{display:none;width:100%;min-height:36px;gap:12px;padding:8px 12px;margin-top:20px;
  background:linear-gradient(0deg,rgba(0,102,255,0.12) 0%,rgba(0,102,255,0.12) 100%),linear-gradient(0deg,#FFFFFF 0%,#FFFFFF 100%);
  border-radius:12px;align-items:flex-start;justify-content:flex-start}
.success-msg-icon{width:16px;height:16px;flex-shrink:0;margin-top:3px}
.success-msg-text{flex:1;color:#181C1F;font-size:14px;line-height:22px}
.error-msg{color:#ff4d4f;font-size:14px;margin-top:8px;display:none}
.loading{display:inline-block;width:16px;height:16px;border:2px solid #fff;border-top-color:transparent;
  border-radius:50%;animation:spin 0.8s linear infinite;margin-right:8px;vertical-align:middle}
@keyframes spin{to{transform:rotate(360deg)}}
</style></head>
<body><div class="container">
<div class="lock-icon"><div class="lock-shackle"></div><div class="lock-body"></div></div>
<h1>该组织尚未开启 CLI 数据访问权限</h1>
<p>你所选择的组织管理员尚未开启「允许成员通过 CLI 访问其个人数据」的权限。</p>
<div class="form-group">
  <label class="form-label">你将使用以下账号授权登录</label>
  <div class="select-wrapper">
    <select id="adminSelect"><option value="">加载中...</option></select>
    <div class="select-arrow"></div>
  </div>
  <div id="errorMsg" class="error-msg"></div>
</div>
<button id="applyBtn" class="btn" disabled>立即申请</button>
<a id="backLink" class="link" href="#">返回选择其他组织</a>
<div id="successMsg" class="success-msg">
  <svg class="success-msg-icon" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M8 1.33333C4.32 1.33333 1.33333 4.32 1.33333 8C1.33333 11.68 4.32 14.6667 8 14.6667C11.68 14.6667 14.6667 11.68 14.6667 8C14.6667 4.32 11.68 1.33333 8 1.33333ZM8 13.3333C5.05333 13.3333 2.66667 10.9467 2.66667 8C2.66667 5.05333 5.05333 2.66667 8 2.66667C10.9467 2.66667 13.3333 5.05333 13.3333 8C13.3333 10.9467 10.9467 13.3333 8 13.3333ZM7.33333 9.33333H8.66667V10.6667H7.33333V9.33333ZM7.33333 5.33333H8.66667V8H7.33333V5.33333Z" fill="#0066FF"/>
  </svg>
  <span class="success-msg-text">已向管理员发送权限申请，正在等待审核<br/>审核通过后，请返回终端继续操作</span>
</div>
</div>
<script>
const select=document.getElementById('adminSelect'),btn=document.getElementById('applyBtn'),
  successMsg=document.getElementById('successMsg'),errorMsg=document.getElementById('errorMsg'),
  backLink=document.getElementById('backLink');
let admins=[],clientId='',applySent=false,selectedAdminId='',pollTimer=null;

// Load status first, then load admins
async function init(){
  try{
    const statusRes=await fetch('/api/status');
    const status=await statusRes.json();
    clientId=status.clientId||'';
    applySent=status.applySent||false;
    selectedAdminId=status.selectedAdminId||'';
    // Set back link URL
    if(clientId){
      const port=location.port;
      const redirectUri=encodeURIComponent('http://127.0.0.1:'+port+'/callback');
      backLink.href='https://login.dingtalk.com/oauth2/auth?client_id='+clientId+
        '&prompt=consent&redirect_uri='+redirectUri+'&response_type=code&scope=openid+corpid';
    }
    // Restore state if apply was sent
    if(applySent){
      setAppliedState();
    }
  }catch(e){console.error('Failed to load status',e);}
  loadAdmins();
}

function setAppliedState(){
  btn.style.display='none';
  select.disabled=true;
  successMsg.style.display='flex';
  backLink.style.pointerEvents='none';
  backLink.style.color='#999';
  backLink.onclick=function(e){e.preventDefault();return false;};
  // Start polling for auth status
  startPolling();
}

function startPolling(){
  if(pollTimer)return;
  pollTimer=setInterval(checkAuthStatus,5000);
  // Also check immediately
  checkAuthStatus();
}

async function checkAuthStatus(){
  try{
    const res=await fetch('/api/cliAuthEnabled');
    const data=await res.json();
    if(data.success&&data.result&&data.result.cliAuthEnabled){
      // Auth enabled! Redirect to success page
      stopPolling();
      location.href='/success';
    }
  }catch(e){console.error('Poll error',e);
}
}

function stopPolling(){
  if(pollTimer){clearInterval(pollTimer);pollTimer=null;}
}

async function loadAdmins(){
  try{
    const res=await fetch('/api/superAdmin');
    const data=await res.json();
    if(data.success&&data.result&&data.result.length>0){
      admins=data.result;
      select.innerHTML='<option value="">请选择</option>'+
        admins.map(a=>'<option value="'+a.staffId+'">'+a.name+'</option>').join('');
      // Restore selected admin if available
      if(selectedAdminId){
        select.value=selectedAdminId;
      }
      btn.disabled=!select.value;
    }else{
      select.innerHTML='<option value="">暂无可选管理员</option>';
      showError(data.errorMsg||'获取管理员列表失败');
    }
  }catch(e){
    select.innerHTML='<option value="">加载失败</option>';
    showError('网络错误，请重试');
  }
}
function showError(msg){errorMsg.textContent=msg;errorMsg.style.display='block';}
function hideError(){errorMsg.style.display='none';}
select.onchange=function(){btn.disabled=!this.value;hideError();};
btn.onclick=async function(){
  if(!select.value)return;
  btn.disabled=true;
  btn.innerHTML='<span class="loading"></span>申请中...';
  hideError();
  try{
    const res=await fetch('/api/sendApply?adminStaffId='+encodeURIComponent(select.value));
    const data=await res.json();
    if(data.success&&data.result){
      setAppliedState();
    }else{
      showError(data.errorMsg||'申请失败，请重试');
      btn.disabled=false;
      btn.textContent='立即申请';
    }
  }catch(e){
    showError('网络错误，请重试');
    btn.disabled=false;
    btn.textContent='立即申请';
  }
};
init();
</script></body></html>`

// CLIAuthStatus represents the response from /cli/cliAuthEnabled API.
type CLIAuthStatus struct {
	Success   bool   `json:"success"`
	ErrorCode string `json:"errorCode,omitempty"`
	ErrorMsg  string `json:"errorMsg,omitempty"`
	Result    struct {
		CLIAuthEnabled bool `json:"cliAuthEnabled"`
	} `json:"result"`
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

// CheckCLIAuthEnabled checks if CLI authorization is enabled for the current corp.
func (p *OAuthProvider) CheckCLIAuthEnabled(ctx context.Context, accessToken string) (*CLIAuthStatus, error) {
	url := GetMCPBaseURL() + CLIAuthEnabledPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-user-access-token", accessToken)

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
func GetSuperAdmins(ctx context.Context, accessToken string) (*SuperAdminResponse, error) {
	url := GetMCPBaseURL() + SuperAdminPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-user-access-token", accessToken)

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
func SendCliAuthApply(ctx context.Context, accessToken, adminStaffID string) (*SendApplyResponse, error) {
	url := GetMCPBaseURL() + SendCliAuthApplyPath + "?adminStaffId=" + adminStaffID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-user-access-token", accessToken)

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
func FetchClientIDFromMCP(ctx context.Context) (string, error) {
	url := GetMCPBaseURL() + ClientIDPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

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
