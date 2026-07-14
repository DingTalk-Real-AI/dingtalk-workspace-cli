// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type attachmentRecordingForwarder struct {
	attachments []connectMediaAttachment
}

func (f *attachmentRecordingForwarder) label() string { return "recording" }
func (f *attachmentRecordingForwarder) forward(context.Context, string, string) (string, error) {
	return "text-only", nil
}
func (f *attachmentRecordingForwarder) forwardWithAttachments(_ context.Context, _ string, _ string, attachments []connectMediaAttachment) (string, error) {
	f.attachments = append([]connectMediaAttachment(nil), attachments...)
	return "with-attachments", nil
}

func TestForwardConnectTurnPreservesAttachmentsForCapableAgent(t *testing.T) {
	fwd := &attachmentRecordingForwarder{}
	want := []connectMediaAttachment{{LocalPath: "/tmp/original.mov", FileName: "original.mov", MediaType: "video"}}
	reply, err := forwardConnectTurn(context.Background(), fwd, "conv", "prompt", want, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply != "with-attachments" || len(fwd.attachments) != 1 || fwd.attachments[0] != want[0] {
		t.Fatalf("reply=%q attachments=%#v", reply, fwd.attachments)
	}
}

func TestExecForwarderAllowsOnlyAttachmentDirectory(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	stub := filepath.Join(dir, "agent")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DWS_ARGS_LOG\"\nprintf 'ok\\n'\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	attachmentDir := t.TempDir()
	attachmentPath := filepath.Join(attachmentDir, "report.md")
	if err := os.WriteFile(attachmentPath, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &execForwarder{name: "workbuddy", argv: []string{stub, "-p"}, env: []string{"DWS_ARGS_LOG=" + logPath}, timeout: time.Second}
	reply, err := f.forwardWithAttachments(context.Background(), "conv", "read it", []connectMediaAttachment{{LocalPath: attachmentPath, FileName: "report.md", MediaType: "file"}})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "ok" {
		t.Fatalf("reply = %q", reply)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, want := range []string{"--allowedTools", "Read", "--add-dir", attachmentDir, "-p", "read it"} {
		if !strings.Contains(args, want+"\n") {
			t.Fatalf("args missing %q:\n%s", want, args)
		}
	}
}

func TestEveryStreamBridgeAgentHasAttachmentDeliveryPath(t *testing.T) {
	implementations := map[string]any{
		"claudecode": (*execForwarder)(nil),
		"codebuddy":  (*execForwarder)(nil),
		"workbuddy":  (*execForwarder)(nil),
		"custom":     (*execForwarder)(nil),
		"qoder":      (*qoderStreamForwarder)(nil),
		"qoderwork":  (*qoderStreamForwarder)(nil),
		"codex":      (*codexAppServerForwarder)(nil),
		"opencode":   (*opencodeForwarder)(nil),
		"gemini":     (*geminiAPIForwarder)(nil),
	}
	for channel := range agentSpecs {
		impl, ok := implementations[channel]
		if !ok {
			t.Errorf("agent channel %q has no declared attachment delivery path", channel)
			continue
		}
		if _, ok := impl.(attachmentForwarder); !ok {
			t.Errorf("agent channel %q implementation %T is not attachment-aware", channel, impl)
		}
	}
}

// TestBrandReply covers the qoderwork identity rewrite using the exact replies
// captured from a real qodercli (QoderWork.app) headless run.
func TestBrandReply(t *testing.T) {
	const persona = "我是 QoderWork 助手，钉钉群里的智能助手。"
	cases := []struct {
		name    string
		channel string
		in      string
		want    string
	}{
		{
			name:    "real reply 1 — whole sentence is the self-intro",
			channel: "qoderwork",
			in:      "我是 Qoder，一个帮助你完成软件工程任务的交互式命令行工具。",
			want:    persona,
		},
		{
			name:    "real reply 2 — self-intro then capability text kept",
			channel: "qoderwork",
			in:      "我是 Qoder，一个交互式命令行工具，帮助用户完成软件工程任务。我可以协助你编写代码、调试问题。",
			want:    persona + "我可以协助你编写代码、调试问题。",
		},
		{
			name:    "real reply 3 — self-intro then newline block kept",
			channel: "qoderwork",
			in:      "我是 Qoder，一个交互式命令行工具，主要帮助用户完成软件工程相关的任务。\n\n我的核心能力包括：",
			want:    persona + "\n\n我的核心能力包括：",
		},
		{
			name:    "english self-intro",
			channel: "qoderwork",
			in:      "I am Qoder, an interactive CLI tool that helps with software engineering.",
			want:    persona,
		},
		{
			name:    "mid-text Qoder mention is NOT rewritten",
			channel: "qoderwork",
			in:      "Qoder 是一家做 AI 编程工具的公司，它的产品不错。",
			want:    "Qoder 是一家做 AI 编程工具的公司，它的产品不错。",
		},
		{
			name:    "normal answer untouched",
			channel: "qoderwork",
			in:      "1+1 等于 2。",
			want:    "1+1 等于 2。",
		},
		{
			name:    "other channels pass through even if they say Qoder",
			channel: "codebuddy",
			in:      "我是 Qoder，一个命令行工具。",
			want:    "我是 Qoder，一个命令行工具。",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := brandReply(tc.channel, tc.in); got != tc.want {
				t.Fatalf("brandReply(%q, %q)\n  got:  %q\n  want: %q", tc.channel, tc.in, got, tc.want)
			}
		})
	}
}

// TestClaudeUserSettingsEnv covers the third-party-provider auth passthrough
// for the claudecode channel (issue PeterGuy326#10): the env block of the
// user-level Claude settings must surface as process env entries, without
// clobbering variables the operator already exported.
func TestClaudeUserSettingsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	unsetEnvForTest(t, "ANTHROPIC_BASE_URL")
	unsetEnvForTest(t, "ANTHROPIC_AUTH_TOKEN")

	// No settings file → no injection.
	if got := claudeUserSettingsEnv(); len(got) != 0 {
		t.Fatalf("expected no env without settings.json, got %v", got)
	}

	writeSettings := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Malformed JSON → fail open with no injection.
	writeSettings(`{not json`)
	if got := claudeUserSettingsEnv(); len(got) != 0 {
		t.Fatalf("expected no env for malformed settings, got %v", got)
	}

	// Provider credentials in the env block are exposed; a key already present
	// in the process environment is left alone.
	t.Setenv("ANTHROPIC_MODEL", "from-shell")
	writeSettings(`{"env":{"ANTHROPIC_BASE_URL":"https://relay.example","ANTHROPIC_AUTH_TOKEN":"tok","ANTHROPIC_MODEL":"from-settings"}}`)
	got := claudeUserSettingsEnv()
	want := map[string]bool{
		"ANTHROPIC_BASE_URL=https://relay.example": false,
		"ANTHROPIC_AUTH_TOKEN=tok":                 false,
	}
	for _, kv := range got {
		if strings.HasPrefix(kv, "ANTHROPIC_MODEL=") {
			t.Fatalf("process env must win over settings env, got %q", kv)
		}
		if _, ok := want[kv]; ok {
			want[kv] = true
		}
	}
	for kv, seen := range want {
		if !seen {
			t.Fatalf("missing %q in %v", kv, got)
		}
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

// TestCheckFDLimit verifies that checkFDLimit runs without panic and respects
// the envDurationMS pattern for the keepAlive default.
func TestCheckFDLimit(t *testing.T) {
	// Should not panic regardless of the actual ulimit.
	checkFDLimit()
}

func TestEnvDurationMS(t *testing.T) {
	def := 30000 * time.Millisecond // 30s
	if got := envDurationMS("DWS_CONNECT_KEEPALIVE_MS_TEST_ABSENT", def); got != def {
		t.Fatalf("default keepAlive = %v, want %v", got, def)
	}
	t.Setenv("DWS_CONNECT_KEEPALIVE_MS_TEST", "10000")
	if got := envDurationMS("DWS_CONNECT_KEEPALIVE_MS_TEST", def); got != 10*time.Second {
		t.Fatalf("env override = %v, want 10s", got)
	}
	t.Setenv("DWS_CONNECT_KEEPALIVE_MS_TEST", "bogus")
	if got := envDurationMS("DWS_CONNECT_KEEPALIVE_MS_TEST", def); got != def {
		t.Fatalf("invalid env falls back to default = %v, want %v", got, def)
	}
}

func TestMergeConnectQueuedTurnsBuildsSinglePrompt(t *testing.T) {
	merged := mergeConnectQueuedTurns([]connectQueuedTurn{
		{convID: "conv-1", text: "第一条", msgID: "m1"},
		{convID: "conv-1", text: "补充：按今天的数据", msgID: "m2"},
		{convID: "conv-1", text: "最后改成周报口径", msgID: "m3"},
	})
	if merged.msgID != "m3" {
		t.Fatalf("merged msgID = %q, want latest m3", merged.msgID)
	}
	for _, want := range []string{"连续发送", "1. 第一条", "2. 补充：按今天的数据", "3. 最后改成周报口径"} {
		if !strings.Contains(merged.text, want) {
			t.Fatalf("merged prompt missing %q:\n%s", want, merged.text)
		}
	}
}

func TestMergeConnectQueuedTurnsPreservesAllPictures(t *testing.T) {
	merged := mergeConnectQueuedTurns([]connectQueuedTurn{
		{convID: "conv-1", text: "第一张", picCodes: []string{"pic-1"}, msgID: "m1"},
		{convID: "conv-1", text: "再补两张", picCodes: []string{"pic-2", "pic-3"}, msgID: "m2"},
	})
	want := []string{"pic-1", "pic-2", "pic-3"}
	if len(merged.picCodes) != len(want) {
		t.Fatalf("merged picCodes = %v, want %v", merged.picCodes, want)
	}
	for i := range want {
		if merged.picCodes[i] != want[i] {
			t.Fatalf("merged picCodes = %v, want %v", merged.picCodes, want)
		}
	}
	for _, text := range []string{"第一张 [同时附有图片]", "再补两张 [同时附有图片]"} {
		if !strings.Contains(merged.text, text) {
			t.Fatalf("merged prompt missing %q:\n%s", text, merged.text)
		}
	}
}

func TestMergeConnectQueuedTurnsPreservesAllAttachments(t *testing.T) {
	merged := mergeConnectQueuedTurns([]connectQueuedTurn{
		{
			convID:    "conv-1",
			text:      "第一批",
			fileInfos: []fileInboundInfo{{DownloadCode: "audio-1", FileName: "语音消息", MediaType: "audio"}},
			msgID:     "m1",
		},
		{
			convID: "conv-1",
			text:   "第二批",
			fileInfos: []fileInboundInfo{
				{DownloadCode: "video-1", FileName: "demo.mov", MediaType: "video"},
				{DownloadCode: "file-1", FileName: "report.md", MediaType: "file"},
			},
			msgID: "m2",
		},
	})
	if len(merged.fileInfos) != 3 {
		t.Fatalf("merged fileInfos = %#v, want all three attachments", merged.fileInfos)
	}
	for i, want := range []string{"audio-1", "video-1", "file-1"} {
		if merged.fileInfos[i].DownloadCode != want {
			t.Fatalf("merged fileInfos[%d] = %#v, want code %q", i, merged.fileInfos[i], want)
		}
	}
}

func TestMergeConnectQueuedTurnsKeepsEveryChatRecordLookup(t *testing.T) {
	merged := mergeConnectQueuedTurns([]connectQueuedTurn{
		{convID: "conv-1", text: "first", chatRecordLookups: []chatRecordLookup{{MsgID: "outer-1", UnknownIndexes: []int{1}}}},
		{convID: "conv-1", text: "second", chatRecordLookups: []chatRecordLookup{{MsgID: "outer-2", UnknownIndexes: []int{0, 2}}}},
	})
	want := []chatRecordLookup{
		{MsgID: "outer-1", UnknownIndexes: []int{1}},
		{MsgID: "outer-2", UnknownIndexes: []int{0, 2}},
	}
	if !reflect.DeepEqual(merged.chatRecordLookups, want) {
		t.Fatalf("chatRecordLookups = %#v, want %#v", merged.chatRecordLookups, want)
	}
}

func TestMergeConnectQueuedTurnsKeepsControlMessagesStandalone(t *testing.T) {
	for _, text := range []string{"/clear", "同意", "拒绝", "重试"} {
		merged := mergeConnectQueuedTurns([]connectQueuedTurn{
			{convID: "conv-1", text: "先查一下", msgID: "m1"},
			{convID: "conv-1", text: text, msgID: "m2"},
		})
		if merged.text != text || merged.msgID != "m2" {
			t.Fatalf("control %q merged to (%q,%q), want standalone latest", text, merged.text, merged.msgID)
		}
	}
}
