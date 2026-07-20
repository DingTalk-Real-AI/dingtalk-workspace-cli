// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

func executeAuditVerifyJSON(t *testing.T, verify func(string) (bool, int, error)) (map[string]any, int, error) {
	t.Helper()
	previousVerify, previousExit := auditVerify, auditExit
	t.Cleanup(func() {
		auditVerify, auditExit = previousVerify, previousExit
	})
	auditVerify = verify
	exitCode := 0
	auditExit = func(code int) { exitCode = code }

	root := &cobra.Command{Use: "dws"}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.PersistentFlags().String("format", "json", "output format")
	root.AddCommand(newAuditVerifyCommand())
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"verify", "--file", "/tmp/audit.jsonl"})
	err := root.Execute()

	var payload map[string]any
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("audit verify stdout must be one JSON document: %v\n%s", decodeErr, stdout.String())
	}
	return payload, exitCode, err
}

func TestCrossPlatformCoverageAuditVerifyJSONOutputIsSingleDocument(t *testing.T) {
	payload, exitCode, err := executeAuditVerifyJSON(t, func(string) (bool, int, error) {
		return true, 0, nil
	})
	if err != nil || exitCode != 0 {
		t.Fatalf("audit verify returned err=%v exit=%d", err, exitCode)
	}
	if payload["valid"] != true || payload["file"] != "/tmp/audit.jsonl" || payload["brokenAt"] != float64(0) {
		t.Fatalf("unexpected audit payload: %#v", payload)
	}
}

func TestCrossPlatformCoverageAuditVerifyBrokenJSONIncludesReasonBeforeExit(t *testing.T) {
	payload, exitCode, err := executeAuditVerifyJSON(t, func(string) (bool, int, error) {
		return false, 3, errors.New("prev_hash mismatch")
	})
	if err != nil || exitCode != 1 {
		t.Fatalf("broken audit verify returned err=%v exit=%d", err, exitCode)
	}
	if payload["valid"] != false || payload["brokenAt"] != float64(3) || payload["reason"] != "prev_hash mismatch" {
		t.Fatalf("unexpected broken audit payload: %#v", payload)
	}
}
