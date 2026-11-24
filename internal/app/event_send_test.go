// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func eventSendTestArgs(payload string) []string {
	return []string{"event", "send", "--event-key", "app_user_test", "--event-id", "event-test-1", "--payload-schema-version", "3", "--user-id", "user-1", "--payload", payload, "-f", "json"}
}

func executeEventSendTest(t *testing.T, stdin io.Reader, args ...string) (map[string]any, string, error) {
	t.Helper()
	root := NewRootCommand(t.Context())
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(stdin)
	root.SetArgs(args)
	err := root.Execute()
	var result map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &result)
	return result, stdout.String() + stderr.String(), err
}

func setupEventSendTest(t *testing.T) {
	t.Helper()
	// Root construction installs process-wide helper dependencies. Restore them
	// so a dry-run caller cannot bypass confirmation in a later standalone tree.
	helpers.InitDepsForTest(t, helpers.GetCaller())
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")
	t.Cleanup(CloseFileLogger)
	testseam.Swap(t, &eventSendURL, func(string) (string, error) {
		return "https://pre-api.dingtalk.com/v1.0/event/user/push", nil
	})
}

func TestCrossPlatformCoverageEventSendRootUsesAppCredentialsAndReturnsAdmission(t *testing.T) {
	for _, status := range []string{"ACCEPTED", "DUPLICATE"} {
		t.Run(status, func(t *testing.T) {
			setupEventSendTest(t)
			testseam.Swap(t, &newAppTokenProvider, func(configDir, clientID, secret string) appTokenGetter {
				if clientID != "app-canary" || secret != "secret-canary" {
					t.Fatal("application credentials did not reach the app token provider")
				}
				return eventSendTestTokenGetter{}
			})
			calls := 0
			testseam.Swap(t, &eventSendPush, func(ctx context.Context, endpoint, token string, req appEventPushRequest) (appEventPushResult, error) {
				calls++
				if token != "app-token" || endpoint != "https://pre-api.dingtalk.com/v1.0/event/user/push" {
					t.Fatalf("wrong application auth or environment")
				}
				if req.EventKey != "app_user_test" || req.EventID != "event-test-1" || req.PayloadSchemaVersion != "3" || req.OccurredAt != "2026-11-12T10:30:00+08:00" {
					t.Fatalf("request metadata = %#v", req)
				}
				if string(req.Payload) != `{"id":9007199254740993}` || !reflect.DeepEqual(req.UserIDs, []string{"user-1", "UserTwo", "usertwo"}) {
					t.Fatalf("payload or recipients changed: %#v", req)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing request deadline")
				}
				return appEventPushResult{EventID: req.EventID, Status: status, Duplicate: status == "DUPLICATE"}, nil
			})
			args := append([]string{"--client-id", "app-canary", "--client-secret", "secret-canary"}, eventSendTestArgs(`{"id":9007199254740993}`)...)
			args = append(args, "--user-id", " UserTwo ", "--user-id", "usertwo", "--user-id", "UserTwo", "--user-id", " ", "--user-id", "user-1", "--occurred-at", "2026-11-12T10:30:00+08:00")
			got, raw, err := executeEventSendTest(t, nil, args...)
			if err != nil || calls != 1 || got["ok"] != true || got["outcome"] != "success" {
				t.Fatalf("calls=%d error=%v output=%s", calls, err, raw)
			}
			data := got["data"].(map[string]any)
			if data["status"] != status || data["duplicate"] != (status == "DUPLICATE") || data["eventId"] != "event-test-1" {
				t.Fatalf("receipt = %#v", data)
			}
			if strings.Contains(raw, "secret-canary") || strings.Contains(raw, "app-token") {
				t.Fatal("credential leaked")
			}
			env := &output.Envelope{OK: true, Outcome: output.OutcomeSuccess, Data: data}
			if output.ExitCodeForEnvelope(env) != 0 {
				t.Fatal("accepted receipt must exit 0")
			}
		})
	}
}

type eventSendTestTokenGetter struct{}

func (eventSendTestTokenGetter) GetToken(context.Context) (string, error) { return "app-token", nil }

func TestCrossPlatformCoverageEventSendReadsEnvironmentAndRejectsPartialFlags(t *testing.T) {
	setupEventSendTest(t)
	t.Setenv("DWS_CLIENT_ID", "environment-app")
	t.Setenv("DWS_CLIENT_SECRET", "environment-secret")
	testseam.Swap(t, &newAppTokenProvider, func(_, clientID, secret string) appTokenGetter {
		if clientID != "environment-app" || secret != "environment-secret" {
			t.Fatal("incorrect environment pair")
		}
		return eventSendTestTokenGetter{}
	})
	calls := 0
	testseam.Swap(t, &eventSendPush, func(_ context.Context, _, _ string, req appEventPushRequest) (appEventPushResult, error) {
		calls++
		return appEventPushResult{EventID: req.EventID, Status: "ACCEPTED"}, nil
	})
	if _, raw, err := executeEventSendTest(t, nil, eventSendTestArgs(`{}`)...); err != nil {
		t.Fatalf("%v %s", err, raw)
	}
	args := append(eventSendTestArgs(`{}`), "--client-id", "partial-app")
	if _, _, err := executeEventSendTest(t, nil, args...); err == nil || apperrors.ExitCode(err) != 2 {
		t.Fatalf("expected auth failure, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("partial credentials sent request, calls=%d", calls)
	}
}

func TestCrossPlatformCoverageEventSendDryRunDefersInputsAndCredentials(t *testing.T) {
	setupEventSendTest(t)
	testseam.Swap(t, &eventSendToken, func(context.Context, string, string) (string, error) {
		t.Fatal("dry-run read credentials")
		return "", nil
	})
	testseam.Swap(t, &eventSendPush, func(context.Context, string, string, appEventPushRequest) (appEventPushResult, error) {
		t.Fatal("dry-run sent request")
		return appEventPushResult{}, nil
	})
	for _, payload := range []string{"-", "@/unreadable-event-send-file", `{"number":9007199254740993}`} {
		args := append(eventSendTestArgs(payload), "--dry-run")
		got, raw, err := executeEventSendTest(t, eventSendFailReader{}, args...)
		if err != nil || got["dry_run"] != true || got["ok"] != true {
			t.Fatalf("%v %s", err, raw)
		}
		if payload[0] != '{' && got["data"].(map[string]any)["payloadSource"] != payload {
			t.Fatalf("source missing %s", raw)
		}
	}
}

type eventSendFailReader struct{}

func (eventSendFailReader) Read([]byte) (int, error) { return 0, errors.New("unexpected input read") }

func TestCrossPlatformCoverageEventSendPayloadSources(t *testing.T) {
	setupEventSendTest(t)
	const payload = `{"id":9007199254740993}`
	path := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &eventSendToken, func(context.Context, string, string) (string, error) { return "token", nil })
	testseam.Swap(t, &eventSendPush, func(_ context.Context, _, _ string, req appEventPushRequest) (appEventPushResult, error) {
		if string(req.Payload) != payload {
			t.Fatalf("JSON changed: %s", req.Payload)
		}
		return appEventPushResult{EventID: req.EventID, Status: "ACCEPTED"}, nil
	})
	for _, source := range []string{payload, "@" + path, "-"} {
		if _, raw, err := executeEventSendTest(t, strings.NewReader(payload), eventSendTestArgs(source)...); err != nil {
			t.Fatalf("%v %s", err, raw)
		}
	}
	formatted := "{" + strings.Repeat(" ", eventSendPayloadLimit) + `"id":9007199254740993}`
	if actual, err := readEventSendPayload(formatted, nil); err != nil || string(actual) != payload {
		t.Fatalf("formatting whitespace changed the payload limit or number: %v %s", err, actual)
	}
	if _, err := readEventSendPayload(strings.Repeat(" ", config.MaxResponseBodySize+1), nil); err == nil {
		t.Fatal("oversized input was accepted")
	}
}

func TestCrossPlatformCoverageEventSendValidationAndFailures(t *testing.T) {
	setupEventSendTest(t)
	testseam.Swap(t, &eventSendToken, func(context.Context, string, string) (string, error) { return "token", nil })
	upstreamErr := apperrors.NewAPI("接纳结果未确认", apperrors.WithDetails(map[string]any{"eventId": "event-test-1"}))
	calls := 0
	testseam.Swap(t, &eventSendPush, func(context.Context, string, string, appEventPushRequest) (appEventPushResult, error) {
		calls++
		return appEventPushResult{}, upstreamErr
	})
	for _, source := range []string{"null", "[]", "{", "@", "@/missing-file", `{"text":"` + strings.Repeat("a", eventSendPayloadLimit) + `"}`} {
		if _, _, err := executeEventSendTest(t, nil, eventSendTestArgs(source)...); err == nil || apperrors.ExitCode(err) != 3 {
			t.Fatalf("source %q error %v", source[:1], err)
		}
	}
	for _, extras := range [][]string{{"--event-id", strings.Repeat("x", 65)}, {"--user-id", strings.Repeat("u", 65)}, {"--occurred-at", "2026-11-12"}, {"--occurred-at", "1970-01-01T00:00:00Z"}} {
		args := append(eventSendTestArgs(`{}`), extras...)
		if _, _, err := executeEventSendTest(t, nil, args...); err == nil || apperrors.ExitCode(err) != 3 {
			t.Fatalf("args %v err %v", extras, err)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid input sent %d requests", calls)
	}
	_, _, err := executeEventSendTest(t, nil, eventSendTestArgs(`{}`)...)
	if !errors.Is(err, upstreamErr) || calls != 1 {
		t.Fatalf("error %v calls %d", err, calls)
	}
	if _, err := readEventSendPayload("-", eventSendFailReader{}); err == nil {
		t.Fatal("read failure lost")
	}
	if _, err := readEventSendPayload("-", nil); err == nil {
		t.Fatal("missing stdin accepted")
	}
}

func TestCrossPlatformCoverageEventSendPreviewValidationAndEnvironment(t *testing.T) {
	setupEventSendTest(t)
	testseam.Swap(t, &eventSendToken, func(context.Context, string, string) (string, error) {
		t.Fatal("invalid preview resolved credentials")
		return "", nil
	})
	args := append(eventSendTestArgs("[]"), "--dry-run")
	if _, _, err := executeEventSendTest(t, nil, args...); err == nil || apperrors.ExitCode(err) != 3 {
		t.Fatalf("invalid preview JSON error = %v", err)
	}
	args = eventSendTestArgs(`{}`)
	for i := 1; i <= 50; i++ {
		args = append(args, "--user-id", fmt.Sprintf("Mixed-%d", i), "--user-id", fmt.Sprintf("mixed-%d", i))
	}
	if _, _, err := executeEventSendTest(t, nil, args...); err == nil || apperrors.ExitCode(err) != 3 {
		t.Fatalf("recipient limit error = %v", err)
	}
	testseam.Swap(t, &eventSendURL, func(string) (string, error) {
		return "", apperrors.NewValidation("unsupported event environment")
	})
	if _, _, err := executeEventSendTest(t, nil, eventSendTestArgs(`{}`)...); err == nil || !strings.Contains(err.Error(), "unsupported event environment") {
		t.Fatalf("environment error = %v", err)
	}
}

func TestCrossPlatformCoverageEventSendSchemaAcceptsRuntimeData(t *testing.T) {
	setupEventSendTest(t)
	testseam.Swap(t, &eventSendToken, func(context.Context, string, string) (string, error) { return "token", nil })
	testseam.Swap(t, &eventSendPush, func(_ context.Context, _, _ string, req appEventPushRequest) (appEventPushResult, error) {
		return appEventPushResult{EventID: req.EventID, Status: "DUPLICATE", Duplicate: true}, nil
	})
	for _, compact := range []bool{false, true} {
		args := []string{"--cli-path", "event send"}
		if compact {
			args = append(args, "--compact")
		}
		leaf := executeShortcutSchemaQuery(t, args...)
		result := leaf["result"].(map[string]any)
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		resource := fmt.Sprintf("https://dws.test/event-send-%t.json", compact)
		if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
			t.Fatal(err)
		}
		schema, err := compiler.Compile(resource)
		if err != nil {
			t.Fatal(err)
		}
		for _, dryRun := range []bool{false, true} {
			commandArgs := eventSendTestArgs(`{}`)
			if dryRun {
				commandArgs = append(commandArgs, "--dry-run")
			}
			got, raw, err := executeEventSendTest(t, nil, commandArgs...)
			if err != nil {
				t.Fatalf("%v %s", err, raw)
			}
			if err := schema.Validate(got["data"]); err != nil {
				t.Fatalf("invalid runtime data: %v %s", err, raw)
			}
		}
	}
}

func TestCrossPlatformCoverageEventSendHelpDiscovery(t *testing.T) {
	setupEventSendTest(t)
	for _, path := range [][]string{{"event", "--help"}, {"event", "send", "--help"}} {
		got := executeHelpOutput(t, path...)
		for _, want := range []string{"自建应用", "send", "dingtalk-event", "references/event-send.md"} {
			if !strings.Contains(got, want) {
				t.Fatalf("%v help missing %q: %s", path, want, got)
			}
		}
	}
}

func TestCrossPlatformCoverageEventSendDryRunRestoresConfirmationDependencies(t *testing.T) {
	helpers.InitDepsForTest(t, nil)
	t.Run("send preview", func(t *testing.T) {
		setupEventSendTest(t)
		args := append(eventSendTestArgs(`{}`), "--dry-run")
		if _, raw, err := executeEventSendTest(t, nil, args...); err != nil {
			t.Fatalf("preview: %v %s", err, raw)
		}
	})
	root, _ := newEventStopSafetyRoot()
	root.SetArgs([]string{"event", "stop", "sub-1"})
	var typed *apperrors.Error
	if err := corecmd.ExecuteForTest(root); !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
		t.Fatalf("send preview affected later confirmation: %v", err)
	}
}
