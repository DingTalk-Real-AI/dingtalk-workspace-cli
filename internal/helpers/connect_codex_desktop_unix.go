// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows

package helpers

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var codexDesktopLstat = os.Lstat

func dialCodexDesktop(ctx context.Context) (net.Conn, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = filepath.Join(userHome, ".codex")
	}
	if !filepath.IsAbs(home) {
		return nil, errors.New("CODEX_HOME must be absolute")
	}
	dir := filepath.Join(home, "ipc")
	path := filepath.Join(dir, "ipc.sock")
	for _, p := range []string{dir, path} {
		info, err := codexDesktopLstat(p)
		if err != nil {
			return nil, err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0077 != 0 || (p == dir && !info.IsDir()) || (p == path && info.Mode()&os.ModeSocket == 0) {
			return nil, errors.New("desktop IPC endpoint is not private to the current user")
		}
	}
	return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", path)
}
