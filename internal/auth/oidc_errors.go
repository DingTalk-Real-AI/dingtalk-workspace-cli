// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	dwserrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Mirrors the public codes in Portal's OidcError. Internal reason/exception
// details stay in server logs; the trace ID lets the administrator locate them.
func oidcErrorMessage(code string) string {
	switch code {
	case "invalid_request":
		return "OIDC login request parameters are invalid"
	case "invalid_state":
		return "OIDC login state is invalid, expired, or already consumed"
	case "unsupported_provider":
		return "OIDC login provider is not supported or registered"
	case "provider_mismatch":
		return "OIDC login provider does not match the transaction"
	case "oidc_code_invalid":
		return "OIDC authorization code is invalid, expired, or already consumed"
	case "oidc_identity_invalid":
		return "OIDC identity verification failed"
	case "rate_limited":
		return "OIDC login rate limit exceeded"
	case "upstream_exchange_uncertain":
		return "OIDC upstream exchange result is uncertain"
	case "session_issue_failed":
		return "DWS session could not be issued after OIDC authentication"
	case "session_result_invalid":
		return "DWS session issuance returned an invalid result"
	case "temporarily_unavailable":
		return "OIDC login service is temporarily unavailable"
	case "provider_unavailable":
		return "OIDC login provider is unavailable"
	case "oidc_configuration_invalid":
		return "OIDC provider configuration is invalid"
	case "internal_error":
		return "OIDC login service encountered an internal error"
	default:
		return "OIDC portal rejected the login request"
	}
}

func (p *OIDCLoginProvider) portalError(path string, status int, content []byte, headerTrace string, request any) error {
	var failure struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Action  string `json:"action"`
		} `json:"error"`
		TraceID string `json:"trace_id"`
	}
	diag := dwserrors.ServerDiagnostics{TraceID: headerTrace}
	action := ""
	if json.Unmarshal(content, &failure) == nil {
		diag.ServerErrorCode = oidcDiagnosticID(failure.Error.Code, request)
		if trace := oidcDiagnosticID(failure.TraceID, request); trace != "" {
			diag.TraceID = trace
		}
		// Only these fixed public messages are emitted by CliOidcController.
		// Do not print arbitrary response text that might quote a credential.
		switch failure.Error.Message {
		case "OIDC login service is unavailable", "OIDC login request was not completed":
			diag.TechnicalDetail = failure.Error.Message
		}
		if failure.Error.Action == "stop" || failure.Error.Action == "restart_login" {
			action = failure.Error.Action
		}
	}
	message := oidcErrorMessage(diag.ServerErrorCode)
	hint := fmt.Sprintf("Run dws auth provider-login %s to restart login; do not retry the previous code/state", p.provider)
	if path == "/cli/auth/oidc/transactions" {
		hint = fmt.Sprintf("Run dws auth exchange-prepare --provider %s again or restart provider login; do not retry the previous code/state", p.provider)
	}
	details := map[string]any{"http_status": status, "provider": p.provider}
	opts := []dwserrors.Option{dwserrors.WithServerDiag(diag), dwserrors.WithRetryable(false)}
	if action != "" {
		details["action"] = action
	}
	if action == "stop" {
		hint = "Check the request and provider/application configuration, or contact the DWS administrator with the Trace ID; do not retry this request"
		if path == "/cli/auth/oidc/transactions" && (diag.ServerErrorCode == "unsupported_provider" || diag.ServerErrorCode == "temporarily_unavailable" || diag.ServerErrorCode == "provider_unavailable") {
			message = fmt.Sprintf("OIDC provider %q is not configured, disabled, or unavailable; contact the DWS administrator", p.provider)
			opts = append(opts, dwserrors.WithReason("OIDC_PROVIDER_UNAVAILABLE"))
		}
	}
	opts = append(opts, dwserrors.WithHint(hint), dwserrors.WithDetails(details))
	return dwserrors.NewAuth(fmt.Sprintf("%s (HTTP %d)", message, status), opts...)
}

// Diagnostic identifiers may be printed in terminals and JSON. Accept bounded
// identifiers, never terminal control sequences, URLs, or arbitrary prose.
func oidcDiagnosticID(value string, request any) string {
	if len(value) == 0 || len(value) > 256 {
		return ""
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':') {
			return ""
		}
	}
	if fields, ok := request.(map[string]string); ok {
		for _, key := range []string{"code", "state", "client_secret", "refreshToken", "clientSecret"} {
			secret := fields[key]
			if secret != "" && (value == secret || len(secret) >= 8 && strings.Contains(value, secret)) {
				return ""
			}
		}
	}
	return value
}

func oidcErrorWithTrace(err error, trace string) error {
	var typed *dwserrors.Error
	if trace == "" || !errors.As(err, &typed) {
		return err
	}
	copy := *typed
	copy.ServerDiag.TraceID = trace
	return &copy
}
