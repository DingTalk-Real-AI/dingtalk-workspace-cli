package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// All switch outcomes use identical CLI arguments and preserve the V2 ID.
func TestCrossPlatformCoverageResourceDownloadV2CredentialSwitchMatrix(t *testing.T) {
	for _, mode := range []string{"legacy", "OSS_PRE_SIGNED", "CENTER_PROXY"} {
		t.Run(mode, func(t *testing.T) {
			t.Chdir(t.TempDir())
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				wantToken := ""
				if mode == "CENTER_PROXY" {
					wantToken = "fixture-token"
				}
				if r.Header.Get("media-token") != wantToken {
					t.Errorf("media-token=%q, want %q", r.Header.Get("media-token"), wantToken)
				}
				_, _ = w.Write([]byte("image-bytes"))
			}))
			defer server.Close()
			testseam.Swap(t, &resourceSecureClient, func() *http.Client { return server.Client() })
			data := map[string]any{"downloadUrl": server.URL + "/image.png?signature=fixture", "expireInSeconds": 3600}
			if mode != "legacy" {
				data["downloadType"] = mode
				data["expireTime"] = time.Now().Add(time.Hour).UnixMilli()
			}
			if mode == "CENTER_PROXY" {
				data["headers"] = map[string]string{"media-token": "fixture-token"}
			}
			payload, err := json.Marshal(map[string]any{"result": data})
			if err != nil {
				t.Fatal(err)
			}
			caller := &larkAlignmentCaller{responses: map[string]string{
				"im/get_resource_download_url": string(payload),
				"im/list_messages_by_ids":      `{"result":[{"openMessageId":"msg","openConversationId":"cid","content":"![图]($iwElAqNqcGcDAQTR)"}]}`,
			}}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{"chat", "+messages-resource-download", "--type", "mediaId", "--resource-id", "$iwElAqNqcGcDAQTR", "--message-id", "msg", "--open-conversation-id", "cid", "--output", "image.png"})
			if err := corecmd.ExecuteContextForTest(root, t.Context()); err != nil {
				t.Fatal(err)
			}
			bytes, err := os.ReadFile("image.png")
			if err != nil || string(bytes) != "image-bytes" || requests != 1 {
				t.Fatalf("bytes=%q err=%v requests=%d", bytes, err, requests)
			}
			if len(caller.calls) != 2 || caller.calls[0].tool != "list_messages_by_ids" || caller.calls[1].tool != "get_resource_download_url" {
				t.Fatalf("calls=%#v", caller.calls)
			}
			args := caller.calls[1].args
			if args["resourceId"] != "$iwElAqNqcGcDAQTR" || args["resourceType"] != "mediaId" || args["openMessageId"] != "msg" || args["openConversationId"] != "cid" {
				t.Fatalf("args=%#v", args)
			}
		})
	}
}
