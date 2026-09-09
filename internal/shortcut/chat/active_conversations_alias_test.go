// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageActiveConversationsCompatibilityMatchesPrimary(t *testing.T) {
	const first = `{"result":{"conversationMessagesList":[{"openConversationId":"group-1","title":"测试群","singleChat":false,"messages":[{"createTime":"2026-09-07 10:00:00"}]}],"hasMore":true,"nextCursor":"page-2"}}`
	const second = `{"result":{"conversationMessagesList":[{"openConversationId":"group-1","title":"测试群","singleChat":false,"messages":[{"createTime":"2026-09-07 12:00:00"}]}],"hasMore":false}}`
	for _, tc := range []struct {
		name      string
		flags     []string
		lastPage  string
		wantCalls int
		wantExit  int
		wantError bool
	}{
		{name: "default start with explicit end", lastPage: second, wantCalls: 2},
		{name: "explicit start", flags: []string{"--start", "2026-09-07"}, lastPage: second, wantCalls: 2},
		{name: "page limit", flags: []string{"--page-limit", "1"}, lastPage: second, wantCalls: 1},
		{name: "later page failure", lastPage: `{"result":{"hasMore":true}}`, wantCalls: 2, wantExit: 7},
		{name: "invalid start", flags: []string{"--start", " "}, wantError: true},
		{name: "invalid total timeout", flags: []string{"--total-timeout", "0"}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var primaryJSON map[string]any
			var primaryCalls any
			var primaryError string
			for _, name := range []string{"+recent-conversations", "+active-conversations"} {
				caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{
					activeConversationsOperation: {first, tc.lastPage},
				}}
				helpers.InitDepsForTest(t, caller)
				root := newPlatformCoverageRoot()
				ctx, _ := output.WithResultStore(context.Background())
				root.SetContext(ctx)
				var stdout, stderr bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				args := []string{"chat", name, "--end", "2026-09-08", "--page-delay", "0", "--format", "json"}
				root.SetArgs(append(args, tc.flags...))
				executed, err := root.ExecuteC()
				if (err != nil) != tc.wantError || len(caller.calls) != tc.wantCalls {
					t.Fatalf("%s: err=%v calls=%d, want error=%v calls=%d", name, err, len(caller.calls), tc.wantError, tc.wantCalls)
				}
				var payload map[string]any
				errText := ""
				if err != nil {
					errText = err.Error()
					if stdout.Len() != 0 {
						t.Fatalf("validation failure leaked stdout: %s", stdout.String())
					}
				} else {
					code, emitted, emitErr := output.EmitStoredResult(executed)
					if emitErr != nil || !emitted || code != tc.wantExit {
						t.Fatalf("%s: code=%d emitted=%v error=%v", name, code, emitted, emitErr)
					}
					if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
						t.Fatalf("%s did not preserve pure JSON output: %v; %s", name, err, stdout.String())
					}
				}
				if stderr.Len() != 0 {
					t.Fatalf("%s must not emit compatibility warnings: %s", name, stderr.String())
				}
				if name == "+recent-conversations" {
					primaryJSON, primaryCalls, primaryError = payload, caller.calls, errText
				} else {
					if !reflect.DeepEqual(payload, primaryJSON) || !reflect.DeepEqual(caller.calls, primaryCalls) || errText != primaryError {
						t.Fatalf("alias changed requests, output, or errors: payload=%#v primary=%#v", payload, primaryJSON)
					}
				}
				if strings.Contains(stdout.String(), "Warning:") {
					t.Fatal("deprecation warning polluted machine output")
				}
			}
		})
	}
}
