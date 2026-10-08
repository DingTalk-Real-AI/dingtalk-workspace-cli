// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package atomicfile 提供不依赖命令层的安全原子文件写入。
package atomicfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// TempFile 是原子写入所需的最小临时文件接口。
type TempFile interface {
	io.Writer
	Name() string
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

// Ops 允许上层保留已有的故障注入测试，同时把写入算法集中在本包。
type Ops struct {
	MkdirAll   func(string, os.FileMode) error
	CreateTemp func(string, string) (TempFile, error)
	Remove     func(string) error
	Rename     func(string, string) error
}

var defaultOps = Ops{
	MkdirAll: os.MkdirAll,
	CreateTemp: func(dir, pattern string) (TempFile, error) {
		return os.CreateTemp(dir, pattern)
	},
	Remove: os.Remove,
	Rename: os.Rename,
}

// Write 将 data 通过同目录临时文件、fsync 和 rename 原子写入 path。
func Write(path string, data []byte, perm os.FileMode) error {
	return WriteWithOps(path, perm, defaultOps, func(tmp TempFile) error {
		_, err := tmp.Write(data)
		return err
	})
}

// WriteJSON 使用 0600 权限原子写入 JSON 数据。
func WriteJSON(path string, data []byte) error {
	return Write(path, data, 0o600)
}

// WriteWithOps 执行原子写入；Ops 仅供兼容测试注入和受控封装使用。
func WriteWithOps(path string, perm os.FileMode, ops Ops, writeFn func(TempFile) error) error {
	dir := filepath.Dir(path)
	if err := ops.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	tmp, err := ops.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = ops.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	if err := writeFn(tmp); err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync to disk: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := ops.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename to final: %w", err)
	}
	success = true
	return nil
}
