// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"maps"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// resolveOIDCAuthorizer snapshots an edition registration for this invocation.
// A missing SDK remains nil so the common flow can report backend configuration
// errors first, then OIDC_SDK_UNAVAILABLE if the backend accepts the provider.
func resolveOIDCAuthorizer(name string) (authpkg.OIDCAuthorizer, error) {
	register := edition.Get().OIDCProviders
	if register == nil {
		return nil, nil
	}
	seen := make(map[string]bool)
	var selected authpkg.OIDCAuthorizer
	for _, provider := range register() {
		if !authpkg.ValidLoginProviderName(provider.Name) || provider.Authorize == nil {
			return nil, apperrors.NewInternal("invalid edition OIDC provider registration: a valid name and authorizer are required")
		}
		if seen[provider.Name] {
			return nil, apperrors.NewInternal("duplicate edition OIDC provider registration")
		}
		seen[provider.Name] = true
		if provider.Name == name {
			selected = editionOIDCAuthorizer{provider: provider}
		}
	}
	return selected, nil
}

// Keep the public extension DTOs independent of internal/auth. The SDK gets its
// own parameter map and cannot mutate the core's validated transaction request.
type editionOIDCAuthorizer struct {
	provider edition.OIDCProvider
}

func (a editionOIDCAuthorizer) Authorize(ctx context.Context, request authpkg.OIDCAuthorizationRequest) (authpkg.OIDCAuthorizationResult, error) {
	result, err := a.provider.Authorize(ctx, edition.OIDCAuthorizationRequest{
		AuthorizationEndpoint: request.AuthorizationEndpoint,
		Parameters:            maps.Clone(request.Parameters),
	})
	if err != nil {
		return authpkg.OIDCAuthorizationResult{}, err
	}
	return authpkg.OIDCAuthorizationResult{Code: result.Code, State: result.State}, nil
}
