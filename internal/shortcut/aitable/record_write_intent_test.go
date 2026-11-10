// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// A transport loss must leave durable evidence created before the caller sees
// the write. The journal is an intent, never a claim that the server applied it.
func TestCrossPlatformCoverageRecordUpsertIntentBeforeLostResponse(t *testing.T) {
	const records = `[{"cells":{"f":"private-cell-value"}}]`
	var file string
	caller := &upsertByKeyCaller{callFn: func(index int, _, tool string, args map[string]any) (string, error) {
		if index != 0 || tool != "record_upsert" {
			t.Fatalf("unexpected replay: %d %s", index, tool)
		}
		files, err := filepath.Glob(filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "aitable", "write-intents", "*.json"))
		if err != nil || len(files) != 1 {
			t.Fatalf("intent not durable before dispatch: %v %v", files, err)
		}
		file = files[0]
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var intent recordWriteIntent
		if err = json.Unmarshal(raw, &intent); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(records))
		if intent.Version != 2 || intent.Operation != "record_upsert" || intent.BaseID != "b" || intent.TableID != "t" || intent.State != "prepared" || intent.ClientToken != args["clientToken"] || len(intent.Identity) != 64 || intent.RequestDigest != hex.EncodeToString(sum[:]) || intent.RecordCount != 1 || intent.CreateCount != 1 || intent.BatchOffset != 0 {
			t.Fatalf("wrong original intent: %#v", intent)
		}
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 || strings.Contains(string(raw), "private-cell-value") {
			t.Fatalf("unsafe persisted content/permissions: %v", info.Mode())
		}
		return "", errors.New("response lost after dispatch")
	}}
	_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t", "--records", records, "--yes")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 1 {
		t.Fatalf("lost response must not replay: %v %#v", err, caller.calls)
	}
	result := typed.Details["result"].(compositeResult)
	files, _ := result.Checkpoint["writeIntentFiles"].([]string)
	if len(files) != 1 || files[0] != file || result.Checkpoint["clientToken"] != caller.calls[0].args["clientToken"] {
		t.Fatalf("recovery evidence lost: %#v", result.Checkpoint)
	}
	if !strings.Contains(result.NextCommand, "+record-write-result") || !strings.Contains(result.NextCommand, file) {
		t.Fatalf("missing bound read-only recovery command: %s", result.NextCommand)
	}
}

func TestCrossPlatformCoverageRecordWriteResultBindsPreparedIntent(t *testing.T) {
	const token = "11111111-1111-4111-8111-111111111111"
	setupRecordWriteTestProfile(t)
	rt := boundaryRuntime(t, RecordUpsert, map[string]string{"base-id": "b", "table-id": "t"}, nil, &upsertByKeyCaller{})
	identity, err := recordWriteIdentity(rt)
	if err != nil {
		t.Fatal(err)
	}
	path, err := persistRecordWriteIntent(rt, []map[string]any{{"cells": map[string]any{"f": "v"}}}, 0, token, identity)
	if err != nil {
		t.Fatal(err)
	}
	caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
		if tool != "get_record_write_result" || args["clientToken"] != token {
			t.Fatalf("unexpected recovery call: %s %#v", tool, args)
		}
		return `{"data":{"baseId":"b","tableId":"t","clientToken":"` + token + `","state":"applied","recordIds":["r"]}}`, nil
	}}
	args := []string{"+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", token, "--write-intent", path}
	output, err := runAITableCompositeCLI(t, caller, args[0], args[1:]...)
	var receipt struct {
		Outcome string `json:"outcome"`
		Data    struct {
			State       string   `json:"state"`
			ClientToken string   `json:"clientToken"`
			RecordIDs   []string `json:"recordIds"`
		} `json:"data"`
	}
	if err != nil || len(caller.calls) != 1 || json.Unmarshal([]byte(output), &receipt) != nil || receipt.Outcome != "success" || receipt.Data.State != "applied" || receipt.Data.ClientToken != token || len(receipt.Data.RecordIDs) != 1 || receipt.Data.RecordIDs[0] != "r" {
		t.Fatalf("bound intent did not recover the original applied write: %v %s", err, output)
	}
	for _, scenario := range []string{"different-target", "different-endpoint", "different-helper-endpoint", "different-configured-gateway", "missing-profile", "missing-file"} {
		t.Run(scenario, func(t *testing.T) {
			localArgs := append([]string(nil), args...)
			switch scenario {
			case "different-target":
				localArgs[2] = "other"
			case "different-endpoint":
				t.Setenv("DINGTALK_AITABLE_MCP_URL", "https://different.invalid/mcp")
			case "different-helper-endpoint":
				t.Setenv("DINGTALK_AITABLE_HELPER_MCP_URL", "https://different.invalid/helper")
			case "different-configured-gateway":
				gatewayFile := filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "mcp_url")
				if err := os.WriteFile(gatewayFile, []byte("https://pre-mcp.dingtalk.com"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Remove(gatewayFile) })
			case "missing-profile":
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			case "missing-file":
				localArgs[len(localArgs)-1] = filepath.Join(t.TempDir(), "missing.json")
			}
			before := len(caller.calls)
			if _, err := runAITableCompositeCLI(t, caller, localArgs[0], localArgs[1:]...); err == nil || len(caller.calls) != before {
				t.Fatalf("invalid recovery context reached backend: %v %#v", err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordUpsertIntentFailureStopsDispatch(t *testing.T) {
	testseam.Swap(t, &aitableTaskWriteJSON, func(string, []byte) error { return os.ErrPermission })
	caller := &upsertByKeyCaller{}
	_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"f":"v"}}]`, "--yes")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || len(caller.calls) != 0 || typed.Retryable {
		t.Fatalf("persistence failure dispatched: %v %#v", err, caller.calls)
	}
	result := typed.Details["result"].(compositeResult)
	if result.Executed || result.Status != "failed" || result.CompletedCount != 0 || result.FailedCount != 1 {
		t.Fatalf("incorrect no-write status: %#v", result)
	}
}

func TestCrossPlatformCoverageRecordUpsertIntentFailurePreservesEarlierBatch(t *testing.T) {
	persist := aitableTaskWriteJSON
	writes := 0
	testseam.Swap(t, &aitableTaskWriteJSON, func(p string, b []byte) error {
		writes++
		if writes == 2 {
			return os.ErrPermission
		}
		return persist(p, b)
	})
	rows := make([]any, 101)
	ids := make([]string, 100)
	returned := make([]any, 100)
	for i := range rows {
		rows[i] = map[string]any{"cells": map[string]any{"f": "v"}}
	}
	for i := range ids {
		ids[i] = fmt.Sprintf("r%d", i)
		returned[i] = map[string]any{"recordId": ids[i], "cells": map[string]any{"f": "v"}}
	}
	raw, _ := json.Marshal(rows)
	created, _ := json.Marshal(map[string]any{"data": map[string]any{"createdRecordIds": ids}})
	caller := &upsertByKeyCaller{callFn: func(index int, _, tool string, args map[string]any) (string, error) {
		if index == 0 {
			return string(created), nil
		}
		if tool != "query_records" {
			t.Fatalf("unexpected second mutation: %s", tool)
		}
		selected := []any{}
		for _, id := range args["recordIds"].([]string) {
			for _, row := range returned {
				if row.(map[string]any)["recordId"] == id {
					selected = append(selected, row)
				}
			}
		}
		read, _ := json.Marshal(map[string]any{"data": map[string]any{"records": selected}})
		return string(read), nil
	}}
	_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t", "--records", string(raw), "--yes")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || len(caller.calls) < 2 || writes != 2 {
		t.Fatalf("unexpected calls: err=%v callCount=%d writes=%d", err, len(caller.calls), writes)
	}
	result := typed.Details["result"].(compositeResult)
	if !result.Executed || result.Status != "partial_success" || result.CompletedCount != 100 || result.FailedCount != 1 {
		t.Fatalf("lost earlier effects: %#v", result)
	}
}

func TestCrossPlatformCoverageRecordWriteIntentRejectsUnboundOrUnserializable(t *testing.T) {
	rt := boundaryRuntime(t, RecordUpsert, map[string]string{"base-id": "b", "table-id": "t"}, nil, &upsertByKeyCaller{})
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	if _, err := persistRecordWriteIntent(rt, []map[string]any{{"cells": map[string]any{"f": "v"}}}, 0, "token", ""); err == nil {
		t.Fatal("accepted missing identity")
	}
	setupRecordWriteTestProfile(t)
	if _, err := persistRecordWriteIntent(rt, []map[string]any{{"cells": make(chan int)}}, 0, "token", "verified-test-identity"); err == nil {
		t.Fatal("accepted non-JSON payload")
	}
}

// The runtime accepts an explicit credential without profiles.json. Recovery
// must bind that credential without requiring or inventing a stored profile.
func TestCrossPlatformCoverageRecordWriteIntentExplicitCredentialWithoutProfile(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	const credential = "test-only-explicit-credential"
	var file, token string
	user := "user-one"
	caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, args map[string]any) (string, error) {
		if tool == "get_current_user_profile" {
			return fmt.Sprintf(`{"success":true,"result":[{"orgEmployeeModel":{"corpId":"corp-one","userId":%q}}]}`, user), nil
		}
		if tool != "record_upsert" {
			t.Fatalf("unexpected recovery write/read: %s", tool)
		}
		files, _ := filepath.Glob(filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "aitable", "write-intents", "*.json"))
		if len(files) != 1 {
			t.Fatalf("intent missing before credential-only dispatch: %v", files)
		}
		file, token = files[0], args["clientToken"].(string)
		return "", errors.New("response lost")
	}}
	rt := boundaryRuntime(t, RecordUpsert, map[string]string{
		"base-id": "b", "table-id": "t", "token": credential,
		"records": `[{"cells":{"f":"v"}}]`,
	}, map[string]bool{"yes": true}, caller)
	if err := RecordUpsert.Execute(rt); err == nil || len(caller.calls) != 2 {
		t.Fatalf("explicit credential rejected before dispatch: %v calls=%d", err, len(caller.calls))
	}
	if err := validateRecordWriteIntent(rt, file, token); err != nil {
		t.Fatal(err)
	}
	gatewayFile := filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "mcp_url")
	if err := os.WriteFile(gatewayFile, []byte("https://pre-mcp.dingtalk.com"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateRecordWriteIntent(rt, file, token); err == nil {
		t.Fatal("accepted a changed configured gateway with the original explicit credential")
	}
	if err := os.Remove(gatewayFile); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file)
	if err != nil || strings.Contains(string(raw), credential) {
		t.Fatalf("credential leaked or intent unreadable: %v", err)
	}
	if err := rt.Command().Flags().Set("token", "different-test-credential"); err != nil {
		t.Fatal(err)
	}
	if err := validateRecordWriteIntent(rt, file, token); err != nil {
		t.Fatalf("same account refreshed credential rejected: %v", err)
	}
	user = "other-user"
	if err := validateRecordWriteIntent(rt, file, token); err == nil {
		t.Fatal("accepted different recovery user")
	}
}
