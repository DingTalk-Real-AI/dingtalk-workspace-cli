// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// AntSDKOIDCProvider reuses the common OIDC flow and implements Ant authorization.
// The current SDK implementation uses the selected portal backend Mock. A real Ant SDK
// belongs in Authorize; transaction, exchange and persistence remain shared.
type AntSDKOIDCProvider struct {
	*OIDCLoginProvider
}

var _ OIDCAuthorizer = (*AntSDKOIDCProvider)(nil)

func NewAntSDKOIDCProvider(baseURL string) *AntSDKOIDCProvider {
	provider := &AntSDKOIDCProvider{}
	provider.OIDCLoginProvider = NewOIDCLoginProvider("ant", baseURL, provider)
	return provider
}

func (s *AntSDKOIDCProvider) Authorize(ctx context.Context, request OIDCAuthorizationRequest) (OIDCAuthorizationResult, error) {
	var result OIDCAuthorizationResult
	_, err := oidcBaseURL(s.baseURL)
	if err != nil || request.AuthorizationEndpoint != s.baseURL+"/cli/auth/oidc/mock/sdk/authorize" {
		return result, errOIDCAuthorizerUnavailable
	}
	status, body, err := oidcJSONRequest(ctx, s.client, s.baseURL+"/cli/auth/oidc/mock/sdk/login", map[string]string{}, "")
	if err != nil {
		return result, err
	}
	var session struct {
		Token     string `json:"sdk_session_token"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &session) != nil || !oidcText(session.Token, 16384) || session.ExpiresIn <= 0 || session.ExpiresIn > 60 {
		return result, dwserrors.NewAuth(fmt.Sprintf("Ant SDK login returned an invalid session (HTTP %d)", status))
	}
	authorizeCtx, cancel := context.WithTimeout(ctx, time.Duration(session.ExpiresIn)*time.Second)
	defer cancel()
	status, body, err = oidcJSONRequest(authorizeCtx, s.client, request.AuthorizationEndpoint, request, session.Token)
	if err != nil {
		return result, err
	}
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil {
		return OIDCAuthorizationResult{}, dwserrors.NewAuth(fmt.Sprintf("Ant SDK authorization failed (HTTP %d)", status))
	}
	return result, nil
}
