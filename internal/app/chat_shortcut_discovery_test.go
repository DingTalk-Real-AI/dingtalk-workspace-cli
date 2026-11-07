// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageChatRoutineIntentOwnersAlignAcrossCatalogRuntimeAndReferences(t *testing.T) {
	rootSkill, err := os.ReadFile("../../skills/multi/dingtalk-chat/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	references := map[string]string{}
	for _, name := range []string{"chat-conversation.md", "group-admin.md", "message-actions.md", "message-query.md"} {
		raw, err := os.ReadFile("../../skills/multi/dingtalk-chat/references/chat/" + name)
		if err != nil {
			t.Fatal(err)
		}
		references[name] = string(raw)
	}

	// Each pair has one routine-intent owner across its detailed Reference,
	// discovery metadata, and delivered Selection.
	// The atomic leaf remains executable for a real raw/lower-level need.
	pairs := []struct {
		atomic    string
		owner     string
		reference string
	}{
		{"chat conversation-info", "chat +conversation-info", "chat-conversation.md"},
		{"chat set-top", "chat +conversation-set-top", "chat-conversation.md"},
		{"chat mute", "chat +conversation-mute", "chat-conversation.md"},
		{"chat mark-unread", "chat +conversation-mark-unread", "chat-conversation.md"},
		{"chat clear-red-point", "chat +conversation-clear-red-point", "chat-conversation.md"},
		{"chat clear-all-red-point", "chat +conversation-clear-all-red-point", "chat-conversation.md"},
		{"chat list-all-conversations", "chat +conversation-list", "chat-conversation.md"},
		{"chat list-top-conversations", "chat +conversation-list-top", "chat-conversation.md"},
		{"chat clear-messages", "chat +conversation-clear-messages", "chat-conversation.md"},
		{"chat mark-read", "chat +conversation-mark-read", "chat-conversation.md"},
		{"chat hide", "chat +conversation-hide", "chat-conversation.md"},
		{"chat category list", "chat +category-list", "chat-conversation.md"},
		{"chat category list-conversations", "chat +category-list-conversations", "chat-conversation.md"},
		{"chat category create", "chat +category-create", "chat-conversation.md"},
		{"chat category delete", "chat +category-delete", "chat-conversation.md"},
		{"chat category rename", "chat +category-rename", "chat-conversation.md"},
		{"chat category add-conv", "chat +category-add-conversation", "chat-conversation.md"},
		{"chat category remove-conv", "chat +category-remove-conversation", "chat-conversation.md"},
		{"chat search", "chat +chat-search", "group-admin.md"},
		{"chat group create", "chat +chat-create", "group-admin.md"},
		{"chat group dismiss", "chat +chat-dismiss", "group-admin.md"},
		{"chat group set-admin", "chat +chat-set-admin", "group-admin.md"},
		{"chat group-mute", "chat +chat-mute", "group-admin.md"},
		{"chat group-mute-member", "chat +chat-mute-member", "group-admin.md"},
		{"chat group-role list", "chat +chat-role-list", "group-admin.md"},
		{"chat group-role add", "chat +chat-role-add", "group-admin.md"},
		{"chat group-role update", "chat +chat-role-update", "group-admin.md"},
		{"chat group-role remove", "chat +chat-role-remove", "group-admin.md"},
		{"chat group-role set-user", "chat +chat-role-set-user", "group-admin.md"},
		{"chat group-role remove-user", "chat +chat-role-remove-user", "group-admin.md"},
		{"chat group-role query-user", "chat +chat-role-query-user", "group-admin.md"},
		{"chat message list", "chat +chat-messages", "message-query.md"},
		{"chat message search", "chat +search-msg", "message-query.md"},
		{"chat message search-advanced", "chat +search-msg", "message-query.md"},
		{"chat message list-unread-conversations", "chat +unread-chats", "message-query.md"},
		{"chat message reply", "chat +messages-reply", "message-actions.md"},
		{"chat message recall", "chat +messages-recall", "message-actions.md"},
		{"chat message forward", "chat +messages-forward", "message-actions.md"},
		{"chat message combine-forward", "chat +messages-combine-forward", "message-actions.md"},
		{"chat message set-pin-msg", "chat +messages-set-pin", "message-actions.md"},
		{"chat message unset-pin-msg", "chat +messages-unset-pin", "message-actions.md"},
		{"chat message set-top-msg", "chat +messages-set-top", "message-actions.md"},
		{"chat message unset-top-msg", "chat +messages-unset-top", "message-actions.md"},
		{"chat message add-favorite", "chat +flag-create", "message-actions.md"},
		{"chat message remove-favorite", "chat +flag-cancel", "message-actions.md"},
		{"chat message add-emoji", "chat +messages-add-emoji", "message-actions.md"},
		{"chat message remove-emoji", "chat +messages-remove-emoji", "message-actions.md"},
	}

	root := NewRootCommand()
	for _, pair := range pairs {
		t.Run(pair.atomic, func(t *testing.T) {
			owner, ok := shortcut.PreferredShortcutForCLIPath(pair.atomic)
			if !ok || owner != pair.owner {
				t.Fatalf("preferred owner = %q, %v; want %q", owner, ok, pair.owner)
			}
			ownerToken := strings.TrimPrefix(pair.owner, "chat ")
			if !strings.Contains(references[pair.reference], ownerToken) {
				t.Errorf("%s lacks owner %q", pair.reference, pair.owner)
			}

			atomicMeta, ok := cli.ResolveMeta(pair.atomic)
			if !ok {
				t.Fatalf("atomic Runtime Selection %q is missing", pair.atomic)
			}
			if len(atomicMeta.Selection.UseWhen) == 0 {
				t.Errorf("atomic Runtime Selection %q has no lower-level use_when", pair.atomic)
			}
			if avoidWhen := strings.Join(atomicMeta.Selection.AvoidWhen, "\n"); !strings.Contains(avoidWhen, pair.owner) {
				t.Errorf("atomic avoid_when = %q, want owner %q", avoidWhen, pair.owner)
			}
			ownerMeta, ok := cli.ResolveMeta(pair.owner)
			if !ok || len(ownerMeta.Selection.UseWhen) == 0 {
				t.Errorf("owner Runtime Selection %q = %#v, %v", pair.owner, ownerMeta.Selection, ok)
			}
		})
	}
	rootText := string(rootSkill)
	for _, owner := range []string{
		"chat +conversation-info",
		"chat +conversation-list",
		"chat +chat-search",
		"chat +chat-create",
		"chat +chat-update",
		"chat +chat-messages",
		"chat +search-msg",
		"chat +unread-chats",
		"chat +messages-reply",
		"chat +messages-recall",
		"chat +flag-create",
		"chat +flag-cancel",
	} {
		if !strings.Contains(rootText, strings.TrimPrefix(owner, "chat ")) {
			t.Errorf("root routine routes lack representative owner %q", owner)
		}
	}
	for _, family := range []struct {
		intent    string
		reference string
	}{
		{"管理群身份", "references/chat/group-admin.md"},
		{"会话分组/分类", "references/chat/chat-conversation.md"},
	} {
		if !strings.Contains(rootText, family.intent) || !strings.Contains(rootText, family.reference) {
			t.Errorf("root routine routes lack %q family handoff to %s", family.intent, family.reference)
		}
	}

	var referenceCorpus strings.Builder
	if err := filepath.WalkDir("../../skills/multi/dingtalk-chat/references", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		referenceCorpus.Write(raw)
		referenceCorpus.WriteByte('\n')
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	owners := shortcut.PreferredShortcutOwnersSnapshot()
	atomicPaths := make([]string, 0, len(owners))
	for atomicPath := range owners {
		if strings.HasPrefix(atomicPath, "chat ") {
			atomicPaths = append(atomicPaths, atomicPath)
		}
	}
	sort.Strings(atomicPaths)
	flagPattern := regexp.MustCompile(`--([a-zA-Z0-9-]+)`)
	for _, atomicPath := range atomicPaths {
		t.Run("catalog/"+atomicPath, func(t *testing.T) {
			owner := owners[atomicPath]
			if !strings.Contains(referenceCorpus.String(), strings.TrimPrefix(owner, "chat ")) {
				t.Errorf("preferred owner %q is absent from Chat references", owner)
			}
			atomicMeta, ok := cli.ResolveMeta(atomicPath)
			if !ok {
				t.Fatalf("atomic Runtime Selection %q is missing", atomicPath)
			}
			if len(atomicMeta.Selection.UseWhen) == 0 {
				t.Errorf("atomic Runtime Selection %q has no lower-level use_when", atomicPath)
			}
			if avoidWhen := strings.Join(atomicMeta.Selection.AvoidWhen, "\n"); !strings.Contains(avoidWhen, owner) {
				t.Errorf("atomic avoid_when = %q, want owner %q", avoidWhen, owner)
			}
			ownerMeta, ok := cli.ResolveMeta(owner)
			if !ok || len(ownerMeta.Selection.UseWhen) == 0 {
				t.Errorf("owner Runtime Selection %q = %#v, %v", owner, ownerMeta.Selection, ok)
			}

			command, remaining, err := root.Find(strings.Fields(atomicPath))
			if err != nil || command == nil || len(remaining) != 0 {
				t.Fatalf("atomic command %q does not resolve: command=%v remaining=%v err=%v", atomicPath, command, remaining, err)
			}
			for _, example := range atomicMeta.Selection.Examples {
				for _, match := range flagPattern.FindAllStringSubmatch(example, -1) {
					flag := command.Flags().Lookup(match[1])
					if flag == nil {
						flag = command.InheritedFlags().Lookup(match[1])
					}
					if flag == nil {
						t.Errorf("Selection example uses unknown --%s: %q", match[1], example)
					} else if flag.Hidden {
						t.Errorf("Selection example exposes hidden --%s: %q", match[1], example)
					}
				}
			}
		})
	}

	product, ok := contract.LookupProductDecl("chat")
	if !ok {
		t.Fatal("chat ProductDecl is missing")
	}
	productRoute := product.Selection.AgentSummary + "\n" + strings.Join(product.Selection.UseWhen, "\n")
	if len(product.Selection.UseWhen) == 0 || len(product.Selection.AvoidWhen) == 0 {
		t.Fatalf("chat ProductDecl selection is incomplete: %#v", product.Selection)
	}
	for _, stableOwner := range []string{"chat category", "chat group"} {
		if !strings.Contains(productRoute, stableOwner) {
			t.Errorf("chat ProductDecl lacks stable route %q: %q", stableOwner, productRoute)
		}
	}
}

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

func TestCrossPlatformCoverageCategorySingleResponseCapabilityContract(t *testing.T) {
	root := NewRootCommand()
	atomic, remaining, err := root.Find([]string{"chat", "category", "list-conversations"})
	if err != nil || atomic == nil || len(remaining) != 0 || !atomic.Runnable() {
		t.Fatalf("category atomic command is not runnable: command=%v remaining=%v err=%v", atomic, remaining, err)
	}
	for _, name := range []string{"limit", "page-size", "cursor", "page-token"} {
		if flag := atomic.LocalNonPersistentFlags().Lookup(name); flag != nil {
			t.Errorf("category atomic unexpectedly exposes continuation flag --%s", name)
		}
	}

	meta, ok := cli.ResolveMeta("chat +category-list-conversations")
	useWhen := strings.Join(meta.Selection.UseWhen, "\n")
	if !ok ||
		!strings.Contains(useWhen, "没有续页参数") ||
		!strings.Contains(useWhen, "分页信号") {
		t.Fatalf("category Shortcut selection does not publish the capability boundary: %#v", meta.Selection)
	}
}
