// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// OIDCAuthorizationRequest is the complete server-issued SDK input. The portal
// owns the client registration, redirect binding, state, nonce and PKCE verifier.
type OIDCAuthorizationRequest struct {
	AuthorizationEndpoint string            `json:"authorization_endpoint"`
	Parameters            map[string]string `json:"parameters"`
}

type OIDCAuthorizationResult struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// OIDCAuthorizer supplies the provider-specific login and authorization step.
// The common flow calls it only after the backend accepts the provider, then
// validates its code/state before exchange. Implementations must honor context.
type OIDCAuthorizer interface {
	Authorize(context.Context, OIDCAuthorizationRequest) (OIDCAuthorizationResult, error)
}

var errOIDCAuthorizerUnavailable = errors.New("provider authorization is unavailable")

// OIDCExchangeCredentials selects the DWS application for this exchange only.
// It is separate from the server-owned OIDC client registration.
type OIDCExchangeCredentials struct {
	ClientID     string
	ClientSecret string
}

func (c *OIDCExchangeCredentials) Validate() error {
	if c != nil && (strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "") {
		return dwserrors.NewValidation("--client-id and --client-secret must be provided together and must not be blank")
	}
	return nil
}

// OIDCLoginProvider owns the shared transaction, exchange and persistence flow.
// Business providers compose it and supply an OIDCAuthorizer.
type OIDCLoginProvider struct {
	provider            string
	baseURL             string
	client              *http.Client
	authorizer          OIDCAuthorizer
	ExchangeCredentials *OIDCExchangeCredentials
	// IdentityEnricher is the same pre-save identity hook used by OAuth login.
	IdentityEnricher func(context.Context, *TokenData) error
}

func NewOIDCLoginProvider(provider, baseURL string, authorizer OIDCAuthorizer) *OIDCLoginProvider {
	return &OIDCLoginProvider{provider: provider, baseURL: strings.TrimRight(baseURL, "/"), client: oauthHTTPClient, authorizer: authorizer}
}

func (p *OIDCLoginProvider) Validate() error {
	if err := p.ExchangeCredentials.Validate(); err != nil {
		return err
	}
	if !validLoginProviderName(p.provider) {
		return dwserrors.NewValidation("login provider must match [a-z][a-z0-9_-]{0,63}")
	}
	_, err := oidcBaseURL(p.baseURL)
	return err
}

func validLoginProviderName(name string) bool {
	if len(name) == 0 || len(name) > 64 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		b := name[i]
		if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '_' && b != '-' {
			return false
		}
	}
	return true
}

// LoginAndSave uses the normal OAuth identity enrichment and token/profile writer.
// The SDK session never enters credential storage.
func (p *OIDCLoginProvider) LoginAndSave(ctx context.Context, configDir string) (*TokenData, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := prepareLoginPersistence(configDir); err != nil {
		return nil, oidcSafeError(ctx, err, "local login state cannot be safely updated")
	}
	data, err := p.Login(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	oauth := &OAuthProvider{configDir: configDir, IdentityEnricher: p.IdentityEnricher}
	if err := oauth.persistLoginToken(ctx, data); err != nil {
		return nil, oidcSafeError(ctx, err, "cannot save OIDC login profile")
	}
	return data, nil
}

func (p *OIDCLoginProvider) Login(ctx context.Context) (*TokenData, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 12*time.Minute)
		defer cancel()
	}
	started := time.Now()
	var transaction oidcTransactionResponse
	status, err := p.post(ctx, "/cli/auth/oidc/transactions", map[string]string{"provider": p.provider}, &transaction)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated {
		return nil, dwserrors.NewAuth(fmt.Sprintf("OIDC login transaction could not be created (HTTP %d); retry login", status))
	}
	request, err := p.authorizationRequest(transaction)
	if err != nil {
		return nil, err
	}
	if p.authorizer == nil {
		return nil, p.sdkUnavailable()
	}

	sdkCtx, cancelSDK := context.WithDeadline(ctx, started.Add(time.Duration(transaction.ExpiresIn)*time.Second))
	defer cancelSDK()
	// Snapshot state before handing the request map to an SDK implementation.
	state := request.Parameters["state"]
	result, err := p.authorizer.Authorize(sdkCtx, request)
	if err := sdkCtx.Err(); err != nil {
		return nil, oidcSafeError(sdkCtx, err, "provider SDK login timed out")
	}
	if err != nil {
		if errors.Is(err, errOIDCAuthorizerUnavailable) {
			return nil, p.sdkUnavailable()
		}
		var typed *dwserrors.Error
		if errors.As(err, &typed) && typed.Reason == "NETWORK_UNREACHABLE" {
			return nil, dwserrors.NewAPI("provider SDK connection failed", dwserrors.WithReason("NETWORK_UNREACHABLE"))
		}
		slog.Debug("auth.oidc.sdk_authorize_failed", "provider", p.provider, "error", err)
		return nil, oidcSafeError(sdkCtx, err, "provider SDK login failed")
	}
	if !oidcValidCode(result.Code) || subtle.ConstantTimeCompare([]byte(result.State), []byte(state)) != 1 {
		return nil, dwserrors.NewAuth("provider SDK returned an invalid authorization result or mismatched state")
	}
	return p.exchange(ctx, result.Code, result.State)
}

func (p *OIDCLoginProvider) sdkUnavailable() error {
	return dwserrors.NewAuth(fmt.Sprintf("provider %q SDK is not integrated in this CLI build; contact the DWS administrator", p.provider), dwserrors.WithReason("OIDC_SDK_UNAVAILABLE"))
}

func oidcValidCode(code string) bool {
	if strings.TrimSpace(code) == "" || len(code) > 8192 {
		return false
	}
	for _, c := range code {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

type oidcTransactionResponse struct {
	Provider             string                   `json:"provider"`
	ExpiresIn            int64                    `json:"expires_in"`
	AuthorizationRequest OIDCAuthorizationRequest `json:"authorization_request"`
}

func oidcBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, dwserrors.NewValidation("invalid OIDC portal URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return nil, dwserrors.NewValidation("OIDC portal requires HTTPS, except a loopback HTTP development endpoint")
	}
	return u, nil
}

func (p *OIDCLoginProvider) authorizationRequest(tx oidcTransactionResponse) (OIDCAuthorizationRequest, error) {
	fail := func() (OIDCAuthorizationRequest, error) {
		return OIDCAuthorizationRequest{}, dwserrors.NewAuth("OIDC portal returned invalid authorization information")
	}
	if tx.Provider != p.provider || tx.ExpiresIn <= 0 || tx.ExpiresIn > 600 {
		return fail()
	}
	params := tx.AuthorizationRequest.Parameters
	if len(params) != 8 || !oidcText(params["redirect_uri"], 2048) || len(params["scope"]) > 2048 || params["response_type"] != "code" || params["code_challenge_method"] != "S256" || !oidcText(params["client_id"], 512) || !oidcRandomValue(params["state"]) || !oidcRandomValue(params["nonce"]) || !oidcRandomValue(params["code_challenge"]) {
		return fail()
	}
	redirect, err := url.Parse(params["redirect_uri"])
	if err != nil || !redirect.IsAbs() || redirect.Hostname() == "" || redirect.User != nil || redirect.Fragment != "" {
		return fail()
	}
	openid := false
	for _, scope := range strings.Fields(params["scope"]) {
		openid = openid || scope == "openid"
	}
	if !openid {
		return fail()
	}
	u, err := url.Parse(tx.AuthorizationRequest.AuthorizationEndpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return fail()
	}
	base, err := oidcBaseURL(p.baseURL)
	if err != nil {
		return fail()
	}
	if u.Scheme != "https" && !(base.Scheme == "http" && u.Scheme == base.Scheme && u.Host == base.Host) {
		return fail()
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return fail()
	}
	for _, sensitive := range []string{"code", "auth_code", "access_token", "refresh_token", "id_token", "client_secret", "code_verifier"} {
		if _, exists := q[sensitive]; exists {
			return fail()
		}
	}
	for key := range params {
		if _, exists := q[key]; exists {
			return fail()
		}
	}
	return tx.AuthorizationRequest, nil
}

func (p *OIDCLoginProvider) exchange(ctx context.Context, code, state string) (*TokenData, error) {
	var response json.RawMessage
	body := map[string]string{"code": code, "state": state}
	if p.ExchangeCredentials != nil {
		body["client_id"] = p.ExchangeCredentials.ClientID
		body["client_secret"] = p.ExchangeCredentials.ClientSecret
	}
	status, err := p.post(ctx, "/cli/auth/oidc/exchange", body, &response)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, dwserrors.NewAuth(fmt.Sprintf("OIDC exchange did not complete (HTTP %d); restart login", status))
	}
	data, err := (&OAuthProvider{}).parseMCPTokenResponse(response)
	if err != nil {
		return nil, oidcSafeError(ctx, err, "OIDC exchange returned an invalid token response")
	}
	data.Source = "mcp"
	data.FreshAuthorization = true
	return data, nil
}

// Each portal request is sent once; failures are returned to the caller.
func (p *OIDCLoginProvider) post(ctx context.Context, path string, body any, dst any) (int, error) {
	status, content, err := oidcJSONRequest(ctx, p.client, p.baseURL+path, body, "")
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		var failure struct {
			Error struct {
				Code   string `json:"code"`
				Action string `json:"action"`
			} `json:"error"`
		}
		if json.Unmarshal(content, &failure) == nil {
			if failure.Error.Action == "stop" && path == "/cli/auth/oidc/transactions" && (failure.Error.Code == "unsupported_provider" || failure.Error.Code == "temporarily_unavailable" || failure.Error.Code == "provider_unavailable") {
				return status, dwserrors.NewAuth(fmt.Sprintf("OIDC provider %q is not configured, disabled, or unavailable; contact the DWS administrator", p.provider), dwserrors.WithReason("OIDC_PROVIDER_UNAVAILABLE"))
			}
		}
		return status, dwserrors.NewAuth(fmt.Sprintf("OIDC portal rejected the login request (HTTP %d); restart login", status))
	}
	if json.Unmarshal(content, dst) != nil {
		return 0, dwserrors.NewAuth("OIDC portal returned an invalid response")
	}
	return status, nil
}

// Shared bounded JSON transport for the portal and provider SDK. Redirects
// are forbidden so a response cannot forward a code or SDK session elsewhere.
func oidcJSONRequest(ctx context.Context, configured *http.Client, endpoint string, body any, bearer string) (int, []byte, error) {
	started := time.Now()
	logEndpoint := oidcLogEndpoint(endpoint)
	if err := ctx.Err(); err != nil {
		slog.Debug("auth.oidc.request_canceled", "endpoint", logEndpoint, "error", err)
		return 0, nil, oidcSafeError(ctx, err, "OIDC request timed out")
	}
	slog.Debug("auth.oidc.request", "endpoint", logEndpoint)
	data, err := json.Marshal(body)
	if err != nil {
		return 0, nil, dwserrors.NewInternal("cannot construct OIDC request")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return 0, nil, dwserrors.NewValidation("invalid OIDC request endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := http.Client{Timeout: 15 * time.Second}
	if configured != nil {
		client = *configured
		if client.Timeout <= 0 || client.Timeout > 15*time.Second {
			client.Timeout = 15 * time.Second
		}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("auth.oidc.request_failed", "endpoint", logEndpoint, "error", err, "elapsed_ms", time.Since(started).Milliseconds())
		return 0, nil, oidcNetworkError(ctx, err, "OIDC portal connection failed")
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil {
		slog.Debug("auth.oidc.read_failed", "endpoint", logEndpoint, "status", resp.StatusCode, "error", err, "elapsed_ms", time.Since(started).Milliseconds())
		return 0, nil, oidcNetworkError(ctx, err, "cannot read OIDC portal response")
	}
	slog.Debug("auth.oidc.response", "endpoint", logEndpoint, "status", resp.StatusCode, "elapsed_ms", time.Since(started).Milliseconds())
	if len(content) > 64<<10 || !oidcUniqueJSON(content) {
		return 0, nil, dwserrors.NewAuth("OIDC portal returned an invalid response")
	}
	return resp.StatusCode, content, nil
}

func oidcSafeError(ctx context.Context, err error, message string) error {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var network net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
		return dwserrors.NewAPI("OIDC login request timed out", dwserrors.WithReason("NETWORK_TIMEOUT"), dwserrors.WithCause(context.DeadlineExceeded), dwserrors.WithHint("Check the connection and retry login"))
	}
	// Preserve the cause only for errors we authored: their text is part of the
	// reviewed surface and never embeds a portal response body (which may carry
	// tokens). Foreign fmt.Errorf chains stay out of the wire output.
	opts := []dwserrors.Option{}
	var typed *dwserrors.Error
	if errors.As(err, &typed) {
		opts = append(opts, dwserrors.WithCause(err))
	}
	return dwserrors.NewAuth(message, opts...)
}

func oidcNetworkError(ctx context.Context, err error, message string) error {
	safe := oidcSafeError(ctx, err, message)
	var typed *dwserrors.Error
	if errors.Is(safe, context.Canceled) || errors.As(safe, &typed) && typed.Category == dwserrors.CategoryAPI {
		return safe
	}
	return dwserrors.NewAPI(message, dwserrors.WithReason("NETWORK_UNREACHABLE"), dwserrors.WithCause(err), dwserrors.WithHint("Check the connection and retry login"))
}

func oidcText(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, c := range value {
		if c < 0x21 || c == 0x7f {
			return false
		}
	}
	return true
}
func oidcRandomValue(value string) bool {
	if len(value) != 43 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// oidcLogEndpoint strips query and fragment before diagnostics so logs never
// persist authorization parameters carried by an endpoint URL.
func oidcLogEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String()
}

// Reject duplicate members before typed decoding; the standard JSON decoder
// otherwise silently picks one value for security-critical state and tokens.
func oidcUniqueJSON(content []byte) bool {
	d := json.NewDecoder(bytes.NewReader(content))
	d.UseNumber()
	var value func() bool
	value = func() bool {
		token, err := d.Token()
		if err != nil {
			return false
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return false
				}
				seen[name] = true
				if !value() {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for d.More() {
				if !value() {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		default:
			return false
		}
	}
	if !value() {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
