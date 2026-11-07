// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageContactOpenIDUnifiedResult(t *testing.T) {
	for _, tc := range []struct {
		name, response, wantError string
		dry                       bool
		args                      []string
		calls                     int
	}{
		{name: "success", response: `{"success":true,"result":{"userList":[{"openDingTalkId":"open-id-1","nick":"示例用户"}]}}`, args: []string{"--keyword", "13800138000"}, calls: 1},
		{name: "empty matches", response: `{"success":true,"result":{"userList":[]}}`, args: []string{"--keyword", "example-user"}, calls: 1},
		{name: "preview", dry: true, args: []string{"--keyword", "example-user"}},
		{name: "missing keyword", wantError: "keyword"},
		{name: "server denied", response: `{"success":false,"errorCode":"ability_not_full_open","errorMsg":"能力未开放"}`, args: []string{"--keyword", "example-user"}, calls: 1, wantError: "能力未开放"},
		{name: "malformed response", response: `not-json`, args: []string{"--keyword", "example-user"}, calls: 1, wantError: "JSON"},
		{name: "null response", response: `null`, args: []string{"--keyword", "example-user"}, calls: 1, wantError: "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Protect(t, &deps)
			testseam.Swap(t, &os.Args, []string{"dws", "contact"})
			caller := &scriptedToolCaller{format: "json", dry: tc.dry, steps: []scriptedToolStep{{text: tc.response}}}
			InitDeps(caller)
			var out bytes.Buffer
			deps.Out.w = &out
			deps.Out.errW = io.Discard
			root := newContactCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.PersistentFlags().String("format", "json", "")
			root.SetOut(&out)
			root.SetErr(io.Discard)
			root.SetArgs(append([]string{"user", "get-open-dingtalk-id"}, tc.args...))
			cmd, err := corecmd.ExecuteCForTest(root)
			if caller.calls != tc.calls {
				t.Fatalf("remote calls = %d, want %d", caller.calls, tc.calls)
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				if out.Len() != 0 {
					t.Fatalf("failure leaked success output: %s", out.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !output.UsesUnifiedResult(cmd) || out.Len() != 0 {
				t.Fatal("command must store a unified result before emission")
			}
			if _, emitted, err := output.EmitStoredResult(cmd); err != nil || !emitted {
				t.Fatalf("emitted=%t err=%v", emitted, err)
			}
			var envelope struct {
				OK      bool           `json:"ok"`
				Outcome string         `json:"outcome"`
				Data    map[string]any `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if !envelope.OK || envelope.Outcome != "success" {
				t.Fatalf("unexpected envelope: %s", out.String())
			}
			if tc.dry {
				if envelope.Data["dry_run"] != true || envelope.Data["executed"] != false {
					t.Fatalf("invalid preview: %#v", envelope.Data)
				}
			} else {
				if envelope.Data["success"] != true || envelope.Data["result"] == nil {
					t.Fatalf("business response missing: %#v", envelope.Data)
				}
				if caller.server != "contact" || caller.tool != "get_open_dingtalk_id" {
					t.Fatalf("unexpected route %s/%s", caller.server, caller.tool)
				}
			}
		})
	}
}
