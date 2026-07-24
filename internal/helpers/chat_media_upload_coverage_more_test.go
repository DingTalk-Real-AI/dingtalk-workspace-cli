package helpers

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChatMediaUploadIsDeprecatedCompatibilityStub(t *testing.T) {
	group := newChatMediaGroup()
	if group.Deprecated == "" || group.Hidden || !group.Runnable() {
		t.Fatalf("media group compatibility contract: deprecated=%q hidden=%v runnable=%v", group.Deprecated, group.Hidden, group.Runnable())
	}

	cmd := newChatMediaUploadCommand()
	if cmd.Deprecated == "" || cmd.Hidden || !cmd.Runnable() {
		t.Fatalf("media upload compatibility contract: deprecated=%q hidden=%v runnable=%v", cmd.Deprecated, cmd.Hidden, cmd.Runnable())
	}
	for _, flag := range []string{"file", "type"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("media upload lost historical --%s flag", flag)
		}
	}

	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--file", "/path/that/does/not/exist.png", "--type", "image"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("deprecated media upload returned nil error")
	}
	for _, want := range []string{"已下线", "chat message send", "--msg-type file", "--file-path", "--media-id"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("media upload migration error missing %q: %v", want, err)
		}
	}
}
