// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

type appEventPushRequest struct {
	EventKey             string          `json:"eventKey"`
	EventID              string          `json:"eventId"`
	PayloadSchemaVersion string          `json:"payloadSchemaVersion"`
	UserIDs              []string        `json:"userIds"`
	Payload              json.RawMessage `json:"payload"`
	OccurredAt           string          `json:"occurredAt,omitempty"`
}

type appEventPushResult struct {
	EventID   string `json:"eventId"`
	Status    string `json:"status"`
	Duplicate bool   `json:"duplicate"`
}

func appEventPushURL(configDir string) (string, error) {
	switch personalEventMCPBaseURL(configDir) {
	case "https://pre-mcp.dingtalk.com":
		return "https://pre-api.dingtalk.com/v1.0/event/user/push", nil
	case "https://mcp.dingtalk.com":
		return "https://api.dingtalk.com/v1.0/event/user/push", nil
	default:
		return "", apperrors.NewValidation("事件上报只支持已知的正式或预发环境")
	}
}

// pushAppEvent performs one request. Both callers retain ownership of retries
// and of what an accepted receipt means for their respective workflows.
func pushAppEvent(ctx context.Context, endpoint, token string, payload appEventPushRequest) (appEventPushResult, error) {
	fail := func(message, reason string, options ...apperrors.Option) (appEventPushResult, error) {
		base := []apperrors.Option{
			apperrors.WithOperation("event.send"),
			apperrors.WithReason(reason),
			apperrors.WithDetails(map[string]any{"eventId": payload.EventID}),
			apperrors.WithHint("重发时沿用原 --event-id 和原请求内容"),
		}
		return appEventPushResult{}, apperrors.NewAPI(message, append(base, options...)...)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fail("事件上报请求体无效", "invalid_request", apperrors.WithCause(err), apperrors.WithExecutionStarted(false))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fail(err.Error(), "invalid_endpoint", apperrors.WithCause(err), apperrors.WithExecutionStarted(false))
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-acs-dingtalk-access-token", token)
	client := &http.Client{
		// The caller's context includes token acquisition and the HTTP request.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return fail("事件上报请求失败，接纳结果尚未确认", "admission_unconfirmed", apperrors.WithCause(err))
	}
	defer response.Body.Close()
	const maxReceiptBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxReceiptBytes+1))
	if err != nil || len(data) > maxReceiptBytes {
		return fail("事件上报未返回有效回执", "admission_unconfirmed", apperrors.WithCause(err))
	}
	type receipt struct {
		EventID   string `json:"eventId"`
		Status    string `json:"status"`
		Duplicate *bool  `json:"duplicate"`
	}
	var envelope struct {
		receipt
		Result    *receipt `json:"result"`
		Success   *bool    `json:"success"`
		Code      any      `json:"code"`
		ErrorCode any      `json:"errorCode"`
		ErrCode   any      `json:"errcode"`
		Message   string   `json:"message"`
		ErrorMsg  string   `json:"errorMsg"`
		ErrMsg    string   `json:"errmsg"`
		RequestID string   `json:"requestId"`
		Retryable *bool    `json:"retryable"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decodeErr := decoder.Decode(&envelope)
	// A second JSON value or trailing garbage is never a valid receipt.
	var trailing any
	if decodeErr == nil && decoder.Decode(&trailing) != io.EOF {
		decodeErr = fmt.Errorf("unexpected data after receipt")
	}
	code := ""
	for _, value := range []any{envelope.Code, envelope.ErrorCode, envelope.ErrCode} {
		if candidate := appEventPushErrorCode(value); candidate != "" {
			code = candidate
			break
		}
	}
	if response.StatusCode != http.StatusOK || code != "" || (envelope.Success != nil && !*envelope.Success) {
		message := fmt.Sprintf("事件上报失败 (HTTP %d)", response.StatusCode)
		for _, detail := range []string{envelope.Message, envelope.ErrorMsg, envelope.ErrMsg} {
			if detail != "" {
				message += ": " + strings.ReplaceAll(detail, token, "[REDACTED]")
				break
			}
		}
		return fail(message, "upstream_error", apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			ServerErrorCode: code, TraceID: envelope.RequestID, ServerRetryable: envelope.Retryable,
		}))
	}
	if decodeErr != nil {
		return fail("事件上报未返回有效回执", "admission_unconfirmed")
	}
	accepted := envelope.receipt
	if envelope.Result != nil {
		accepted = *envelope.Result
	}
	if accepted.EventID != payload.EventID || (accepted.Status != "ACCEPTED" && accepted.Status != "DUPLICATE") {
		return fail("上报入口未确认接纳本次事件", "admission_unconfirmed")
	}
	duplicate := accepted.Status == "DUPLICATE"
	if accepted.Duplicate != nil && *accepted.Duplicate != duplicate {
		return fail("事件上报回执的状态与重复标记不一致", "admission_unconfirmed")
	}
	return appEventPushResult{EventID: accepted.EventID, Status: accepted.Status, Duplicate: duplicate}, nil
}

func appEventPushErrorCode(value any) string {
	var code string
	switch value := value.(type) {
	case string:
		code = strings.TrimSpace(value)
	case json.Number:
		code = value.String()
	}
	switch strings.ToLower(code) {
	case "", "0", "200", "ok", "success":
		return ""
	default:
		return code
	}
}
