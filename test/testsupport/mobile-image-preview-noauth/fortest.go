// Package previewfixture contains only test boundaries for mobile-image-preview-noauth.
package previewfixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

const Env = "DWS_MOBILE_IMAGE_PREVIEW_NOAUTH"

type Caller struct {
	t             *testing.T
	Tools         []string
	Elements      []map[string]any
	ResourceIDs   []string
	ResourceURL   string
	Uploads       int
	FailStage     string
	OnCredentials func()
	UploadInfo    map[string]any
	RawUploadText string
	WrapReceipt   bool
	ReadbackURL   string
	OnReadback    func()
	Context       context.Context
	Result        any
}

func New(t *testing.T) *Caller {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("DINGTALK_AGENT", "Contract-Agent")
	caller := &Caller{t: t, ResourceURL: "https://media.example.test/not-the-resource-id"}
	helpers.InitDepsForTest(t, caller)
	helpers.SwapHTTPPutFileForTest(t, func(_ context.Context, url string, headers map[string]string, file string, size int64) error {
		caller.Uploads++
		info, err := os.Stat(file)
		if err != nil || info.Size() != size || url != "https://upload.example.test/put" || headers["Content-Type"] == "" {
			t.Fatalf("invalid OSS boundary: file=%q size=%d url=%q headers=%v err=%v", file, size, url, headers, err)
		}
		if caller.FailStage == "upload" {
			return errors.New("injected upload failure")
		}
		return nil
	})
	return caller
}

func (c *Caller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.Tools = append(c.Tools, tool)
	if product != "doc" {
		c.t.Fatalf("unexpected product %q", product)
	}
	if tool == c.FailStage {
		return nil, errors.New("injected " + tool + " failure")
	}
	var payload any
	switch tool {
	case "get_doc_attachment_upload_info":
		if c.OnCredentials != nil {
			c.OnCredentials()
		}
		id := fmt.Sprintf("12345678-1234-1234-1234-%012d", len(c.ResourceIDs)+1)
		info := map[string]any{"uploadUrl": "https://upload.example.test/put", "resourceId": id, "resourceUrl": c.ResourceURL}
		if c.UploadInfo != nil {
			info = c.UploadInfo
			id, _ = info["resourceId"].(string)
		}
		c.ResourceIDs = append(c.ResourceIDs, id)
		if c.RawUploadText != "" {
			return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.RawUploadText}}}, nil
		}
		payload = info
		if c.WrapReceipt {
			payload = map[string]any{"result": info}
		}
	case "insert_document_block":
		element, ok := args["element"].(map[string]any)
		if !ok || args["nodeId"] != "node-1" {
			c.t.Fatalf("invalid block insertion: %#v", args)
		}
		c.Elements = append(c.Elements, element)
		payload = map[string]any{"blockId": fmt.Sprintf("media-%d", len(c.Elements))}
	case "list_document_blocks":
		if args["format"] != "jsonml" {
			c.t.Fatalf("unexpected readback format: %#v", args)
		}
		blocks := make([]any, 0, len(c.Elements))
		for i, element := range c.Elements {
			if c.ReadbackURL != "" {
				element = map[string]any{"blockType": "paragraph", "children": []any{map[string]any{"elementType": "image", "properties": map[string]any{"src": c.ReadbackURL}}}}
			}
			blocks = append(blocks, map[string]any{"blockId": fmt.Sprintf("media-%d", i+1), "element": element})
		}
		payload = map[string]any{"blocks": blocks, "hasMore": false}
		if c.OnReadback != nil {
			c.OnReadback()
		}
	case "create_document":
		payload = map[string]any{"nodeId": "node-1"}
	case "get_document_content":
		payload = map[string]any{"markdown": "body"}
	default:
		c.t.Fatalf("unexpected additional request %q: %#v", tool, args)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(encoded)}}}, nil
}

func (*Caller) Format() string { return "json" }
func (*Caller) DryRun() bool   { return false }
func (*Caller) Fields() string { return "" }
func (*Caller) JQ() string     { return "" }

func AssertElement(t *testing.T, caller *Caller, index int, inline, hasID bool, mime, file string) {
	t.Helper()
	if len(caller.Elements) <= index || len(caller.ResourceIDs) <= index {
		t.Fatalf("missing upload/insertion at %d: ids=%v Elements=%#v", index, caller.ResourceIDs, caller.Elements)
	}
	resourceID := caller.ResourceIDs[index]
	var want map[string]any
	if inline {
		properties := map[string]any{"src": caller.ResourceURL}
		if hasID {
			properties["extraData"] = map[string]any{"resourceId": resourceID}
		}
		want = map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": ""}, "children": []any{map[string]any{"elementType": "image", "properties": properties}}}
	} else {
		want = map[string]any{"blockType": "attachment", "attachment": map[string]any{"resourceId": resourceID, "type": mime, "name": file, "viewType": "preview"}}
	}
	if !reflect.DeepEqual(caller.Elements[index], want) {
		t.Errorf("mobile-image-preview-noauth payload mismatch (inline=%t hasID=%t):\n got %#v\nwant %#v", inline, hasID, caller.Elements[index], want)
	}
}

func AssertCalls(t *testing.T, caller *Caller, created bool, count int) {
	t.Helper()
	var want []string
	if created {
		want = append(want, "create_document", "get_document_content")
	}
	for i := 0; i < count; i++ {
		want = append(want, "get_doc_attachment_upload_info", "insert_document_block", "list_document_blocks")
	}
	if !reflect.DeepEqual(caller.Tools, want) || caller.Uploads != count {
		t.Errorf("unexpected requests/retry: Tools=%v Uploads=%d, want Tools=%v Uploads=%d", caller.Tools, caller.Uploads, want, count)
	}
}

func MakeFile(t *testing.T, file string, size int64) {
	t.Helper()
	f, err := os.Create(filepath.Clean(file))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func RunShared(t *testing.T, caller *Caller, file, mime string, size int64) error {
	t.Helper()
	MakeFile(t, file, size)
	cmd := &cobra.Command{Use: "insert"}
	ctx := caller.Context
	if ctx == nil {
		ctx = context.Background()
	}
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.Flags().String("mime-type", mime, "")
	return helpers.InsertDocMediaFile(cmd, "node-1", file, func(result any) error {
		caller.Result = result
		return nil
	})
}

func MustRunShared(t *testing.T, caller *Caller, file, mime string, size int64) {
	t.Helper()
	if err := RunShared(t, caller, file, mime, size); err != nil {
		t.Fatalf("shared helper failed before payload assertion: %v", err)
	}
}
