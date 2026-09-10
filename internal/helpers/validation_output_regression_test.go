// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func executeUnifiedProductCommand(t *testing.T, root *cobra.Command, caller *scriptedToolCaller, args ...string) []byte {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(caller)

	var stdout, stderr bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = &stderr
	installExampleGlobalFlags(root)
	if root.PersistentFlags().Lookup("debug") == nil {
		root.PersistentFlags().Bool("debug", false, "")
	}
	if root.PersistentFlags().Lookup("verbose") == nil {
		root.PersistentFlags().Bool("verbose", false, "")
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)

	ctx, _ := output.WithResultStore(context.Background())
	executed, err := root.ExecuteContextC(ctx)
	if err != nil {
		t.Fatalf("ExecuteContextC(%v): %v\nstderr:\n%s", args, err, stderr.String())
	}
	_, emitted, err := output.EmitStoredResult(executed)
	if err != nil {
		t.Fatalf("EmitStoredResult(%v): %v", args, err)
	}
	if !emitted {
		t.Fatalf("command %v returned without a unified result", args)
	}
	return stdout.Bytes()
}

func assertUnifiedSuccessEnvelope(t *testing.T, raw []byte) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("output is not one JSON envelope: %v\n%s", err, raw)
	}
	if envelope["ok"] != true || envelope["outcome"] != "success" {
		t.Fatalf("envelope = %#v, want ok=true outcome=success", envelope)
	}
	if _, ok := envelope["data"]; !ok {
		t.Fatalf("envelope has no data: %#v", envelope)
	}
}

func TestCrossPlatformCoverageChatAndDriveUseUnifiedResultEnvelope(t *testing.T) {
	t.Run("chat message send", func(t *testing.T) {
		out := executeUnifiedProductCommand(t, newChatCommand(), &scriptedToolCaller{
			format: "json",
			steps:  []scriptedToolStep{{text: `{"success":true,"result":{"openMessageId":"msg-1","openConversationId":"cid-1"}}`}},
		}, "message", "send", "--conversation-id", "cid-1", "--content", "hello", "--format", "json")
		assertUnifiedSuccessEnvelope(t, out)
	})

	t.Run("drive task get", func(t *testing.T) {
		out := executeUnifiedProductCommand(t, newDriveCommand(), &scriptedToolCaller{
			format: "json",
			steps:  []scriptedToolStep{{text: `{"status":"SUCCESS","resultUrl":"https://example.invalid/result"}`}},
		}, "task", "get", "--type", "export", "--id", "task-1", "--format", "json")
		assertUnifiedSuccessEnvelope(t, out)
	})
}

func TestCrossPlatformCoverageChatAndDriveUnifiedResultDeclarations(t *testing.T) {
	for _, test := range []struct {
		name string
		root *cobra.Command
		path []string
	}{
		{name: "chat message send", root: newChatCommand(), path: []string{"message", "send"}},
		{name: "drive task get", root: newDriveCommand(), path: []string{"task", "get"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			leaf, _, err := test.root.Find(test.path)
			if err != nil || leaf == nil {
				t.Fatalf("Find(%v): command=%v err=%v", test.path, leaf, err)
			}
			if !output.UsesUnifiedResult(leaf) {
				t.Fatalf("%s rollout = %q, want unified", test.name, output.CommandRollout(leaf))
			}
			final, ok := contractfinal.RuntimeContractFinal(leaf)
			if !ok || final.Result == nil || len(final.Result.Outcomes) == 0 || len(final.Result.DataSchema) == 0 {
				t.Fatalf("%s result declaration = %#v, found=%v", test.name, final.Result, ok)
			}
		})
	}
}
