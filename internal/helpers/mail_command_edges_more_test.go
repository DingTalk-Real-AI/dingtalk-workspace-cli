package helpers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeMailEdge(t *testing.T, caller *scriptedToolCaller, args ...string) error {
	t.Helper()
	previous, previousArgs := deps, os.Args
	os.Args = append([]string{"dws", "mail"}, os.Args[1:]...)
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	defer func() {
		deps = previous
		os.Args = previousArgs
	}()

	cmd := newMailCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageMailSearchSendAndThreadEdges(t *testing.T) {
	valid := [][]string{
		{"message", "search", "--email", "a@example.com", "--query", "subject:x"},
		{"message", "list", "--email", "a@example.com"},
		{"message", "send", "--from", "a@example.com", "--to", "b@example.com", "--subject", "s", "--content", "body", "--cc", "c@example.com"},
	}
	for _, args := range valid {
		if err := executeMailEdge(t, &scriptedToolCaller{}, args...); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
	}
	if err := executeMailEdge(t, &scriptedToolCaller{}, "message", "send", "--from", "a@example.com", "--to", "b@example.com", "--subject", "s"); err == nil {
		t.Fatal("missing content returned nil")
	}

	many := strings.TrimSuffix(strings.Repeat("id,", 101), ",")
	for _, args := range [][]string{
		{"thread", "batch-update", "--email", "a@example.com", "--ids", many, "--action", "markRead"},
		{"thread", "trash", "--email", "a@example.com", "--id", "1"},
		{"thread", "batch-trash", "--email", "a@example.com", "--ids", "1"},
		{"thread", "batch-trash", "--email", "a@example.com", "--ids", many, "--yes"},
		{"message", "batch-update", "--email", "a@example.com", "--ids", "1", "--action", "addTags"},
		{"message", "batch-get", "--email", "a@example.com", "--ids", ","},
		{"message", "batch-get", "--email", "a@example.com", "--ids", strings.TrimSuffix(strings.Repeat("id,", 21), ",")},
	} {
		if err := executeMailEdge(t, &scriptedToolCaller{}, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
	if err := executeMailEdge(t, &scriptedToolCaller{dry: true}, "message", "batch-get", "--email", "a@example.com", "--ids", "1,2"); err != nil {
		t.Fatalf("dry batch get: %v", err)
	}
}

func TestCrossPlatformCoverageMailReplyForwardAndDraftEdges(t *testing.T) {
	draftSteps := func() []scriptedToolStep {
		return []scriptedToolStep{{text: `{}`}, {text: `{"result":{"message":{"id":"message"}}}`}, {text: `{}`}}
	}
	for _, args := range [][]string{
		{"message", "reply", "--from", "a@example.com", "--id", "1", "--to", "b@example.com", "--subject", "s", "--content", "body"},
		{"message", "reply-all", "--from", "a@example.com", "--id", "1", "--to", "b@example.com", "--subject", "s", "--content", "body"},
		{"message", "forward", "--from", "a@example.com", "--id", "1", "--to", "b@example.com", "--subject", "s", "--content", "body"},
	} {
		if err := executeMailEdge(t, &scriptedToolCaller{steps: draftSteps()}, args...); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
	}

	attachment := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(attachment, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalPut := mailPutAttachment
	mailPutAttachment = func(context.Context, string, string, string, int64) error { return nil }
	defer func() { mailPutAttachment = originalPut }()
	upload := `{"uploadUrl":"https://upload.invalid"}`

	if err := executeMailEdge(t, &scriptedToolCaller{}, "draft", "create", "--from", "a@example.com", "--subject", "s"); err != nil {
		t.Fatalf("plain create draft: %v", err)
	}
	createSteps := []scriptedToolStep{{text: `{}`}, {text: `{"messageId":"message"}`}, {text: upload}, {text: `{}`}}
	if err := executeMailEdge(t, &scriptedToolCaller{steps: createSteps}, "draft", "create", "--from", "a@example.com", "--subject", "s", "--attachment", attachment); err != nil {
		t.Fatalf("attachment create draft: %v", err)
	}
	if err := executeMailEdge(t, &scriptedToolCaller{}, "draft", "update", "--from", "a@example.com", "--id", "message"); err != nil {
		t.Fatalf("plain update draft: %v", err)
	}
	updateSteps := []scriptedToolStep{{text: `{}`}, {text: `{}`}, {text: upload}, {text: `{}`}}
	if err := executeMailEdge(t, &scriptedToolCaller{steps: updateSteps}, "draft", "update", "--from", "a@example.com", "--id", "message", "--attachment", attachment); err != nil {
		t.Fatalf("attachment update draft: %v", err)
	}

	sendSteps := []scriptedToolStep{{text: `{}`}, {text: `{"messageId":"message"}`}, {text: upload}, {text: `{}`}}
	if err := executeMailEdge(t, &scriptedToolCaller{steps: sendSteps}, "message", "send", "--from", "a@example.com", "--to", "b@example.com", "--subject", "s", "--content", "body", "--attachment", attachment); err != nil {
		t.Fatalf("attachment send: %v", err)
	}
}

func TestCrossPlatformCoverageMailRulesRecallAndTemplateEdges(t *testing.T) {
	if err := executeMailEdge(t, &scriptedToolCaller{}, "sent-message", "recall", "--email", "a@example.com", "--id", "1", "--subject", "s"); err == nil {
		t.Fatal("recall without confirmation returned nil")
	}
	if err := executeMailEdge(t, &scriptedToolCaller{}, "template", "create", "--email", "a@example.com", "--subject", "s", "--name", "n"); err == nil {
		t.Fatal("template missing content returned nil")
	}
	invalidParameter := `{"success":false,"message":"Invalid parameter"}`
	if err := executeMailEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: invalidParameter}}}, "template", "update", "--email", "a@example.com", "--id", "1"); err == nil {
		t.Fatal("template invalid parameter returned nil")
	}

	condition := `[{"object":"subject","or":[{"and":[{"operation":"include"}]}]}]`
	actions := `[{"action":"ActFlagMail2","parameters":["asread"]}]`
	for _, args := range [][]string{
		{"rule", "create", "--email", "a@example.com", "--name", "n", "--enabled", "true", "--conditions", condition, "--actions", actions},
		{"rule", "update", "--email", "a@example.com", "--id", "1", "--name", "n", "--enabled", "false", "--conditions", condition, "--actions", actions},
	} {
		if err := executeMailEdge(t, &scriptedToolCaller{}, args...); err != nil {
			t.Fatalf("rule %v: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"rule", "create", "--email", "a@example.com", "--name", "n", "--enabled", "true", "--conditions", `bad`, "--actions", actions},
		{"rule", "update", "--email", "a@example.com", "--id", "1", "--name", "n", "--enabled", "true", "--conditions", `bad`, "--actions", actions},
	} {
		if err := executeMailEdge(t, &scriptedToolCaller{}, args...); err == nil {
			t.Fatalf("bad rule %v returned nil", args)
		}
	}
}

func TestCrossPlatformCoverageMailDraftInlineFailureEdges(t *testing.T) {
	previousArgs := os.Args
	os.Args = []string{"dws", "mail"}
	defer func() { os.Args = previousArgs }()
	inline := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(inline, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	mailboxes := `{"emailAccounts":[{"email":"a@example.com","type":"PERSONAL"}]}`
	draft := `{"messageId":"message"}`
	upload := `{"uploadUrl":"https://upload.invalid"}`
	boom := errors.New("boom")

	for _, steps := range [][]scriptedToolStep{
		{{text: mailboxes}, {err: boom}},
		{{text: mailboxes}, {text: draft}, {err: boom}},
		{{text: mailboxes}, {text: draft}, {text: `{}`}},
	} {
		caller := &scriptedToolCaller{steps: steps}
		previous := deps
		InitDeps(caller)
		deps.Out.w, deps.Out.errW = io.Discard, io.Discard
		_, _ = runMailDraftWithAttachment("create_draft", map[string]any{"from": "a@example.com"}, "", "body", nil, []string{inline})
		deps = previous
	}

	previousPut := mailPutAttachment
	mailPutAttachment = func(context.Context, string, string, string, int64) error { return boom }
	defer func() { mailPutAttachment = previousPut }()
	previous := deps
	InitDeps(&scriptedToolCaller{steps: []scriptedToolStep{{text: mailboxes}, {text: draft}, {text: upload}}})
	deps.Out.w, deps.Out.errW = io.Discard, io.Discard
	_, _ = runMailDraftWithAttachment("create_draft", map[string]any{"from": "a@example.com"}, "", "body", nil, []string{inline})
	deps = previous

	previous = deps
	InitDeps(&scriptedToolCaller{steps: []scriptedToolStep{{text: mailboxes}, {err: boom}}})
	deps.Out.w, deps.Out.errW = io.Discard, io.Discard
	_, _ = runMailDraftWithAttachment("ignored", map[string]any{"from": "a@example.com"}, "message", "body", nil, nil)
	deps = previous
}

func TestCrossPlatformCoverageMailDownloadWorkflowEdges(t *testing.T) {
	previousArgs := os.Args
	os.Args = []string{"dws", "mail"}
	defer func() { os.Args = previousArgs }()
	makeCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "download"}
		cmd.Flags().String("email", "a@example.com", "")
		cmd.Flags().String("message-id", "message", "")
		cmd.Flags().String("attachment-id", "attachment", "")
		cmd.Flags().String("name", "file.txt", "")
		cmd.Flags().String("output", t.TempDir(), "")
		return cmd
	}
	boom := errors.New("boom")
	cases := [][]scriptedToolStep{
		{{err: boom}},
		{{text: `{}`}, {err: boom}},
		{{text: `{}`}, {text: `{}`}},
	}
	for _, steps := range cases {
		previous := deps
		InitDeps(&scriptedToolCaller{steps: steps})
		deps.Out.w, deps.Out.errW = io.Discard, io.Discard
		_ = runMailAttachmentDownload(makeCmd())
		deps = previous
	}

	previousGet := mailGetAttachment
	defer func() { mailGetAttachment = previousGet }()
	for _, getErr := range []error{boom, nil} {
		mailGetAttachment = func(context.Context, string, string, string) error { return getErr }
		previous := deps
		InitDeps(&scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}, {text: `{"downloadUrl":"https://download.invalid"}`}}})
		deps.Out.w, deps.Out.errW = io.Discard, io.Discard
		_ = runMailAttachmentDownload(makeCmd())
		deps = previous
	}
}

type mailRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mailRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type mailFailingReadCloser struct{}

func (mailFailingReadCloser) Read([]byte) (int, error) { return 0, errors.New("read") }
func (mailFailingReadCloser) Close() error             { return nil }

func TestCrossPlatformCoverageMailHTTPAndLimitRemainingEdges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := httpPutMailAttachment(context.Background(), "PERSONAL", "http://[::1", file, 4); err == nil {
		t.Fatal("invalid upload URL returned nil")
	}
	if err := httpGetMailAttachment(context.Background(), "PERSONAL", "http://[::1", filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("invalid download URL returned nil")
	}

	previousClient := mailHTTPClient
	defer func() { mailHTTPClient = previousClient }()
	mailHTTPClient = func() *http.Client {
		return &http.Client{Transport: mailRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("round trip")
		})}
	}
	if err := httpPutMailAttachment(context.Background(), "PERSONAL", "https://upload.invalid", file, 4); err == nil {
		t.Fatal("upload transport error returned nil")
	}

	mailHTTPClient = func() *http.Client {
		return &http.Client{Transport: mailRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: mailFailingReadCloser{}}, nil
		})}
	}
	if err := httpGetMailAttachment(context.Background(), "PERSONAL", "https://download.invalid", filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("download copy error returned nil")
	}

	if _, err := validateMailboxThreadLimit(&cobra.Command{Use: "missing"}); err == nil {
		t.Fatal("missing limit flag returned nil")
	}
}
