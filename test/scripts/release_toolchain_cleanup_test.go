// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCrossReleaseWrapperDownloadFailureCleansPathsAndKeepsStatus(t *testing.T) {
	if runtime.GOOS == "windows" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("requires a supported Unix release host")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "run-goreleaser-cross.sh"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	cache := filepath.Join(root, "cache with spaces 'quote'")
	cleanupTrap := filepath.Join(root, "cleanup.sh")
	curlArgs := filepath.Join(root, "curl-args.txt")
	dockerLog := filepath.Join(root, "docker-called")
	bashEnv := filepath.Join(root, "download-fixture.sh")
	mustWriteFile(t, filepath.Join(binDir, "curl"), []byte("#!/bin/sh\nexit 28\n"), 0o755)
	mustWriteFile(t, filepath.Join(binDir, "docker"), []byte("#!/bin/sh\ntouch \"$DWS_TEST_DOCKER_LOG\"\n"), 0o755)
	mustWriteFile(t, bashEnv, []byte(`curl() {
  trap -p EXIT > "$DWS_TEST_CLEANUP_TRAP"
  printf '%s\n' "$@" > "$DWS_TEST_CURL_ARGS"
  return 28
}
`), 0o600)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script, "--version")
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DWS_RELEASE_TOOL_CACHE="+cache,
		"BASH_ENV="+bashEnv,
		"DWS_TEST_CLEANUP_TRAP="+cleanupTrap,
		"DWS_TEST_CURL_ARGS="+curlArgs,
		"DWS_TEST_DOCKER_LOG="+dockerLog,
	)
	output, runErr := cmd.CombinedOutput()
	assertReleaseDownloadFailure(t, runErr, output)
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) != 0 {
		t.Fatalf("下载失败后仍有安装目录或锁: entries=%v, err=%v", entries, err)
	}
	if _, err := os.Stat(dockerLog); !os.IsNotExist(err) {
		t.Fatalf("下载失败后不应启动 Docker: %v", err)
	}
	// 显式在安装函数局部变量失效后重放登记的 trap，覆盖不同 Bash 的退出行为。
	cleanup := exec.CommandContext(ctx, "bash", "-c", `set -u; source "$1"; exit 28`, "cleanup-test", cleanupTrap)
	cleanupOutput, cleanupErr := cleanup.CombinedOutput()
	assertReleaseDownloadFailure(t, cleanupErr, cleanupOutput)
	args, err := os.ReadFile(curlArgs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(args), "--retry\n3\n--connect-timeout\n20\n--max-time\n300\n-fsSL\n") {
		t.Fatalf("下载未设置有界重试和超时: %s", args)
	}
}

func assertReleaseDownloadFailure(t *testing.T, err error, output []byte) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 28 {
		t.Fatalf("原始 curl 失败码未保留: err=%v, output=%s", err, output)
	}
	if strings.Contains(string(output), "unbound variable") {
		t.Fatalf("清理仍依赖已失效的局部变量: %s", output)
	}
}
