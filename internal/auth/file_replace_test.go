// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type authFileWriterCase struct {
	name   string
	file   string
	rename *func(string, string) error
	write  func(string, string) error
}

func authFileWritersForTest() []authFileWriterCase {
	return []authFileWriterCase{
		{"profiles", profilesJSONFile, &profilesRename, func(dir, version string) error {
			return SaveProfiles(dir, &ProfilesConfig{Profiles: []Profile{{Name: version, CorpID: version}}})
		}},
		{"secure", secureDataFile, &secureRename, func(dir, version string) error {
			return SaveSecureTokenData(dir, &TokenData{AccessToken: "fixture-" + version})
		}},
	}
}

func TestCrossPlatformCoverageAuthFileWritersRetryWithoutRemovingOldFile(t *testing.T) {
	for _, writer := range authFileWritersForTest() {
		t.Run(writer.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := writer.write(dir, "old"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, writer.file)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			busy := errors.New("reader holds file")
			testseam.Swap(t, &authFileRenameBusy, func(err error) bool { return errors.Is(err, busy) })
			attempts := 0
			testseam.Swap(t, writer.rename, func(src, dst string) error {
				attempts++
				old, err := os.ReadFile(dst)
				if err != nil || !bytes.Equal(old, before) {
					t.Fatalf("old file changed before replacement: %v", err)
				}
				if attempts == 1 {
					return busy
				}
				return os.Rename(src, dst)
			})
			if err := writer.write(dir, "new"); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil || bytes.Equal(after, before) || attempts != 2 {
				t.Fatalf("replacement failed: attempts=%d err=%v", attempts, err)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 1 || files[0].Name() != writer.file {
				t.Fatalf("unexpected files after replacement: %v", err)
			}
		})
	}
}
