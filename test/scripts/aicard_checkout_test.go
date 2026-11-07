// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui"
)

func TestCrossPlatformCoverageAicardProtocolGitCheckout(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	checkout := t.TempDir()
	emptyConfig := filepath.Join(t.TempDir(), "empty-git-config")
	mustWriteFile(t, emptyConfig, nil, 0o600)
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{
			"-c", "core.autocrlf=true",
			"-c", "core.safecrlf=false",
			"-c", "core.attributesfile=" + emptyConfig,
		}, args...)...)
		command.Dir = checkout
		command.Env = append(os.Environ(),
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+emptyConfig, "GIT_ATTR_NOSYSTEM=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("隔离仓库 git %v 失败: %v\n%s", args, err, output)
		}
	}
	runGit("init", "--quiet")
	attributes, err := os.ReadFile(filepath.Join(repository, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(checkout, ".gitattributes"), attributes, 0o644)

	const protocolPath = "skills/multi/dingtalk-aicard/references/protocol"
	entries, err := os.ReadDir(filepath.Join(repository, filepath.FromSlash(protocolPath)))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := protocolPath + "/" + entry.Name()
		data, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		mustWriteFile(t, filepath.Join(checkout, filepath.FromSlash(path)), data, 0o644)
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		t.Fatal("没有找到真实协议 JSON 文件")
	}
	runGit(append([]string{"add", "--", ".gitattributes"}, paths...)...)
	// 删除工作区副本后从 index 重新检出，实际经过 Git 的换行转换。
	for _, path := range paths {
		if err := os.Remove(filepath.Join(checkout, filepath.FromSlash(path))); err != nil {
			t.Fatal(err)
		}
	}
	runGit("checkout-index", "--all", "--force")
	var crlfFiles []string
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(checkout, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("\r\n")) {
			crlfFiles = append(crlfFiles, path)
		}
	}
	t.Logf("Git 检出协议文件 %d 个，包含 CRLF 的文件 %d 个", len(paths), len(crlfFiles))
	source := os.DirFS(filepath.Join(checkout, "skills", "multi", "dingtalk-aicard"))
	if _, err := a2ui.Load(source); err != nil {
		t.Fatalf("Git 检出后的真实协议无法通过 manifest 校验: %v", err)
	}
	if len(crlfFiles) != 0 {
		t.Fatalf("协议文件检出后未保留 LF: %v", crlfFiles)
	}
}
