package runtimepayload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAdjacentEmbeddedWarmReuse(t *testing.T) {
	if _, err := LibraryName(runtime.GOOS, runtime.GOARCH); err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	container := Embedded()
	path, err := MaterializeAdjacent(container, root, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	// Guard filesystem work deterministically instead of asserting wall time on
	// shared CI hosts. Use the real payload, without loading the native library.
	testseam.Swap(t, &makeCacheTemporary, func(string, string) (string, error) {
		t.Error("warm reuse attempted to stage a second bundle")
		return "", errors.New("unexpected staging")
	})
	testseam.Swap(t, &extractPayload, func(io.Reader, string) error {
		t.Error("warm reuse attempted to extract the payload")
		return errors.New("unexpected extraction")
	})
	hashed := make(map[string]int)
	testseam.Swap(t, &openHashInput, func(path string) (io.ReadCloser, error) {
		hashed[path]++
		return os.Open(path)
	})
	got, err := MaterializeAdjacent(container, root, runtime.GOOS, runtime.GOARCH)
	if err != nil || got != path {
		t.Fatalf("warm reuse: path=%q, err=%v", got, err)
	}
	if len(hashed) != 124 {
		t.Fatalf("hashed %d resources, want the library and all 123 ps files", len(hashed))
	}
	for path, count := range hashed {
		if count != 1 || (filepath.Dir(path) != root && filepath.Dir(path) != filepath.Join(root, "ps")) {
			t.Fatalf("resource %q hashed %d times", path, count)
		}
	}
}

func TestCrossPlatformCoverageAdjacentRejectsSelfAssertedChecksums(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path, err := MaterializeAdjacent(container, root, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	original, _, err := readOwnership(root, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	forged := original
	content := []byte("untrusted replacement")
	digest := sha256.Sum256(content)
	forged.Manifest.LibrarySHA256 = hex.EncodeToString(digest[:])
	data, _ := json.Marshal(forged)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ownershipName), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateRootManifest(root, forged.Manifest, "linux", "amd64"); err != nil {
		t.Fatal("fixture must pass its own forged checksums", err)
	}
	if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	current, _, err := readOwnership(root, "linux", "amd64")
	if err != nil || current != original || validateRootManifest(root, original.Manifest, "linux", "amd64") != nil {
		t.Fatal("reuse trusted a self-asserted disk checksum")
	}
	t.Run("unreadable embedded manifest", func(t *testing.T) {
		testseam.Swap(t, &newPayloadGzipReader, func(io.Reader) (io.ReadCloser, error) {
			return nil, errors.New("private failure")
		})
		if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil || err.Error() != "adjacent runtime payload unavailable" {
			t.Fatal("manifest failure must fail closed with a neutral error")
		}
	})
}

func TestCrossPlatformCoverageArchiveManifestValidation(t *testing.T) {
	for name, data := range map[string][]byte{
		"gzip":         []byte("invalid"),
		"missing":      archiveWithHeader(t, &tar.Header{Name: "libx7k2m9p4q1w8.so", Typeflag: tar.TypeReg}),
		"path":         archiveWithHeader(t, &tar.Header{Name: "../manifest.json", Typeflag: tar.TypeReg}),
		"symlink":      archiveWithHeader(t, &tar.Header{Name: "manifest.json", Typeflag: tar.TypeSymlink}),
		"invalid JSON": maliciousArchive(t, "manifest.json"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readArchiveManifest(bytes.NewReader(data)); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	for _, scenario := range []string{"manifest size", "negative size", "file size", "total size", "entry count"} {
		t.Run(scenario, func(t *testing.T) {
			header := &tar.Header{Name: "libx7k2m9p4q1w8.so", Typeflag: tar.TypeReg}
			switch scenario {
			case "manifest size":
				header.Name, header.Size = "manifest.json", 8193
			case "negative size":
				header.Size = -1
			case "file size":
				header.Size = maxFileBytes + 1
			case "total size":
				header.Size = maxFileBytes
			}
			testseam.Swap(t, &nextPayloadEntry, func(*tar.Reader) (*tar.Header, error) { return header, nil })
			data := archiveWithHeader(t, &tar.Header{Name: "manifest.json", Typeflag: tar.TypeReg})
			if _, err := readArchiveManifest(bytes.NewReader(data)); err == nil {
				t.Fatal("archive bounds ignored")
			}
		})
	}
	t.Run("truncated manifest", func(t *testing.T) {
		var output bytes.Buffer
		gz := gzip.NewWriter(&output)
		tarWriter := tar.NewWriter(gz)
		if err := tarWriter.WriteHeader(&tar.Header{Name: "manifest.json", Typeflag: tar.TypeReg, Size: 10}); err != nil {
			t.Fatal(err)
		}
		// Deliberately close gzip before supplying the declared tar body.
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := readArchiveManifest(bytes.NewReader(output.Bytes())); err == nil {
			t.Fatal("truncated manifest accepted")
		}
	})
}
