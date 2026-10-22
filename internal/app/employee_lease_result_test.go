// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageEmployeeLeaseFinalRootResult(t *testing.T) {
	for _, released := range []bool{true, false} {
		name := "released"
		if !released {
			name = "unconfirmed"
		}
		t.Run(name, func(t *testing.T) {
			configDir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", configDir)
			t.Setenv("DWS_NO_UPDATE_CHECK", "1")
			previousProfile := auth.RuntimeProfile()
			t.Cleanup(func() { auth.SetRuntimeProfile(previousProfile) })
			const profile = "lease-corp:lease-employee"
			const agentUUID = "lease-result-employee"
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(profile)))
			bindingDir := filepath.Join(configDir, "digital-employees")
			if err := os.MkdirAll(bindingDir, 0700); err != nil {
				t.Fatal(err)
			}
			binding := map[string]any{
				"schemaVersion": 1, "agentUuid": agentUUID, "dwsProfile": profile,
				"operatorOpenDingTalkId": "lease-operator", "channel": "dsh",
				"bindingRevision": 3, "bindingState": "bound", "desiredState": "running",
				"runtimeBindingId": "lease-binding",
			}
			raw, err := json.Marshal(binding)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bindingDir, digest+".json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"dws", "--profile", profile, "dingtalk-tag", "connect", "--agent-uuid", agentUUID,
				"--channel", "dsh", "--local-lease", "--binding-revision", "3", "--runtime-instance-id",
				"11111111-1111-4111-8111-111111111111", "--yes", "--format", "json"}
			testseam.Swap(t, &os.Args, args)
			var stdout, stderr bytes.Buffer
			newRoot := rootNewRootCommandWithEngine
			testseam.Swap(t, &rootNewRootCommandWithEngine, func(ctx context.Context, engine *pipeline.Engine) *cobra.Command {
				// 保留最终生产命令树、准备步骤和进程出口，只替换测试输入输出。
				root := newRoot(ctx, engine)
				input := "released\n"
				if !released {
					input = "unconfirmed\n"
				}
				root.SetIn(strings.NewReader(input))
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				return root
			})
			code := Execute()
			var envelope struct {
				Outcome string         `json:"outcome"`
				Data    map[string]any `json:"data"`
			}
			decoder := json.NewDecoder(&stdout)
			if err := decoder.Decode(&envelope); err != nil {
				t.Fatalf("exit=%d invalid result: %v stderr=%s", code, err, stderr.String())
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				t.Fatalf("expected exactly one final result, got %v", err)
			}
			if strings.Count(stderr.String(), "[employee] leased") != 1 {
				t.Fatalf("lease was not announced exactly once: %s", stderr.String())
			}
			guard := filepath.Join(bindingDir, "employee-"+digest, "lease-guard.json")
			_, guardErr := os.Stat(guard)
			if released {
				if code != 0 || envelope.Outcome != "success" || envelope.Data["status"] != "released" || envelope.Data["agentUuid"] != agentUUID || envelope.Data["dwsProfile"] != profile || envelope.Data["channel"] != "dsh" {
					t.Fatalf("released lease exit=%d result=%#v stderr=%s", code, envelope, stderr.String())
				}
				if !os.IsNotExist(guardErr) {
					t.Fatalf("confirmed release retained guard: %v", guardErr)
				}
			} else {
				if code == 0 || envelope.Outcome != "failure" || guardErr != nil {
					t.Fatalf("unconfirmed release falsely succeeded: exit=%d result=%#v guard=%v", code, envelope, guardErr)
				}
			}
		})
	}
}
