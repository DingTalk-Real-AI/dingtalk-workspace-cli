// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

// writeRelativeImage chdir's into a fresh temp dir and writes a small file with a
// supported avatar extension, returning its CWD-relative path (SafeInputPath only
// accepts relative paths within the working directory).
func writeRelativeImage(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(name, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	return "./" + name
}

func TestCrossPlatformCoverageDeapAgentAvatarURLInputBranches(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("ok.png", []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("dir.png", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("bad.txt", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("big.png", make([]byte, deapAgentAvatarMaxFileSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		in        string
		wantLocal bool
		wantErr   string
	}{
		{"", false, ""},
		{"https://cdn.example/a.png", false, ""},
		{"http://", false, "完整的 HTTP(S)"},
		{"/abs/path.png", true, "路径不安全"},
		{"bad.txt", true, "只支持"},
		{"missing.png", true, "不可读"},
		{"dir.png", true, "普通文件"},
		{"big.png", true, "10 MiB"},
		{"ok.png", true, ""},
	} {
		_, local, err := deapAgentValidateAvatarURLInput(tc.in)
		if tc.wantErr == "" {
			if err != nil {
				t.Fatalf("%q unexpected err %v", tc.in, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%q err=%v want contains %q", tc.in, err, tc.wantErr)
		}
		if local != tc.wantLocal {
			t.Fatalf("%q local=%v want %v", tc.in, local, tc.wantLocal)
		}
	}
}

func TestCrossPlatformCoverageDeapAgentAvatarStageErrorFormatting(t *testing.T) {
	var nilErr *deapAgentAvatarStageError
	if nilErr.Error() != "头像处理失败" {
		t.Fatalf("nil error message = %q", nilErr.Error())
	}
	if nilErr.Unwrap() != nil {
		t.Fatal("nil unwrap must be nil")
	}
	boom := errors.New("boom")
	withAgent := &deapAgentAvatarStageError{Stage: "上传", AgentUUID: "agent-1", Err: boom}
	msg := withAgent.Error()
	if !strings.Contains(msg, "agent-1") || !strings.Contains(msg, "boom") || !strings.Contains(msg, "请勿重复创建") {
		t.Fatalf("with-agent message = %q", msg)
	}
	if !errors.Is(withAgent.Unwrap(), boom) {
		t.Fatal("unwrap must return the wrapped error")
	}
	noAgent := &deapAgentAvatarStageError{Stage: "上传", Err: boom}
	if !strings.Contains(noAgent.Error(), "头像上传失败") || strings.Contains(noAgent.Error(), "agentUuid") {
		t.Fatalf("no-agent message = %q", noAgent.Error())
	}
	noDetail := &deapAgentAvatarStageError{Stage: "创建结果解析"}
	if !strings.Contains(noDetail.Error(), "创建结果解析") {
		t.Fatalf("no-detail message = %q", noDetail.Error())
	}
}

func TestCrossPlatformCoverageDeapAgentParseCreatedUUIDBranches(t *testing.T) {
	if _, err := deapAgentParseCreatedUUID("{"); err == nil || !strings.Contains(err.Error(), "格式非法") {
		t.Fatalf("invalid json err=%v", err)
	}
	for _, tc := range []struct{ in, want string }{
		{`{"agentUuid":"a"}`, "a"},
		{`{"data":{"agentUuid":"b"}}`, "b"},
		{`{"result":{"agentUuid":"c"}}`, "c"},
		{`{"content":{"agentUuid":"d"}}`, "d"},
	} {
		got, err := deapAgentParseCreatedUUID(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%s -> %q %v", tc.in, got, err)
		}
	}
	if _, err := deapAgentParseCreatedUUID(`{"data":{}}`); err == nil || !strings.Contains(err.Error(), "缺少 agentUuid") {
		t.Fatalf("missing uuid err=%v", err)
	}
}

func TestCrossPlatformCoverageDeapAgentProfileHelpersAndInvalidType(t *testing.T) {
	args := map[string]any{
		"digitalTagEmployeeProfile":                  map[string]any{"keep": "v"},
		"digitalTagEmployeeProfile.supervisorUserId": "u-1",
		"digitalTagEmployeeProfile.responseMode":     "mention_only",
	}
	deapAgentPrepareProfile(args)
	prof, _ := args["digitalTagEmployeeProfile"].(map[string]any)
	if prof["keep"] != "v" || prof["supervisorUserId"] != "u-1" || prof["responseMode"] != "mention_only" {
		t.Fatalf("prepared profile = %#v", prof)
	}
	if _, ok := args["digitalTagEmployeeProfile.responseMode"]; ok {
		t.Fatal("flat response-mode key was not consumed")
	}
	empty := map[string]any{}
	deapAgentPrepareProfile(empty)
	if _, ok := empty["digitalTagEmployeeProfile"]; ok {
		t.Fatal("empty args must not add a profile key")
	}
	if err := deapAgentInvalidMainProgramType(); err == nil || !strings.Contains(err.Error(), "open_code") {
		t.Fatalf("invalid main program type err=%v", err)
	}

	caller, _ := newDeapAgentTestTree(t, false)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := deapAgentCallWithProfile(cmd, deapAgentDetailTool, map[string]any{"agentUuid": "a-1"}); err != nil {
		t.Fatalf("CallWithProfile err=%v", err)
	}
	if err := deapAgentCallWithProfileAndDraftFiles(cmd, deapAgentSaveDraftTool, map[string]any{"agentUuid": "a-1"}); err != nil {
		t.Fatalf("CallWithProfileAndDraftFiles err=%v", err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(caller.calls))
	}
}

func TestCrossPlatformCoverageDeapAgentSaveDraftResponseModeAndAvatarValidate(t *testing.T) {
	ok := &cobra.Command{}
	ok.Flags().String("main-program-type", "", "")
	ok.Flags().String("response-mode", "", "")
	if err := ok.Flags().Set("main-program-type", "open_code"); err != nil {
		t.Fatal(err)
	}
	if err := ok.Flags().Set("response-mode", "mention_only"); err != nil {
		t.Fatal(err)
	}
	if err := deapAgentValidateSaveDraftResponseMode(ok); err != nil {
		t.Fatalf("open_code with response-mode must pass, got %v", err)
	}
	missing := &cobra.Command{}
	missing.Flags().String("main-program-type", "", "")
	missing.Flags().String("response-mode", "", "")
	if err := missing.Flags().Set("main-program-type", "open_code"); err != nil {
		t.Fatal(err)
	}
	if err := deapAgentValidateSaveDraftResponseMode(missing); err == nil {
		t.Fatal("open_code without response-mode must fail")
	}

	// save-draft leaf Validate rejects an invalid avatar-url before any write.
	newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})
	save := deapFindLeaf(t, root, "save-draft")
	save.Flags().Bool("yes", false, "test confirmation")
	for name, value := range map[string]string{"agent-uuid": "a-1", "avatar-url": "http://", "yes": "true"} {
		if err := save.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := save.RunE(save, nil); err == nil || !strings.Contains(err.Error(), "avatar-url") {
		t.Fatalf("save-draft avatar validation err=%v", err)
	}
}

func TestCrossPlatformCoverageDeapAgentCreateAvatarCallBranches(t *testing.T) {
	t.Run("validate error", func(t *testing.T) {
		newDeapAgentTestTree(t, false)
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		err := deapAgentCallCreateWithAvatar(cmd, deapAgentCreateTool, map[string]any{"avatarUrl": "http://"})
		if err == nil || !strings.Contains(err.Error(), "avatar-url") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("dry-run preview", func(t *testing.T) {
		rel := writeRelativeImage(t, "a.png")
		_, out := newDeapAgentTestTree(t, true)
		uploader := &deapAgentAvatarUploaderStub{fileURL: "https://oss/a.png"}
		testseam.Swap(t, &deapAgentAvatarFileUploader, deapAgentAvatarUploader(uploader))
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		if err := deapAgentCallCreateWithAvatar(cmd, deapAgentCreateTool, map[string]any{"avatarUrl": rel, "name": "n"}); err != nil {
			t.Fatalf("dry-run err=%v", err)
		}
		if !strings.Contains(out.String(), "create_then_"+deapAgentAvatarUploadAction) || !strings.Contains(out.String(), "dryRun") {
			t.Fatalf("dry-run preview = %q", out.String())
		}
		if uploader.gotPath != "" {
			t.Fatalf("dry-run must not upload, got %q", uploader.gotPath)
		}
	})
	t.Run("create call error", func(t *testing.T) {
		rel := writeRelativeImage(t, "a.png")
		caller, _ := newDeapAgentTestTree(t, false)
		caller.err = errors.New("create transport down")
		testseam.Swap(t, &deapAgentAvatarFileUploader, deapAgentAvatarUploader(&deapAgentAvatarUploaderStub{fileURL: "https://oss/a.png"}))
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		if err := deapAgentCallCreateWithAvatar(cmd, deapAgentCreateTool, map[string]any{"avatarUrl": rel}); err == nil {
			t.Fatal("create transport error not propagated")
		}
	})
	t.Run("parse uuid error", func(t *testing.T) {
		rel := writeRelativeImage(t, "a.png")
		caller, _ := newDeapAgentTestTree(t, false)
		caller.resultText = "{"
		testseam.Swap(t, &deapAgentAvatarFileUploader, deapAgentAvatarUploader(&deapAgentAvatarUploaderStub{fileURL: "https://oss/a.png"}))
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		err := deapAgentCallCreateWithAvatar(cmd, deapAgentCreateTool, map[string]any{"avatarUrl": rel})
		if err == nil || !strings.Contains(err.Error(), "创建结果解析") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("upload error", func(t *testing.T) {
		rel := writeRelativeImage(t, "a.png")
		caller, _ := newDeapAgentTestTree(t, false)
		caller.resultText = `{"data":{"agentUuid":"agent-x"}}`
		testseam.Swap(t, &deapAgentAvatarFileUploader, deapAgentAvatarUploader(&deapAgentAvatarUploaderStub{err: errors.New("upload denied")}))
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		err := deapAgentCallCreateWithAvatar(cmd, deapAgentCreateTool, map[string]any{"avatarUrl": rel})
		if err == nil || !strings.Contains(err.Error(), "上传") || !strings.Contains(err.Error(), "agent-x") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestCrossPlatformCoverageDeapAgentSaveAvatarCallBranches(t *testing.T) {
	t.Run("prepare draft files error", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		if err := os.WriteFile("bad.json", []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		newDeapAgentTestTree(t, false)
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		if err := deapAgentCallSaveWithAvatar(cmd, deapAgentSaveDraftTool, map[string]any{"skillsFile": "./bad.json"}); err == nil {
			t.Fatal("malformed skills file must fail prepareDraftFiles")
		}
	})
	t.Run("validate error", func(t *testing.T) {
		newDeapAgentTestTree(t, false)
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		err := deapAgentCallSaveWithAvatar(cmd, deapAgentSaveDraftTool, map[string]any{"avatarUrl": "http://"})
		if err == nil || !strings.Contains(err.Error(), "avatar-url") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("dry-run preview", func(t *testing.T) {
		rel := writeRelativeImage(t, "a.png")
		_, out := newDeapAgentTestTree(t, true)
		uploader := &deapAgentAvatarUploaderStub{fileURL: "https://oss/a.png"}
		testseam.Swap(t, &deapAgentAvatarFileUploader, deapAgentAvatarUploader(uploader))
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		if err := deapAgentCallSaveWithAvatar(cmd, deapAgentSaveDraftTool, map[string]any{"avatarUrl": rel, "agentUuid": "a-1"}); err != nil {
			t.Fatalf("dry-run err=%v", err)
		}
		if !strings.Contains(out.String(), deapAgentAvatarUploadAction) || !strings.Contains(out.String(), "dryRun") {
			t.Fatalf("dry-run preview = %q", out.String())
		}
	})
	t.Run("upload error", func(t *testing.T) {
		rel := writeRelativeImage(t, "a.png")
		newDeapAgentTestTree(t, false)
		testseam.Swap(t, &deapAgentAvatarFileUploader, deapAgentAvatarUploader(&deapAgentAvatarUploaderStub{err: errors.New("upload denied")}))
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		err := deapAgentCallSaveWithAvatar(cmd, deapAgentSaveDraftTool, map[string]any{"avatarUrl": rel, "agentUuid": "a-1"})
		if err == nil || !strings.Contains(err.Error(), "上传") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestCrossPlatformCoverageDeapAgentAvatarUploaderDelegates(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(file, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `{"result":{"fileUrl":"https://oss.example/avatar.png"}}`)
	}))
	defer server.Close()
	delegate := deapAgentOpenAPISkillUploader{
		baseURL:        server.URL,
		httpClient:     server.Client(),
		validateTarget: func(string) error { return nil },
		resolveCredential: func(context.Context, string) (string, error) {
			return "fixture-credential", nil
		},
	}
	got, err := deapAgentOpenAPIAvatarUploader{delegate: delegate}.Upload(context.Background(), "agent-1", file)
	if err != nil || got != "https://oss.example/avatar.png" {
		t.Fatalf("avatar uploader delegate got=%q err=%v", got, err)
	}
}

func TestCrossPlatformCoverageDeapAgentSkillUploadCredentialMCPBranches(t *testing.T) {
	// No injected resolver -> the real MCP credential path is exercised.
	uploader := deapAgentOpenAPISkillUploader{}
	caller, _ := newDeapAgentTestTree(t, false)

	// MCP call error preserves the server-side cause (no credential exists yet).
	caller.err = errors.New("credential tool unavailable")
	if _, err := uploader.temporaryCredential(context.Background(), "agent-1"); err == nil ||
		!strings.Contains(err.Error(), "获取上传凭证失败") {
		t.Fatalf("mcp call error not preserved: %v", err)
	}

	// Success parses the credential envelope.
	caller.err = nil
	caller.resultText = `{"success":true,"data":{"temporaryApiKey":"sk-fixture","expireAt":123}}`
	if got, err := uploader.temporaryCredential(context.Background(), "agent-1"); err != nil || got != "sk-fixture" {
		t.Fatalf("credential parse got=%q err=%v", got, err)
	}

	// Incomplete credential surfaces the parse failure through the same wrapper.
	caller.resultText = `{"success":true,"data":{}}`
	if _, err := uploader.temporaryCredential(context.Background(), "agent-1"); err == nil ||
		!strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete credential err=%v", err)
	}
}
