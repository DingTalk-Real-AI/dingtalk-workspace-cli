// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// PrepareExchange creates a transaction without invoking an SDK or touching
// local credentials. Login uses the same validation before handing off to an SDK.
func (p *OIDCLoginProvider) PrepareExchange(ctx context.Context) (*OIDCExchangePreparation, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var tx OIDCExchangePreparation
	status, err := p.post(ctx, "/cli/auth/oidc/transactions", map[string]string{"provider": p.provider}, &tx)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated {
		return nil, dwserrors.NewAuth(fmt.Sprintf("OIDC login transaction could not be created (HTTP %d); retry preparation", status))
	}
	if _, err := p.authorizationRequest(tx); err != nil {
		return nil, err
	}
	return &tx, nil
}

// ValidateExchange checks opaque inputs without modifying or decoding them.
func (p *OIDCLoginProvider) ValidateExchange(code, state string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if !utf8.ValidString(code) || !oidcValidCode(code) {
		return dwserrors.NewValidation("--code must be non-blank, at most 8192 bytes, and contain no control characters")
	}
	if !utf8.ValidString(state) || !oidcText(state, 1024) {
		return dwserrors.NewValidation("--state must be the original transaction state, at most 1024 bytes, without whitespace or control characters")
	}
	return nil
}

// Exchange submits a business-obtained code once. The provider asserts the
// transaction's identity source; it cannot select a different upstream endpoint.
func (p *OIDCLoginProvider) Exchange(ctx context.Context, code, state string) (*TokenData, error) {
	if err := p.ValidateExchange(code, state); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := p.exchange(ctx, code, state)
	var classified *dwserrors.Error
	if errors.As(err, &classified) && classified.Details["action"] != "stop" {
		// A failed request may already have consumed the transaction. Preserve
		// its classification, but never suggest retrying this same code/state.
		copy := *classified
		copy.Hint = "Run dws auth exchange-prepare again and obtain a new code; do not retry the previous code/state"
		return nil, &copy
	}
	return data, err
}

// ExchangeAndSave verifies the new token online and uses the existing external
// exchange writer to preserve the current default profile. No IdP session,
// authorization code or state enters local storage. Explicit DWS application
// secrets are saved atomically in the existing application keychain slot.
func (p *OIDCLoginProvider) ExchangeAndSave(ctx context.Context, configDir, code, state string, resolve ManagedIdentityResolver) (*TokenData, error) {
	if err := p.ValidateExchange(code, state); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if resolve == nil {
		return nil, dwserrors.NewInternal("request-scoped identity lookup unavailable")
	}
	if err := p.preparePersistence(configDir); err != nil {
		return nil, oidcSafeError(ctx, err, "local login state cannot be safely updated")
	}
	data, err := p.Exchange(ctx, code, state)
	if err != nil {
		return nil, err
	}
	corpID := strings.TrimSpace(data.CorpID)
	if corpID == "" {
		return nil, dwserrors.NewAuth("OIDC exchange returned no organization identity")
	}
	identity, err := resolve(ctx, data.AccessToken, corpID)
	if err != nil {
		const message = "OIDC token identity lookup failed; credentials were not saved"
		safe := oidcSafeError(ctx, err, message)
		var classified *dwserrors.Error
		if errors.As(safe, &classified) && classified.Category == dwserrors.CategoryAuth {
			// Unlike our portal parser, an injected identity resolver may return
			// typed errors with untrusted response data. Do not retain that cause.
			safe = dwserrors.NewAuth(message)
		}
		return nil, safe
	}
	if err := ctx.Err(); err != nil {
		return nil, oidcSafeError(ctx, err, "OIDC token identity lookup timed out")
	}
	userID := strings.TrimSpace(identity.UserID)
	if userID == "" || strings.TrimSpace(identity.CorpID) != corpID || (strings.TrimSpace(data.UserID) != "" && strings.TrimSpace(data.UserID) != userID) {
		return nil, dwserrors.NewAuth("OIDC token identity does not match the resolved current user; credentials were not saved")
	}
	data.CorpID, data.UserID, data.UserName = corpID, userID, strings.TrimSpace(identity.UserName)
	if name := strings.TrimSpace(identity.CorpName); name != "" {
		data.CorpName = name
	}
	data.OIDCProvider = p.provider
	if err := persistExternalExchangeTokenWithSecret(configDir, data, p.exchangeSecret()); err != nil {
		return nil, oidcSafeError(ctx, err, "cannot save OIDC exchange profile")
	}
	return data, nil
}
