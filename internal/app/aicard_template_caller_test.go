// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageAicardTemplateHostUsesPublishedTransport(t *testing.T) {
	caller := publishedURLCaller()
	client := &mcpPublishedTestTransport{callResult: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "file": "[]"}}}
	ctx := t.Context()
	args := map[string]any{"templateId": "template-1", "version": "1"}
	result, err := invokeAicardTemplate(ctx, caller, func(received context.Context) (mcpPublishedTransport, error) {
		if received != ctx {
			t.Fatal("factory lost invocation context")
		}
		return client, nil
	}, "service-1", "get_aicard_template", args)
	if err != nil {
		t.Fatal(err)
	}
	if caller.args["mcpId"] != "service-1" || client.callEndpoint != "https://example.test/path-secret?key=secret&token=private" || client.callTool != "get_aicard_template" || client.callContext != ctx || client.callArgs["version"] != "1" || result.Result.StructuredContent["success"] != true {
		t.Fatalf("wrong dispatch: caller=%#v client=%#v", caller.args, client)
	}
	if result.InputSchemaDigest != strings.Repeat("a", 64) {
		t.Fatal("validation evidence missing")
	}
}

func TestCrossPlatformCoverageAicardTemplateHostFailsBeforeInvoke(t *testing.T) {
	caller := publishedURLCaller()
	client := &mcpPublishedTestTransport{}
	failure := errors.New("factory unavailable")
	_, err := invokeAicardTemplate(t.Context(), caller, func(context.Context) (mcpPublishedTransport, error) {
		return nil, failure
	}, "service-1", "create_aicard_template", nil)
	if !errors.Is(err, failure) || client.callTool != "" {
		t.Fatal(err)
	}
	_, err = invokeAicardTemplate(t.Context(), nil, func(context.Context) (mcpPublishedTransport, error) {
		t.Fatal("resolved a transport without a caller")
		return client, nil
	}, "service-1", "create_aicard_template", nil)
	if err == nil {
		t.Fatal("nil caller accepted")
	}
	for _, adapter := range []*toolCallerAdapter{nil, {flags: &GlobalFlags{DryRun: true}}} {
		if _, err := adapter.InvokeAicardTemplate(t.Context(), "service-1", "create_aicard_template", nil); err == nil {
			t.Fatal("dry-run or nil adapter reached transport")
		}
	}
	if _, err := (recordingToolCaller{}).InvokeAicardTemplate(t.Context(), "service-1", "create_aicard_template", nil); err == nil {
		t.Fatal("unsupported caller accepted")
	}
}

type aicardHostTestCaller struct {
	*mcpURLTestCaller
	calls int
	err   error
}

func (c *aicardHostTestCaller) InvokeAicardTemplate(context.Context, string, string, map[string]any) (publishedmcp.ValidatedInvocationResult, error) {
	c.calls++
	return publishedmcp.ValidatedInvocationResult{Result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true}}}, c.err
}

func TestCrossPlatformCoverageAicardTemplateRecordingCallerRetainsCapability(t *testing.T) {
	for _, failure := range []error{nil, errors.New("execution unknown")} {
		inner := &aicardHostTestCaller{mcpURLTestCaller: publishedURLCaller(), err: failure}
		caller, ok := newRecordingToolCaller(inner).(helpers.AicardTemplateCaller)
		if !ok {
			t.Fatal("recording wrapper discarded template capability")
		}
		_, err := caller.InvokeAicardTemplate(t.Context(), "service-1", "create_aicard_template", map[string]any{"file": "private"})
		if !errors.Is(err, failure) || inner.calls != 1 {
			t.Fatalf("calls=%d err=%v", inner.calls, err)
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateLogHidesTemplateBody(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	base := transport.NewClient(nil)
	base.FileLogger = logger
	client := aicardTemplateTransport(base)
	if client == base || base.FileLogger != logger || client.FileLogger == logger {
		t.Fatal("template transport must be an isolated copy that leaves the shared transport untouched")
	}
	args := map[string]any{"file": `[{"version":"v1.0","note":"secret-business-value"}]`, "templateId": "t"}
	// Exercise the real transport logging helper rather than a hand-built record.
	logging.LogRequestBody(client.FileLogger, "tools/call", "execution-1", "create_aicard_template", args)
	client.FileLogger.With("arguments_summary", "secret-business-value", "attribute", "kept").WithGroup("group").Info("grouped", "field", "visible")
	logged := buf.String()
	if strings.Contains(logged, "secret-business-value") || !strings.Contains(logged, `"arguments_summary":"***"`) ||
		!strings.Contains(logged, "create_aicard_template") || !strings.Contains(logged, "kept") || !strings.Contains(logged, "visible") {
		t.Fatalf("unexpected diagnostic log: %s", logged)
	}
	if aicardTemplateTransport(transport.NewClient(nil)).FileLogger != nil {
		t.Fatal("a transport without a file logger must stay without one")
	}
}

func TestCrossPlatformCoverageAicardTemplateTransportFactory(t *testing.T) {
	factory := newAicardTemplateTransportFactory(&runtimeRunner{transport: transport.NewClient(nil)}, &GlobalFlags{Token: "factory-token"})
	if client, err := factory(t.Context()); err != nil || client == nil {
		t.Fatalf("factory: %v", err)
	}
	testseam.Swap(t, &runtimeTokenManager, NewTokenManager())
	testseam.Swap(t, &newAccessTokenProvider, func(string) accessTokenGetter {
		return fakeAccessTokenGetter{err: errors.New("oauth load failed")}
	})
	testseam.Swap(t, &newLegacyTokenManager, func(string) legacyTokenGetter {
		return fakeLegacyTokenGetter{err: errors.New("legacy load failed")}
	})
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	if _, err := newAicardTemplateTransportFactory(nil, nil)(t.Context()); err == nil {
		t.Fatal("factory should propagate token resolution errors")
	}
}

func TestCrossPlatformCoverageAicardTemplateAdapterTimeoutValidation(t *testing.T) {
	for _, seconds := range []int{0, -1} {
		adapter := &toolCallerAdapter{flags: &GlobalFlags{Timeout: seconds}}
		if _, err := adapter.InvokeAicardTemplate(t.Context(), "service", "get_aicard_template", nil); err == nil || !strings.Contains(err.Error(), "--timeout") {
			t.Fatalf("invalid timeout %d: %v", seconds, err)
		}
	}
	for _, flags := range []*GlobalFlags{nil, {Timeout: 1}} {
		adapter := &toolCallerAdapter{flags: flags}
		if _, err := adapter.InvokeAicardTemplate(t.Context(), "service", "get_aicard_template", nil); err == nil {
			t.Fatal("missing runner accepted")
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateFinalSchemaDelivery(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)
	for _, operation := range []string{"create", "get", "update"} {
		canonical := "aicard.template." + operation
		tool := snapshot.Tools[canonical]
		if tool == nil {
			t.Fatalf("missing delivered command %s", canonical)
		}
		if tool["cli_path"] != "aicard template "+operation {
			t.Fatal(tool)
		}
		result, ok := tool["result"].(map[string]any)
		if !ok {
			t.Fatalf("%s missing result declaration", canonical)
		}
		schema, ok := result["data_schema"].(map[string]any)
		if !ok || schema["type"] != "object" {
			t.Fatal(result)
		}
		props := schema["properties"].(map[string]any)
		for _, key := range []string{"executed", "templateId", "templateDigest", "digestAlgorithm"} {
			if props[key] == nil {
				t.Fatalf("%s missing result field %s", canonical, key)
			}
		}
		if operation == "get" {
			if props["requestedVersion"] == nil || props["versionConfirmed"] == nil || props["version"] != nil {
				t.Fatal("get falsely confirms a version")
			}
		} else if props["version"] == nil {
			t.Fatal("write omits assigned version")
		}
		// Compare through the embedded Schema query path, not only declaration DTOs.
		full := aicardTemplateSchemaLeaf(t, canonical, false)
		compact := aicardTemplateSchemaLeaf(t, canonical, true)
		if !reflect.DeepEqual(full["result"], compact["result"]) || !reflect.DeepEqual(full["result"], result) {
			t.Fatal("result projection drift")
		}
		wire, err := json.Marshal(compact)
		if err != nil || len(wire) == 0 {
			t.Fatal(err)
		}
	}
}

func aicardTemplateSchemaLeaf(t *testing.T, canonical string, compact bool) map[string]any {
	t.Helper()
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	args := []string{"schema", canonical, "--format", "json"}
	if compact {
		args = append(args, "--compact")
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Schema query: %v %s", err, buf.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
