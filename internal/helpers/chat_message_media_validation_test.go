package helpers

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func TestValidateChatMessageMediaSelectionRejectsNonImageMediaID(t *testing.T) {
	cmd := &cobra.Command{Use: "send"}
	cmd.Flags().String("media-id", "", "")
	cmd.Flags().String("msg-type", "", "")
	cmd.Flags().String("file-path", "", "")
	cmd.Flags().String("text", "", "")

	for _, msgType := range []string{"", "text", "markdown", "file"} {
		t.Run("msgType="+msgType, func(t *testing.T) {
			err := validateChatMessageMediaSelection("media-id", msgType, cmd)
			if err == nil {
				t.Fatal("validateChatMessageMediaSelection() returned nil")
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error type = %T, want *errors.Error", err)
			}
			if typed.Reason != "ambiguous_media_message" || typed.Hint == "" || len(typed.Actions) != 3 {
				t.Fatalf("structured error = %#v", typed)
			}
			for _, want := range []string{"--msg-type image", "--msg-type file", "--file-path"} {
				if !strings.Contains(typed.Hint, want) {
					t.Fatalf("hint = %q, want %q", typed.Hint, want)
				}
			}
		})
	}
}

func TestValidateChatMessageMediaSelectionAllowsImageOrNoMediaID(t *testing.T) {
	cmd := &cobra.Command{Use: "send"}
	for _, test := range []struct {
		mediaID string
		msgType string
	}{
		{},
		{mediaID: "media-id", msgType: "image"},
		{msgType: "text"},
		{msgType: "markdown"},
		{msgType: "file"},
	} {
		if err := validateChatMessageMediaSelection(test.mediaID, test.msgType, cmd); err != nil {
			t.Fatalf("validateChatMessageMediaSelection(%q, %q) = %v", test.mediaID, test.msgType, err)
		}
	}
}

func TestChatMessageSendRejectsMediaIDForNonImageTypesBeforeToolCall(t *testing.T) {
	for _, msgType := range []string{"text", "markdown", "file"} {
		t.Run(msgType, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			args := []string{
				"message", "send",
				"--group", "conversation-1",
				"--msg-type", msgType,
				"--media-id", "media-id",
				"--text", "example.png",
				"--yes",
			}
			if msgType == "file" {
				args = append(args, "--file-path", "example.png")
			}
			err := executeGuardedMutationCommand(t, caller, newChatCommand, args...)
			if err == nil {
				t.Fatal("message send unexpectedly succeeded")
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "ambiguous_media_message" {
				t.Fatalf("error = %T %v, want ambiguous_media_message", err, err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %#v, want none", caller.calls)
			}
		})
	}
}
