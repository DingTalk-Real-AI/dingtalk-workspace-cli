// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// 同一身份的新凭据必须能恢复；不能把刷新凭据当成换号。
func TestCrossPlatformCoverageRecordWriteIdentityCredentialRotation(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	corp, user := "corp-one", "user-one"
	lookups := 0
	caller := &upsertByKeyCaller{callFn: func(_ int, product, tool string, _ map[string]any) (string, error) {
		if product != "contact" || tool != "get_current_user_profile" {
			t.Fatalf("unexpected call %s/%s", product, tool)
		}
		lookups++
		return fmt.Sprintf(`{"success":true,"result":[{"orgEmployeeModel":{"corpId":%q,"userId":%q}}]}`, corp, user), nil
	}}
	rt := boundaryRuntime(t, RecordUpsert, map[string]string{"base-id": "b", "table-id": "t", "token": "test-credential-A"}, nil, caller)
	first, err := recordWriteIdentity(rt)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Command().Flags().Set("token", "test-credential-B"); err != nil {
		t.Fatal(err)
	}
	refreshed, err := recordWriteIdentity(rt)
	if err != nil || first != refreshed || lookups != 2 {
		t.Fatalf("same account refresh rejected: err=%v equal=%v lookups=%d", err, first == refreshed, lookups)
	}
	user = "user-two"
	other, err := recordWriteIdentity(rt)
	if err != nil || other == first {
		t.Fatal("different user not isolated")
	}
	user, corp = "user-one", "corp-two"
	other, err = recordWriteIdentity(rt)
	if err != nil || other == first {
		t.Fatal("different organization not isolated")
	}
}

func TestCrossPlatformCoverageRecordWriteIdentityPreservesAuthFailure(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
		if tool != "get_current_user_profile" {
			t.Fatal("unverified identity reached reconciliation")
		}
		return "", apperrors.NewAuth("test credential rejected")
	}}
	const token = "11111111-1111-4111-8111-111111111111"
	path := filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "intent.json")
	raw, _ := json.Marshal(recordWriteIntent{Version: 2, Operation: "record_upsert", State: "prepared", Identity: "old-stable-identity", BaseID: "b", TableID: "t", ClientToken: token})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	rt := boundaryRuntime(t, RecordWriteResult, map[string]string{"base-id": "b", "table-id": "t", "token": "external", "client-token": token, "write-intent": path}, nil, caller)
	err := RecordWriteResult.Execute(rt)
	var classified *apperrors.Error
	if !errors.As(err, &classified) || classified.Category != apperrors.CategoryAuth {
		t.Fatalf("auth error classification lost: %v", err)
	}
	rt = boundaryRuntime(t, RecordUpsert, map[string]string{"base-id": "b", "table-id": "t", "token": "external", "records": `[{"cells":{"f":"v"}}]`}, nil, caller)
	err = RecordUpsert.Execute(rt)
	if !errors.As(err, &classified) || classified.Category != apperrors.CategoryAuth {
		t.Fatalf("pre-write auth error classification lost: %v", err)
	}
}

func TestCrossPlatformCoverageRecordWriteIdentityRejectsUnprovenPrincipal(t *testing.T) {
	for _, body := range []string{
		`{`, `{}`, `{"success":false,"result":[{"orgEmployeeModel":{"corpId":"c","userId":"u"}}]}`,
		`{"success":true,"result":[]}`, `{"success":true,"result":"invalid"}`,
		`{"success":true,"result":[{"orgEmployeeModel":{"userId":"u"}}]}`,
		`{"success":true,"result":[{"orgEmployeeModel":{"corpId":"c","orgUserId":"u"}}]}`,
		`{"success":true,"result":[{"orgEmployeeModel":{"corpId":"c","userId":"u","userid":"other"}}]}`,
		`{"success":true,"result":[{"orgEmployeeModel":{"corpId":"c","userId":"u"}},{"orgEmployeeModel":{"corpId":"d","userId":"u"}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			// 即使本地有合法 profile，也不能借给一个无法确认身份的外部 Token。
			setupRecordWriteTestProfile(t)
			caller := &upsertByKeyCaller{callFn: func(_ int, product, tool string, _ map[string]any) (string, error) {
				if product != "contact" || tool != "get_current_user_profile" {
					t.Fatalf("unverified identity reached %s/%s", product, tool)
				}
				return body, nil
			}}
			rt := boundaryRuntime(t, RecordUpsert, map[string]string{"base-id": "b", "table-id": "t", "token": "external", "records": `[{"cells":{"f":"v"}}]`}, nil, caller)
			if err := RecordUpsert.Execute(rt); err == nil {
				t.Fatal("accepted unproven identity")
			}
			if len(caller.calls) != 1 {
				t.Fatalf("unexpected replay: %d", len(caller.calls))
			}
			files, _ := filepath.Glob(filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "aitable/write-intents/*.json"))
			if len(files) != 0 {
				t.Fatal("unverified identity persisted")
			}
		})
	}
}

func TestCrossPlatformCoverageRecordWriteIdentityLegacyAndDryRun(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	caller := &upsertByKeyCaller{callFn: func(_ int, _, _ string, _ map[string]any) (string, error) {
		t.Fatal("legacy/dry-run must not query")
		return "", nil
	}}
	const token = "11111111-1111-4111-8111-111111111111"
	rt := boundaryRuntime(t, RecordWriteResult, map[string]string{"base-id": "b", "table-id": "t", "token": "old-credential", "client-token": token}, nil, caller)
	identity, err := legacyRecordWriteIdentity(rt)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "legacy.json")
	intent := recordWriteIntent{Version: 1, Operation: "record_upsert", State: "prepared", Identity: identity, BaseID: "b", TableID: "t", ClientToken: token}
	write := func() {
		raw, _ := json.Marshal(intent)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if err := validateRecordWriteIntent(rt, path, token); err != nil {
		t.Fatal(err)
	}
	if err := rt.Command().Flags().Set("token", "refreshed-credential"); err != nil {
		t.Fatal(err)
	}
	if err := validateRecordWriteIntent(rt, path, token); err == nil {
		t.Fatal("legacy binding silently relaxed")
	}
	intent.Version = 99
	write()
	if err := validateRecordWriteIntent(rt, path, token); err == nil {
		t.Fatal("unsupported intent accepted")
	}
	rt = boundaryRuntime(t, RecordWriteResult, map[string]string{"base-id": "b", "table-id": "t", "token": "external", "client-token": token, "write-intent": path}, map[string]bool{"dry-run": true}, caller)
	if err := RecordWriteResult.Execute(rt); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageRecordWriteIdentityResolvedOncePerBatchRun(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	lookups, writes := 0, 0
	caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
		switch tool {
		case "get_current_user_profile":
			lookups++
			return `{"success":true,"result":[{"orgEmployeeModel":{"corpId":"c","userId":"u"}}]}`, nil
		case "record_upsert":
			writes++
			return `{"data":{}}`, nil
		default:
			return "", errors.New("unexpected tool")
		}
	}}
	rt := boundaryRuntime(t, RecordUpsert, map[string]string{"base-id": "b", "table-id": "t", "token": "external"}, nil, caller)
	rows := make([]map[string]any, 101)
	for i := range rows {
		rows[i] = map[string]any{"cells": map[string]any{"f": "v"}}
	}
	// 该编排测试隔离既有字段回读逻辑，仅验证两批请求共享一次真实身份查询。
	verify := func(*shortcut.RuntimeContext, string, string, []map[string]any, map[string]any) (map[string]any, error) {
		return map[string]any{"status": "verified"}, nil
	}
	if err := executeRecordBatches(rt, "record_upsert", "record_upsert", serverHelper, rows, verify); err != nil {
		t.Fatal(err)
	}
	if lookups != 1 || writes != 2 {
		t.Fatalf("lookups=%d writes=%d", lookups, writes)
	}
	files, _ := filepath.Glob(filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "aitable/write-intents/*.json"))
	if len(files) != 2 {
		t.Fatal("missing per-batch intents")
	}
}

// 无外部凭据时继续使用本地执行 profile；不增加身份网络依赖。
func TestCrossPlatformCoverageRecordWriteIdentityProfileNeedsNoLookup(t *testing.T) {
	setupRecordWriteTestProfile(t)
	caller := &upsertByKeyCaller{callFn: func(_ int, _, _ string, _ map[string]any) (string, error) {
		t.Fatal("profile path must not query identity")
		return "", nil
	}}
	rt := boundaryRuntime(t, RecordUpsert, map[string]string{"base-id": "b", "table-id": "t"}, nil, caller)
	if identity, err := recordWriteIdentity(rt); err != nil || identity == "" {
		t.Fatalf("identity error: %v", err)
	}
}
