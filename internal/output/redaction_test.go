// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package output

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type redactionFixture struct {
	Password  string         `json:"password"`
	ClientID  string         `json:"client_id"`
	RequestID string         `json:"request_id"`
	Nested    map[string]any `json:"nested"`
}

func TestWriteEnvelopePreservesDeclaredBusinessDataAndRedactsFrameworkChannels(t *testing.T) {
	data := &redactionFixture{
		Password:  "data-password-canary",
		ClientID:  "client-id-canary",
		RequestID: "request-id-canary",
		Nested: map[string]any{
			"ToKeN": "nested-token-canary",
			"items": []any{
				map[string]string{"access_token": "slice-token-canary"},
				"https://user:url-password-canary@example.test/path?x=usable&x-oss-signature=url-signature-canary",
			},
		},
	}
	notice := map[string]any{"cookie": "notice-cookie-canary", "message": "token=notice-text-canary"}
	env := &Envelope{
		OK:      true,
		Outcome: OutcomeSuccess,
		Data:    data,
		Meta: &Meta{Operation: &OperationInfo{
			ID:          "task-id-canary",
			State:       OperationStateProcessing,
			NextCommand: "dws op get 'https://example.test/poll?token=next-command-canary&safe=yes'",
		}},
		Notice: notice,
	}
	originalData := cloneResultData(data)
	originalNotice := cloneResultData(notice)

	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, env, FormatJSON, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo: %v", err)
	}
	output := buf.String()
	for _, secret := range []string{
		"notice-cookie-canary", "notice-text-canary", "next-command-canary",
	} {
		if strings.Contains(output, secret) {
			t.Errorf("output leaked %q: %s", secret, output)
		}
	}
	for _, retained := range []string{
		"data-password-canary", "nested-token-canary", "slice-token-canary",
		"url-password-canary", "url-signature-canary", "client-id-canary",
		"request-id-canary", "task-id-canary", "x=usable", "safe=yes",
	} {
		if !strings.Contains(output, retained) {
			t.Errorf("output over-redacted %q: %s", retained, output)
		}
	}
	if got := strings.Count(output, redactedValue); got < 3 {
		t.Errorf("redaction placeholder count = %d, want at least 3: %s", got, output)
	}
	if !reflect.DeepEqual(data, originalData) || !reflect.DeepEqual(notice, originalNotice) {
		t.Fatalf("redaction mutated caller values\ndata: %#v\nnotice: %#v", data, notice)
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("caller envelope became invalid: %v", err)
	}
}

func TestEmitResultRedactsEveryErrorInfoCanary(t *testing.T) {
	info := &ErrorInfo{
		Type:            "api",
		Message:         "Authorization: Bearer message-auth-canary",
		Hint:            "use https://hint-user:hint-pass-canary@example.test/help?access_token=hint-query-canary&doc=usable",
		Details:         map[string]any{"PASSWORD": "details-password-canary"},
		RPCData:         map[string]any{"client_secret": "rpc-secret-canary", "task_id": "rpc-task-id-canary"},
		TechnicalDetail: `upstream {"refresh_token":"technical-token-canary"}`,
		FriendlyHint:    "cookie=friendly-cookie-canary",
		ActionURL:       "https://example.test/retry?signature=action-signature-canary&step=2",
		Cause:           "token:cause-token-canary",
		Actions:         []string{"dws retry --url 'https://example.test/run?x-oss-signature=actions-signature-canary&mode=safe'"},
		RequestID:       "error-request-id-canary",
	}
	original := cloneErrorInfo(info)
	result := Failure(info)
	cmd := &cobra.Command{Use: "test"}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	code, err := EmitResult(cmd, result)
	if err != nil {
		t.Fatalf("EmitResult: %v", err)
	}
	if code != exitCodeAPI {
		t.Fatalf("exit code = %d, want %d", code, exitCodeAPI)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	output := stdout.String()
	for _, secret := range []string{
		"message-auth-canary", "hint-pass-canary", "hint-query-canary",
		"details-password-canary", "rpc-secret-canary", "technical-token-canary",
		"friendly-cookie-canary", "action-signature-canary", "cause-token-canary",
		"actions-signature-canary",
	} {
		if strings.Contains(output, secret) {
			t.Errorf("output leaked %q: %s", secret, output)
		}
	}
	for _, retained := range []string{"error-request-id-canary", "rpc-task-id-canary", "doc=usable", "step=2", "mode=safe"} {
		if !strings.Contains(output, retained) {
			t.Errorf("output over-redacted %q: %s", retained, output)
		}
	}
	var decoded Envelope
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not a typed envelope: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("redacted envelope is invalid: %v", err)
	}
	if !reflect.DeepEqual(info, original) {
		t.Fatalf("EmitResult mutated ErrorInfo\ngot:  %#v\nwant: %#v", info, original)
	}
}

func TestEmitResultHumanFailureRedactsBeforeShortcutRendering(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("format", "table", "")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	_, err := EmitResult(cmd, Failure(&ErrorInfo{Type: "api", Message: "password=human-failure-canary"}))
	if err != nil {
		t.Fatalf("EmitResult: %v", err)
	}
	if strings.Contains(stderr.String(), "human-failure-canary") || !strings.Contains(stderr.String(), redactedValue) {
		t.Fatalf("human failure was not redacted: %q", stderr.String())
	}
}

func TestAdaptMCPPreservesBusinessData(t *testing.T) {
	data := map[string]any{
		"authorization": "mcp-auth-canary",
		"client_id":     "mcp-client-id-canary",
	}
	result := Success(data)
	mcpResult, err := AdaptMCP(result)
	if err != nil {
		t.Fatalf("AdaptMCP: %v", err)
	}
	raw, err := json.Marshal(mcpResult)
	if err != nil {
		t.Fatalf("marshal MCPResult: %v", err)
	}
	if !strings.Contains(string(raw), "mcp-auth-canary") {
		t.Fatalf("MCP business data was unexpectedly redacted: %s", raw)
	}
	if !strings.Contains(string(raw), "mcp-client-id-canary") {
		t.Fatalf("MCP output over-redacted client_id: %s", raw)
	}
	if data["authorization"] != "mcp-auth-canary" {
		t.Fatalf("AdaptMCP mutated caller data: %#v", data)
	}
}
