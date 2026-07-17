// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZip_BasicFiles(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	targetDir := filepath.Join(dir, "extracted")

	createTestZip(t, zipPath, map[string]string{
		"README.md":    "# Hello",
		"dws/SKILL.md": "# Skill",
		"dws/data.txt": "some data",
	})

	if err := ExtractZip(zipPath, targetDir); err != nil {
		t.Fatalf("ExtractZip() error = %v", err)
	}

	assertFileContent(t, filepath.Join(targetDir, "README.md"), "# Hello")
	assertFileContent(t, filepath.Join(targetDir, "dws", "SKILL.md"), "# Skill")
	assertFileContent(t, filepath.Join(targetDir, "dws", "data.txt"), "some data")
}

func TestExtractZip_ZipSlipProtection(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")
	targetDir := filepath.Join(dir, "extracted")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	// Attempt path traversal
	ew, _ := w.Create("../../etc/passwd")
	ew.Write([]byte("root:x:0:0"))
	// Normal file
	nw, _ := w.Create("normal.txt")
	nw.Write([]byte("ok"))
	w.Close()
	f.Close()

	if err := ExtractZip(zipPath, targetDir); err != nil {
		t.Fatalf("ExtractZip() error = %v", err)
	}

	// Path-traversal entry should be skipped
	evilPath := filepath.Join(dir, "..", "etc", "passwd")
	if _, err := os.Stat(evilPath); err == nil {
		t.Error("zip-slip protection failed: traversal file was extracted")
	}

	// Normal file should exist
	assertFileContent(t, filepath.Join(targetDir, "normal.txt"), "ok")
}

func TestExtractZip_InvalidZip(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.zip")
	os.WriteFile(badPath, []byte("not a zip"), 0644)

	err := ExtractZip(badPath, filepath.Join(dir, "out"))
	if err == nil {
		t.Error("ExtractZip() expected error for invalid zip")
	}
}

func TestExtractZip_EmptyZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "empty.zip")
	createTestZip(t, zipPath, map[string]string{})

	targetDir := filepath.Join(dir, "extracted")
	if err := ExtractZip(zipPath, targetDir); err != nil {
		t.Fatalf("ExtractZip() error = %v for empty zip", err)
	}
}

func TestFindBinaryInDir_TopLevel(t *testing.T) {
	dir := t.TempDir()
	dwsPath := filepath.Join(dir, "dws")
	os.WriteFile(dwsPath, []byte("#!/bin/sh"), 0755)

	got := FindBinaryInDir(dir)
	if got != dwsPath {
		t.Errorf("FindBinaryInDir() = %q, want %q", got, dwsPath)
	}
}

func TestFindBinaryInDir_Nested(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "subdir")
	os.MkdirAll(nested, 0755)
	dwsPath := filepath.Join(nested, "dws")
	os.WriteFile(dwsPath, []byte("#!/bin/sh"), 0755)

	got := FindBinaryInDir(dir)
	if got != dwsPath {
		t.Errorf("FindBinaryInDir() = %q, want %q", got, dwsPath)
	}
}

func TestFindBinaryInDir_Windows(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "dws.exe")
	os.WriteFile(exePath, []byte("MZ"), 0755)

	got := FindBinaryInDir(dir)
	if got != exePath {
		t.Errorf("FindBinaryInDir() = %q, want %q", got, exePath)
	}
}

func TestFindBinaryInDir_NotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "other.bin"), []byte("x"), 0755)

	got := FindBinaryInDir(dir)
	if got != "" {
		t.Errorf("FindBinaryInDir() = %q, want empty", got)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	os.WriteFile(src, []byte("copy me"), 0644)

	if err := copyFile(src, dst, 0755); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "copy me" {
		t.Errorf("dst content = %q, want %q", string(got), "copy me")
	}

}

func TestCopyFile_SrcNotExist(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "missing"), filepath.Join(dir, "dst"), 0644)
	if err == nil {
		t.Error("copyFile() expected error for missing src")
	}
}

func TestReplaceExeFile_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	dst := filepath.Join(dir, "dws")

	os.WriteFile(dst, []byte("old"), 0755)
	os.WriteFile(src, []byte("new"), 0755)

	if err := replaceExeFile(src, dst); err != nil {
		t.Fatalf("replaceExeFile() error = %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "new" {
		t.Errorf("dst content = %q, want %q", string(got), "new")
	}

	// src should no longer exist (it was renamed)
	if _, err := os.Stat(src); err == nil {
		t.Error("src should have been renamed away")
	}
}

func TestWindowsReplace_Basic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	dst := filepath.Join(dir, "dws.exe")

	os.WriteFile(dst, []byte("old-exe"), 0755)
	os.WriteFile(src, []byte("new-exe"), 0755)

	if err := windowsReplace(src, dst); err != nil {
		t.Fatalf("windowsReplace() error = %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "new-exe" {
		t.Errorf("dst content = %q, want %q", string(got), "new-exe")
	}

	// .old should have been cleaned up
	if _, err := os.Stat(dst + ".old"); err == nil {
		t.Error(".old file should have been cleaned up")
	}
}

func TestWindowsReplace_CleansUpStaleOld(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	dst := filepath.Join(dir, "dws.exe")
	oldPath := dst + ".old"

	os.WriteFile(dst, []byte("current"), 0755)
	os.WriteFile(src, []byte("newer"), 0755)
	os.WriteFile(oldPath, []byte("stale-old"), 0755)

	if err := windowsReplace(src, dst); err != nil {
		t.Fatalf("windowsReplace() error = %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "newer" {
		t.Errorf("dst content = %q, want %q", string(got), "newer")
	}
}

func TestCleanupStaleFiles(t *testing.T) {
	// Just verify it doesn't panic; the actual files to clean depend on os.Executable()
	CleanupStaleFiles()
}

// --- helpers ---

func createTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(content))
	}
	w.Close()
	f.Close()
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("cannot read %s: %v", path, err)
		return
	}
	if string(got) != want {
		t.Errorf("content of %s = %q, want %q", path, string(got), want)
	}
}
