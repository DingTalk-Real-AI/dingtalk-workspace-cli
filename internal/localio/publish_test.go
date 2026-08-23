// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package localio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoveragePublishBytesAtomicNoClobberE2E(t *testing.T) {
	base := t.TempDir()
	result, err := PublishBytes([]byte("verified artifact"), PublishBytesOptions{BaseDir: base, Output: "out/", PreferredName: "transcript.json"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(base, result.RelativePath))
	if err != nil || string(raw) != "verified artifact" || result.SizeBytes != int64(len(raw)) {
		t.Fatalf("publish result=%#v raw=%q err=%v", result, raw, err)
	}
	if _, err := PublishBytes([]byte("overwrite"), PublishBytesOptions{BaseDir: base, Output: "out/transcript.json"}); err == nil {
		t.Fatal("existing output was overwritten")
	}
	if _, err := PublishBytes([]byte("escape"), PublishBytesOptions{BaseDir: base, Output: "../escape"}); err == nil {
		t.Fatal("path escape accepted")
	}
}

func TestCrossPlatformCoveragePublishBytesFailureBranchesE2E(t *testing.T) {
	base := t.TempDir()
	if _, err := PublishBytes([]byte("xx"), PublishBytesOptions{BaseDir: base, Output: "large", MaxBytes: 1}); err == nil {
		t.Fatal("oversize payload accepted")
	}
	if _, err := PublishBytes([]byte("x"), PublishBytesOptions{BaseDir: base, Output: "default-limit", MaxBytes: 0}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		make func(*os.Root) (downloadTempFile, string, error)
	}{
		{name: "create", make: func(*os.Root) (downloadTempFile, string, error) { return nil, "", errors.New("create") }},
		{name: "write", make: func(root *os.Root) (downloadTempFile, string, error) {
			file, name, err := createDownloadTempInRoot(root)
			return &coverageTempFile{file: file.(*os.File), writeErr: errors.New("write")}, name, err
		}},
		{name: "sync", make: func(root *os.Root) (downloadTempFile, string, error) {
			file, name, err := createDownloadTempInRoot(root)
			return &coverageTempFile{file: file.(*os.File), syncErr: errors.New("sync")}, name, err
		}},
		{name: "close", make: func(root *os.Root) (downloadTempFile, string, error) {
			file, name, err := createDownloadTempInRoot(root)
			return &coverageTempFile{file: file.(*os.File), closeErr: errors.New("close")}, name, err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &createDownloadTemp, tc.make)
			if _, err := PublishBytes([]byte("x"), PublishBytesOptions{BaseDir: base, Output: tc.name}); err == nil {
				t.Fatalf("%s failure ignored", tc.name)
			}
		})
	}

	t.Run("initial parent verification", func(t *testing.T) {
		testseam.Swap(t, &verifyPublishTarget, func(*downloadTarget) error { return errors.New("verify") })
		if _, err := PublishBytes([]byte("x"), PublishBytesOptions{BaseDir: base, Output: "verify-initial"}); err == nil {
			t.Fatal("initial verification failure ignored")
		}
	})
	t.Run("second parent verification", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &verifyPublishTarget, func(*downloadTarget) error {
			calls++
			if calls > 1 {
				return errors.New("changed")
			}
			return nil
		})
		if _, err := PublishBytes([]byte("x"), PublishBytesOptions{BaseDir: base, Output: "verify-second"}); err == nil {
			t.Fatal("second verification failure ignored")
		}
	})
	t.Run("publish", func(t *testing.T) {
		testseam.Swap(t, &downloadRootLink, func(*os.Root, string, string) error { return errors.New("link") })
		if _, err := PublishBytes([]byte("x"), PublishBytesOptions{BaseDir: base, Output: "publish"}); err == nil {
			t.Fatal("publish failure ignored")
		}
	})
}
