// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func aicardUnicodeTestCard(t *testing.T, model string) string {
	t.Helper()
	card := strings.Replace(aicardTestSnapshot, `"dataModel":{}`, `"dataModel":`+model, 1)
	if card == aicardTestSnapshot {
		t.Fatal("fixture initialization was not replaced")
	}
	return card
}

func TestCrossPlatformCoverageAicardTemplateUnicodeBoundary(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	// These escaped inputs intentionally exercise UTF-16 interoperability.
	for _, tc := range []struct {
		name  string
		model string
		valid bool
	}{
		{"plain", `{"value":"Report"}`, true},
		{"high", `{"value":"\ud800"}`, false},
		{"low", `{"value":"\udfff"}`, false},
		{"high-key", `{"\ud800":"value"}`, false},
		{"low-key", `{"\udfff":"value"}`, false},
		{"high-then-text", `{"value":"\ud800x"}`, false},
		{"high-then-ordinary-escape", `{"value":"\ud800\u0041"}`, false},
		{"high-high", `{"value":"\ud800\ud800"}`, false},
		{"low-high", `{"value":"\udc00\ud800"}`, false},
		{"pair-then-high", `{"value":"\ud800\udc00\ud800"}`, false},
		{"minimum-pair", `{"value":"\ud800\udc00"}`, true},
		{"maximum-pair", `{"value":"\udbff\udfff"}`, true},
		{"uppercase-pair", `{"value":"\uD83D\uDE00"}`, true},
		{"literal-backslash", `{"value":"\\ud800"}`, true},
		{"backslash-then-high", `{"value":"\\\ud800"}`, false},
		{"escaped-quote", `{"value":"\"\\ud800\""}`, true},
		{"replacement", `{"value":"\ufffd"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card := aicardUnicodeTestCard(t, tc.model)
			digest, err := validateAicardTemplate([]byte(card))
			if (err == nil) != tc.valid || tc.valid && digest == "" {
				t.Fatalf("valid=%t digest=%q err=%v", tc.valid, digest, err)
			}
			for _, operation := range []string{"create", "update"} {
				caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "templateId": "t", "version": "1"}}}
				args := []string{operation, "--file", aicardTestFile(t, card)}
				if operation == "update" {
					args = append(args, "--template-id", "t")
				}
				_, err := executeAicardTemplate(t, caller, args...)
				wantCalls := 0
				if tc.valid {
					wantCalls = 1
				}
				if (err == nil) != tc.valid || caller.invocations != wantCalls {
					t.Fatalf("%s: valid=%t calls=%d err=%v", operation, tc.valid, caller.invocations, err)
				}
			}
			if !tc.valid {
				path := filepath.Join(t.TempDir(), "retrieved.json")
				caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "file": card}}}
				if _, err := executeAicardTemplate(t, caller, "get", "--template-id", "t", "--version", "1", "--out", path); err == nil {
					t.Fatal("retrieval accepted an unpaired surrogate")
				}
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("invalid retrieved content reached disk: %v", err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAicardTemplateUnicodeDigestCompatibility(t *testing.T) {
	// Non-ASCII literals are intentional Unicode interoperability controls.
	for _, pair := range [][2]string{
		{`{"value":"\ud83d\ude00"}`, `{"value":"😀"}`},
		{`{"value":"\ufffd"}`, `{"value":"�"}`},
		{`{"\ud83d\ude00":"value"}`, `{"😀":"value"}`},
	} {
		first, err := validateAicardTemplate([]byte(aicardUnicodeTestCard(t, pair[0])))
		if err != nil {
			t.Fatal(err)
		}
		second, err := validateAicardTemplate([]byte(aicardUnicodeTestCard(t, pair[1])))
		if err != nil || first != second {
			t.Fatalf("valid Unicode normalization changed: %q %q %v", first, second, err)
		}
	}
}
