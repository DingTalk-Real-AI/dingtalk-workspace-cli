package packagemanifest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildWriteAndVerifyTree(t *testing.T) {
	root, identity := createPackage(t, Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH})
	manifest, err := Build(root, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, manifest); err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyTree(root, identity)
	if err != nil {
		t.Fatal(err)
	}
	if verified != manifest {
		t.Fatalf("verified manifest = %#v, want %#v", verified, manifest)
	}
	data, err := os.ReadFile(filepath.Join(root, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := MarshalCanonical(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, canonical) || len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("written manifest is not canonical: %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(root, ManifestName))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("manifest mode = %04o", info.Mode().Perm())
		}
	}
}

func TestWindowsFixedPathsAndMode(t *testing.T) {
	target := Target{GOOS: "windows", GOARCH: "amd64"}
	launcher, core, err := Paths(target)
	if err != nil {
		t.Fatal(err)
	}
	if launcher != filepath.Join("bin", "dws.exe") || core != filepath.Join("libexec", "dws-core.exe") {
		t.Fatalf("paths = %q, %q", launcher, core)
	}
	root, identity := createPackage(t, target)
	manifest, err := Build(root, identity)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Launcher.Mode != 0 || manifest.Core.Mode != 0 {
		t.Fatalf("Windows modes = %o, %o", manifest.Launcher.Mode, manifest.Core.Mode)
	}
}

func TestLegacyUpgradeEntryMustBeExactCore(t *testing.T) {
	for _, target := range []Target{{GOOS: "darwin", GOARCH: "arm64"}, {GOOS: "linux", GOARCH: "amd64"}, {GOOS: "windows", GOARCH: "amd64"}} {
		t.Run(target.GOOS, func(t *testing.T) {
			root, identity := createPackage(t, target)
			manifest, err := Build(root, identity)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(root, manifest.Core.Path))
			if err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(t.TempDir(), "dws")
			if err := os.WriteFile(legacy, body, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := VerifyLegacyEntry(legacy, manifest); err != nil {
				t.Fatalf("exact core rejected: %v", err)
			}
			body[0] ^= 1
			if err := os.WriteFile(legacy, body, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := VerifyLegacyEntry(legacy, manifest); err == nil {
				t.Fatal("same-size tampered legacy upgrade entry accepted")
			}
		})
	}
}

func TestDecodeIsStrict(t *testing.T) {
	root, identity := createPackage(t, Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH})
	manifest, err := Build(root, identity)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := MarshalCanonical(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(bytes.NewReader(canonical)); err != nil {
		t.Fatal(err)
	}

	var object map[string]any
	if err := json.Unmarshal(canonical, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	cases := map[string][]byte{
		"unknown":   unknown,
		"trailing":  append(append([]byte(nil), canonical...), []byte("{}")...),
		"duplicate": []byte(strings.Replace(string(canonical), `"layout_version":1`, `"layout_version":1,"layout_version":1`, 1)),
		"oversized": bytes.Repeat([]byte(" "), maxManifestSize+1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(bytes.NewReader(data)); err == nil {
				t.Fatal("Decode succeeded")
			}
		})
	}
}

func TestManifestValidation(t *testing.T) {
	root, identity := createPackage(t, Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH})
	valid, err := Build(root, identity)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Manifest){
		"layout":           func(m *Manifest) { m.LayoutVersion++ },
		"empty release":    func(m *Manifest) { m.Release.Version = "" },
		"control release":  func(m *Manifest) { m.Release.Commit = "bad\ncommit" },
		"invalid target":   func(m *Manifest) { m.Target.GOOS = "WINDOWS" },
		"invalid pair":     func(m *Manifest) { m.Target = Target{GOOS: "windows", GOARCH: "wasm"} },
		"arbitrary path":   func(m *Manifest) { m.Launcher.Path = "other/dws" },
		"uppercase digest": func(m *Manifest) { m.Launcher.SHA256 = strings.ToUpper(m.Launcher.SHA256) },
		"short digest":     func(m *Manifest) { m.Core.SHA256 = "00" },
		"zero size":        func(m *Manifest) { m.Core.Size = 0 },
		"large size":       func(m *Manifest) { m.Core.Size = MaxExecutableSize + 1 },
	}
	if valid.Target.GOOS == "windows" {
		tests["windows mode"] = func(m *Manifest) { m.Core.Mode = 0o755 }
	} else {
		tests["non-executable mode"] = func(m *Manifest) { m.Core.Mode = 0o644 }
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate succeeded")
			}
		})
	}
}

func TestVerifyTreeRejectsInvalidTrees(t *testing.T) {
	tests := map[string]func(*testing.T, string, Identity){
		"extra file": func(t *testing.T, root string, _ Identity) {
			writeFile(t, filepath.Join(root, "extra"), []byte("extra"), 0o755)
		},
		"missing core": func(t *testing.T, root string, identity Identity) {
			_, core, _ := Paths(identity.Target)
			if err := os.Remove(filepath.Join(root, core)); err != nil {
				t.Fatal(err)
			}
		},
		"launcher bytes": func(t *testing.T, root string, identity Identity) {
			launcher, _, _ := Paths(identity.Target)
			writeFile(t, filepath.Join(root, launcher), []byte("changed launcher"), executableMode(identity.Target))
		},
		"same size hash": func(t *testing.T, root string, identity Identity) {
			launcher, _, _ := Paths(identity.Target)
			writeFile(t, filepath.Join(root, launcher), []byte("LAUNCHER BYTES"), executableMode(identity.Target))
		},
		"identity mismatch": func(t *testing.T, _ string, identity Identity) {
			identity.Release.Commit = "other"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root, identity := finalizedPackage(t)
			mutate(t, root, identity)
			if name == "identity mismatch" {
				identity.Release.Commit = "other"
			}
			if _, err := VerifyTree(root, identity); err == nil {
				t.Fatal("VerifyTree succeeded")
			}
		})
	}
}

func TestVerifyTreeRejectsModesSymlinksAndAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no portable symlink, hard-link, or executable permission semantics")
	}
	t.Run("manifest mode", func(t *testing.T) {
		root, identity := finalizedPackage(t)
		if err := os.Chmod(filepath.Join(root, ManifestName), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyTree(root, identity); err == nil {
			t.Fatal("VerifyTree succeeded")
		}
	})
	t.Run("binary mode", func(t *testing.T) {
		root, identity := finalizedPackage(t)
		launcher, _, _ := Paths(identity.Target)
		if err := os.Chmod(filepath.Join(root, launcher), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyTree(root, identity); err == nil {
			t.Fatal("VerifyTree succeeded")
		}
	})
	t.Run("different executable mode", func(t *testing.T) {
		root, identity := finalizedPackage(t)
		launcher, _, _ := Paths(identity.Target)
		if err := os.Chmod(filepath.Join(root, launcher), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyTree(root, identity); err == nil {
			t.Fatal("VerifyTree succeeded")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		root, identity := finalizedPackage(t)
		launcher, _, _ := Paths(identity.Target)
		path := filepath.Join(root, launcher)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "libexec", "dws-core"), path); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyTree(root, identity); err == nil {
			t.Fatal("VerifyTree succeeded")
		}
	})
	t.Run("hard-link alias", func(t *testing.T) {
		root, identity := createPackage(t, Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH})
		launcher, core, _ := Paths(identity.Target)
		if err := os.Remove(filepath.Join(root, core)); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(root, launcher), filepath.Join(root, core)); err != nil {
			t.Fatal(err)
		}
		if _, err := Build(root, identity); err == nil {
			t.Fatal("Build succeeded")
		}
	})
}

func TestVerifyTreeRejectsManifestSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require privileges")
	}
	root, identity := finalizedPackage(t)
	manifestPath := filepath.Join(root, ManifestName)
	realPath := filepath.Join(t.TempDir(), ManifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, realPath, data, 0o644)
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPath, manifestPath); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyTree(root, identity); err == nil {
		t.Fatal("VerifyTree succeeded")
	}
}

func finalizedPackage(t *testing.T) (string, Identity) {
	t.Helper()
	root, identity := createPackage(t, Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH})
	manifest, err := Build(root, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, manifest); err != nil {
		t.Fatal(err)
	}
	return root, identity
}

func createPackage(t *testing.T, target Target) (string, Identity) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "libexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcher, core, err := Paths(target)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, launcher), []byte("launcher bytes"), executableMode(target))
	writeFile(t, filepath.Join(root, core), []byte("core bytes"), executableMode(target))
	return root, Identity{
		Release: Release{Version: "v1.2.3", Commit: "0123456789abcdef", Edition: "internal"},
		Target:  target,
	}
}

func executableMode(target Target) os.FileMode {
	if target.GOOS == "windows" {
		return 0o644
	}
	return 0o755
}

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
