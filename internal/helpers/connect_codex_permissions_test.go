// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func codexPermissionFixture(t *testing.T) codexSavedSettings {
	t.Helper()
	cwd := t.TempDir()
	profile, _ := json.Marshal(map[string]any{"type": "managed", "network": "restricted", "file_system": map[string]any{"type": "restricted", "entries": []any{
		map[string]any{"path": map[string]any{"type": "special", "value": map[string]string{"kind": "root"}}, "access": "read"},
		map[string]any{"path": map[string]string{"type": "path", "path": cwd}, "access": "write"},
	}}})
	return codexSavedSettings{Cwd: cwd, Roots: []string{cwd}, Approval: json.RawMessage(`"on-request"`), Reviewer: "user", Profile: profile}
}

func codexPermissionReadFixture(t *testing.T, thread string, settings codexSavedSettings) json.RawMessage {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	meta, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]string{"id": thread}})
	turn, _ := json.Marshal(map[string]any{"type": "turn_context", "payload": settings})
	if err := os.WriteFile(path, append(append(meta, '\n'), append(turn, '\n')...), 0600); err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(map[string]any{"thread": map[string]string{"id": thread, "path": path}})
	return result
}

func codexPermissionResumeFixture(t *testing.T, thread string, settings codexSavedSettings) json.RawMessage {
	t.Helper()
	params, err := settings.resumeParams(thread)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]any{"thread": map[string]string{"id": thread}}
	for _, key := range []string{"cwd", "approvalPolicy", "approvalsReviewer", "runtimeWorkspaceRoots"} {
		result[key] = params[key]
	}
	if name, ok := params["permissions"]; ok {
		result["activePermissionProfile"] = map[string]any{"id": name}
	} else {
		mode := params["sandbox"]
		policy := map[string]any{"type": map[any]string{"read-only": "readOnly", "workspace-write": "workspaceWrite", "danger-full-access": "dangerFullAccess"}[mode]}
		if mode == "workspace-write" {
			config := params["config"].(map[string]any)["sandbox_workspace_write"].(map[string]any)
			for wire, key := range map[string]string{"writableRoots": "writable_roots", "networkAccess": "network_access", "excludeTmpdirEnvVar": "exclude_tmpdir_env_var", "excludeSlashTmp": "exclude_slash_tmp"} {
				policy[wire] = config[key]
			}
		}
		result["sandbox"] = policy
	}
	raw, _ := json.Marshal(result)
	return raw
}

func TestCrossPlatformCoverageCodexSavedPermissionReadback(t *testing.T) {
	for _, mode := range []string{"profile", "workspace-write", "read-only", "danger-full-access"} {
		s := codexPermissionFixture(t)
		if mode != "profile" {
			s.Profile = nil
			s.Sandbox = json.RawMessage(`{"type":"` + mode + `"}`)
		}
		params, err := s.resumeParams("thread")
		if err != nil {
			t.Fatal(err)
		}
		raw := codexPermissionResumeFixture(t, "thread", s)
		if err := verifyCodexResumedPermissions(raw, params); err != nil {
			t.Fatal("正确权限被拒绝", mode, err)
		}
		if err := verifyCodexResumedPermissions(json.RawMessage(`!`), params); err == nil {
			t.Fatal("无效回执被接受")
		}
		for _, key := range []string{"cwd", "approvalPolicy", "approvalsReviewer", "runtimeWorkspaceRoots", "activePermissionProfile", "sandbox"} {
			var changed map[string]any
			_ = json.Unmarshal(raw, &changed)
			if _, exists := changed[key]; !exists {
				continue
			}
			delete(changed, key)
			bad, _ := json.Marshal(changed)
			if err := verifyCodexResumedPermissions(bad, params); err == nil {
				t.Fatal("缺失权限回执被接受", mode, key)
			}
		}
		if mode == "workspace-write" {
			var changed map[string]any
			_ = json.Unmarshal(raw, &changed)
			changed["sandbox"].(map[string]any)["networkAccess"] = true
			bad, _ := json.Marshal(changed)
			if err := verifyCodexResumedPermissions(bad, params); err == nil {
				t.Fatal("网络权限扩大未拦截")
			}
		}
	}
}

func TestCrossPlatformCoverageCodexSavedPermissionResume(t *testing.T) {
	settings := codexPermissionFixture(t)
	read := codexPermissionReadFixture(t, "thread", settings)
	writer := &bufferWriteCloser{}
	c := unitCodexClient(writer, codexRPCMessage{ID: codexIntPtr(1), Result: read})
	params, err := c.employeeResumeParams(context.Background(), "thread")
	if err != nil || params["cwd"] != settings.Cwd || params["approvalsReviewer"] != "user" {
		t.Fatalf("恢复时丢失人工设置: %v %v", params, err)
	}
	profile := params["config"].(map[string]any)["permissions"].(map[string]any)[params["permissions"].(string)].(map[string]any)
	fs := profile["filesystem"].(map[string]string)
	if fs[":root"] != "read" || fs[settings.Cwd] != "write" || profile["network"].(map[string]bool)["enabled"] {
		t.Fatalf("工作区写权限被放宽: %v", profile)
	}
	if !strings.Contains(writer.String(), `"method":"thread/read"`) || strings.Contains(writer.String(), "thread/resume") {
		t.Fatal("取权限阶段不得先恢复并覆盖原权限")
	}
}

func TestCrossPlatformCoverageCodexSavedPermissionLatestSettings(t *testing.T) {
	settings := codexPermissionFixture(t)
	read := codexPermissionReadFixture(t, "thread", settings)
	var result struct{ Thread struct{ Path string } }
	_ = json.Unmarshal(read, &result)
	for _, mode := range []string{"full", "readonly", "workspace"} {
		if mode == "full" {
			settings.Profile = json.RawMessage(`{"type":"disabled"}`)
			settings.Approval = json.RawMessage(`"never"`)
		} else {
			fs := []any{map[string]any{"path": map[string]any{"type": "special", "value": map[string]string{"kind": "root"}}, "access": "read"}}
			if mode == "workspace" {
				fs = append(fs, map[string]any{"path": map[string]string{"type": "path", "path": settings.Cwd}, "access": "write"})
			}
			settings.Profile, _ = json.Marshal(map[string]any{"type": "managed", "network": "restricted", "file_system": map[string]any{"type": "restricted", "entries": fs}})
			settings.Approval = json.RawMessage(`"on-request"`)
		}
		event, _ := json.Marshal(map[string]any{"type": "event_msg", "payload": map[string]any{"type": "thread_settings_applied", "thread_id": "thread", "thread_settings": settings}})
		f, err := os.OpenFile(result.Thread.Path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(append(event, '\n'))
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readCodexSavedSettings(result.Thread.Path, "thread")
		if err != nil || string(actual.Profile) != string(settings.Profile) || string(actual.Approval) != string(settings.Approval) {
			t.Fatalf("%s 人工设置未生效: %+v %v", mode, actual, err)
		}
		params, err := actual.resumeParams("thread")
		if err != nil {
			t.Fatal(err)
		}
		if mode == "full" && params["permissions"] != ":danger-full-access" {
			t.Fatal("人工选择的完全访问未继承")
		}
	}
}

func TestCrossPlatformCoverageCodexSavedPermissionReadErrors(t *testing.T) {
	for _, result := range []string{`!`, `{}`, `{"thread":{"id":"other","path":"relative"}}`, `{"thread":{"id":"thread","path":"relative"}}`} {
		c := unitCodexClient(&bufferWriteCloser{}, codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(result)})
		if _, err := c.employeeResumeParams(context.Background(), "thread"); !errors.Is(err, errCodexSavedPermissions) {
			t.Fatal("无有效会话不能猜测权限", err)
		}
	}
	missing, _ := json.Marshal(map[string]any{"thread": map[string]string{"id": "thread", "path": filepath.Join(t.TempDir(), "missing")}})
	unreadable := unitCodexClient(&bufferWriteCloser{}, codexRPCMessage{ID: codexIntPtr(1), Result: missing})
	if _, err := unreadable.employeeResumeParams(context.Background(), "thread"); !errors.Is(err, errCodexSavedPermissions) {
		t.Fatal("读取失败不能恢复会话", err)
	}
	c := unitCodexClient(&bufferWriteCloser{}, codexRPCMessage{ID: codexIntPtr(1), Error: &codexRPCError{Message: "read failed"}})
	if _, err := c.employeeResumeParams(context.Background(), "thread"); err == nil {
		t.Fatal("RPC 失败被忽略")
	}
	c.stdin = failingCodexWriter{}
	if _, err := c.employeeResumeParams(context.Background(), "thread"); err == nil {
		t.Fatal("写失败被忽略")
	}
	if _, err := readCodexSavedSettings(filepath.Join(t.TempDir(), "missing"), "thread"); err == nil {
		t.Fatal("缺失文件被忽略")
	}
	for _, content := range []string{"", "!", `{"type":"session_meta","payload":42}`, `{"type":"session_meta","payload":{"id":"other"}}`, `{"type":"turn_context","payload":42}`, `{"type":"event_msg","payload":42}`, `{"type":"event_msg","payload":{"type":"thread_settings_applied","thread_id":"other"}}`, strings.Repeat("x", codexDesktopFrameLimit+1)} {
		path := filepath.Join(t.TempDir(), "rollout")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readCodexSavedSettings(path, "thread"); !errors.Is(err, errCodexSavedPermissions) {
			t.Fatal("损坏的权限记录被接受", err)
		}
	}
}

type failingCodexWriter struct{}

func (failingCodexWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (failingCodexWriter) Close() error              { return nil }

func TestCrossPlatformCoverageCodexSavedPermissionValidation(t *testing.T) {
	valid := codexPermissionFixture(t)
	for _, change := range []func(*codexSavedSettings){
		func(s *codexSavedSettings) { s.Cwd = "relative" },
		func(s *codexSavedSettings) { s.Roots = []string{"relative"} },
		func(s *codexSavedSettings) { s.Approval = nil },
		func(s *codexSavedSettings) { s.Approval = json.RawMessage(`null`) },
		func(s *codexSavedSettings) { s.Reviewer = "unknown" },
	} {
		s := valid
		change(&s)
		if _, err := s.resumeParams("thread"); err == nil {
			t.Fatal("不完整权限未拒绝")
		}
	}
	for _, profile := range []string{`!`, `{}`, `{"type":"external"}`, `{"type":"disabled","network":"enabled"}`, `{"type":"managed","file_system":{"type":"restricted"},"network":"enabled"}`, `{"type":"managed","file_system":{"type":"unrestricted"},"network":"restricted"}`, `{"type":"managed","file_system":{"type":"restricted"},"network":"unknown"}`, `{"type":"managed","future":true}`} {
		s := valid
		s.Profile = json.RawMessage(profile)
		if _, err := s.resumeParams("thread"); err == nil {
			t.Fatal("不支持的权限不能降级放行")
		}
	}
	for _, entry := range []string{
		`{"path":{"type":"special","value":{"kind":"tmpdir"}},"access":"write","missing_path_behavior":"skip"}`,
		`{"path":{"type":"special","value":{"kind":"future"}},"access":"read"}`,
		`{"path":{"type":"future"},"access":"read"}`,
		`{"path":{"type":"path","path":"relative"},"access":"read"}`,
		`{"path":{"type":"special","value":{"kind":"root"}},"access":"future"}`,
		`{"path":{"type":"special","value":{"kind":"root"}},"access":"read","missing_path_behavior":"future"}`,
	} {
		s := valid
		s.Profile = json.RawMessage(`{"type":"managed","network":"restricted","file_system":{"type":"restricted","entries":[` + entry + `]}}`)
		_, err := s.resumeParams("thread")
		if (err == nil) != strings.Contains(entry, `"skip"`) {
			t.Fatalf("目录权限校验错误: %s %v", entry, err)
		}
	}
	s := valid
	s.Profile = json.RawMessage(`{"type":"managed","network":"restricted","file_system":{"type":"restricted","entries":[{"path":{"type":"special","value":{"kind":"root"}},"access":"deny"},{"path":{"type":"special","value":{"kind":"root"}},"access":"write"}]}}`)
	if _, err := s.resumeParams("thread"); err == nil {
		t.Fatal("重复规则丢失 deny")
	}
}

func TestCrossPlatformCoverageCodexSavedPermissionLegacy(t *testing.T) {
	for _, mode := range []string{"workspace-write", "read-only", "danger-full-access"} {
		s := codexPermissionFixture(t)
		s.Profile = nil
		s.Sandbox, _ = json.Marshal(map[string]any{"type": mode, "writable_roots": []string{s.Cwd}})
		p, err := s.resumeParams("thread")
		if err != nil || p["sandbox"] != mode {
			t.Fatalf("旧版沙箱恢复失败: %v %v", p, err)
		}
	}
	for _, sandbox := range []string{`!`, `{}`, `{"type":"external-sandbox"}`, `{"type":"read-only","network_access":true}`, `{"type":"workspace-write","writable_roots":["relative"]}`} {
		s := codexPermissionFixture(t)
		s.Profile = nil
		s.Sandbox = json.RawMessage(sandbox)
		if _, err := s.resumeParams("thread"); err == nil {
			t.Fatal("旧版未知策略未拒绝")
		}
	}
}
