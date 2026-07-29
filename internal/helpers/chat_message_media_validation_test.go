package helpers

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func TestValidateChatMessageMediaSelectionRejectsAmbiguousMediaID(t *testing.T) {
	cmd := &cobra.Command{Use: "send"}
	cmd.Flags().String("media-id", "", "")
	cmd.Flags().String("msg-type", "", "")
	cmd.Flags().String("file-path", "", "")
	cmd.Flags().String("text", "", "")

	err := validateChatMessageMediaSelection("media-id", "", cmd)
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
}

func TestValidateChatMessageMediaSelectionAllowsExplicitPaths(t *testing.T) {
	cmd := &cobra.Command{Use: "send"}
	for _, test := range []struct {
		mediaID string
		msgType string
	}{
		{},
		{mediaID: "media-id", msgType: "image"},
		{msgType: "file"},
	} {
		if err := validateChatMessageMediaSelection(test.mediaID, test.msgType, cmd); err != nil {
			t.Fatalf("validateChatMessageMediaSelection(%q, %q) = %v", test.mediaID, test.msgType, err)
		}
	}
}
