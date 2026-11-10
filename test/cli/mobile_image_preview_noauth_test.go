package cli_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	docshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/doc"
	previewfixture "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/testsupport/mobile-image-preview-noauth"
	"github.com/spf13/cobra"
)

// Current main moved --media-files from +create to +create-with-media.
// Exercise the real public declaration, command preparation, confirmation and
// shared helper; replace only MCP and OSS boundaries using existing test seams.
func TestCrossPlatformCoverageMobileImagePreviewNoauthShortcutEntries(t *testing.T) {
	t.Run("mobile-image-preview-noauth", func(t *testing.T) {
		for _, entry := range []string{"+media-insert", "+create-with-media"} {
			for _, enabled := range []bool{false, true} {
				for _, receipt := range []struct {
					name               string
					wrapped, missingID bool
				}{
					{"bare", false, false}, {"result", true, false},
					{"bare-src-only", false, true}, {"result-src-only", true, true},
				} {
					t.Run(fmt.Sprintf("%s/legacy-env-%t/%s", entry, enabled, receipt.name), func(t *testing.T) {
						value := "0"
						if enabled {
							value = "1"
						}
						t.Setenv(previewfixture.Env, value)
						caller := previewfixture.New(t)
						caller.WrapReceipt = receipt.wrapped
						if receipt.missingID {
							caller.UploadInfo = map[string]any{"uploadUrl": "https://upload.example.test/put", "resourceUrl": caller.ResourceURL}
						}
						previewfixture.MakeFile(t, "asset.png", 4)
						declaration := docshortcut.MediaInsert
						args := []string{"--node", "node-1", "--file", "asset.png", "--mime-type", "image/png", "--yes"}
						if entry == "+create-with-media" {
							declaration = docshortcut.CreateWithMedia
							args = []string{"--name", "report", "--content", "body", "--media-files", "asset.png", "--yes"}
						}
						root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
						root.PersistentFlags().Bool("yes", false, "")
						root.PersistentFlags().Bool("dry-run", false, "")
						root.PersistentFlags().String("format", "json", "")
						service := &cobra.Command{Use: "doc"}
						service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
						root.AddCommand(service)
						root.SetOut(io.Discard)
						root.SetErr(io.Discard)
						ctx, _ := output.WithResultStore(context.Background())
						root.SetContext(ctx)
						root.SetArgs(append([]string{"doc", declaration.Command}, args...))
						if err := corecmd.ExecuteForTest(root); err != nil {
							t.Fatalf("%s failed before payload assertion: %v", entry, err)
						}
						previewfixture.AssertElement(t, caller, 0, true, !receipt.missingID, "image/png", "asset.png")
						previewfixture.AssertCalls(t, caller, entry == "+create-with-media", 1)
					})
				}
			}
		}
	})
}
