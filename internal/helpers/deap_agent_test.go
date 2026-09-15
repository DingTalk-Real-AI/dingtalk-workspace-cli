// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type deapAgentSkillUploaderStub struct {
	gotPath      string
	gotAgentUUID string
	fileURL      string
	err          error
}

func (s *deapAgentSkillUploaderStub) Upload(_ context.Context, agentUUID, filePath string) (string, error) {
	s.gotPath = filePath
	s.gotAgentUUID = agentUUID
	return s.fileURL, s.err
}

func TestDevDeapAgentSkillCreateUsesUploadFacadeAndSafeOutput(t *testing.T) {
	caller, output := newDeapAgentTestTree(t, false)
	tempDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	file, err := os.Create("skill.zip")
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("# Weather")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	fileURL := "https://signed.example/temporary-secret"
	uploader := &deapAgentSkillUploaderStub{fileURL: fileURL}
	testseam.Swap(t, &deapAgentSkillUploader, deapAgentSkillPackageUploader(uploader))
	caller.resultText = `{"success":true,"data":{"skillId":"skill-1","skill":{"skillId":"skill-1","name":"weather","displayName":"天气","description":"查询天气"}}}`
	deap := deapHandler{}.Command(&captureRunner{})
	create, rest, err := deap.Find([]string{"capability", "skill", "create"})
	if err != nil || len(rest) != 0 {
		t.Fatalf("find skill create: command=%v rest=%v err=%v", create, rest, err)
	}
	create.Flags().Bool("yes", false, "test confirmation")
	for name, value := range map[string]string{"agent-uuid": "agent-1", "file": "./skill.zip", "yes": "true"} {
		if err := create.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := create.RunE(create, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
	if !strings.HasSuffix(uploader.gotPath, "skill.zip") {
		t.Fatalf("upload facade file = %q", uploader.gotPath)
	}
	if uploader.gotAgentUUID != "agent-1" {
		t.Fatalf("upload facade agentUuid = %q", uploader.gotAgentUUID)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("local ZIP create made %d MCP calls, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != deapAgentServerID || call.toolName != "create_skill_by_url" {
		t.Fatalf("create route = %s/%s", call.productID, call.toolName)
	}
	if call.args["agentUuid"] != "agent-1" || call.args["fileUrl"] != fileURL {
		t.Fatalf("create args = %#v", call.args)
	}
	got := output.String()
	for _, want := range []string{"skill-1", "weather", "天气", "查询天气"} {
		if !strings.Contains(got, want) {
			t.Fatalf("safe create output missing %q: %s", want, got)
		}
	}
	for _, forbidden := range []string{"fileUrl", "uploadUrl", "credential", "secret"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("create output leaked forbidden field %q: %s", forbidden, got)
		}
	}
}

func TestDevDeapAgentSkillCreateLabelsStagesAndRedactsURLs(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	tempDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	if err := os.WriteFile("bad.zip", []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}

	deap := deapHandler{}.Command(&captureRunner{})
	invalid, _, err := deap.Find([]string{"capability", "skill", "create"})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"agent-uuid": "agent-1", "file": "./bad.zip"} {
		if err := invalid.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := invalid.RunE(invalid, nil); err == nil || !strings.Contains(err.Error(), "validate 阶段失败") {
		t.Fatalf("invalid package error = %v", err)
	}

	file, err := os.Create("valid.zip")
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("# Skill"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Run("upload", func(t *testing.T) {
		uploader := &deapAgentSkillUploaderStub{err: errors.New("request https://oss.example.test/object?token=secret failed")}
		testseam.Swap(t, &deapAgentSkillUploader, deapAgentSkillPackageUploader(uploader))
		command := deapHandler{}.Command(&captureRunner{})
		create, _, findErr := command.Find([]string{"capability", "skill", "create"})
		if findErr != nil {
			t.Fatal(findErr)
		}
		create.Flags().Bool("yes", false, "test confirmation")
		for name, value := range map[string]string{"agent-uuid": "agent-1", "file": "./valid.zip", "yes": "true"} {
			if setErr := create.Flags().Set(name, value); setErr != nil {
				t.Fatal(setErr)
			}
		}
		runErr := create.RunE(create, nil)
		if runErr == nil || !strings.Contains(runErr.Error(), "upload 阶段失败") {
			t.Fatalf("stage error = %v", runErr)
		}
		if strings.Contains(runErr.Error(), "oss.example") || strings.Contains(runErr.Error(), "token=secret") {
			t.Fatalf("stage error leaked temporary URL: %v", runErr)
		}
	})
	for _, stage := range []string{"create", "query"} {
		t.Run(stage, func(t *testing.T) {
			uploader := &deapAgentSkillUploaderStub{fileURL: "https://oss.example.test/object?token=secret"}
			testseam.Swap(t, &deapAgentSkillUploader, deapAgentSkillPackageUploader(uploader))
			caller.err = fmt.Errorf("skill create failed at %s stage: https://oss.example.test/object?token=secret", stage)
			t.Cleanup(func() { caller.err = nil })
			command := deapHandler{}.Command(&captureRunner{})
			create, _, findErr := command.Find([]string{"capability", "skill", "create"})
			if findErr != nil {
				t.Fatal(findErr)
			}
			create.Flags().Bool("yes", false, "test confirmation")
			for name, value := range map[string]string{"agent-uuid": "agent-1", "file": "./valid.zip", "yes": "true"} {
				if setErr := create.Flags().Set(name, value); setErr != nil {
					t.Fatal(setErr)
				}
			}
			runErr := create.RunE(create, nil)
			if runErr == nil || !strings.Contains(runErr.Error(), stage+" 阶段失败") {
				t.Fatalf("stage error = %v", runErr)
			}
			if strings.Contains(runErr.Error(), "oss.example") || strings.Contains(runErr.Error(), "token=secret") {
				t.Fatalf("stage error leaked temporary URL: %v", runErr)
			}
		})
	}
}

func TestDeapAgentOpenAPISkillUploaderStreamsMultipartAndReturnsFileURL(t *testing.T) {
	tempDir := t.TempDir()
	filePath := tempDir + "/skill.zip"
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("# Skill"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1.0/assistant/skills/upload" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-upload" {
			t.Errorf("authorization header = %q", got)
		}
		if got := r.Header.Get(apiclient.AuthHeader); got != "" {
			t.Errorf("OAuth auth header must be empty, got %q", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm() error = %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if got := r.FormValue("agentUuid"); got != "" {
			t.Errorf("upload unexpectedly bound agentUuid = %q", got)
		}
		part, header, openErr := r.FormFile("file")
		if openErr != nil {
			t.Errorf("FormFile() error = %v", openErr)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer part.Close()
		body, readErr := io.ReadAll(part)
		if readErr != nil || len(body) == 0 || header.Filename != "skill.zip" {
			t.Errorf("uploaded file name=%q bytes=%d err=%v", header.Filename, len(body), readErr)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"fileUrl":"https://signed.example/temp"}`)
	}))
	defer server.Close()
	testTransport := server.Client().Transport
	testClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "https"
		clone.URL.Host = strings.TrimPrefix(server.URL, "https://")
		return testTransport.RoundTrip(clone)
	})}

	uploader := deapAgentOpenAPISkillUploader{
		baseURL:    "https://api-deap.dingtalk.com",
		httpClient: testClient,
		resolveCredential: func(_ context.Context, agentUUID string) (string, error) {
			if agentUUID != "agent-1" {
				t.Fatalf("credential resolver agentUuid = %q", agentUUID)
			}
			return "sk-upload", nil
		},
	}
	fileURL, err := uploader.Upload(context.Background(), "agent-1", filePath)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if fileURL != "https://signed.example/temp" {
		t.Fatalf("Upload() fileUrl = %q", fileURL)
	}
}

func TestDeapAgentParseSkillUploadCredential(t *testing.T) {
	for _, raw := range []string{
		`{"temporaryApiKey":"sk-top","expireAt":1}`,
		`{"success":true,"data":{"temporaryApiKey":"sk-data","expireAt":2}}`,
		`{"result":{"temporaryApiKey":"sk-result","expireAt":3}}`,
	} {
		credential, err := deapAgentParseSkillUploadCredential(raw)
		if err != nil {
			t.Fatalf("deapAgentParseSkillUploadCredential(%s) error = %v", raw, err)
		}
		if !strings.HasPrefix(credential, "sk-") {
			t.Fatalf("credential = %q", credential)
		}
	}
	for _, raw := range []string{
		`{"success":false}`,
		`{"temporaryApiKey":"","expireAt":1}`,
		`{"temporaryApiKey":"sk-missing-expiry"}`,
	} {
		if _, err := deapAgentParseSkillUploadCredential(raw); err == nil {
			t.Fatalf("deapAgentParseSkillUploadCredential(%s) unexpectedly succeeded", raw)
		}
	}
}

func TestDeapAgentTemporaryCredentialUsesPublishedToolName(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	caller.resultText = `{"success":true,"data":{"temporaryApiKey":"sk-upload","expireAt":2}}`

	credential, err := (deapAgentOpenAPISkillUploader{}).
		temporaryCredential(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("temporaryCredential() error = %v", err)
	}
	if credential != "sk-upload" {
		t.Fatalf("temporaryCredential() = %q", credential)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("MCP call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != deapAgentServerID || call.toolName != "create_skill_upload_credential" {
		t.Fatalf("credential route = %s/%s", call.productID, call.toolName)
	}
	if call.args["agentUuid"] != "agent-1" {
		t.Fatalf("credential args = %#v", call.args)
	}
}

func TestDeapAgentSkillUploadBaseURLFollowsMCPEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mcpURL  string
		wantURL string
		wantErr bool
	}{
		{name: "production", mcpURL: "https://mcp.dingtalk.com", wantURL: deapAgentSkillUploadProdBase},
		{name: "pre", mcpURL: "https://pre-mcp.dingtalk.com", wantURL: deapAgentSkillUploadPreBase},
		{name: "pre gateway", mcpURL: "https://pre-mcp-gw.example.net", wantURL: deapAgentSkillUploadPreBase},
		{name: "custom fails closed", mcpURL: "https://custom.example.net", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", configDir)
			if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(tc.mcpURL), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := (deapAgentOpenAPISkillUploader{}).uploadBaseURL()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("uploadBaseURL() = %q, want error", got)
				}
				return
			}
			if err != nil || got != tc.wantURL {
				t.Fatalf("uploadBaseURL() = %q, %v; want %q", got, err, tc.wantURL)
			}
		})
	}
}

func TestDeapAgentSkillPackageValidation(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	writeZIP := func(name string, entries map[string]string) {
		t.Helper()
		file, createErr := os.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		writer := zip.NewWriter(file)
		for entryName, body := range entries {
			entry, entryErr := writer.Create(entryName)
			if entryErr != nil {
				t.Fatal(entryErr)
			}
			if _, entryErr := entry.Write([]byte(body)); entryErr != nil {
				t.Fatal(entryErr)
			}
		}
		if closeErr := writer.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}

	writeZIP("valid.zip", map[string]string{"SKILL.md": "# Example", "scripts/run.sh": "echo ok"})
	if _, err := deapAgentValidateSkillPackage("./valid.zip"); err != nil {
		t.Fatalf("valid package rejected: %v", err)
	}

	writeZIP("wrong.tar", map[string]string{"SKILL.md": "# Example"})
	writeZIP("missing-skill.zip", map[string]string{"README.md": "missing"})
	writeZIP("traversal.zip", map[string]string{"SKILL.md": "# Example", "../escape.sh": "bad"})
	if err := os.WriteFile("corrupt.zip", []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("directory.zip", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("too-large.zip", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate("too-large.zip", deapAgentSkillMaxPackageSize+1); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path    string
		wantErr string
	}{
		{"./wrong.tar", ".zip"},
		{"./directory.zip", "普通文件"},
		{"./corrupt.zip", "ZIP"},
		{"./too-large.zip", "50 MiB"},
		{"./missing-skill.zip", "SKILL.md"},
		{"./traversal.zip", "路径"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if _, err := deapAgentValidateSkillPackage(tc.path); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validate(%q) error = %v, want containing %q", tc.path, err, tc.wantErr)
			}
		})
	}
}

type deapAgentCall struct {
	productID string
	toolName  string
	args      map[string]any
}

func TestDevDeapAgentSkillAndMCPCommandsRouteFrozenContracts(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	tempDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	if err := os.WriteFile("mcp.json", []byte(`{"name":"weather","description":"查询天气","detailIntro":"天气 MCP","userQuestionTips":["请输入城市"],"configType":"JSON","configString":"{\"url\":\"https://mcp.example.test\",\"token\":\"secret\"}","envs":{"API_TOKEN":"env-secret"},"toolsDisabled":{"search":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("skills.json", []byte(`[{"skillId":"skill-1","enabled":true,"attributes":{"configDefinitions":{"city":"hangzhou"}}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("mcps.json", []byte(`[{"mcpId":"mcp-1","enabled":true,"config":{"credentialRef":"cred-1"}}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path      []string
		tool      string
		flags     map[string]string
		wantArgs  map[string]any
		confirmed bool
	}{
		{path: []string{"capability", "skill", "list"}, tool: "list_skills", flags: map[string]string{"agent-uuid": "agent-1"}, wantArgs: map[string]any{"agentUuid": "agent-1", "snapshot": "draft"}},
		{path: []string{"capability", "skill", "query"}, tool: "query_skill", flags: map[string]string{"agent-uuid": "agent-1", "skill-id": "skill-1", "snapshot": "published"}, wantArgs: map[string]any{"agentUuid": "agent-1", "skillId": "skill-1", "snapshot": "published"}},
		{path: []string{"capability", "mcp", "create"}, tool: "create_mcp", flags: map[string]string{"agent-uuid": "agent-1", "config-file": "./mcp.json"}, wantArgs: map[string]any{"agentUuid": "agent-1", "name": "weather", "description": "查询天气", "detailIntro": "天气 MCP", "userQuestionTips": []any{"请输入城市"}, "configType": "JSON", "configString": `{"url":"https://mcp.example.test","token":"secret"}`, "envs": map[string]any{"API_TOKEN": "env-secret"}, "toolsDisabled": map[string]any{"search": false}}, confirmed: true},
		{path: []string{"capability", "mcp", "list"}, tool: "list_mcps", flags: map[string]string{"agent-uuid": "agent-1"}, wantArgs: map[string]any{"agentUuid": "agent-1", "keywords": "", "page": 1, "pageSize": 20}},
		{path: []string{"capability", "mcp", "query"}, tool: "query_mcp", flags: map[string]string{"agent-uuid": "agent-1", "mcp-id": "mcp-1"}, wantArgs: map[string]any{"agentUuid": "agent-1", "mcpId": "mcp-1"}},
		{path: []string{"manage", "detail"}, tool: "get_digital_employee_detail", flags: map[string]string{"agent-uuid": "agent-1", "type": "published"}, wantArgs: map[string]any{"agentUuid": "agent-1", "type": "published"}},
		{path: []string{"manage", "save-draft"}, tool: "update_digital_employee_draft", flags: map[string]string{"agent-uuid": "agent-1", "skills-file": "./skills.json", "mcps-file": "./mcps.json"}, wantArgs: map[string]any{"agentUuid": "agent-1", "skills": []any{map[string]any{"skillId": "skill-1", "enabled": true, "attributes": map[string]any{"configDefinitions": map[string]any{"city": "hangzhou"}}}}, "mcps": []any{map[string]any{"mcpId": "mcp-1", "enabled": true, "config": map[string]any{"credentialRef": "cred-1"}}}}, confirmed: true},
	}

	for _, tc := range cases {
		t.Run(strings.Join(tc.path, "_"), func(t *testing.T) {
			caller.calls = nil
			deap := deapHandler{}.Command(&captureRunner{})
			lookup := tc.path
			leaf, rest, findErr := deap.Find(lookup)
			if findErr != nil || len(rest) != 0 {
				t.Fatalf("find %v: leaf=%v rest=%v err=%v", lookup, leaf, rest, findErr)
			}
			if tc.confirmed {
				leaf.Flags().Bool("yes", false, "test confirmation")
				tc.flags["yes"] = "true"
			}
			for name, value := range tc.flags {
				if setErr := leaf.Flags().Set(name, value); setErr != nil {
					t.Fatal(setErr)
				}
			}
			if runErr := leaf.RunE(leaf, nil); runErr != nil {
				t.Fatalf("RunE() error = %v", runErr)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("MCP call count = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.productID != deapAgentServerID || call.toolName != tc.tool {
				t.Fatalf("route = %s/%s, want %s/%s", call.productID, call.toolName, deapAgentServerID, tc.tool)
			}
			if !reflect.DeepEqual(call.args, tc.wantArgs) {
				t.Fatalf("args = %#v, want %#v", call.args, tc.wantArgs)
			}
		})
	}
}

func TestDevDeapAgentConfigFilesStayRedactedInDryRun(t *testing.T) {
	caller, output := newDeapAgentTestTree(t, true)
	tempDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	files := map[string]string{
		"mcp.json":    `{"name":"weather","description":"查询天气","userQuestionTips":["请输入城市"],"configType":"JSON","configString":"{\"token\":\"mcp-secret\"}","envs":{"API_TOKEN":"env-secret"},"toolsDisabled":{"search":false}}`,
		"skills.json": `[{"skillId":"skill-1","attributes":{"configDefinitions":{"apiToken":"skill-secret"}}}]`,
		"mcps.json":   `[{"mcpId":"mcp-1","credential":{"password":"draft-secret"}}]`,
	}
	for name, body := range files {
		if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	run := func(path []string, flags map[string]string) string {
		t.Helper()
		output.Reset()
		deap := deapHandler{}.Command(&captureRunner{})
		command, rest, findErr := deap.Find(path)
		if findErr != nil || len(rest) != 0 {
			t.Fatalf("find %v: rest=%v err=%v", path, rest, findErr)
		}
		command.Flags().Bool("yes", false, "test confirmation")
		flags["yes"] = "true"
		for name, value := range flags {
			if setErr := command.Flags().Set(name, value); setErr != nil {
				t.Fatal(setErr)
			}
		}
		if runErr := command.RunE(command, nil); runErr != nil {
			t.Fatalf("run %v: %v", path, runErr)
		}
		return output.String()
	}

	mcpOutput := run([]string{"capability", "mcp", "create"}, map[string]string{"agent-uuid": "agent-1", "config-file": "./mcp.json"})
	draftOutput := run([]string{"manage", "save-draft"}, map[string]string{"agent-uuid": "agent-1", "skills-file": "./skills.json", "mcps-file": "./mcps.json"})
	for label, got := range map[string]string{"mcp create": mcpOutput, "save-draft": draftOutput} {
		for _, secret := range []string{"mcp-secret", "env-secret", "skill-secret", "draft-secret"} {
			if strings.Contains(got, secret) {
				t.Fatalf("%s dry-run leaked %q: %s", label, secret, got)
			}
		}
		if !strings.Contains(got, "redacted") {
			t.Fatalf("%s dry-run lacks redacted marker: %s", label, got)
		}
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run made %d MCP calls", len(caller.calls))
	}
}

func TestDevDeapAgentSaveDraftDistinguishesAbsentAndExplicitEmptyConfigs(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	tempDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	if err := os.WriteFile("empty.json", []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(flags map[string]string) map[string]any {
		t.Helper()
		caller.calls = nil
		deap := deapHandler{}.Command(&captureRunner{})
		save, _, findErr := deap.Find([]string{"manage", "save-draft"})
		if findErr != nil {
			t.Fatal(findErr)
		}
		save.Flags().Bool("yes", false, "test confirmation")
		flags["yes"] = "true"
		for name, value := range flags {
			if setErr := save.Flags().Set(name, value); setErr != nil {
				t.Fatal(setErr)
			}
		}
		if runErr := save.RunE(save, nil); runErr != nil {
			t.Fatal(runErr)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("MCP call count = %d, want 1", len(caller.calls))
		}
		return caller.calls[0].args
	}

	absent := run(map[string]string{"agent-uuid": "agent-1"})
	if _, ok := absent["skills"]; ok {
		t.Fatalf("omitted --skills-file unexpectedly changed skills: %#v", absent)
	}
	if _, ok := absent["mcps"]; ok {
		t.Fatalf("omitted --mcps-file unexpectedly changed mcps: %#v", absent)
	}
	empty := run(map[string]string{"agent-uuid": "agent-1", "skills-file": "./empty.json", "mcps-file": "./empty.json"})
	if skills, ok := empty["skills"].([]any); !ok || len(skills) != 0 {
		t.Fatalf("explicit empty skills = %#v, want []", empty["skills"])
	}
	if mcps, ok := empty["mcps"].([]any); !ok || len(mcps) != 0 {
		t.Fatalf("explicit empty mcps = %#v, want []", empty["mcps"])
	}
}

type deapAgentCaller struct {
	dryRun     bool
	calls      []deapAgentCall
	resultText string
	err        error
}

func (c *deapAgentCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, deapAgentCall{productID: productID, toolName: toolName, args: args})
	if c.err != nil {
		return nil, c.err
	}
	text := c.resultText
	if text == "" {
		text = `{}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*deapAgentCaller) Format() string { return "json" }
func (c *deapAgentCaller) DryRun() bool { return c.dryRun }
func (*deapAgentCaller) Fields() string { return "" }
func (*deapAgentCaller) JQ() string     { return "" }

func newDeapAgentTestTree(t *testing.T, dryRun bool) (*deapAgentCaller, *bytes.Buffer) {
	t.Helper()
	caller := &deapAgentCaller{dryRun: dryRun}
	InitDepsForTest(t, caller)
	out := &bytes.Buffer{}
	deps.Out.w = out
	return caller, out
}

// deapFindLeaf 在 `dingtalk-tag manage` / `dingtalk-tag run` 两个子组里找叶子。
//
// 为何不让用例自己写子组名：绝大多数用例关心的是“这个叶子的行为”而不是“它挂在哪个
// 子组”；把子组名写进每个用例会让以后调整归类（如新增子组、或把某命令从管理态
// 移到执行态）逐处改。归类本身由 TestDeapCommandTreeUsesManageRunAndCapability 单独钉住。
func deapFindLeaf(t *testing.T, root *cobra.Command, leaf string) *cobra.Command {
	t.Helper()
	for _, group := range []string{"manage", "run"} {
		cmd, remaining, err := root.Find([]string{group, leaf})
		if err == nil && len(remaining) == 0 && cmd.Name() == leaf {
			return cmd
		}
	}
	t.Fatalf("dingtalk-tag leaf %q not found under manage/run", leaf)
	return nil
}

// TestDeapCommandTreeUsesManageRunAndCapability 钉住现有三类 DEAP 命令，并确保新增
// connect/channel 作为独立接入面挂在顶层，不改动 manage/run/capability 的既有归类。
//
// 为何钉归类而不只钉叶子集合：管理态含不可逆写操作，观测态全是只读；两者混放会
// 让调用方（含 Agent）失去“这一类命令安全属性相同”这个判断依据。
func TestDeapCommandTreeUsesManageRunAndCapability(t *testing.T) {
	newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})

	wantGroups := map[string][]string{
		"connect": {"status", "list", "stop", "restart", "bind", "unbind", "rebind"},
		"manage":  {"create", "detail", "list", "login", "save-draft", "publish", "delete"},
		"run":     {"run-status", "trace"},
	}
	if got := len(root.Commands()); got != len(wantGroups)+2 {
		t.Fatalf("dingtalk-tag direct child count = %d, want %d", got, len(wantGroups)+2)
	}
	for groupName, wantLeaves := range wantGroups {
		group, remaining, err := root.Find([]string{groupName})
		if err != nil || len(remaining) != 0 {
			t.Fatalf("find deap %s: group=%v remaining=%v err=%v", groupName, group, remaining, err)
		}
		if got := len(group.Commands()); got != len(wantLeaves) {
			t.Fatalf("dingtalk-tag %s leaf count = %d, want %d", groupName, got, len(wantLeaves))
		}
		for _, name := range wantLeaves {
			leaf, rest, findErr := group.Find([]string{name})
			if findErr != nil || len(rest) != 0 || leaf == group {
				t.Fatalf("find deap %s %q: leaf=%v rest=%v err=%v", groupName, name, leaf, rest, findErr)
			}
			if leaf.HasSubCommands() {
				t.Errorf("dingtalk-tag %s %s has an intermediate subtree", groupName, name)
			}
			if !leaf.Runnable() {
				t.Errorf("dingtalk-tag %s %s is not runnable", groupName, name)
			}
			if leaf.Args == nil || leaf.Args(leaf, []string{"unexpected"}) == nil {
				t.Errorf("dingtalk-tag %s %s must reject positional arguments", groupName, name)
			}
			final, ok := contractfinal.RuntimeContractFinal(leaf)
			if !ok || final.Identity == nil || final.Interface == nil || final.Safety == nil {
				t.Errorf("dingtalk-tag %s %s has incomplete ContractFinal: %+v ok=%v", groupName, name, final, ok)
			}
		}
	}
	capability, rest, findErr := root.Find([]string{"capability"})
	if findErr != nil || len(rest) != 0 || capability == root {
		t.Fatalf("find capability: command=%v rest=%v err=%v", capability, rest, findErr)
	}
	for _, name := range []string{"skill", "mcp"} {
		subgroup, remaining, subgroupErr := capability.Find([]string{name})
		if subgroupErr != nil || len(remaining) != 0 || subgroup == capability {
			t.Fatalf("find capability subgroup %q: command=%v rest=%v err=%v", name, subgroup, remaining, subgroupErr)
		}
		if !subgroup.HasSubCommands() {
			t.Errorf("dingtalk-tag %s must be a command group", name)
		}
	}
}

func TestDingTalkTagReplacesDeapTopLevelCommand(t *testing.T) {
	newDeapAgentTestTree(t, false)
	handler := deapHandler{}
	if got := handler.Name(); got != "dingtalk-tag" {
		t.Fatalf("handler name = %q, want dingtalk-tag", got)
	}
	command := handler.Command(&captureRunner{})
	if got := command.Name(); got != "dingtalk-tag" {
		t.Fatalf("top-level command name = %q, want dingtalk-tag", got)
	}
	if command.HasAlias("deap") {
		t.Fatal("retired top-level command deap must not remain as an alias")
	}
}

func TestDeapHelpDescribesBuiltInEndpointResolution(t *testing.T) {
	newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})
	if !strings.Contains(root.Long, "跟随当前 MCP 环境") {
		t.Fatal("dingtalk-tag help must describe standard MCP environment resolution")
	}
	if strings.Contains(root.Long, "DINGTALK_DEAP_DEV_MCP_URL 显式配置") {
		t.Fatal("dingtalk-tag help must not require a product-specific endpoint override")
	}
}

func TestDevDeapAgentAvailableLeavesRouteExactMCPTools(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	cases := []struct {
		leaf      string
		tool      string
		flags     map[string]string
		wantArgs  map[string]any
		confirmed bool
	}{
		{
			leaf: "create", tool: "create_digital_employee",
			flags: map[string]string{
				"name": "值班助手", "description": "处理值班问题",
				"dept-id": "dept-1", "dept-name": "值班组",
				"profile-json":      `{"employeeNo":"JSON-001","positionName":"值班员","mainProgramType":"open_code"}`,
				"employee-no":       "E001",
				"supervisor-uid":    "supervisor-1",
				"main-program-type": "local_agent",
				"response-mode":     "targeted_proactive, mention_only",
			},
			wantArgs: map[string]any{
				"name": "值班助手", "description": "处理值班问题",
				"deptId": "dept-1", "deptName": "值班组",
				"digitalTagEmployeeProfile": map[string]any{
					"employeeNo": "E001", "positionName": "值班员",
					"directSupervisorUid": "supervisor-1", "mainProgramType": "local_agent",
					"responseMode": "mention_only,targeted_proactive",
				},
			},
		},
		{
			leaf: "detail", tool: "get_digital_employee_detail",
			flags:    map[string]string{"agent-uuid": "agent-1", "type": "published"},
			wantArgs: map[string]any{"agentUuid": "agent-1", "type": "published"},
		},
		{
			leaf: "list", tool: "list_digital_employees",
			flags: map[string]string{
				"keyword": "值班", "main-program-type": "local_agent", "page": "2", "page-size": "101",
			},
			wantArgs: map[string]any{
				"keyword": "值班", "mainProgramType": "local_agent", "page": 2, "pageSize": 101,
			},
		},
		{
			leaf: "save-draft", tool: "update_digital_employee_draft", confirmed: true,
			flags: map[string]string{
				"agent-uuid": "agent-1", "name": "新名称", "prompt": "你是值班助手",
				"profile-json":      `{"employeeNo":"E001","positionName":"旧岗位","mainProgramType":"open_code","responseMode":"mention_only,targeted_proactive"}`,
				"position-name":     "值班员",
				"main-program-type": "local_agent",
				"response-mode":     "targeted_proactive",
			},
			wantArgs: map[string]any{
				"agentUuid": "agent-1", "name": "新名称", "prompt": "你是值班助手",
				"digitalTagEmployeeProfile": map[string]any{
					"employeeNo": "E001", "positionName": "值班员", "mainProgramType": "local_agent",
					"responseMode": "targeted_proactive",
				},
			},
		},
		{
			leaf: "publish", tool: "publish_digital_employee", confirmed: true,
			flags:    map[string]string{"agent-uuid": "agent-1"},
			wantArgs: map[string]any{"agentUuid": "agent-1"},
		},
		{
			leaf: "delete", tool: "delete_digital_employee", confirmed: true,
			flags:    map[string]string{"agent-uuid": "agent-1"},
			wantArgs: map[string]any{"agentUuid": "agent-1"},
		},
		{
			leaf: "run-status", tool: "query_de_run_status",
			flags:    map[string]string{"agent-uuid": "agent-1", "source-id": "open-message-1", "source-type": "im_message"},
			wantArgs: map[string]any{"agentUuid": "agent-1", "sourceId": "open-message-1", "sourceType": "im_message"},
		},
		{
			leaf: "trace", tool: "query_de_trace",
			flags:    map[string]string{"agent-uuid": "agent-1", "source-id": "open-message-1", "source-type": "trigger_rule"},
			wantArgs: map[string]any{"agentUuid": "agent-1", "sourceId": "open-message-1", "sourceType": "trigger_rule"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.leaf, func(t *testing.T) {
			caller.calls = nil
			root := deapHandler{}.Command(&captureRunner{})
			leaf := deapFindLeaf(t, root, tc.leaf)
			if tc.confirmed {
				leaf.Flags().Bool("yes", false, "test confirmation")
				tc.flags["yes"] = "true"
			}
			for name, value := range tc.flags {
				if setErr := leaf.Flags().Set(name, value); setErr != nil {
					t.Fatal(setErr)
				}
			}
			if runErr := leaf.RunE(leaf, nil); runErr != nil {
				t.Fatalf("RunE() error = %v", runErr)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("MCP call count = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.productID != "deap-dev" || call.toolName != tc.tool {
				t.Fatalf("route = %s/%s, want deap-dev/%s", call.productID, call.toolName, tc.tool)
			}
			if !reflect.DeepEqual(call.args, tc.wantArgs) {
				t.Fatalf("args = %#v, want %#v", call.args, tc.wantArgs)
			}
			for _, forbidden := range []string{"identity", "corpId", "userId", "orgId", "agentType"} {
				if _, ok := call.args[forbidden]; ok {
					t.Errorf("trusted or retired field %s leaked into MCP arguments", forbidden)
				}
			}
		})
	}
}

func TestDeapDetailDefaultsToDraft(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})
	detail := deapFindLeaf(t, root, "detail")
	if err := detail.Flags().Set("agent-uuid", "agent-1"); err != nil {
		t.Fatal(err)
	}

	if err := detail.RunE(detail, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}

	want := map[string]any{"agentUuid": "agent-1", "type": "draft"}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("detail call = %#v, want args %#v", caller.calls, want)
	}
}

func TestDeapAgentResponseModeNormalization(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "mention only", raw: "mention_only", want: "mention_only"},
		{name: "targeted proactive", raw: "targeted_proactive", want: "targeted_proactive"},
		{name: "canonical combination", raw: "mention_only,targeted_proactive", want: "mention_only,targeted_proactive"},
		{name: "reverse combination with spaces", raw: " targeted_proactive , mention_only ", want: "mention_only,targeted_proactive"},
		{name: "empty", raw: "", wantErr: true},
		{name: "empty item", raw: "mention_only,", wantErr: true},
		{name: "duplicate", raw: "mention_only,mention_only", wantErr: true},
		{name: "unknown", raw: "mention_only,always_reply", wantErr: true},
		{name: "too many", raw: "mention_only,targeted_proactive,mention_only", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deapAgentNormalizeResponseMode(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("deapAgentNormalizeResponseMode(%q) = %q, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("deapAgentNormalizeResponseMode(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("deapAgentNormalizeResponseMode(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDevDeapAgentConstraintsFailBeforeMCP(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	cases := []struct {
		leaf    string
		flags   map[string]string
		wantErr string
	}{
		{leaf: "run-status", flags: map[string]string{"source-id": "src-1", "source-type": "im_message"}, wantErr: "agent-uuid"},
		{leaf: "run-status", flags: map[string]string{"agent-uuid": "agent-1"}, wantErr: "source-id"},
		{leaf: "run-status", flags: map[string]string{"agent-uuid": "agent-1", "source-id": "src-1"}, wantErr: "source-type"},
		{leaf: "trace", flags: map[string]string{"source-id": "src-1", "source-type": "im_message"}, wantErr: "agent-uuid"},
		{leaf: "trace", flags: map[string]string{"agent-uuid": "agent-1"}, wantErr: "source-id"},
		{leaf: "trace", flags: map[string]string{"agent-uuid": "agent-1", "source-id": "src-1"}, wantErr: "source-type"},
		{leaf: "list", flags: map[string]string{"page": "0"}, wantErr: "--page 不能小于 1"},
		{leaf: "list", flags: map[string]string{"page-size": "0"}, wantErr: "--page-size 不能小于 1"},
		{leaf: "list", flags: map[string]string{"main-program-type": "a2a"}, wantErr: "--main-program-type"},
		{leaf: "detail", flags: map[string]string{"agent-uuid": "agent-1", "type": "merged"}, wantErr: "--type"},
		{leaf: "login", flags: map[string]string{}, wantErr: "agent-uuid"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"profile-json": `{"tag":"forbidden"}`,
		}, wantErr: "不接受字段"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"response-mode": "always_reply",
		}, wantErr: "响应模式只允许"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"response-mode": "mention_only,always_reply",
		}, wantErr: "响应模式只允许"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"profile-json": `{"responseMode":"mention_only,mention_only"}`,
		}, wantErr: "响应模式只允许"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"profile-json": `{"responseMode":["mention_only","targeted_proactive"]}`,
		}, wantErr: "必须是字符串"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"profile-json": `{"mainProgramType":123}`,
		}, wantErr: "mainProgramType 必须是字符串"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"profile-json": `{"mainProgramType":"  "}`,
		}, wantErr: "mainProgramType 不能为空"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"main-program-type": "a2a",
		}, wantErr: "--main-program-type"},
		{leaf: "create", flags: map[string]string{
			"name": "值班助手", "description": "处理值班问题", "dept-id": "dept-1", "dept-name": "值班组",
			"profile-json": `{"mainProgramType":"a2a"}`,
		}, wantErr: "a2a 暂不支持"},
		{leaf: "save-draft", flags: map[string]string{
			"agent-uuid": "agent-1", "employee-no": strings.Repeat("E", 65),
		}, wantErr: "最多允许 64"},
		{leaf: "save-draft", flags: map[string]string{
			"agent-uuid": "agent-1", "position-name": strings.Repeat("岗", 129),
		}, wantErr: "最多允许 128"},
		{leaf: "save-draft", flags: map[string]string{
			"agent-uuid": "agent-1", "prompt": strings.Repeat("提", 5001),
		}, wantErr: "最多允许 5000"},
	}
	for _, tc := range cases {
		t.Run(tc.leaf+tc.wantErr, func(t *testing.T) {
			caller.calls = nil
			root := deapHandler{}.Command(&captureRunner{})
			leaf := deapFindLeaf(t, root, tc.leaf)
			for name, value := range tc.flags {
				if setErr := leaf.Flags().Set(name, value); setErr != nil {
					t.Fatal(setErr)
				}
			}
			runErr := leaf.RunE(leaf, nil)
			if runErr == nil || !strings.Contains(runErr.Error(), tc.wantErr) {
				t.Fatalf("RunE() error = %v, want containing %q", runErr, tc.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid input made %d MCP call(s)", len(caller.calls))
			}
		})
	}
}

func TestDevDeapAgentRemovesRetiredFlagsAndKeepsIdentityHidden(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})
	create := deapFindLeaf(t, root, "create")
	for _, forbidden := range []string{"org-id", "user-id", "agent-type", "developers-json"} {
		if flag := create.Flags().Lookup(forbidden); flag != nil {
			t.Fatalf("forbidden identity/retired flag --%s is exposed", forbidden)
		}
	}
	list := deapFindLeaf(t, root, "list")
	if flag := list.Flags().Lookup("sort-by"); flag != nil {
		t.Fatal("retired --sort-by is exposed")
	}
	// send-message 已下线：不能用 deapFindLeaf（它找不到就 Fatal，语义刚好反了），
	// 直接断言两个子组里都没有它。
	for _, group := range []string{"manage", "run"} {
		if cmd, _, findErr := root.Find([]string{group, "send-message"}); findErr == nil &&
			cmd != nil && cmd.Name() == "send-message" {
			t.Fatalf("retired send-message command is exposed under %s; 推送能力已从观测接口移除", group)
		}
	}
	trace := deapFindLeaf(t, root, "trace")
	if flag := trace.Flags().Lookup("trace-id"); flag != nil {
		t.Fatal("retired --trace-id is exposed; current MCP input uses the run locator")
	}
	if flag := trace.Flags().Lookup("run-id"); flag != nil {
		t.Fatal("retired --run-id is exposed; 调用方拿不到 runId，只能按来源定位")
	}
	save := deapFindLeaf(t, root, "save-draft")
	for _, forbidden := range []string{
		"developers-json", "prompt-config-json", "model-config-json", "knowledge-config-json",
		"memory-config-json", "selected-skills-json", "deleted-skills-json", "scopes-json",
		"shortcuts-json", "scheduled-task-json",
	} {
		if flag := save.Flags().Lookup(forbidden); flag != nil {
			t.Fatalf("retired save-draft flag --%s is exposed", forbidden)
		}
	}

	for name, value := range map[string]string{
		"name": "值班助手", "description": "处理值班问题",
		"dept-id": "dept-1", "dept-name": "值班组",
	} {
		if setErr := create.Flags().Set(name, value); setErr != nil {
			t.Fatal(setErr)
		}
	}
	if err := create.RunE(create, nil); err != nil {
		t.Fatalf("create RunE() error = %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("MCP call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != "deap-dev" || call.toolName != "create_digital_employee" {
		t.Fatalf("route = %s/%s, want deap-dev/create_digital_employee", call.productID, call.toolName)
	}
	want := map[string]any{
		"name": "值班助手", "description": "处理值班问题",
		"deptId": "dept-1", "deptName": "值班组",
	}
	if !reflect.DeepEqual(call.args, want) {
		t.Fatalf("create args = %#v, want %#v", call.args, want)
	}
	for _, forbidden := range []string{"identity", "corpId", "orgId", "userId", "agentType"} {
		if _, ok := call.args[forbidden]; ok {
			t.Fatalf("trusted identity field %s leaked into MCP arguments", forbidden)
		}
	}
}

func TestDevDeapAgentHelpMatchesCurrentMCPInputs(t *testing.T) {
	newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})
	login := deapFindLeaf(t, root, "login")
	for _, scenario := range []string{"A2A", "dws dingtalk-tag connect", "dws --profile", "dws profile use", "不会输出 AuthCode 或 Token"} {
		if !strings.Contains(login.Long, scenario) {
			t.Fatalf("login help is missing scenario guidance %q: %q", scenario, login.Long)
		}
	}
	if strings.Contains(login.Long, "原样输出") {
		t.Fatalf("login help still describes raw credential output: %q", login.Long)
	}

	publish := deapFindLeaf(t, root, "publish")
	if flag := publish.Flags().Lookup("allow-join-group"); flag == nil || flag.DefValue != "false" {
		t.Fatalf("allow-join-group default = %v, current MCP declares an optional boolean without a default", flag)
	}
	for _, leafName := range []string{"create", "list", "save-draft"} {
		command := deapFindLeaf(t, root, leafName)
		if flag := command.Flags().Lookup("main-program-type"); flag == nil {
			t.Fatalf("%s is missing --main-program-type", leafName)
		}
	}
	for _, leafName := range []string{"create", "save-draft"} {
		command := deapFindLeaf(t, root, leafName)
		flag := command.Flags().Lookup("response-mode")
		if flag == nil || !strings.Contains(flag.Usage, "mention_only,targeted_proactive") {
			t.Fatalf("%s response-mode help does not describe the combined value: %v", leafName, flag)
		}
	}

	for _, name := range []string{"run-status", "trace"} {
		command := deapFindLeaf(t, root, name)
		if flag := command.Flags().Lookup("agent-uuid"); flag == nil {
			t.Fatalf("%s is missing MCP input --agent-uuid", name)
		}
		for _, flagName := range []string{"source-id", "source-type"} {
			if flag := command.Flags().Lookup(flagName); flag == nil {
				t.Fatalf("%s is missing MCP input --%s", name, flagName)
			}
		}
		if !strings.Contains(command.Long, "--source-id") || !strings.Contains(command.Long, "--source-type") {
			t.Fatalf("%s help does not explain the current MCP run locator: %q", name, command.Long)
		}
	}
}

func TestDevDeapAgentHelpExplainsFullReplacementAndTraceAuthorization(t *testing.T) {
	caller, _ := newDeapAgentTestTree(t, false)
	root := deapHandler{}.Command(&captureRunner{})
	save := deapFindLeaf(t, root, "save-draft")
	save.SetOut(io.Discard)
	if helpErr := save.Help(); helpErr != nil {
		t.Fatal(helpErr)
	}
	if !strings.Contains(save.Long, "全量覆写") || !strings.Contains(save.Long, "detail") || strings.Contains(save.Long, "export-draft") {
		t.Fatalf("save-draft help does not explain read-before-write full replacement: %q", save.Long)
	}
	trace := deapFindLeaf(t, root, "trace")
	final, ok := contractfinal.RuntimeContractFinal(trace)
	if !ok || final.Interface == nil || final.Interface.Availability != "available" {
		t.Fatalf("trace ContractFinal interface = %+v ok=%v, want available", final.Interface, ok)
	}
	if strings.Contains(trace.Long, "暂不可") || strings.Contains(trace.Long, "fail-closed") {
		t.Fatalf("trace help still claims unavailable: %q", trace.Long)
	}
	if !strings.Contains(trace.Long, "授权") {
		t.Fatalf("trace help does not explain server-side authorization: %q", trace.Long)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("help inspection made %d remote call(s)", len(caller.calls))
	}
}
