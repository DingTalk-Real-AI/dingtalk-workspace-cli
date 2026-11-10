package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

// The relaxed receipt compatibility is exclusive to document inline insertion.
// Exercise every other real parser caller so a shared-parser change cannot
// silently upload or write a resource that has no ID.
func TestCrossPlatformCoverageMobileImagePreviewNoauthOtherUploadsStayStrict(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, id := range []struct {
			name  string
			value any
			omit  bool
		}{
			{"omitted", nil, true}, {"null", nil, false}, {"empty", "", false},
			{"blank", " \t", false}, {"non-string", 7, false},
		} {
			for _, entry := range []string{"doc-media-upload", "doc-cover", "sheet-media-upload", "sheet-write-image"} {
				t.Run(entry+"/"+id.name+map[bool]string{false: "/bare", true: "/result"}[wrapped], func(t *testing.T) {
					info := map[string]any{"uploadUrl": "https://upload.example.test/put", "resourceUrl": "https://media.example.test/image"}
					if !id.omit {
						info["resourceId"] = id.value
					}
					var payload any = info
					if wrapped {
						payload = map[string]any{"result": info}
					}
					encoded, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: string(encoded)}}}
					installScriptedCaller(t, caller)
					puts := 0
					testseam.Swap(t, &httpPutFile, func(context.Context, string, map[string]string, string, int64) error { puts++; return nil })
					file := writeTempImage(t, "asset.png")
					var command *cobra.Command
					switch entry {
					case "doc-media-upload":
						command = newDocCommand()
						command.SetArgs([]string{"media", "upload", "--node", "node-1", "--file", file, "--yes"})
					case "doc-cover":
						command = newDocStyleCommand()
						command.SetArgs([]string{"cover", "set", "--node", "node-1", "--file", file, "--yes"})
					case "sheet-media-upload":
						command = newSheetCommand()
						command.SetArgs([]string{"media-upload", "--node", "node-1", "--file", file, "--yes"})
					case "sheet-write-image":
						command = newSheetCommand()
						command.SetArgs([]string{"write-image", "--node", "node-1", "--sheet-id", "sheet-1", "--range", "A1:A1", "--file", file, "--yes"})
					}
					if entry != "doc-media-upload" {
						command.PersistentFlags().Bool("yes", false, "")
					}
					command.SetOut(io.Discard)
					command.SetErr(io.Discard)
					if err := corecmd.ExecuteForTest(command); err == nil || !strings.Contains(err.Error(), "resourceId") {
						t.Fatalf("non-insert upload must reject receipt ID before writing: %v", err)
					}
					if puts != 0 || caller.calls != 1 || caller.tool != "get_doc_attachment_upload_info" {
						t.Fatalf("missing ID crossed write boundary: puts=%d calls=%v", puts, caller.toolLog)
					}
				})
			}
		}
	}
}

func TestCrossPlatformCoverageMobileImagePreviewNoauthWrongURLRemainsPartial(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(map[bool]string{false: "bare", true: "result"}[wrapped], func(t *testing.T) {
			text := `{"uploadUrl":"https://upload.example.test/put","resourceUrl":"https://media.example.test/a"}`
			if wrapped {
				text = `{"result":` + text + `}`
			}
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
				{text: text}, {text: `{"blockId":"media-1"}`},
				{text: `{"blocks":[{"blockId":"media-1","element":{"children":[{"elementType":"image","properties":{"src":"https://media.example.test/b"}}]}}],"hasMore":false}`},
			}}
			installScriptedCaller(t, caller)
			puts, waits := 0, 0
			testseam.Swap(t, &httpPutFile, func(context.Context, string, map[string]string, string, int64) error { puts++; return nil })
			testseam.Swap(t, &docMediaVerifyWait, func(context.Context, time.Duration) error { waits++; return nil })
			file := writeTempImage(t, "asset.png")
			command := &cobra.Command{Use: "insert"}
			command.SetContext(context.Background())
			emitted := false
			err := InsertDocMediaFile(command, "node-1", file, func(any) error { emitted = true; return nil })
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "doc_media_insert_verification_failed" || typed.Retryable || typed.Details["status"] != "partial_success" || typed.Details["verified"] != false {
				t.Fatalf("wrong URL lost bounded partial result: %v", err)
			}
			want := []string{"get_doc_attachment_upload_info", "insert_document_block", "list_document_blocks", "list_document_blocks", "list_document_blocks", "list_document_blocks", "list_document_blocks", "list_document_blocks", "list_document_blocks"}
			if puts != 1 || waits != 6 || emitted || !reflect.DeepEqual(caller.toolLog, want) {
				t.Fatalf("wrong URL retried mutation or skipped verification: puts=%d waits=%d emitted=%t tools=%v", puts, waits, emitted, caller.toolLog)
			}
		})
	}
}
