package helpers

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func executeChatPortsCoverageCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageChatIDFlagsExposeCanonicalNames(t *testing.T) {
	tests := []struct {
		path      []string
		visible   []string
		hidden    []string
		notHidden []string
	}{
		{
			path:      []string{"message", "list"},
			visible:   []string{"conversation-id", "user-id"},
			hidden:    []string{"group", "id", "chat", "user"},
			notHidden: []string{"conversation-id", "user-id", "open-dingtalk-id"},
		},
		{
			path:      []string{"message", "update-text-emotion"},
			visible:   []string{"conversation-id", "message-id"},
			hidden:    []string{"group", "id", "chat", "msg-id"},
			notHidden: []string{"conversation-id", "message-id"},
		},
		{
			path:      []string{"group", "get-mute-config"},
			visible:   []string{"conversation-id"},
			hidden:    []string{"group"},
			notHidden: []string{"conversation-id"},
		},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.path, " "), func(t *testing.T) {
			cmd, _, err := newChatCommand().Find(test.path)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range test.visible {
				flag := cmd.Flags().Lookup(name)
				if flag == nil {
					t.Fatalf("flag --%s missing on %s", name, strings.Join(test.path, " "))
				}
			}
			for _, name := range test.hidden {
				flag := cmd.Flags().Lookup(name)
				if flag == nil || !flag.Hidden {
					t.Fatalf("flag --%s hidden = %v, want true", name, flag != nil && flag.Hidden)
				}
			}
			for _, name := range test.notHidden {
				flag := cmd.Flags().Lookup(name)
				if flag == nil || flag.Hidden {
					t.Fatalf("flag --%s hidden = %v, want false", name, flag != nil && flag.Hidden)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageChatIDFlagSchemaUsesCanonicalNames(t *testing.T) {
	tests := []struct {
		path      []string
		forbidden []string
	}{
		{
			path:      []string{"message", "list"},
			forbidden: []string{"group", "id", "chat", "user", "open-conversation-id"},
		},
		{
			path:      []string{"message", "update-text-emotion"},
			forbidden: []string{"group", "id", "chat", "msg-id", "open-message-id"},
		},
		{
			path:      []string{"group", "get-mute-config"},
			forbidden: []string{"group", "open-conversation-id"},
		},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.path, " "), func(t *testing.T) {
			cmd, _, err := newChatCommand().Find(test.path)
			if err != nil {
				t.Fatal(err)
			}
			payload, ok := contractfinal.RuntimeContractFinal(cmd)
			if !ok {
				t.Fatalf("missing RuntimeContractFinal for %s", strings.Join(test.path, " "))
			}
			names := map[string]bool{}
			for _, parameter := range payload.Parameters {
				names[parameter.Name] = true
			}
			for _, name := range test.forbidden {
				if names[name] {
					t.Fatalf("schema parameter --%s leaked; got %#v", name, names)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageChatIDHelperBranches(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("group", "", "")
	_ = root.PersistentFlags().Set("group", " inherited ")
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	if got := chatFlagOrFallback(child, "conversation-id", "group"); got != "inherited" {
		t.Fatalf("inherited alias value = %q, want inherited", got)
	}
	if got := chatFlagOrFallback(child, "conversation-id"); got != "" {
		t.Fatalf("missing flag value = %q, want empty", got)
	}
	inheritedOnly := &cobra.Command{Use: "inherited"}
	inheritedOnly.InheritedFlags().String("conversation-id", "", "")
	_ = inheritedOnly.InheritedFlags().Set("conversation-id", " inherited-direct ")
	if got := chatFlagOrFallback(inheritedOnly, "conversation-id"); got != "inherited-direct" {
		t.Fatalf("direct inherited flag value = %q, want inherited-direct", got)
	}

	if err := chatValidateRequiredFlagWithAliases(child, "conversation-id", "group"); err != nil {
		t.Fatalf("inherited alias should satisfy required group: %v", err)
	}
	empty := &cobra.Command{Use: "empty"}
	if err := chatValidateRequiredFlagWithAliases(empty, "group", "id"); err == nil ||
		!strings.Contains(err.Error(), "conversation-id") {
		t.Fatalf("required alias error = %v, want canonical name", err)
	}

	if got := rewriteChatIDFlagRefs("use --group and --id2 and --msg-id"); got != "use --conversation-id and --id2 and --message-id" {
		t.Fatalf("rewritten refs = %q", got)
	}
	if !isChatFlagRefBoundary("--group", len("--group")) {
		t.Fatal("end of string should be a flag boundary")
	}

	merged := mergeChatCanonicalParamDecl(contract.ParamDecl{}, contract.ParamDecl{Property: "openConversationId"})
	if merged.Property != "openConversationId" {
		t.Fatalf("merged property = %q, want openConversationId", merged.Property)
	}
	canonicalCmd := &cobra.Command{Use: "canonical"}
	canonicalCmd.Flags().String("conversation-id", "", "")
	canonicalCmd.Flags().String("other", "", "")
	_ = canonicalCmd.Flags().MarkHidden("other")
	if got := chatCanonicalFlagNameForCommand(canonicalCmd, "other"); got != "other" {
		t.Fatalf("hidden unrelated flag canonical = %q, want other", got)
	}
}

func TestCrossPlatformCoverageChatContractAndConstraintRewriteBranches(t *testing.T) {
	cmd := &cobra.Command{Use: "edit"}
	cmd.Flags().String("conversation-id", "", "")
	cmd.Flags().String("id", "", "")
	_ = cmd.Flags().MarkHidden("id")
	cmd.Flags().String("chat", "", "")
	_ = cmd.Flags().MarkHidden("chat")

	required := true
	contractfinal.RegisterRuntimeContractFinal(cmd, contract.ContractFinalPayload{
		Selection: &contract.SelectionSpec{Examples: []string{"dws chat edit --group cid"}},
		Parameters: []contract.ParamDecl{
			{Name: "", Description: "--group ignored"},
			{Name: "id", Property: "openConversationId"},
			{Name: "chat", Required: &required, Description: "--chat value", RequiredWhen: "--group set", Enum: []string{"cid"}},
		},
	})
	rewriteChatContractFlagRefs(cmd)
	payload, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok {
		t.Fatal("missing rewritten contract")
	}
	if len(payload.Parameters) != 1 || payload.Parameters[0].Name != "conversation-id" ||
		payload.Parameters[0].Property != "openConversationId" ||
		payload.Parameters[0].Required == nil || !*payload.Parameters[0].Required ||
		payload.Parameters[0].Description != "--conversation-id value" ||
		payload.Parameters[0].RequiredWhen != "--conversation-id set" ||
		!reflect.DeepEqual(payload.Parameters[0].Enum, []string{"cid"}) {
		t.Fatalf("rewritten params = %#v", payload.Parameters)
	}
	if payload.Selection == nil || payload.Selection.Examples[0] != "dws chat edit --conversation-id cid" {
		t.Fatalf("rewritten selection = %#v", payload.Selection)
	}

	runtimeannotate.AnnotateRuntimeConstraints(cmd, runtimeannotate.RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"group", "id", "conversation-id"}},
	})
	rewriteChatConstraintFlagRefs(cmd)
	if got := runtimeannotate.CommandConstraints(cmd).RequireOneOf; !reflect.DeepEqual(got, [][]string{{"conversation-id", "id"}}) {
		t.Fatalf("rewritten constraints = %#v", got)
	}

	delete(cmd.Annotations, runtimeannotate.AnnotationConstraints)
	runtimeannotate.AnnotateRuntimeConstraints(cmd, runtimeannotate.RuntimeSchemaConstraints{
		RequireTogether: [][]string{{"group", "chat"}},
	})
	rewriteChatConstraintFlagRefs(cmd)
	if _, ok := cmd.Annotations[runtimeannotate.AnnotationConstraints]; ok {
		t.Fatalf("empty canonical constraints annotation still present: %#v", cmd.Annotations)
	}
}

func TestCrossPlatformCoverageChatAliasPreRunBranches(t *testing.T) {
	cmd := &cobra.Command{Use: "edit"}
	cmd.Flags().String("conversation-id", "", "")
	cmd.Flags().String("group", "", "")
	_ = cmd.Flags().Set("conversation-id", "cid-1")
	preRunCalled := false
	cmd.PreRun = func(*cobra.Command, []string) { preRunCalled = true }
	installChatAliasSync(cmd)
	cmd.PreRun(cmd, nil)
	if !preRunCalled {
		t.Fatal("old PreRun was not called")
	}
	if got, _ := cmd.Flags().GetString("group"); got != "cid-1" {
		t.Fatalf("synced group = %q, want cid-1", got)
	}

	preRunECalled := false
	cmdWithE := &cobra.Command{Use: "edit"}
	cmdWithE.Flags().String("conversation-id", "", "")
	cmdWithE.Flags().String("group", "", "")
	_ = cmdWithE.Flags().Set("group", "cid-2")
	cmdWithE.PreRunE = func(*cobra.Command, []string) error { preRunECalled = true; return nil }
	installChatAliasSync(cmdWithE)
	if err := cmdWithE.PreRunE(cmdWithE, nil); err != nil {
		t.Fatal(err)
	}
	if !preRunECalled {
		t.Fatal("old PreRunE was not called")
	}
	if got, _ := cmdWithE.Flags().GetString("conversation-id"); got != "cid-2" {
		t.Fatalf("synced conversation-id = %q, want cid-2", got)
	}

	fallbackCalled := false
	cmdFallback := &cobra.Command{Use: "edit"}
	cmdFallback.Flags().String("conversation-id", "", "")
	cmdFallback.Flags().String("group", "", "")
	cmdFallback.PreRun = func(*cobra.Command, []string) { fallbackCalled = true }
	installChatAliasSync(cmdFallback)
	if err := cmdFallback.PreRunE(cmdFallback, nil); err != nil {
		t.Fatal(err)
	}
	if !fallbackCalled {
		t.Fatal("PreRunE wrapper did not fall back to old PreRun")
	}
}

func TestCrossPlatformCoverageChatUpdateTextEmotion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want guardedMutationCall
	}{
		{
			name: "conversation-id flag",
			args: []string{
				"message", "update-text-emotion",
				"--conversation-id", "conv-1",
				"--msg-id", "msg-1",
				"--old-emotion-id", "old-1",
				"--emotion-id", "new-1",
				"--emotion-name", "like",
				"--text", "nice",
				"--background-id", "im_bg_5",
			},
			want: guardedMutationCall{
				productID: "im",
				toolName:  "update_text_emotion",
				args: map[string]any{
					"openConversationId": "conv-1",
					"openMsgId":          "msg-1",
					"oldEmotionId":       "old-1",
					"emotionId":          "new-1",
					"emotionName":        "like",
					"text":               "nice",
					"backgroundId":       "im_bg_5",
				},
			},
		},
		{
			name: "group alias for conversation-id",
			args: []string{
				"message", "update-text-emotion",
				"--group", "conv-2",
				"--msg-id", "msg-2",
				"--old-emotion-id", "old-2",
				"--emotion-id", "new-2",
				"--emotion-name", "heart",
				"--text", "great",
				"--background-id", "im_bg_1",
			},
			want: guardedMutationCall{
				productID: "im",
				toolName:  "update_text_emotion",
				args: map[string]any{
					"openConversationId": "conv-2",
					"openMsgId":          "msg-2",
					"oldEmotionId":       "old-2",
					"emotionId":          "new-2",
					"emotionName":        "heart",
					"text":               "great",
					"backgroundId":       "im_bg_1",
				},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newChatCommand, test.args...)
			if err != nil {
				t.Fatalf("chat %s returned error: %v", strings.Join(test.args, " "), err)
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], test.want) {
				t.Fatalf("tool calls = %#v, want %#v", caller.calls, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageChatUpdateTextEmotionRequiredFlags(t *testing.T) {
	fullArgs := []string{
		"message", "update-text-emotion",
		"--conversation-id", "conv-1",
		"--msg-id", "msg-1",
		"--old-emotion-id", "old-1",
		"--emotion-id", "new-1",
		"--emotion-name", "like",
		"--text", "nice",
		"--background-id", "im_bg_5",
	}
	dropFlag := func(name string) []string {
		out := make([]string, 0, len(fullArgs))
		for i := 0; i < len(fullArgs); i++ {
			if fullArgs[i] == name {
				i++ // skip the flag value too
				continue
			}
			out = append(out, fullArgs[i])
		}
		return out
	}
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing conversation-id and aliases",
			args:    dropFlag("--conversation-id"),
			wantErr: "at least one of the flags in the group [conversation-id group id chat] is required",
		},
		{
			name:    "missing old-emotion-id",
			args:    dropFlag("--old-emotion-id"),
			wantErr: `required flag(s) "old-emotion-id" not set`,
		},
		{
			name: "missing msg-id and background-id",
			args: []string{
				"message", "update-text-emotion",
				"--conversation-id", "conv-1",
				"--old-emotion-id", "old-1",
				"--emotion-id", "new-1",
				"--emotion-name", "like",
				"--text", "nice",
			},
			wantErr: `required flag(s) "background-id", "message-id" not set`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newChatCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("err = %v, want message containing %q", err, test.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %#v, want none", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageChatGroupGetMuteConfig(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "get-mute-config", "--group", "conv-1")
	if err != nil {
		t.Fatalf("get-mute-config returned error: %v", err)
	}
	want := guardedMutationCall{
		productID: "im",
		toolName:  "get_group_mute_config",
		args:      map[string]any{"openConversationId": "conv-1"},
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], want) {
		t.Fatalf("tool calls = %#v, want %#v", caller.calls, want)
	}
}

func TestCrossPlatformCoverageChatGroupGetMuteConfigRecordsRawArgs(t *testing.T) {
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"muteBlackList":[],"muteWhiteList":[]}`},
	}}
	err := executeChatPortsCoverageCommand(t, caller,
		"group", "get-mute-config", "--group", "conv-9")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", caller.calls)
	}
	if caller.calls[0]["openConversationId"] != "conv-9" {
		t.Fatalf("raw args = %#v", caller.calls[0])
	}
}

func TestCrossPlatformCoverageChatGroupGetMuteConfigRequiresGroup(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newChatCommand,
		"group", "get-mute-config")
	if err == nil || !strings.Contains(err.Error(), "--group") {
		t.Fatalf("err = %v, want message containing --group", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("tool calls = %#v, want none", caller.calls)
	}
}
