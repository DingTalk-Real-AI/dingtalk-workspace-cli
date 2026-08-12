package app

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

// TestChatDownloadMediaAliasPreRunNormalizesRequiredFlag is the root-level
// regression for the alias normalization order: root's persistent pre-run must
// not run Cobra's required-flag validation ahead of the leaf PreRunE.
// chat message download-media copies --msg-id / --open-message-id into the
// required --message-id flag in its PreRunE; validating early failed that
// documented alias path with "missing required flag(s): --message-id".
func TestChatDownloadMediaAliasPreRunNormalizesRequiredFlag(t *testing.T) {
	for _, alias := range []string{"msg-id", "open-message-id"} {
		t.Run(alias, func(t *testing.T) {
			root := NewRootCommand(context.Background())
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			target := filepath.Join(t.TempDir(), "download.bin")
			root.SetArgs([]string{
				"chat", "message", "download-media",
				"--type", "mediaId",
				"--resource-id", "media-1",
				"--" + alias, "msg-1",
				"--open-conversation-id", "cid-1",
				"--output", target,
				"--dry-run", "--format", "json",
			})
			if _, err := root.ExecuteC(); err != nil {
				t.Fatalf("ExecuteC with alias --%s: %v\nstdout: %s\nstderr: %s", alias, err, stdout.String(), stderr.String())
			}
		})
	}
}
