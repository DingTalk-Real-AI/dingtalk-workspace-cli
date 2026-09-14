// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package skillprovenance

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeDirEntry struct {
	name string
	mode fs.FileMode
}

func (entry fakeDirEntry) Name() string               { return entry.name }
func (entry fakeDirEntry) IsDir() bool                { return entry.mode.IsDir() }
func (entry fakeDirEntry) Type() fs.FileMode          { return entry.mode.Type() }
func (entry fakeDirEntry) Info() (fs.FileInfo, error) { return nil, errors.New("unused") }

func swapProvenanceSeam[T any](t *testing.T, target *T, replacement T) {
	t.Helper()
	original := *target
	*target = replacement
	t.Cleanup(func() { *target = original })
}

func TestCrossPlatformCoverageSkillProvenanceBuildDigestAndMerge(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "a.md"), []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := Build("dingtalk-a", dir, "", SourceUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != "unknown" || record.Source != SourceUpgrade || record.DigestScope != DigestScope {
		t.Fatalf("record = %#v", record)
	}
	first := record.Digest
	if err := os.WriteFile(filepath.Join(dir, "references", "a.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := Build("dingtalk-a", dir, "v2", SourceSkillSetup)
	if err != nil || second.Digest == first {
		t.Fatalf("second = %#v, %v", second, err)
	}
	merged := Merge([]Record{record, {Name: "dingtalk-b"}}, []Record{second, {}})
	if !reflect.DeepEqual(merged, []Record{second, {Name: "dingtalk-b"}}) {
		t.Fatalf("merged = %#v", merged)
	}
	if got := Names(merged); !got["dingtalk-a"] || !got["dingtalk-b"] || len(got) != 2 {
		t.Fatalf("names = %#v", got)
	}
	if _, err := Build("", dir, "v1", "test"); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := Build("dingtalk-a", dir, "v1", " "); err == nil {
		t.Fatal("empty source accepted")
	}
	if _, err := Build("dingtalk-a", filepath.Join(dir, "missing"), "v1", "test"); err == nil {
		t.Fatal("missing Skill directory accepted")
	}
}

func TestCrossPlatformCoverageSkillProvenanceDigestErrors(t *testing.T) {
	want := errors.New("injected")
	t.Run("walk", func(t *testing.T) {
		swapProvenanceSeam(t, &provenanceWalkDir, func(string, fs.WalkDirFunc) error { return want })
		if _, err := DigestDir(t.TempDir()); !errors.Is(err, want) {
			t.Fatal(err)
		}
	})
	t.Run("callback", func(t *testing.T) {
		swapProvenanceSeam(t, &provenanceWalkDir, func(root string, walk fs.WalkDirFunc) error { return walk(root, nil, want) })
		if _, err := DigestDir(t.TempDir()); !errors.Is(err, want) {
			t.Fatal(err)
		}
	})
	t.Run("relative", func(t *testing.T) {
		swapProvenanceSeam(t, &provenanceWalkDir, func(root string, walk fs.WalkDirFunc) error {
			return walk(filepath.Join(root, "file"), fakeDirEntry{name: "file"}, nil)
		})
		swapProvenanceSeam(t, &provenanceRelative, func(string, string) (string, error) { return "", want })
		if _, err := DigestDir(t.TempDir()); !errors.Is(err, want) {
			t.Fatal(err)
		}
	})
	t.Run("nonregular", func(t *testing.T) {
		swapProvenanceSeam(t, &provenanceWalkDir, func(root string, walk fs.WalkDirFunc) error {
			return walk(filepath.Join(root, "link"), fakeDirEntry{name: "link", mode: fs.ModeSymlink}, nil)
		})
		if _, err := DigestDir(t.TempDir()); err == nil {
			t.Fatal("nonregular accepted")
		}
	})
	t.Run("read", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644)
		swapProvenanceSeam(t, &provenanceReadFile, func(string) ([]byte, error) { return nil, want })
		if _, err := DigestDir(dir); !errors.Is(err, want) {
			t.Fatal(err)
		}
	})
}
