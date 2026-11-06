// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

var errCodexSavedPermissions = errors.New("cannot safely restore saved Codex permissions")

type codexSavedSettings struct {
	Cwd      string          `json:"cwd"`
	Roots    []string        `json:"runtime_workspace_roots"`
	Approval json.RawMessage `json:"approval_policy"`
	Reviewer string          `json:"approvals_reviewer"`
	Profile  json.RawMessage `json:"permission_profile"`
	Sandbox  json.RawMessage `json:"sandbox_policy"`
}

// thread/resume 省略 sandbox 时会采用新进程的默认配置。先只读定位原会话，
// 再恢复最新持久化权限；不能拿连接配置或第一次连接的缓存替代人工修改。
func (c *codexAppServerClient) employeeResumeParams(ctx context.Context, threadID string) (map[string]any, error) {
	id := c.requestID()
	if err := c.send(map[string]any{"id": id, "method": "thread/read", "params": map[string]any{"threadId": threadID}}); err != nil {
		return nil, err
	}
	raw, err := c.waitResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	var result struct {
		Thread struct{ ID, Path string } `json:"thread"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Thread.ID != threadID || !filepath.IsAbs(result.Thread.Path) {
		return nil, errCodexSavedPermissions
	}
	saved, err := readCodexSavedSettings(result.Thread.Path, threadID)
	if err != nil {
		return nil, err
	}
	return saved.resumeParams(threadID)
}

func readCodexSavedSettings(path, threadID string) (codexSavedSettings, error) {
	f, err := os.Open(path)
	if err != nil {
		return codexSavedSettings{}, errCodexSavedPermissions
	}
	defer f.Close()
	return readCodexSavedSettingsFrom(f, threadID)
}

// 先核验文件首部的会话身份，再从尾部寻找最新完整设置，避免逐轮解析全部历史。
// 每轮重新读取，不缓存权限；残缺或无法识别的新记录仍停止独立恢复。
func readCodexSavedSettingsFrom(r io.ReadSeeker, threadID string) (codexSavedSettings, error) {
	var empty codexSavedSettings
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), codexDesktopFrameLimit)
	var meta struct {
		Type    string
		Payload struct{ ID string }
	}
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &meta) != nil || meta.Type != "session_meta" || meta.Payload.ID != threadID {
		return empty, errCodexSavedPermissions
	}
	cursor, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return empty, errCodexSavedPermissions
	}
	// 跨块记录只保存未完成的片段，总量受单帧上限约束，不分配整个历史文件。
	var fragments [][]byte
	fragmentSize := 0
	consume := func(prefix []byte) (*codexSavedSettings, error) {
		if len(prefix)+fragmentSize > codexDesktopFrameLimit {
			return nil, errCodexSavedPermissions
		}
		line := prefix
		if len(fragments) > 0 {
			line = make([]byte, 0, len(prefix)+fragmentSize)
			line = append(line, prefix...)
			for i := len(fragments) - 1; i >= 0; i-- {
				line = append(line, fragments[i]...)
			}
		}
		fragments = nil
		fragmentSize = 0
		if len(line) == 0 {
			return nil, nil
		}
		var record struct {
			Type    string
			Payload json.RawMessage
		}
		if json.Unmarshal(line, &record) != nil {
			return nil, errCodexSavedPermissions
		}
		switch record.Type {
		case "turn_context":
			var saved codexSavedSettings
			if json.Unmarshal(record.Payload, &saved) != nil {
				return nil, errCodexSavedPermissions
			}
			return &saved, nil
		case "event_msg":
			var event struct {
				Type     string
				ThreadID string             `json:"thread_id"`
				Settings codexSavedSettings `json:"thread_settings"`
			}
			if json.Unmarshal(record.Payload, &event) != nil {
				return nil, errCodexSavedPermissions
			}
			if event.Type == "thread_settings_applied" {
				if event.ThreadID != threadID {
					return nil, errCodexSavedPermissions
				}
				return &event.Settings, nil
			}
		}
		return nil, nil
	}
	for cursor > 0 {
		start := max(int64(0), cursor-64*1024)
		if _, err := r.Seek(start, io.SeekStart); err != nil {
			return empty, errCodexSavedPermissions
		}
		chunk := make([]byte, cursor-start)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return empty, errCodexSavedPermissions
		}
		end := len(chunk)
		for end > 0 {
			boundary := bytes.LastIndexByte(chunk[:end], '\n')
			if boundary < 0 {
				if fragmentSize+end > codexDesktopFrameLimit {
					return empty, errCodexSavedPermissions
				}
				fragments = append(fragments, chunk[:end])
				fragmentSize += end
				break
			}
			saved, err := consume(chunk[boundary+1 : end])
			if err != nil {
				return empty, err
			}
			if saved != nil {
				return *saved, nil
			}
			end = boundary
		}
		cursor = start
	}
	// 剩余片段只能是已核验的首行 session_meta，不含可恢复的设置。
	return empty, errCodexSavedPermissions
}

func (s codexSavedSettings) resumeParams(threadID string) (map[string]any, error) {
	if !filepath.IsAbs(s.Cwd) || len(s.Approval) == 0 || bytes.Equal(s.Approval, []byte("null")) || (s.Reviewer != "user" && s.Reviewer != "auto_review" && s.Reviewer != "guardian_subagent") {
		return nil, errCodexSavedPermissions
	}
	var policy string
	if json.Unmarshal(s.Approval, &policy) != nil {
		return nil, errCodexSavedPermissions
	}
	switch policy {
	case "untrusted", "on-failure", "on-request", "never":
	default:
		return nil, errCodexSavedPermissions
	}
	p := map[string]any{"threadId": threadID, "cwd": s.Cwd, "approvalPolicy": s.Approval, "approvalsReviewer": s.Reviewer}
	if s.Roots != nil {
		for _, root := range s.Roots {
			if !filepath.IsAbs(root) {
				return nil, errCodexSavedPermissions
			}
		}
		p["runtimeWorkspaceRoots"] = s.Roots
	}
	if len(s.Profile) == 0 || bytes.Equal(s.Profile, []byte("null")) {
		return s.legacyResumeParams(p)
	}
	var profile struct {
		Type       string `json:"type"`
		Network    string `json:"network"`
		FileSystem struct {
			Type    string `json:"type"`
			Entries []struct {
				Path struct {
					Type  string `json:"type"`
					Path  string `json:"path"`
					Value struct {
						Kind string `json:"kind"`
					} `json:"value"`
				} `json:"path"`
				Access  string `json:"access"`
				Missing string `json:"missing_path_behavior"`
			} `json:"entries"`
		} `json:"file_system"`
	}
	decoder := json.NewDecoder(bytes.NewReader(s.Profile))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&profile) != nil {
		return nil, errCodexSavedPermissions
	}
	if profile.Type == "disabled" {
		if profile.Network != "" || profile.FileSystem.Type != "" {
			return nil, errCodexSavedPermissions
		}
		p["permissions"] = ":danger-full-access"
		return p, nil
	}
	if profile.Type != "managed" || profile.FileSystem.Type != "restricted" || (profile.Network != "restricted" && profile.Network != "enabled") {
		return nil, errCodexSavedPermissions
	}
	// 已落盘的 profile 不包含命名网络代理的域名规则，无法据此重建网络放行范围。
	// 这类会话交给原桌面 owner 处理；不能仅凭 enabled 丢掉其域名限制。
	if profile.Network == "enabled" {
		return nil, errCodexSavedPermissions
	}
	filesystem := map[string]string{}
	for _, entry := range profile.FileSystem.Entries {
		if entry.Access != "read" && entry.Access != "write" && entry.Access != "deny" {
			return nil, errCodexSavedPermissions
		}
		if entry.Missing != "" && entry.Missing != "skip" {
			return nil, errCodexSavedPermissions
		}
		path := entry.Path.Path
		switch entry.Path.Type {
		case "path":
			if !filepath.IsAbs(path) {
				return nil, errCodexSavedPermissions
			}
		case "special":
			switch entry.Path.Value.Kind {
			case "root", "minimal", "workspace_roots", "tmpdir", "slash_tmp":
				path = ":" + entry.Path.Value.Kind
			default:
				return nil, errCodexSavedPermissions
			}
		default:
			return nil, errCodexSavedPermissions
		}
		if old, exists := filesystem[path]; exists && old != entry.Access {
			return nil, errCodexSavedPermissions
		}
		filesystem[path] = entry.Access
	}
	// 每次从最新设置生成请求内的 profile，保留目录、deny 和网络限制。
	// 不写 config.toml；服务端仍执行本机的托管权限约束，拒绝时不降级。
	name := fmt.Sprintf("dws-resume-%x", sha256.Sum256(s.Profile))
	p["permissions"] = name
	p["config"] = map[string]any{"permissions": map[string]any{name: map[string]any{"filesystem": filesystem, "network": map[string]bool{"enabled": false}}}}
	return p, nil
}

func (s codexSavedSettings) legacyResumeParams(p map[string]any) (map[string]any, error) {
	var sandbox struct {
		Type          string   `json:"type"`
		WritableRoots []string `json:"writable_roots"`
		Network       bool     `json:"network_access"`
		ExcludeTmp    bool     `json:"exclude_tmpdir_env_var"`
		ExcludeSlash  bool     `json:"exclude_slash_tmp"`
	}
	d := json.NewDecoder(bytes.NewReader(s.Sandbox))
	d.DisallowUnknownFields()
	if d.Decode(&sandbox) != nil {
		return nil, errCodexSavedPermissions
	}
	switch sandbox.Type {
	case "danger-full-access":
	case "read-only":
		if sandbox.Network {
			return nil, errCodexSavedPermissions
		}
	case "workspace-write":
		if sandbox.WritableRoots == nil {
			sandbox.WritableRoots = []string{}
		}
		for _, root := range sandbox.WritableRoots {
			if !filepath.IsAbs(root) {
				return nil, errCodexSavedPermissions
			}
		}
		p["config"] = map[string]any{"sandbox_workspace_write": map[string]any{"writable_roots": sandbox.WritableRoots, "network_access": sandbox.Network, "exclude_tmpdir_env_var": sandbox.ExcludeTmp, "exclude_slash_tmp": sandbox.ExcludeSlash}}
	default:
		return nil, errCodexSavedPermissions
	}
	p["sandbox"] = sandbox.Type
	return p, nil
}

// 老版本可能忽略未知参数。只有服务端确认恢复结果后才允许提交新回合。
func verifyCodexResumedPermissions(raw json.RawMessage, params map[string]any) error {
	var actual map[string]any
	if json.Unmarshal(raw, &actual) != nil {
		return errCodexSavedPermissions
	}
	for _, key := range []string{"cwd", "approvalPolicy", "approvalsReviewer", "runtimeWorkspaceRoots"} {
		if expected, exists := params[key]; exists {
			encoded, _ := json.Marshal(expected)
			var value any
			_ = json.Unmarshal(encoded, &value)
			// 空根目录与服务端省略零值等价；非空目录仍须逐项完全一致。
			if roots, ok := value.([]any); key == "runtimeWorkspaceRoots" && ok && len(roots) == 0 && actual[key] == nil {
				continue
			}
			if !reflect.DeepEqual(value, actual[key]) {
				return errCodexSavedPermissions
			}
		}
	}
	if name, ok := params["permissions"].(string); ok {
		profile, _ := actual["activePermissionProfile"].(map[string]any)
		if profile["id"] != name {
			return errCodexSavedPermissions
		}
		return nil
	}
	sandbox, _ := actual["sandbox"].(map[string]any)
	mode := params["sandbox"]
	if sandbox["type"] != map[any]string{"read-only": "readOnly", "workspace-write": "workspaceWrite", "danger-full-access": "dangerFullAccess"}[mode] {
		return errCodexSavedPermissions
	}
	if mode == "workspace-write" {
		config := params["config"].(map[string]any)["sandbox_workspace_write"].(map[string]any)
		for wire, key := range map[string]string{"writableRoots": "writable_roots", "networkAccess": "network_access", "excludeTmpdirEnvVar": "exclude_tmpdir_env_var", "excludeSlashTmp": "exclude_slash_tmp"} {
			encoded, _ := json.Marshal(config[key])
			var value any
			_ = json.Unmarshal(encoded, &value)
			if !reflect.DeepEqual(value, sandbox[wire]) {
				return errCodexSavedPermissions
			}
		}
	}
	return nil
}
