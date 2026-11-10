// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows

package helpers

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type codexDesktopInfo struct {
	os.FileInfo
	mode os.FileMode
	stat *syscall.Stat_t
}

func (i codexDesktopInfo) Mode() os.FileMode { return i.mode }
func (i codexDesktopInfo) IsDir() bool       { return i.mode.IsDir() }
func (i codexDesktopInfo) Sys() any          { return i.stat }

func TestCrossPlatformCoverageCodexDesktopEndpoint(t *testing.T) {
	t.Setenv("CODEX_HOME", "relative")
	if _, err := dialCodexDesktop(context.Background()); err == nil {
		t.Fatal("不能使用相对端点")
	}
	t.Setenv("CODEX_HOME", "")
	t.Setenv("HOME", "")
	if _, err := dialCodexDesktop(context.Background()); err == nil {
		t.Fatal("缺少用户目录应失败")
	}
	t.Setenv("HOME", t.TempDir())
	if _, err := dialCodexDesktop(context.Background()); err == nil {
		t.Fatal("缺少端点应失败")
	}
	t.Setenv("CODEX_HOME", t.TempDir())
	for _, scenario := range []string{"socket-missing", "owner", "directory", "permissions", "socket-type", "dial"} {
		t.Run(scenario, func(t *testing.T) {
			testseam.Swap(t, &codexDesktopLstat, func(path string) (os.FileInfo, error) {
				isDir := filepath.Base(path) == "ipc"
				mode := os.ModeSocket | 0600
				if isDir {
					mode = os.ModeDir | 0700
				}
				if scenario == "socket-missing" && !isDir {
					return nil, os.ErrNotExist
				}
				stat := &syscall.Stat_t{Uid: uint32(os.Geteuid())}
				if scenario == "owner" {
					stat.Uid++
				}
				if scenario == "directory" && isDir {
					mode = os.ModeSymlink | 0700
				}
				if scenario == "permissions" {
					mode |= 0020
				}
				if scenario == "socket-type" && !isDir {
					mode = 0600
				}
				return codexDesktopInfo{mode: mode, stat: stat}, nil
			})
			if conn, err := dialCodexDesktop(context.Background()); err == nil {
				conn.Close()
				t.Fatal("未确认的端点不应连接成功")
			}
		})
	}
}
