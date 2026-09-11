package runtimepayload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageLibraryOnlyAssets(t *testing.T) {
	for _, target := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64"} {
		t.Run(target, func(t *testing.T) {
			container, err := os.ReadFile(filepath.Join("assets", target+".payload"))
			if err != nil {
				t.Fatal(err)
			}
			descriptor, err := Inspect(container)
			if err != nil {
				t.Fatal(err)
			}
			goos, goarch, _ := strings.Cut(target, "-")
			library, err := Materialize(container, t.TempDir(), goos, goarch)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Dir(library)
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 2 {
				t.Fatalf("library-only root = %v, %v", entries, err)
			}
			if _, err := os.Lstat(filepath.Join(root, "ps")); !os.IsNotExist(err) {
				t.Fatalf("retired data directory: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
			if err != nil || bytes.Contains(data, []byte("ps_")) || !bytes.Contains(data, []byte(PayloadVersion)) {
				t.Fatalf("manifest still requires retired data: %s, %v", data, err)
			}
			gz, err := gzip.NewReader(bytes.NewReader(container[containerHeader : containerHeader+descriptor.Size]))
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			archive := tar.NewReader(gz)
			for range 2 {
				header, err := archive.Next()
				if err != nil || (header.Name != "manifest.json" && header.Name != filepath.Base(library)) {
					t.Fatalf("unexpected archive entry: %v, %v", header, err)
				}
			}
			if _, err := archive.Next(); err != io.EOF {
				t.Fatalf("extra archive entry: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageLibraryOnlyLegacyUpgrade(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"20260825", "20260908"} {
		for _, state := range []string{"ready", "pending"} {
			t.Run(version+"/"+state, func(t *testing.T) {
				root := t.TempDir()
				library, err := MaterializeAdjacent(container, root, "linux", "amd64")
				if err != nil {
					t.Fatal(err)
				}
				previous, _, err := readOwnership(root, "linux", "amd64")
				if err != nil {
					t.Fatal(err)
				}
				previous.State = state
				previous.Manifest.FormatVersion = 1
				previous.Manifest.PayloadVersion = version
				previous.Manifest.PSFileCount = 123
				previous.Manifest.PSManifestSHA256 = strings.Repeat("a", 64)
				previous.PayloadSHA256 = strings.Repeat("b", 64)
				data, _ := json.Marshal(previous)
				if err := os.WriteFile(filepath.Join(root, ownershipName), data, 0600); err != nil {
					t.Fatal(err)
				}
				ps := filepath.Join(root, "ps")
				if err := os.Mkdir(ps, 0700); err != nil {
					t.Fatal(err)
				}
				legacyFile := filepath.Join(ps, "preserve-user-data")
				if err := os.WriteFile(legacyFile, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(library, []byte("obsolete library"), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
					t.Fatal(err)
				}
				current, owned, err := readOwnership(root, "linux", "amd64")
				if err != nil || !owned || current.State != "ready" || current.Manifest.FormatVersion != manifestVersion || current.Manifest.PayloadVersion != PayloadVersion || current.Manifest.PSFileCount != 0 || current.Manifest.PSManifestSHA256 != "" {
					t.Fatal("legacy ownership was not upgraded", err)
				}
				if data, err := os.ReadFile(legacyFile); err != nil || string(data) != "preserve" {
					t.Fatal("legacy data was modified", err)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageLibraryOnlySameVersionRefresh(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			source := writePayloadFixture(t, "darwin", arch)
			previous, err := BuildContainer(source, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			adjacent, cache := t.TempDir(), t.TempDir()
			previousAdjacent, err := MaterializeAdjacent(previous, adjacent, "darwin", arch)
			if err != nil {
				t.Fatal(err)
			}
			previousCache, err := Materialize(previous, cache, "darwin", arch)
			if err != nil {
				t.Fatal(err)
			}
			previousOwnership, owned, err := readOwnership(adjacent, "darwin", arch)
			if err != nil || !owned {
				t.Fatalf("previous ownership = %v, %v", owned, err)
			}

			content := []byte("refreshed-library-darwin-" + arch)
			digest := sha256.Sum256(content)
			if err := os.WriteFile(filepath.Join(source, previousOwnership.Manifest.Library), content, 0o700); err != nil {
				t.Fatal(err)
			}
			rewritePayloadManifest(t, source, func(value *manifest) {
				value.LibrarySHA256 = hex.EncodeToString(digest[:])
			})
			refreshed, err := BuildContainer(source, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := readManifest(source)
			if err != nil {
				t.Fatal(err)
			}
			descriptor, err := Inspect(refreshed)
			if err != nil {
				t.Fatal(err)
			}
			if expected.PayloadVersion != previousOwnership.Manifest.PayloadVersion {
				t.Fatal("fixture changed the collection version")
			}
			if hex.EncodeToString(descriptor.SHA256[:]) == previousOwnership.PayloadSHA256 {
				t.Fatal("fixture did not change the payload digest")
			}

			updatedAdjacent, err := MaterializeAdjacent(refreshed, adjacent, "darwin", arch)
			if err != nil || updatedAdjacent != previousAdjacent {
				t.Fatalf("adjacent refresh = %q, %v", updatedAdjacent, err)
			}
			current, owned, err := readOwnership(adjacent, "darwin", arch)
			if err != nil || !owned || current.State != "ready" || current.Manifest != expected || current.PayloadSHA256 != hex.EncodeToString(descriptor.SHA256[:]) {
				t.Fatalf("refreshed ownership = %#v, %v", current, err)
			}
			updatedCache, err := Materialize(refreshed, cache, "darwin", arch)
			if err != nil || filepath.Dir(updatedCache) == filepath.Dir(previousCache) {
				t.Fatalf("cache did not isolate the refreshed payload: %q, %v", updatedCache, err)
			}
			if old, err := os.ReadFile(previousCache); err != nil || string(old) != "library-darwin-"+arch {
				t.Fatalf("previous cache was modified: %q, %v", old, err)
			}

			for _, location := range []struct {
				name  string
				path  string
				reuse func() (string, error)
			}{
				{"adjacent", updatedAdjacent, func() (string, error) { return MaterializeAdjacent(refreshed, adjacent, "darwin", arch) }},
				{"cache", updatedCache, func() (string, error) { return Materialize(refreshed, cache, "darwin", arch) }},
			} {
				t.Run(location.name, func(t *testing.T) {
					if data, err := os.ReadFile(location.path); err != nil || !bytes.Equal(data, content) {
						t.Fatalf("refresh returned old library: %q, %v", data, err)
					}
					if err := validateRootManifest(filepath.Dir(location.path), expected, "darwin", arch); err != nil {
						t.Fatal(err)
					}
					before, err := os.Stat(location.path)
					if err != nil {
						t.Fatal(err)
					}
					if reused, err := location.reuse(); err != nil || reused != location.path {
						t.Fatalf("refreshed reuse = %q, %v", reused, err)
					}
					after, err := os.Stat(location.path)
					if err != nil || !os.SameFile(before, after) {
						t.Fatalf("valid refreshed library was replaced: %v", err)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageLibraryOnlyIgnoresUnownedPS(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			ps := filepath.Join(root, "ps")
			switch kind {
			case "file":
				err = os.WriteFile(ps, []byte("user data"), 0600)
			case "directory":
				err = os.Mkdir(ps, 0700)
			case "symlink":
				err = os.Symlink(filepath.Join(t.TempDir(), "missing"), ps)
				if err != nil {
					t.Skip("symlinks unavailable")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(ps)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
					t.Fatal("unused ps path prevented installation or reuse", err)
				}
			}
			after, err := os.Lstat(ps)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("unowned ps path was replaced", err)
			}
		})
	}
}

func TestCrossPlatformCoverageLibraryOnlyRejectsPSArchive(t *testing.T) {
	archive := maliciousArchive(t, "ps/00000000000000000000000000000000")
	if err := extractArchive(bytes.NewReader(archive), t.TempDir()); err == nil {
		t.Fatal("retired ps file accepted")
	}
}

func TestCrossPlatformCoverageLibraryOnlyRejectsInvalidOwnership(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"legacy version", "legacy count", "legacy digest", "new count", "new digest", "unknown format"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			library, err := MaterializeAdjacent(container, root, "linux", "amd64")
			if err != nil {
				t.Fatal(err)
			}
			value, _, err := readOwnership(root, "linux", "amd64")
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(scenario, "legacy") {
				value.Manifest.FormatVersion = 1
				value.Manifest.PayloadVersion = "20260908"
				value.Manifest.PSFileCount = 123
				value.Manifest.PSManifestSHA256 = strings.Repeat("a", 64)
			}
			switch scenario {
			case "legacy version":
				value.Manifest.PayloadVersion = "19990101"
			case "legacy count", "new count":
				value.Manifest.PSFileCount = 1
			case "legacy digest", "new digest":
				value.Manifest.PSManifestSHA256 = "invalid"
			case "unknown format":
				value.Manifest.FormatVersion = 3
			}
			data, _ := json.Marshal(value)
			if err := os.WriteFile(filepath.Join(root, ownershipName), data, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(library)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil {
				t.Fatal("invalid ownership authorized replacement")
			}
			after, err := os.Stat(library)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("invalid ownership changed the library", err)
			}
		})
	}
}

func TestCrossPlatformCoverageLibraryOnlyCachePinsEmbeddedManifest(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	library, err := Materialize(container, cache, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(library)
	forged := []byte("replacement with a self-asserted checksum")
	digest := sha256.Sum256(forged)
	if err := os.WriteFile(library, forged, 0600); err != nil {
		t.Fatal(err)
	}
	rewritePayloadManifest(t, root, func(value *manifest) { value.LibrarySHA256 = hex.EncodeToString(digest[:]) })
	if err := validateRoot(root, "linux", "amd64"); err != nil {
		t.Fatal("fixture should pass its own checksums", err)
	}
	// The forged destination can win a publication race, but it must never be
	// returned as a load source. A subsequent uncontended call repairs it.
	t.Run("racing destination", func(t *testing.T) {
		testseam.Swap(t, &publishPayload, func(string, string, string, string) error { return nil })
		if _, err := Materialize(container, cache, "linux", "amd64"); err == nil {
			t.Fatal("self-asserted cache was returned after publication")
		}
	})
	if _, err := Materialize(container, cache, "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(library); err != nil || bytes.Equal(data, forged) {
		t.Fatal("cache was not repaired", err)
	}
	t.Run("unreadable embedded manifest", func(t *testing.T) {
		testseam.Swap(t, &newPayloadGzipReader, func(io.Reader) (io.ReadCloser, error) { return nil, io.ErrUnexpectedEOF })
		if _, err := Materialize(container, cache, "linux", "amd64"); err == nil {
			t.Fatal("cached files bypassed invalid embedded manifest")
		}
	})
}
