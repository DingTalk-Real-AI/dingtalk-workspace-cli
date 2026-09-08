// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageChatAtomicOwnersResolveToPublicShortcuts(t *testing.T) {
	root := NewRootCommand()
	owners := shortcut.PreferredShortcutOwnersSnapshot()
	if len(owners) < 80 {
		t.Fatalf("reviewed Chat atomic owners = %d, want at least 80", len(owners))
	}
	for atomicPath, ownerPath := range owners {
		atomic, remaining, err := root.Find(strings.Fields(atomicPath))
		if err != nil || atomic == nil || len(remaining) != 0 || !atomic.Runnable() {
			t.Errorf("atomic path %q is not runnable: command=%v remaining=%v err=%v", atomicPath, atomic, remaining, err)
			continue
		}
		if got := atomic.Annotations[preferredShortcutCLIPathAnnotation]; got != ownerPath {
			t.Errorf("atomic path %q preferred Shortcut = %q, want %q", atomicPath, got, ownerPath)
		}
		owner, remaining, err := root.Find(strings.Fields(ownerPath))
		if err != nil || owner == nil || len(remaining) != 0 || owner.Hidden || !owner.Runnable() {
			t.Errorf("preferred Shortcut %q is not public runnable: command=%v remaining=%v err=%v", ownerPath, owner, remaining, err)
		}
	}
}

func TestCrossPlatformCoverageChatDiscoveryDefensiveBranches(t *testing.T) {
	chat := &cobra.Command{Use: "chat"}
	mute := &cobra.Command{Use: "mute", Run: func(*cobra.Command, []string) {}}
	chat.AddCommand(mute)
	annotatePreferredShortcutOwners([]*cobra.Command{nil, chat})
	if got := mute.Annotations[preferredShortcutCLIPathAnnotation]; got != "chat +conversation-mute" {
		t.Fatalf("chat mute preferred Shortcut = %q", got)
	}

	var output bytes.Buffer
	renderChatHelpCommandSection(&output, "Empty:", nil)
	if output.Len() != 0 {
		t.Fatalf("empty Help command section rendered output: %q", output.String())
	}
}

func TestCrossPlatformCoverageFilteredIMSearchUsesResourceAndAnswerShapeBoundary(t *testing.T) {
	_ = NewRootCommand()
	contains := func(values []string, needle string) bool {
		for _, value := range values {
			if strings.Contains(value, needle) {
				return true
			}
		}
		return false
	}

	aisearch, ok := contract.LookupProductDecl("aisearch")
	if !ok || !contains(aisearch.Selection.AvoidWhen, "答案形态") || !contains(aisearch.Selection.AvoidWhen, "逐条消息记录") {
		t.Fatalf("aisearch ProductDecl does not defer message-record outcomes to Chat: %#v", aisearch.Selection)
	}
	chat, ok := contract.LookupProductDecl("chat")
	if !ok || !contains(chat.Selection.UseWhen, "资源范围仅为 IM") || !contains(chat.Selection.UseWhen, "可枚举消息记录") {
		t.Fatalf("chat ProductDecl does not own structured IM records: %#v", chat.Selection)
	}

	for _, path := range []string{"aisearch enterprise", "aisearch behavior"} {
		meta, ok := cli.ResolveMeta(path)
		if !ok || !contains(meta.Selection.AvoidWhen, "chat +search-msg") || !contains(meta.Selection.AvoidWhen, "消息") {
			t.Errorf("%s final selection does not defer structured IM records: %#v", path, meta.Selection)
		}
	}
	search, ok := cli.ResolveMeta("chat +search-msg")
	if !ok || !contains(search.Selection.UseWhen, "资源范围仅为 IM") || !contains(search.Selection.UseWhen, "结构化谓词") {
		t.Fatalf("chat +search-msg final selection does not encode the resource/answer/predicate decision: %#v", search.Selection)
	}
}
