// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package localio

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageReadTextInputLiteralStdinAndFileE2E(t *testing.T) {
	if got, err := ReadTextInput("literal", nil, 10); err != nil || got != "literal" {
		t.Fatalf("literal = %q, %v", got, err)
	}
	if got, err := ReadTextInput("-", strings.NewReader("stdin"), 10); err != nil || got != "stdin" {
		t.Fatalf("stdin = %q, %v", got, err)
	}
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.WriteFile(filepath.Join(dir, "input.json"), []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadTextInput("@input.json", nil, 10); err != nil || got != "file" {
		t.Fatalf("file = %q, %v", got, err)
	}
}

func TestCrossPlatformCoverageReadTextInputRejectsEscapeAndOversizeE2E(t *testing.T) {
	for _, spec := range []string{"@../secret", "@/tmp/secret", "@"} {
		if _, err := ReadTextInput(spec, nil, 10); err == nil {
			t.Fatalf("unsafe input accepted: %q", spec)
		}
	}
	if _, err := ReadTextInput("too-large", nil, 2); err == nil {
		t.Fatal("oversize literal accepted")
	}
	if _, err := ReadTextInput("-", strings.NewReader("too-large"), 2); err == nil {
		t.Fatal("oversize stdin accepted")
	}
}

func TestCrossPlatformCoverageReadTextInputFailureBranchesE2E(t *testing.T) {
	if got, err := ReadTextInput("literal", nil, 0); err != nil || got != "literal" {
		t.Fatalf("default limit literal = %q, %v", got, err)
	}
	if _, err := ReadTextInput("-", nil, 1); err == nil {
		t.Fatal("nil stdin accepted")
	}
	t.Run("stdin read", func(t *testing.T) {
		testseam.Swap(t, &readTextInputAll, func(io.Reader) ([]byte, error) { return nil, errors.New("read") })
		if _, err := ReadTextInput("-", strings.NewReader("x"), 1); err == nil {
			t.Fatal("stdin read failure ignored")
		}
	})

	dir := t.TempDir()
	file := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(file, []byte("long"), 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if _, err := ReadTextInput("@input.txt", nil, 1); err == nil {
		t.Fatal("oversize file accepted")
	}
	if _, err := ReadTextInput("@.", nil, 10); err == nil {
		t.Fatal("directory accepted")
	}

	for name, run := range map[string]func(*testing.T){
		"getwd": func(t *testing.T) {
			testseam.Swap(t, &localGetwd, func() (string, error) { return "", errors.New("getwd") })
		},
		"eval base": func(t *testing.T) {
			testseam.Swap(t, &localEvalSymlinks, func(string) (string, error) { return "", errors.New("eval base") })
		},
		"eval path": func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &localEvalSymlinks, func(path string) (string, error) {
				calls++
				if calls == 2 {
					return "", errors.New("eval path")
				}
				return path, nil
			})
		},
		"relative": func(t *testing.T) {
			testseam.Swap(t, &readTextInputRel, func(string, string) (string, error) { return "", errors.New("rel") })
		},
		"path stat": func(t *testing.T) {
			testseam.Swap(t, &statTextInputPath, func(string) (os.FileInfo, error) { return nil, errors.New("stat") })
		},
		"open": func(t *testing.T) {
			testseam.Swap(t, &openTextInputFile, func(string) (*os.File, error) { return nil, errors.New("open") })
		},
		"opened file stat": func(t *testing.T) {
			testseam.Swap(t, &openTextInputFile, func(candidate string) (*os.File, error) {
				file, openErr := os.Open(candidate)
				if openErr == nil {
					_ = file.Close()
				}
				return file, openErr
			})
		},
		"read file": func(t *testing.T) {
			testseam.Swap(t, &readTextInputAll, func(io.Reader) ([]byte, error) { return nil, errors.New("read file") })
		},
	} {
		t.Run(name, func(t *testing.T) {
			run(t)
			if _, err := ReadTextInput("@input.txt", nil, 10); err == nil {
				t.Fatalf("%s failure ignored", name)
			}
		})
	}

	t.Run("resolved escape", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &localEvalSymlinks, func(path string) (string, error) {
			calls++
			if calls == 2 {
				return filepath.Join(filepath.Dir(path), "..", "outside"), nil
			}
			return path, nil
		})
		if _, err := ReadTextInput("@input.txt", nil, 10); err == nil {
			t.Fatal("resolved escape accepted")
		}
	})
}
