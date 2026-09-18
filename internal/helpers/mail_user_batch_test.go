// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func mailLookupTestJSON(t *testing.T, raw []byte) any {
	t.Helper()
	var data any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCrossPlatformCoverageMailUserBatchKeepsConfirmedResults(t *testing.T) {
	for _, result := range []string{
		`{"users":[null,1,{"uid":"bad","orgEmail":"b@example.com"},{"uid":9223372036854775806,"orgEmail":"A@example.com"}],"notFoundOrgEmails":[null,3,"missing@example.com"]}`,
		`{"users":[{"uid":9223372036854775806,"orgEmail":"A@example.com"}],"notFoundOrgEmails":["missing@example.com"]}`,
	} {
		caller := &mailUserGetCaller{response: `{"success":true,"result":` + result + `}`}
		got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com,b@example.com,missing@example.com")
		if err != nil || code != 7 || caller.calls != 1 {
			t.Fatalf("output=%s code=%d err=%v calls=%d", got, code, err, caller.calls)
		}
		envelope := mailLookupTestJSON(t, got).(map[string]any)
		data := envelope["data"].(map[string]any)
		succeeded := data["succeeded"].([]any)
		unknown := data["unknown"].([]any)
		if envelope["outcome"] != "partial_failure" || len(succeeded) != 2 || len(unknown) != 1 || len(data["failed"].([]any)) != 0 || data["total"] != json.Number("3") {
			t.Fatalf("lost or misclassified entries: %s", got)
		}
		found, missing := succeeded[0].(map[string]any), succeeded[1].(map[string]any)
		if found["found"] != true || found["user"].(map[string]any)["uid"] != json.Number("9223372036854775806") || missing["found"] != false || unknown[0].(map[string]any)["id"] != "b@example.com" {
			t.Fatalf("incorrect per-address results: %s", got)
		}
	}
}

func TestCrossPlatformCoverageMailUserBatchIndependentChannelsAndEmptyInput(t *testing.T) {
	for _, result := range []string{
		`{"users":[{"uid":123,"orgEmail":"a@example.com"}]}`,
		`{"users":[{"uid":123,"orgEmail":"a@example.com"}],"notFoundOrgEmails":null}`,
		`{"users":[{"uid":123,"orgEmail":"a@example.com"}],"notFoundOrgEmails":{}}`,
		`{"users":null,"notFoundOrgEmails":["a@example.com"]}`,
		`{"users":{},"notFoundOrgEmails":["a@example.com"]}`,
	} {
		caller := &mailUserGetCaller{response: `{"success":true,"result":` + result + `}`}
		got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com")
		if err != nil || code != 0 || mailLookupTestJSON(t, got).(map[string]any)["outcome"] != "success" {
			t.Fatalf("confirmed channel discarded: %s code=%d err=%v", got, code, err)
		}
	}
	caller := &mailUserGetCaller{response: `{"success":true,"result":{"users":[{"uid":123,"orgEmail":"a@example.com"}],"notFoundOrgEmails":[]}}`}
	got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", " a@example.com, ,A@EXAMPLE.COM")
	if err != nil || code != 7 || caller.calls != 1 {
		t.Fatalf("empty item discarded valid results: %s code=%d err=%v", got, code, err)
	}
	data := mailLookupTestJSON(t, got).(map[string]any)["data"].(map[string]any)
	if data["total"] != json.Number("2") || len(data["succeeded"].([]any)) != 1 || data["failed"].([]any)[0].(map[string]any)["id"] != "org-emails[1]" {
		t.Fatalf("deduplication or input index changed: %s", got)
	}
	if !reflect.DeepEqual(caller.args["orgEmails"], []string{"a@example.com", "A@EXAMPLE.COM"}) {
		t.Fatalf("empty address sent upstream: %#v", caller.args)
	}
}

const mailBatchAddressRejection = `{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"orgEmails[1]必须是完整有效的企业邮箱地址"}`

func TestCrossPlatformCoverageMailUserBatchInvalidAddressFallback(t *testing.T) {
	var requested []string
	caller := &mailUserGetCaller{respond: func(_ context.Context, tool string, args map[string]any) (string, error) {
		if tool == "batch_get_users_by_org_emails" {
			return mailBatchAddressRejection, nil
		}
		if tool != "get_user_by_org_email" || len(args) != 1 {
			t.Fatalf("unexpected fallback RPC: %s %#v", tool, args)
		}
		email := args["orgEmail"].(string)
		requested = append(requested, email)
		switch email {
		case "a@example.com":
			return `{"success":true,"result":{"uid":9223372036854775806,"orgEmail":"a@example.com"}}`, nil
		case "missing@example.com":
			return `{"success":true,"result":null}`, nil
		default:
			return `{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"orgEmail必须是完整有效的企业邮箱地址"}`, nil
		}
	}}
	got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com,invalid,missing@example.com,A@EXAMPLE.COM")
	if err != nil || code != 7 || caller.calls != 4 || !reflect.DeepEqual(requested, []string{"a@example.com", "invalid", "missing@example.com"}) {
		t.Fatalf("fallback: %s code=%d err=%v calls=%d requested=%v", got, code, err, caller.calls, requested)
	}
	data := mailLookupTestJSON(t, got).(map[string]any)["data"].(map[string]any)
	if len(data["succeeded"].([]any)) != 2 || len(data["unknown"].([]any)) != 0 || data["failed"].([]any)[0].(map[string]any)["id"] != "invalid" {
		t.Fatalf("invalid address erased usable results: %s", got)
	}
}

func TestCrossPlatformCoverageMailUserBatchNoFallbackForGlobalFailure(t *testing.T) {
	for _, response := range []string{
		`{"success":false,"errorCode":"noPermission","errorMsg":"Not a member"}`,
		`{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"upstream unavailable"}`,
		`invalid`,
	} {
		caller := &mailUserGetCaller{response: response}
		_, err := executeMailUserLookup(t, caller, "batch-get", "--org-emails", "a@example.com,b@example.com")
		if err == nil || caller.calls != 1 {
			t.Fatalf("global failure retried: response=%s err=%v calls=%d", response, err, caller.calls)
		}
	}
	caller := &mailUserGetCaller{respond: func(context.Context, string, map[string]any) (string, error) {
		return "", errors.New("connection refused")
	}}
	if _, err := executeMailUserLookup(t, caller, "batch-get", "--org-emails", "a@example.com,b@example.com"); err == nil || caller.calls != 1 {
		t.Fatalf("connection failure retried: err=%v calls=%d", err, caller.calls)
	}
}

func TestCrossPlatformCoverageMailUserBatchFallbackStopsOnPermissionFailure(t *testing.T) {
	caller := &mailUserGetCaller{respond: func(_ context.Context, tool string, args map[string]any) (string, error) {
		if tool == "batch_get_users_by_org_emails" {
			return mailBatchAddressRejection, nil
		}
		if args["orgEmail"] == "a@example.com" {
			return `{"success":true,"result":{"uid":123}}`, nil
		}
		return `{"success":false,"errorCode":"noPermission","errorMsg":"Not a member"}`, nil
	}}
	got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com,b@example.com,c@example.com")
	if err == nil || apperrors.ExitCode(err) != 1 || caller.calls != 3 || len(got) != 0 {
		t.Fatalf("permission failure: %s code=%d err=%v calls=%d", got, code, err, caller.calls)
	}
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != CodeMCPToolError {
		t.Fatalf("permission failure lost its original classification: %v", err)
	}
	progress := typed.Details["partialResult"].(map[string]any)
	if len(progress["succeeded"].([]any)) != 1 || len(progress["unknown"].([]output.PartialUnknownEntry)) != 2 {
		t.Fatalf("permission failure lost confirmed progress: %#v", progress)
	}
}

func TestCrossPlatformCoverageMailUserBatchEmptyLabelCannotCollideWithBadAddress(t *testing.T) {
	caller := &mailUserGetCaller{respond: func(_ context.Context, tool string, args map[string]any) (string, error) {
		if tool == "batch_get_users_by_org_emails" {
			return mailBatchAddressRejection, nil
		}
		if args["orgEmail"] == "a@example.com" {
			return `{"success":true,"result":{"uid":123}}`, nil
		}
		return `{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"invalid email"}`, nil
	}}
	got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com, ,org-emails[1]")
	if err != nil || code != 7 || caller.calls != 3 {
		t.Fatalf("colliding input discarded good employee: %s code=%d err=%v calls=%d", got, code, err, caller.calls)
	}
	data := mailLookupTestJSON(t, got).(map[string]any)["data"].(map[string]any)
	failed := data["failed"].([]any)
	if len(failed) != 2 || failed[0].(map[string]any)["id"] == failed[1].(map[string]any)["id"] || len(data["succeeded"].([]any)) != 1 {
		t.Fatalf("input identities collided: %s", got)
	}
}

func TestCrossPlatformCoverageMailUserBatchRequiresStringSliceFlag(t *testing.T) {
	for _, wrongType := range []bool{false, true} {
		t.Run(fmt.Sprint(wrongType), func(t *testing.T) {
			caller := &mailUserGetCaller{}
			testseam.Protect(t, &deps)
			InitDeps(caller)
			cmd := &cobra.Command{}
			if wrongType {
				cmd.Flags().String("org-emails", "a@example.com", "")
			}
			if err := validateMailUserBatchGet(cmd, nil); err == nil {
				t.Fatal("validation accepted a missing or incorrectly typed flag")
			}
			result, err := callMailUserBatchLookupResult(cmd, "batch_get_users_by_org_emails", nil)
			if err == nil || result != nil || caller.calls != 0 {
				t.Fatalf("invalid command declaration dispatched RPC: result=%v err=%v calls=%d", result, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMailUserBatchFallbackResponseValidation(t *testing.T) {
	for _, response := range []string{
		`{"success":true,"result":[]}`,
		`{"success":true,"result":{"uid":"bad"}}`,
		`{"success":true,"result":{"uid":123,"orgEmail":42}}`,
		`{"success":true,"result":{"uid":123,"orgEmail":"other@example.com"}}`,
	} {
		t.Run(response, func(t *testing.T) {
			caller := &mailUserGetCaller{respond: func(_ context.Context, tool string, args map[string]any) (string, error) {
				if tool == "batch_get_users_by_org_emails" {
					return mailBatchAddressRejection, nil
				}
				if args["orgEmail"] == "a@example.com" {
					return `{"success":true,"result":{"uid":123}}`, nil
				}
				return response, nil
			}}
			got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com,b@example.com")
			if err != nil || code != 7 || caller.calls != 3 {
				t.Fatalf("fallback response: output=%s code=%d err=%v calls=%d", got, code, err, caller.calls)
			}
			data := mailLookupTestJSON(t, got).(map[string]any)["data"].(map[string]any)
			if len(data["succeeded"].([]any)) != 1 || len(data["failed"].([]any)) != 0 || len(data["unknown"].([]any)) != 1 || data["unknown"].([]any)[0].(map[string]any)["id"] != "b@example.com" {
				t.Fatalf("malformed employee must remain unknown without losing valid results: %s", got)
			}
		})
	}
}

func TestCrossPlatformCoverageMailUserBatchFallbackOmittedEmployee(t *testing.T) {
	caller := &mailUserGetCaller{respond: func(_ context.Context, tool string, _ map[string]any) (string, error) {
		if tool == "batch_get_users_by_org_emails" {
			return mailBatchAddressRejection, nil
		}
		return `{"success":true}`, nil
	}}
	got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "missing@example.com")
	if err != nil || code != 0 || caller.calls != 2 {
		t.Fatalf("omitted employee: output=%s code=%d err=%v calls=%d", got, code, err, caller.calls)
	}
	result := mailLookupTestJSON(t, got).(map[string]any)["data"].(map[string]any)["result"].(map[string]any)
	if len(result["users"].([]any)) != 0 || !reflect.DeepEqual(result["notFoundOrgEmails"], []any{"missing@example.com"}) {
		t.Fatalf("mapped empty result changed: %s", got)
	}
}

func TestCrossPlatformCoverageMailUserBatchFallbackCancellationPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caller := &mailUserGetCaller{respond: func(_ context.Context, tool string, _ map[string]any) (string, error) {
		if tool == "batch_get_users_by_org_emails" {
			return mailBatchAddressRejection, nil
		}
		cancel()
		return `{"success":true,"result":{"uid":123}}`, nil
	}}
	got, code, err := executeMailUserLookupWithContext(t, ctx, caller, "batch-get", "--org-emails", "a@example.com,b@example.com")
	if !errors.Is(err, context.Canceled) || len(got) != 0 || caller.calls != 2 {
		t.Fatalf("cancellation: output=%s code=%d err=%v calls=%d", got, code, err, caller.calls)
	}
}

func TestCrossPlatformCoverageMailUserBatchFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		global bool
	}{
		{"ordinary", errors.New("ordinary error"), false},
		{"auth", &CLIError{Code: CodeAuthTokenExpired}, true},
		{"permission", &CLIError{Code: CodeAuthPermission}, true},
		{"timeout", &CLIError{Code: CodeNetworkTimeout}, true},
		{"unreachable", &CLIError{Code: CodeNetworkUnreachable}, true},
		{"malformed business error", &CLIError{Code: CodeMCPToolError, Message: "invalid JSON"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", tc.err)
			if mailUserBatchRejectedAddress(err) {
				t.Fatal("unrelated error incorrectly enables batch fallback")
			}
			if got := mailUserLookupGlobalFailure(err); got != tc.global {
				t.Fatalf("global failure=%v, want %v", got, tc.global)
			}
		})
	}
}

func TestCrossPlatformCoverageMailUserBatchRejectsInconsistentPartialIdentity(t *testing.T) {
	result, err := mailUserBatchResult([]*mailUserBatchItem{
		{id: "same", email: "a@example.com", notFound: true},
		{id: "same", failure: &output.ErrorInfo{Type: "validation", Message: "invalid input"}},
	}, nil)
	if err == nil || result != nil {
		t.Fatalf("duplicate partial identities must fail closed: result=%v err=%v", result, err)
	}
}

func TestCrossPlatformCoverageMailUserBatchTypedFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		fallback bool
		global   bool
	}{
		{"address rejection", apperrors.NewAPI("orgEmails[1]必须是完整有效的企业邮箱地址", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "SYSTEM_ERROR"})), true, false},
		{"other system error", apperrors.NewAPI("upstream unavailable", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "SYSTEM_ERROR"})), false, false},
		{"mapping syntax error", apperrors.NewAPI("business error: success=false", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "PARAM_ERROR", TechnicalDetail: "Expected ',' in expression"})), false, false},
		{"missing code", apperrors.NewAPI("orgEmails[1]必须是完整有效的企业邮箱地址"), false, false},
		{"auth", apperrors.NewAuth("expired"), false, true},
		{"permission", apperrors.NewAPI("Not a member", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "noPermission"})), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", tc.err)
			if got := mailUserBatchRejectedAddress(err); got != tc.fallback {
				t.Fatalf("fallback=%v, want %v", got, tc.fallback)
			}
			if got := mailUserLookupGlobalFailure(err); got != tc.global {
				t.Fatalf("global=%v, want %v", got, tc.global)
			}
		})
	}
	for _, reason := range []string{"request_timeout", "request_cancelled", "http_client_timeout", "tls_timeout", "connection_refused", "dns_resolution_failed", "io_timeout", "request_failed"} {
		t.Run(reason, func(t *testing.T) {
			caller := &mailUserGetCaller{respond: func(_ context.Context, tool string, args map[string]any) (string, error) {
				if tool == "batch_get_users_by_org_emails" {
					return "", apperrors.NewAPI("orgEmails[1]必须是完整有效的企业邮箱地址", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "SYSTEM_ERROR"}))
				}
				if args["orgEmail"] == "a@example.com" {
					return `{"success":true,"result":{"uid":123}}`, nil
				}
				return "", apperrors.NewAPI("request failed", apperrors.WithReason(reason))
			}}
			got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com,b@example.com,c@example.com")
			if err == nil || apperrors.ExitCode(err) != 1 || len(got) != 0 || caller.calls != 3 {
				t.Fatalf("connection failure did not stop fallback: output=%s code=%d err=%v calls=%d", got, code, err, caller.calls)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != reason {
				t.Fatalf("connection failure lost its original classification: %v", err)
			}
			data := typed.Details["partialResult"].(map[string]any)
			if len(data["succeeded"].([]any)) != 1 || len(data["failed"].([]output.PartialFailedEntry)) != 0 || len(data["unknown"].([]output.PartialUnknownEntry)) != 2 {
				t.Fatalf("connection failure lost confirmed results: %#v", data)
			}
		})
	}
}

func TestCrossPlatformCoverageMailUserBatchGlobalFailurePreservesError(t *testing.T) {
	for _, cause := range []error{
		apperrors.NewAuth("expired", apperrors.WithDetails(map[string]any{"original": true})),
		&CLIError{Code: CodeAuthTokenExpired, Details: map[string]any{"original": true}},
		&CLIError{Code: CodeAuthPermission},
		&apperrors.PATError{RawJSON: `{"success":false,"code":"PAT_NO_PERMISSION"}`},
		&CLIError{Code: CodeNetworkTimeout, Cause: context.DeadlineExceeded},
		&CLIError{Code: CodeUnclassified, Cause: context.Canceled},
	} {
		t.Run(fmt.Sprintf("%T/%d", cause, apperrors.ExitCode(cause)), func(t *testing.T) {
			caller := &mailUserGetCaller{respond: func(_ context.Context, tool string, args map[string]any) (string, error) {
				if tool == "batch_get_users_by_org_emails" {
					return mailBatchAddressRejection, nil
				}
				if args["orgEmail"] == "a@example.com" {
					return `{"success":true,"result":{"uid":123}}`, nil
				}
				return "", cause
			}}
			got, _, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com,b@example.com,c@example.com")
			preserved := errors.Is(err, cause)
			if original, ok := cause.(*apperrors.PATError); ok {
				var pat apperrors.RawStderrError
				preserved = errors.As(err, &pat) && reflect.DeepEqual(mailLookupTestJSON(t, []byte(pat.RawStderr())), mailLookupTestJSON(t, []byte(original.RawJSON)))
			}
			if !preserved || apperrors.ExitCode(err) != apperrors.ExitCode(cause) || len(got) != 0 || caller.calls != 3 {
				t.Fatalf("global failure changed: got=%s err=%v cause=%v calls=%d", got, err, cause, caller.calls)
			}
			switch original := cause.(type) {
			case *apperrors.Error:
				if original.Details["partialResult"] != nil {
					t.Fatal("caller-owned error was mutated")
				}
			case *CLIError:
				if original.Details["partialResult"] != nil {
					t.Fatal("caller-owned error was mutated")
				}
			}
		})
	}
}
