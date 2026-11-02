package runtimepayload

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageConfigIntegrityAndUnownedFiles(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "skt.bin")
			switch kind {
			case "file":
				err = os.WriteFile(path, []byte("unknown"), 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "symlink":
				err = os.Symlink(filepath.Join(t.TempDir(), "missing"), path)
				if err != nil {
					t.Skip("symlinks unavailable")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil {
				t.Fatal("unowned configuration replaced")
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("unowned configuration changed")
			}
			library, err := Materialize(container, t.TempDir(), "linux", "amd64")
			if err != nil {
				t.Fatal("cache fallback failed", err)
			}
			config, err := os.ReadFile(filepath.Join(filepath.Dir(library), "skt.bin"))
			if err != nil || string(config) != "config-linux" {
				t.Fatal("cache configuration missing")
			}
		})
	}
	for _, where := range []string{"cache", "adjacent"} {
		t.Run(where, func(t *testing.T) {
			root := t.TempDir()
			materialize := func() (string, error) { return MaterializeAdjacent(container, root, "linux", "amd64") }
			if where == "cache" {
				materialize = func() (string, error) { return Materialize(container, root, "linux", "amd64") }
			}
			library, err := materialize()
			if err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(filepath.Dir(library), "skt.bin")
			if err := os.WriteFile(config, []byte("corrupt"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := materialize(); err != nil {
				t.Fatal("configuration repair failed", err)
			}
			raw, err := os.ReadFile(config)
			if err != nil || !bytes.Equal(raw, []byte("config-linux")) {
				t.Fatal("damaged configuration reused")
			}
		})
	}
}

func TestCrossPlatformCoverageFormatTwoUpgradeDoesNotOwnNewConfig(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, conflict := range []bool{false, true} {
		root := t.TempDir()
		library, err := MaterializeAdjacent(container, root, "linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		old, _, err := readOwnership(root, "linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		old.Manifest.FormatVersion = 2
		old.Manifest.PayloadVersion = "20260909"
		old.Manifest.Config = ""
		old.Manifest.ConfigSHA256 = ""
		old.PayloadSHA256 = strings.Repeat("a", 64)
		raw, _ := json.Marshal(old)
		if err := os.WriteFile(filepath.Join(root, ownershipName), raw, 0600); err != nil {
			t.Fatal(err)
		}
		config := filepath.Join(root, "skt.bin")
		if err := os.Remove(config); err != nil {
			t.Fatal(err)
		}
		if conflict {
			if err := os.WriteFile(config, []byte("unowned"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		_, err = MaterializeAdjacent(container, root, "linux", "amd64")
		if conflict {
			if err == nil {
				t.Fatal("old manifest authorized overwriting new configuration")
			}
			raw, _ := os.ReadFile(config)
			if string(raw) != "unowned" {
				t.Fatal("unowned configuration overwritten")
			}
		} else {
			if err != nil {
				t.Fatal("old library could not upgrade", err)
			}
			metadata, _, err := readOwnership(root, "linux", "amd64")
			if err != nil || metadata.Manifest.FormatVersion != 3 || metadata.Manifest.Config != "skt.bin" {
				t.Fatal("format three not published")
			}
		}
		if _, err := os.Stat(library); err != nil {
			t.Fatal("library lost")
		}
	}
}

func TestCrossPlatformCoverageConfigOwnershipFailureBoundaries(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	original, _, err := readOwnership(root, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*manifest)
	}{
		{"legacy version", func(m *manifest) {
			m.FormatVersion = 2
			m.PayloadVersion = "20261028"
			m.Config = ""
			m.ConfigSHA256 = ""
		}},
		{"legacy config", func(m *manifest) { m.FormatVersion = 2; m.PayloadVersion = "20260909" }},
		{"current config", func(m *manifest) { m.Config = "other.bin" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := original
			tc.mutate(&candidate.Manifest)
			raw, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ownershipName), raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, owned, err := readOwnership(dir, "linux", "amd64"); err == nil || owned {
				t.Fatal("invalid ownership accepted")
			}
		})
	}
	if err := os.Remove(filepath.Join(root, "skt.bin")); err != nil {
		t.Fatal(err)
	}
	if err := validateRootManifest(root, original.Manifest, "linux", "amd64"); err == nil {
		t.Fatal("missing configuration accepted")
	}
}
