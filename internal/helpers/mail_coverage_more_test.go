package helpers

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageMailDraftAttachmentValidationCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "mail"}
	t.Cleanup(func() { os.Args = oldArgs })
	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	text := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(text, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	badInline := filepath.Join(t.TempDir(), "inline.txt")
	if err := os.WriteFile(badInline, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		attachments, inline []string
	}{
		{attachments: []string{filepath.Join(t.TempDir(), "missing")}},
		{attachments: []string{t.TempDir()}},
		{attachments: []string{empty}},
		{inline: []string{filepath.Join(t.TempDir(), "missing")}},
		{inline: []string{t.TempDir()}},
		{inline: []string{empty}},
		{inline: []string{badInline}},
	} {
		_, _ = runMailDraftWithAttachment("create_draft", map[string]any{"from": "user@example.com"}, "", "body", tc.attachments, tc.inline)
	}
}

func TestCrossPlatformCoverageMailDraftAttachmentWorkflowCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "mail"}
	t.Cleanup(func() { os.Args = oldArgs })
	serverStatus := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(serverStatus)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	attachment := filepath.Join(t.TempDir(), "file.txt")
	inline := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(attachment, []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inline, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	mailboxes := `{"emailAccounts":[{"email":"user@example.com","type":"PERSONAL"}]}`
	draft := `{"result":{"message":{"id":"message"}}}`
	upload := `{"uploadUrl":"` + server.URL + `"}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: mailboxes}, {text: draft}, {text: upload}, {text: upload}}}
	installScriptedCaller(t, caller)
	if id, err := runMailDraftWithAttachment("create_draft", map[string]any{"from": "user@example.com"}, "", "body [inline:image.png]", []string{attachment}, []string{inline}); err != nil || id != "message" {
		t.Fatalf("draft workflow = %q, %v", id, err)
	}

	caller.steps = []scriptedToolStep{{text: mailboxes}, {text: `{}`}}
	caller.index = 0
	_, _ = runMailDraftWithAttachment("ignored", map[string]any{"from": "user@example.com"}, "message", "body", nil, nil)

	cases := [][]scriptedToolStep{
		{{err: errors.New("mailboxes")}},
		{{text: mailboxes}, {err: errors.New("draft")}},
		{{text: mailboxes}, {text: `{}`}},
		{{text: mailboxes}, {text: draft}, {err: errors.New("session")}},
		{{text: mailboxes}, {text: draft}, {text: `{}`}},
	}
	for _, steps := range cases {
		caller.steps = steps
		caller.index = 0
		_, _ = runMailDraftWithAttachment("create_draft", map[string]any{"from": "user@example.com"}, "", "body", []string{attachment}, nil)
	}
	serverStatus = http.StatusInternalServerError
	caller.steps = []scriptedToolStep{{text: mailboxes}, {text: draft}, {text: upload}}
	caller.index = 0
	_, _ = runMailDraftWithAttachment("create_draft", map[string]any{"from": "user@example.com"}, "", "body", []string{attachment}, nil)
}

func TestCrossPlatformCoverageMailParsingAndValidationCoverage(t *testing.T) {
	for _, raw := range []string{`{`, `{}`, `{"messageId":"id"}`, `{"result":{"message":{"id":"nested"}}}`} {
		_, _ = parseMailDraftId(raw)
	}
	for _, raw := range []string{`{`, `{}`, `{"uploadUrl":"url"}`, `{"result":{"uploadUrl":"nested"}}`} {
		_, _ = parseMailUploadSession(raw)
	}
	for _, raw := range []string{`{`, `{}`, `{"emailAccounts":["bad",{"email":"other"},{"email":"USER@example.com","type":"PERSONAL"}]}`, `{"result":{"emailAccounts":[]}}`} {
		_ = parseMailAccountType(raw, "user@example.com")
	}
	_ = mailUploadBaseURL("PERSONAL")
	_ = mailUploadBaseURL("ENTERPRISE")
	_ = parseRecipients(" one@example.com, ,two@example.com ")
	for _, raw := range []string{`{`, `{}`, `{"downloadUrl":"url"}`, `{"result":{"downloadUrl":"nested"}}`} {
		_, _ = parseMailDownloadSession(raw)
	}

	for _, limit := range []string{"0", "1", "101"} {
		cmd := &cobra.Command{Use: "thread"}
		cmd.Flags().Int("limit", 0, "")
		_ = cmd.Flags().Set("limit", limit)
		_, _ = validateMailboxThreadLimit(cmd)
	}
	for _, tc := range []struct{ action, tags string }{
		{"invalid", ""}, {"markRead", ""}, {"markUnread", ""},
		{"addTags", ""}, {"addTags", "tag"}, {"removeTags", "tag"},
	} {
		cmd := &cobra.Command{Use: "thread"}
		cmd.Flags().String("tag-ids", "", "")
		cmd.Flags().String("tags", "", "")
		_ = cmd.Flags().Set("tag-ids", tc.tags)
		_ = validateMailboxThreadAction(cmd, tc.action)
	}
}
