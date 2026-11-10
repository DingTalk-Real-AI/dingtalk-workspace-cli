// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// A creation batch that fails verification must keep the token it actually
// sent, so the next step can reconcile instead of creating the rows again.
func TestCrossPlatformCoverageRecordUpsertVerifyFailureKeepsSentToken(t *testing.T) {
	testseam.Swap(t, &recordReadbackWait, func(context.Context, time.Duration) error { return nil })
	caller := &upsertByKeyCaller{callFn: func(index int, _, tool string, _ map[string]any) (string, error) {
		if index == 0 {
			return `{"data":{"createdRecordIds":["rec-1"]}}`, nil
		}
		return `{"data":{"records":[{"recordId":"rec-1","cells":{"f":"other"}}]}}`, nil
	}}
	_, err := runAITableCompositeCLI(t, caller, "+record-upsert", "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"f":"v"}}]`, "--yes")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Retryable {
		t.Fatalf("err=%#v", err)
	}
	sent, _ := caller.calls[0].args["clientToken"].(string)
	if caller.calls[0].tool != "record_upsert" || aitableprotocol.ValidateClientToken(sent) != nil || sent == "" {
		t.Fatalf("first call = %#v", caller.calls[0])
	}
	for _, call := range caller.calls[1:] {
		if call.tool == "record_upsert" {
			t.Fatalf("write was repeated: %#v", caller.calls)
		}
	}
	result := typed.Details["result"].(compositeResult)
	if result.Checkpoint["clientToken"] != sent {
		t.Fatalf("checkpoint = %#v, want token %s", result.Checkpoint, sent)
	}
}

// Writes under the Cobra test seam use isolated metadata and recovery storage;
// they must never read or change the operator's real DWS profile directory.
func setupRecordWriteTestProfile(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(`{"version":3,"currentProfile":"corp:user","profiles":[{"name":"test","corpId":"corp","userId":"user","status":"active"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
}
