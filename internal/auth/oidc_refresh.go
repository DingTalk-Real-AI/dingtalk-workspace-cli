// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"errors"
	"fmt"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func (data *TokenData) isOIDCSession() bool {
	return data != nil && data.OIDCProvider != ""
}

func validateOIDCRefreshBinding(data *TokenData) error {
	if data.Source != "mcp" || !oidcText(data.ClientID, 512) ||
		!ValidLoginProviderName(data.OIDCProvider) {
		return dwserrors.NewAuth("saved DWS application binding is invalid; log in again")
	}
	if _, err := oidcBaseURL(data.MCPBaseURL); err != nil {
		return dwserrors.NewAuth("saved DWS Portal binding is missing or invalid; log in again")
	}
	return nil
}

func oidcRefreshReauthorizationError(data *TokenData, cause error) error {
	provider := "<provider>"
	if ValidLoginProviderName(data.OIDCProvider) {
		provider = data.OIDCProvider
	}
	message := fmt.Sprintf("DWS login credentials could not be refreshed; run dws auth provider-login %s again or repeat the original exchange-prepare/token-exchange --save flow", provider)
	if data.MCPClientSecretRequired {
		message += "; use the original --client-id/--client-secret pair"
	}
	var typed *dwserrors.Error
	if errors.As(cause, &typed) {
		copy := *typed
		copy.Hint = message
		return &copy
	}
	return dwserrors.NewAuth("DWS login credentials could not be refreshed",
		dwserrors.WithHint(message), dwserrors.WithCause(cause))
}
