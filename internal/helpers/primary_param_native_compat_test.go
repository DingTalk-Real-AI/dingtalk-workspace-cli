// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type nativePrimaryParamCall struct {
	server string
	tool   string
	args   map[string]any
}

type nativePrimaryParamCaller struct {
	calls                 []nativePrimaryParamCall
	messageConversationID string
}

func (c *nativePrimaryParamCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	c.calls = append(c.calls, nativePrimaryParamCall{server: server, tool: tool, args: copied})

	switch tool {
	case "list_messages_by_ids":
		conversationID := c.messageConversationID
		if conversationID == "" {
			conversationID = "cid"
		}
		return textToolResult(fmt.Sprintf(`{"result":{"messages":[{"openMessageId":"mid","openConversationId":%q}]}}`, conversationID)), nil
	case "get_conversation_info":
		return textToolResult(`{"result":{"convThreadEnabled":false}}`), nil
	case "init_conversation_file_upload", "init_todo_file_upload":
		return textToolResult(`{"resourceUrl":"https://upload.invalid/file","uploadKey":"upload-key"}`), nil
	case "commit_conversation_file_upload":
		fileName, _ := args["fileName"].(string)
		payload, _ := json.Marshal(map[string]any{
			"dentryId":    123,
			"spaceId":     456,
			"downloadUrl": "https://download.invalid/" + fileName,
		})
		return textToolResult(string(payload)), nil
	case "commit_todo_file_upload":
		return textToolResult(`{"dentryId":123,"spaceId":456}`), nil
	default:
		return textToolResult(`{}`), nil
	}
}

func (*nativePrimaryParamCaller) Format() string { return "json" }
func (*nativePrimaryParamCaller) DryRun() bool   { return false }
func (*nativePrimaryParamCaller) Fields() string { return "" }
func (*nativePrimaryParamCaller) JQ() string     { return "" }

func (c *nativePrimaryParamCaller) lastCall(t *testing.T, tool string) nativePrimaryParamCall {
	t.Helper()
	for i := len(c.calls) - 1; i >= 0; i-- {
		if c.calls[i].tool == tool {
			return c.calls[i]
		}
	}
	t.Fatalf("tool %q not called; calls = %#v", tool, c.calls)
	return nativePrimaryParamCall{}
}

func executeNativePrimaryTodo(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "todo"}
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	cmd := newTodoCommand()
	if cmd.PersistentFlags().Lookup("yes") == nil {
		cmd.PersistentFlags().Bool("yes", false, "confirm execution")
	}
	if cmd.PersistentFlags().Lookup("dry-run") == nil {
		cmd.PersistentFlags().Bool("dry-run", false, "preview without executing")
	}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(context.Background())
}

func findNativePrimaryLeaf(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	leaf, remaining, err := root.Find(path)
	if err != nil {
		t.Fatalf("find %v: %v", path, err)
	}
	if len(remaining) != 0 {
		t.Fatalf("find %v left arguments %v", path, remaining)
	}
	return leaf
}

func assertNativePrimaryFlagPair(t *testing.T, cmd *cobra.Command, primary, legacy string) {
	t.Helper()
	primaryFlag := cmd.Flags().Lookup(primary)
	if primaryFlag == nil || primaryFlag.Hidden {
		t.Fatalf("%s --%s = %#v, want visible Primary", cmd.CommandPath(), primary, primaryFlag)
	}
	legacyFlag := cmd.Flags().Lookup(legacy)
	if legacyFlag == nil || !legacyFlag.Hidden {
		t.Fatalf("%s --%s = %#v, want hidden compatibility alias", cmd.CommandPath(), legacy, legacyFlag)
	}
	if got := legacyFlag.Annotations[runtimeannotate.AnnotationFlagAliasOf]; len(got) != 1 || got[0] != primary {
		t.Fatalf("%s --%s alias_of = %#v, want %q", cmd.CommandPath(), legacy, got, primary)
	}
	if got := legacyFlag.Annotations[runtimeannotate.AnnotationFlagAliasOrigin]; len(got) != 1 || got[0] != runtimeannotate.FlagAliasOriginCorecmdV1 {
		t.Fatalf("%s --%s alias_origin = %#v, want %q", cmd.CommandPath(), legacy, got, runtimeannotate.FlagAliasOriginCorecmdV1)
	}
}

func TestNativePrimaryParamFlagSurfaces(t *testing.T) {
	aisearch := newAisearchCommand()
	assertNativePrimaryFlagPair(t, aisearch, "query", "keyword")
	person := findNativePrimaryLeaf(t, aisearch, "person")
	assertNativePrimaryFlagPair(t, person, "query", "keyword")
	for _, cmd := range []*cobra.Command{aisearch, person} {
		if got := cmd.Flags().Lookup("keyword").Shorthand; got != "w" {
			t.Fatalf("%s --keyword shorthand = %q, want preserved -w", cmd.CommandPath(), got)
		}
		if got := cmd.Flags().Lookup("query").Shorthand; got != "" {
			t.Fatalf("%s --query shorthand = %q, want none", cmd.CommandPath(), got)
		}
	}

	chat := newChatCommand()
	send := findNativePrimaryLeaf(t, chat, "message", "send")
	assertNativePrimaryFlagPair(t, send, "content", "text")
	assertNativePrimaryFlagPair(t, send, "file", "file-path")
	sendByWebhook := findNativePrimaryLeaf(t, chat, "message", "send-by-webhook")
	assertNativePrimaryFlagPair(t, sendByWebhook, "content", "text")
	sendByBot := findNativePrimaryLeaf(t, chat, "message", "send-by-bot")
	for _, name := range []string{"text", "file-path"} {
		flag := sendByBot.Flags().Lookup(name)
		if flag == nil || flag.Hidden {
			t.Fatalf("%s --%s = %#v, want unchanged visible Primary", sendByBot.CommandPath(), name, flag)
		}
	}
	for _, name := range []string{"content", "file"} {
		if flag := sendByBot.Flags().Lookup(name); flag != nil && !flag.Hidden {
			t.Fatalf("%s --%s = %#v, pending migration must not become visible", sendByBot.CommandPath(), name, flag)
		}
	}
	reply := findNativePrimaryLeaf(t, chat, "message", "reply")
	assertNativePrimaryFlagPair(t, reply, "group", "conversation-id")
	assertNativePrimaryFlagPair(t, reply, "content", "text")

	doc := newDocCommand()
	assertNativePrimaryFlagPair(t, findNativePrimaryLeaf(t, doc, "block", "insert"), "content", "text")
	assertNativePrimaryFlagPair(t, findNativePrimaryLeaf(t, doc, "block", "update"), "content", "text")

	todo := newTodoCommand()
	assertNativePrimaryFlagPair(t, findNativePrimaryLeaf(t, todo, "task", "add-attachment"), "file", "file-path")
}

type nativePrimaryParamSpelling struct {
	name string
	args []string
	want string
}

func nativePrimaryParamSpellings(primary, legacy string) []nativePrimaryParamSpelling {
	return []nativePrimaryParamSpelling{
		{name: "legacy only", args: []string{"--" + legacy, "legacy-value"}, want: "legacy-value"},
		{name: "primary only", args: []string{"--" + primary, "primary-value"}, want: "primary-value"},
		{name: "both legacy wins", args: []string{"--" + primary, "primary-value", "--" + legacy, "legacy-value"}, want: "legacy-value"},
	}
}

func nativePrimaryJSONField(t *testing.T, raw any, field string) string {
	t.Helper()
	encoded, ok := raw.(string)
	if !ok {
		t.Fatalf("payload = %#v, want JSON string", raw)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(encoded), &payload); err != nil {
		t.Fatalf("decode payload %q: %v", encoded, err)
	}
	value, _ := payload[field].(string)
	return value
}

func nativePrimaryParagraphText(t *testing.T, call nativePrimaryParamCall) string {
	t.Helper()
	element, ok := call.args["element"].(map[string]any)
	if !ok {
		t.Fatalf("%s element = %#v", call.tool, call.args["element"])
	}
	paragraph, ok := element["paragraph"].(map[string]any)
	if !ok {
		t.Fatalf("%s paragraph = %#v", call.tool, element["paragraph"])
	}
	text, _ := paragraph["text"].(string)
	return text
}

func TestNativePrimaryParamTextPayloadCompatibility(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	t.Run("chat message send text to content", func(t *testing.T) {
		for _, spelling := range nativePrimaryParamSpellings("content", "text") {
			t.Run(spelling.name, func(t *testing.T) {
				caller := &nativePrimaryParamCaller{}
				args := []string{"message", "send", "--group", "cid", "--title", "fixed-title"}
				args = append(args, spelling.args...)
				if err := runChatCoverageCommand(t, caller, args...); err != nil {
					t.Fatal(err)
				}
				call := caller.lastCall(t, "send_personal_message")
				if got := nativePrimaryJSONField(t, call.args["content"], "text"); got != spelling.want {
					t.Fatalf("content.text = %q, want %q; payload = %#v", got, spelling.want, call.args)
				}
			})
		}
	})

	t.Run("chat message send-by-webhook text to content", func(t *testing.T) {
		for _, spelling := range nativePrimaryParamSpellings("content", "text") {
			t.Run(spelling.name, func(t *testing.T) {
				caller := &nativePrimaryParamCaller{}
				args := []string{"message", "send-by-webhook", "--token", "token", "--title", "fixed-title"}
				args = append(args, spelling.args...)
				if err := runChatCoverageCommand(t, caller, args...); err != nil {
					t.Fatal(err)
				}
				call := caller.lastCall(t, "send_message_by_custom_robot")
				if got, _ := call.args["text"].(string); got != spelling.want {
					t.Fatalf("text = %q, want %q; payload = %#v", got, spelling.want, call.args)
				}
			})
		}
	})

	t.Run("chat message reply conversation-id to group", func(t *testing.T) {
		for _, spelling := range nativePrimaryParamSpellings("group", "conversation-id") {
			t.Run(spelling.name, func(t *testing.T) {
				caller := &nativePrimaryParamCaller{messageConversationID: spelling.want}
				args := []string{"message", "reply", "--ref-msg-id", "mid", "--ref-sender", helperCurrentDOpenID, "--content", "fixed-content"}
				args = append(args, spelling.args...)
				if err := runChatCoverageCommand(t, caller, args...); err != nil {
					t.Fatal(err)
				}
				call := caller.lastCall(t, "send_personal_message")
				if got, _ := call.args["openConversationId"].(string); got != spelling.want {
					t.Fatalf("openConversationId = %q, want %q; payload = %#v", got, spelling.want, call.args)
				}
			})
		}
	})

	t.Run("chat message reply text to content", func(t *testing.T) {
		for _, spelling := range nativePrimaryParamSpellings("content", "text") {
			t.Run(spelling.name, func(t *testing.T) {
				caller := &nativePrimaryParamCaller{}
				args := []string{"message", "reply", "--group", "cid", "--ref-msg-id", "mid", "--ref-sender", helperCurrentDOpenID}
				args = append(args, spelling.args...)
				if err := runChatCoverageCommand(t, caller, args...); err != nil {
					t.Fatal(err)
				}
				call := caller.lastCall(t, "send_personal_message")
				if got := nativePrimaryJSONField(t, call.args["content"], "content"); got != spelling.want {
					t.Fatalf("reply content = %q, want %q; payload = %#v", got, spelling.want, call.args)
				}
			})
		}
	})

	for _, command := range []struct {
		name string
		path []string
		tool string
	}{
		{name: "doc block insert text to content", path: []string{"block", "insert", "--node", "node"}, tool: "insert_document_block"},
		{name: "doc block update text to content", path: []string{"block", "update", "--node", "node", "--block-id", "block"}, tool: "update_document_block"},
	} {
		command := command
		t.Run(command.name, func(t *testing.T) {
			for _, spelling := range nativePrimaryParamSpellings("content", "text") {
				t.Run(spelling.name, func(t *testing.T) {
					caller := &nativePrimaryParamCaller{}
					args := append(append([]string(nil), command.path...), spelling.args...)
					if err := runDocCoverageCommand(t, caller, args...); err != nil {
						t.Fatal(err)
					}
					call := caller.lastCall(t, command.tool)
					if got := nativePrimaryParagraphText(t, call); got != spelling.want {
						t.Fatalf("paragraph text = %q, want %q; payload = %#v", got, spelling.want, call.args)
					}
				})
			}
		})
	}
}

func TestNativePrimaryParamFilePayloadCompatibility(t *testing.T) {
	previousDeps, previousPut, previousArgs := deps, httpPutFile, os.Args
	t.Cleanup(func() {
		deps = previousDeps
		httpPutFile = previousPut
		os.Args = previousArgs
	})
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }

	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "legacy-file.txt")
	primaryPath := filepath.Join(dir, "primary-file.txt")
	if err := os.WriteFile(legacyPath, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primaryPath, []byte("primary"), 0o600); err != nil {
		t.Fatal(err)
	}
	spellings := []nativePrimaryParamSpelling{
		{name: "legacy only", args: []string{"--file-path", legacyPath}, want: filepath.Base(legacyPath)},
		{name: "primary only", args: []string{"--file", primaryPath}, want: filepath.Base(primaryPath)},
		{name: "both legacy wins", args: []string{"--file", primaryPath, "--file-path", legacyPath}, want: filepath.Base(legacyPath)},
	}

	t.Run("chat message send file-path to file", func(t *testing.T) {
		os.Args = []string{"dws", "chat"}
		for _, spelling := range spellings {
			t.Run(spelling.name, func(t *testing.T) {
				caller := &nativePrimaryParamCaller{}
				args := []string{"message", "send", "--group", "cid", "--msg-type", "file"}
				args = append(args, spelling.args...)
				if err := runChatCoverageCommand(t, caller, args...); err != nil {
					t.Fatal(err)
				}
				call := caller.lastCall(t, "send_personal_message")
				if got := nativePrimaryJSONField(t, call.args["content"], "fileName"); got != spelling.want {
					t.Fatalf("fileName = %q, want %q; payload = %#v", got, spelling.want, call.args)
				}
			})
		}
	})

	t.Run("todo task add-attachment file-path to file", func(t *testing.T) {
		os.Args = []string{"dws", "todo"}
		for _, spelling := range spellings {
			t.Run(spelling.name, func(t *testing.T) {
				caller := &nativePrimaryParamCaller{}
				args := []string{"task", "add-attachment", "--task-id", "42"}
				args = append(args, spelling.args...)
				if err := executeNativePrimaryTodo(t, caller, args...); err != nil {
					t.Fatal(err)
				}
				call := caller.lastCall(t, "add_todo_attachment")
				request, ok := call.args["todoAttachmentAddRequest"].(map[string]any)
				if !ok {
					t.Fatalf("request = %#v", call.args)
				}
				attachments, ok := request["attachmentList"].([]any)
				if !ok || len(attachments) != 1 {
					t.Fatalf("attachments = %#v", request["attachmentList"])
				}
				attachment, ok := attachments[0].(map[string]any)
				if !ok {
					t.Fatalf("attachment = %#v", attachments[0])
				}
				if got, _ := attachment["fileName"].(string); got != spelling.want {
					t.Fatalf("fileName = %q, want %q; payload = %#v", got, spelling.want, call.args)
				}
			})
		}
	})
}

func nativePrimaryErrorMentionsFlag(message, name string) bool {
	return strings.Contains(message, "--"+name) || strings.Contains(message, fmt.Sprintf("%q", name))
}

func assertNativePrimaryErrorHint(t *testing.T, err error, primary, legacy string) {
	t.Helper()
	if err == nil {
		t.Fatalf("missing --%s returned nil", primary)
	}
	message := err.Error()
	if !nativePrimaryErrorMentionsFlag(message, primary) {
		t.Fatalf("error %q does not recommend --%s", message, primary)
	}
	if nativePrimaryErrorMentionsFlag(message, legacy) {
		t.Fatalf("error %q still recommends legacy --%s", message, legacy)
	}
}

func TestNativePrimaryParamMissingErrorsRecommendPrimaryOnly(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	installScriptedCaller(t, &scriptedToolCaller{})
	assertNativePrimaryErrorHint(t, executeFilterCoverage(t, newAisearchCommand(), "person"), "query", "keyword")

	os.Args = []string{"dws", "chat"}
	assertNativePrimaryErrorHint(t,
		runChatCoverageCommand(t, &nativePrimaryParamCaller{}, "message", "send", "--group", "cid"),
		"content", "text")
	assertNativePrimaryErrorHint(t,
		runChatCoverageCommand(t, &nativePrimaryParamCaller{}, "message", "send-by-webhook", "--token", "token", "--title", "title"),
		"content", "text")
	assertNativePrimaryErrorHint(t,
		runChatCoverageCommand(t, &nativePrimaryParamCaller{}, "message", "reply", "--ref-msg-id", "mid", "--ref-sender", helperCurrentDOpenID, "--content", "body"),
		"group", "conversation-id")
	assertNativePrimaryErrorHint(t,
		runChatCoverageCommand(t, &nativePrimaryParamCaller{}, "message", "reply", "--group", "cid", "--ref-msg-id", "mid", "--ref-sender", helperCurrentDOpenID),
		"content", "text")

	os.Args = []string{"dws", "doc"}
	assertNativePrimaryErrorHint(t,
		runDocCoverageCommand(t, &nativePrimaryParamCaller{}, "block", "insert", "--node", "node"),
		"content", "text")
	assertNativePrimaryErrorHint(t,
		runDocCoverageCommand(t, &nativePrimaryParamCaller{}, "block", "update", "--node", "node", "--block-id", "block"),
		"content", "text")

	os.Args = []string{"dws", "todo"}
	assertNativePrimaryErrorHint(t,
		executeTodoEdge(t, &scriptedToolCaller{}, "task", "add-attachment", "--task-id", "42"),
		"file", "file-path")
}

func TestNativePrimaryParamPromotionErrorsPropagate(t *testing.T) {
	chat := newChatCommand()
	assertPromotionError := func(t *testing.T, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "trying to get string value of flag of type bool") {
			t.Fatalf("promotion error = %v", err)
		}
	}

	t.Run("webhook RunE", func(t *testing.T) {
		target := findNativePrimaryLeaf(t, chat, "message", "send-by-webhook")
		cmd := &cobra.Command{}
		cmd.Flags().String("content", "", "")
		cmd.Flags().Bool("text", false, "")
		if err := cmd.Flags().Set("text", "true"); err != nil {
			t.Fatal(err)
		}
		assertPromotionError(t, target.RunE(cmd, nil))
	})

	reply := findNativePrimaryLeaf(t, chat, "message", "reply")
	newInvalidGroupCommand := func(t *testing.T) *cobra.Command {
		t.Helper()
		cmd := &cobra.Command{}
		cmd.Flags().String("group", "", "")
		cmd.Flags().Bool("conversation-id", false, "")
		if err := cmd.Flags().Set("conversation-id", "true"); err != nil {
			t.Fatal(err)
		}
		return cmd
	}

	t.Run("reply PreRunE group", func(t *testing.T) {
		assertPromotionError(t, reply.PreRunE(newInvalidGroupCommand(t), nil))
	})

	t.Run("reply RunE group", func(t *testing.T) {
		assertPromotionError(t, reply.RunE(newInvalidGroupCommand(t), nil))
	})

	t.Run("reply RunE content", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("group", "cid", "")
		cmd.Flags().String("conversation-id", "", "")
		cmd.Flags().String("content", "", "")
		cmd.Flags().Bool("text", false, "")
		if err := cmd.Flags().Set("text", "true"); err != nil {
			t.Fatal(err)
		}
		assertPromotionError(t, reply.RunE(cmd, nil))
	})
}
