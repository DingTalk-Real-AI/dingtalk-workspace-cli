package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestChatHelpRendersReviewedSelectionToStderr(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	chat := &cobra.Command{Use: "chat"}
	message := &cobra.Command{Use: "message"}
	search := &cobra.Command{Use: "search", Short: "search messages"}
	root.AddCommand(chat)
	chat.AddCommand(message)
	message.AddCommand(search)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	configureRootHelp(root)

	if err := search.Help(); err != nil {
		t.Fatalf("search.Help() error = %v", err)
	}
	got := stderr.String()
	for _, want := range []string{
		"Agent guidance:",
		"Use when:",
		"Avoid when:",
		"Example:",
		"--format json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr = %q, want %q", got, want)
		}
	}
}

func TestNonChatHelpDoesNotRenderChatSelection(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	dev := &cobra.Command{Use: "dev"}
	app := &cobra.Command{Use: "app"}
	list := &cobra.Command{Use: "list", Short: "list apps"}
	root.AddCommand(dev)
	dev.AddCommand(app)
	app.AddCommand(list)

	var stderr bytes.Buffer
	root.SetErr(&stderr)
	configureRootHelp(root)

	if err := list.Help(); err != nil {
		t.Fatalf("list.Help() error = %v", err)
	}
	if strings.Contains(stderr.String(), "Agent guidance:") {
		t.Fatalf("non-chat help unexpectedly rendered chat guidance: %q", stderr.String())
	}
}
