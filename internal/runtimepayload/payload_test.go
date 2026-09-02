package runtimepayload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAppendInspectAndMaterialize(t *testing.T) {
	root := writePayloadFixture(t, "linux", "amd64")
	container, err := BuildContainer(root, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(t.TempDir(), "dws")
	base := []byte("fake executable\n")
	binary := append(append(append([]byte{}, base...), container...), []byte("signature suffix")...)
	if err := os.WriteFile(binaryPath, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := Find(first)
	if err != nil || descriptor.Offset != len(base) || descriptor.Size <= 0 || descriptor.ContainerSize != len(container) {
		t.Fatalf("descriptor = %#v, %v", descriptor, err)
	}

	cache := t.TempDir()
	library, err := MaterializeFile(binaryPath, cache, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(library); err != nil || string(content) != "library-linux-amd64" {
		t.Fatalf("library = %q, %v", content, err)
	}
	if entries, err := os.ReadDir(filepath.Join(filepath.Dir(library), "ps")); err != nil || len(entries) != 123 {
		t.Fatalf("payload entries = %d, %v", len(entries), err)
	}

	if err := InjectFile(binaryPath, root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("deterministic payload replacement changed the executable")
	}
	if secondLibrary, err := Materialize(container, cache, "linux", "amd64"); err != nil || secondLibrary != library {
		t.Fatalf("cached library = %q, %v", secondLibrary, err)
	}
}

func TestCrossPlatformCoverageMaterializeConcurrentAndRepairsCache(t *testing.T) {
	root := writePayloadFixture(t, "darwin", "arm64")
	container, err := BuildContainer(root, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	var wait sync.WaitGroup
	paths := make(chan string, 12)
	errorsFound := make(chan error, 12)
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			path, err := Materialize(container, cache, "darwin", "arm64")
			paths <- path
			errorsFound <- err
		}()
	}
	wait.Wait()
	close(paths)
	close(errorsFound)
	var expected string
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	for path := range paths {
		if expected == "" {
			expected = path
		}
		if path != expected {
			t.Fatalf("cache paths differ: %q != %q", path, expected)
		}
	}
	psEntries, err := os.ReadDir(filepath.Join(filepath.Dir(expected), "ps"))
	if err != nil {
		t.Fatal(err)
	}
	corrupt := filepath.Join(filepath.Dir(expected), "ps", psEntries[0].Name())
	if err := os.WriteFile(corrupt, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	repaired, err := Materialize(container, cache, "darwin", "arm64")
	if err != nil || repaired != expected {
		t.Fatalf("repaired cache = %q, %v", repaired, err)
	}
	if content, _ := os.ReadFile(corrupt); string(content) == "corrupt" {
		t.Fatal("corrupt cache file was not repaired")
	}
}

func TestCrossPlatformCoverageRejectsInvalidBundleAndArchive(t *testing.T) {
	if _, err := Inspect(nil); !errors.Is(err, ErrBundleUnavailable) {
		t.Fatalf("empty inspect error = %v", err)
	}
	badContainer := make([]byte, containerHeader)
	copy(badContainer, containerMagic[:])
	if _, err := Inspect(badContainer); err == nil {
		t.Fatal("zero-sized container accepted")
	}

	root := writePayloadFixture(t, "windows", "amd64")
	container, err := BuildContainer(root, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	container[containerHeader] ^= 0xff
	if _, err := Materialize(container, t.TempDir(), "windows", "amd64"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("corrupt bundle error = %v", err)
	}

	archive := maliciousArchive(t, "../escape")
	if err := extractArchive(bytes.NewReader(archive), t.TempDir()); err == nil || !strings.Contains(err.Error(), "invalid embedded") {
		t.Fatalf("traversal archive error = %v", err)
	}
	archive = maliciousArchive(t, "ps/00000000000000000000000000000000", "ps/00000000000000000000000000000000")
	if err := extractArchive(bytes.NewReader(archive), t.TempDir()); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate archive error = %v", err)
	}
	if _, err := LibraryName("plan9", "amd64"); err == nil {
		t.Fatal("unsupported target accepted")
	}
}

func TestCrossPlatformCoverageContainerAndFileErrors(t *testing.T) {
	root := writePayloadFixture(t, "linux", "amd64")
	if len(Embedded()) == 0 {
		t.Fatal("embedded payload is empty")
	}
	for _, capacity := range []int{0, maxContainerBytes + 1, containerHeader + 1} {
		if _, err := BuildContainer(root, capacity); err == nil {
			t.Fatalf("capacity %d was accepted", capacity)
		}
	}
	if _, err := BuildContainer(filepath.Join(t.TempDir(), "missing"), 1<<20); err == nil {
		t.Fatal("missing payload root was accepted")
	}

	output := filepath.Join(t.TempDir(), "payload")
	if err := WriteContainer(output, root, 1<<20); err != nil {
		t.Fatal(err)
	}
	if err := WriteContainer(output, root, 1); err == nil {
		t.Fatal("invalid generated capacity was accepted")
	}
	parentFile := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteContainer(filepath.Join(parentFile, "payload"), root, 1<<20); err == nil {
		t.Fatal("file parent was accepted")
	}
	if err := WriteContainer(t.TempDir(), root, 1<<20); err == nil {
		t.Fatal("directory output was accepted")
	}

	container, err := BuildContainer(root, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	invalidFormat := append([]byte(nil), container...)
	invalidFormat[56] = 2
	if _, err := Inspect(invalidFormat); err == nil {
		t.Fatal("unsupported format was accepted")
	}
	invalidSize := append([]byte(nil), container...)
	for index := 16; index < 24; index++ {
		invalidSize[index] = 0
	}
	if _, err := Inspect(invalidSize); err == nil {
		t.Fatal("zero archive size was accepted")
	}
	withDecoy := append([]byte(nil), containerMagic[:]...)
	withDecoy = append(withDecoy, make([]byte, containerHeader)...)
	withDecoy = append(withDecoy, container...)
	if descriptor, err := Find(withDecoy); err != nil || descriptor.Offset != len(containerMagic)+containerHeader {
		t.Fatalf("decoy lookup = %#v, %v", descriptor, err)
	}
	if _, err := Find([]byte("no payload")); !errors.Is(err, ErrBundleUnavailable) {
		t.Fatalf("missing marker error = %v", err)
	}

	if err := InjectFile(filepath.Join(t.TempDir(), "missing"), root); err == nil {
		t.Fatal("missing executable was accepted")
	}
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(plain, []byte("plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InjectFile(plain, root); err == nil {
		t.Fatal("executable without a slot was accepted")
	}
	binaryPath := filepath.Join(t.TempDir(), "dws")
	if err := os.WriteFile(binaryPath, append([]byte("binary"), container...), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InjectFile(binaryPath, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("invalid source payload was accepted")
	}
	t.Run("write executable", func(t *testing.T) {
		testseam.Swap(t, &writeExecutableFile, func(string, []byte, os.FileMode) error {
			return errors.New("write failed")
		})
		if err := InjectFile(binaryPath, root); err == nil || !strings.Contains(err.Error(), "write executable") {
			t.Fatalf("write error = %v", err)
		}
	})

	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := MaterializeFile(missing, t.TempDir(), "linux", "amd64"); err == nil {
		t.Fatal("missing payload file was accepted")
	}
	if _, err := MaterializeFile(plain, t.TempDir(), "linux", "amd64"); err == nil {
		t.Fatal("plain payload file was accepted")
	}
	if _, err := MaterializeFile(binaryPath, t.TempDir(), "plan9", "amd64"); err == nil {
		t.Fatal("unsupported materialization target was accepted")
	}
}

func TestCrossPlatformCoverageMaterializeFailures(t *testing.T) {
	root := writePayloadFixture(t, "linux", "amd64")
	container, err := BuildContainer(root, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(nil, t.TempDir(), "linux", "amd64"); err == nil {
		t.Fatal("empty container was accepted")
	}
	if _, err := Materialize(container, t.TempDir(), "plan9", "amd64"); err == nil {
		t.Fatal("unsupported target was accepted")
	}

	tests := []struct {
		name  string
		setup func(*testing.T)
	}{
		{
			name: "cache directory",
			setup: func(t *testing.T) {
				testseam.Swap(t, &makeCacheDirectory, func(string, os.FileMode) error { return errors.New("mkdir") })
			},
		},
		{
			name: "temporary directory",
			setup: func(t *testing.T) {
				testseam.Swap(t, &makeCacheTemporary, func(string, string) (string, error) { return "", errors.New("temp") })
			},
		},
		{
			name: "extract",
			setup: func(t *testing.T) {
				testseam.Swap(t, &extractPayload, func(io.Reader, string) error { return errors.New("extract") })
			},
		},
		{
			name: "staged validation",
			setup: func(t *testing.T) {
				calls := 0
				testseam.Swap(t, &validatePayloadRoot, func(string, string, string) error {
					calls++
					if calls == 1 {
						return errors.New("cache miss")
					}
					return errors.New("staged invalid")
				})
			},
		},
		{
			name: "publish",
			setup: func(t *testing.T) {
				calls := 0
				testseam.Swap(t, &validatePayloadRoot, func(string, string, string) error {
					calls++
					if calls == 1 {
						return errors.New("cache miss")
					}
					return nil
				})
				testseam.Swap(t, &publishPayload, func(string, string, string, string) error { return errors.New("publish") })
			},
		},
		{
			name: "published validation",
			setup: func(t *testing.T) {
				calls := 0
				testseam.Swap(t, &validatePayloadRoot, func(string, string, string) error {
					calls++
					if calls == 1 || calls == 3 {
						return errors.New("invalid")
					}
					return nil
				})
				testseam.Swap(t, &publishPayload, func(string, string, string, string) error { return nil })
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.setup(t)
			if _, err := Materialize(container, t.TempDir(), "linux", "amd64"); err == nil {
				t.Fatal("materialization error was ignored")
			}
		})
	}
}

func TestCrossPlatformCoverageArchiveFailureSeams(t *testing.T) {
	root := writePayloadFixture(t, "linux", "amd64")
	if err := writeArchive(io.Discard, root); err != nil {
		t.Fatal(err)
	}
	t.Run("ignored directory", func(t *testing.T) {
		entries, err := os.ReadDir(filepath.Join(root, "ps"))
		if err != nil {
			t.Fatal(err)
		}
		extraRoot := t.TempDir()
		if err := os.Mkdir(filepath.Join(extraRoot, "ignored"), 0o700); err != nil {
			t.Fatal(err)
		}
		extra, err := os.ReadDir(extraRoot)
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &readPayloadDirectory, func(string) ([]os.DirEntry, error) {
			return append(entries, extra[0]), nil
		})
		if err := writeArchive(io.Discard, root); err != nil {
			t.Fatal(err)
		}
	})

	tests := []struct {
		name  string
		setup func(*testing.T)
	}{
		{"compressor", func(t *testing.T) {
			testseam.Swap(t, &newPayloadGzipWriter, func(io.Writer, int) (*gzip.Writer, error) { return nil, errors.New("gzip") })
		}},
		{"directory", func(t *testing.T) {
			testseam.Swap(t, &readPayloadDirectory, func(string) ([]os.DirEntry, error) { return nil, errors.New("readdir") })
		}},
		{"stat", func(t *testing.T) {
			testseam.Swap(t, &lstatPayloadFile, func(string) (os.FileInfo, error) { return nil, errors.New("stat") })
		}},
		{"non regular", func(t *testing.T) {
			info, err := os.Stat(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &lstatPayloadFile, func(string) (os.FileInfo, error) { return info, nil })
		}},
		{"header", func(t *testing.T) {
			testseam.Swap(t, &writePayloadHeader, func(*tar.Writer, *tar.Header) error { return errors.New("header") })
		}},
		{"open", func(t *testing.T) {
			testseam.Swap(t, &openPayloadFile, func(string) (io.ReadCloser, error) { return nil, errors.New("open") })
		}},
		{"copy", func(t *testing.T) {
			testseam.Swap(t, &copyPayloadFile, func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") })
		}},
		{"file close", func(t *testing.T) {
			testseam.Swap(t, &closePayloadFile, func(io.Closer) error { return errors.New("close") })
		}},
		{"tar close", func(t *testing.T) {
			testseam.Swap(t, &closePayloadTar, func(*tar.Writer) error { return errors.New("tar close") })
		}},
		{"gzip close", func(t *testing.T) {
			testseam.Swap(t, &closePayloadGzip, func(*gzip.Writer) error { return errors.New("gzip close") })
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.setup(t)
			if err := writeArchive(io.Discard, root); err == nil {
				t.Fatal("archive failure was ignored")
			}
		})
	}

	missing := filepath.Join(t.TempDir(), "missing")
	if err := writeArchive(io.Discard, missing); err == nil {
		t.Fatal("missing manifest was accepted")
	}
	invalidTarget := writePayloadFixture(t, "linux", "amd64")
	rewritePayloadManifest(t, invalidTarget, func(value *manifest) { value.Target = "invalid" })
	if err := writeArchive(io.Discard, invalidTarget); err == nil {
		t.Fatal("invalid target was accepted")
	}
	invalidRoot := writePayloadFixture(t, "linux", "amd64")
	rewritePayloadManifest(t, invalidRoot, func(value *manifest) { value.LibrarySHA256 = strings.Repeat("0", 64) })
	if err := writeArchive(io.Discard, invalidRoot); err == nil {
		t.Fatal("invalid source root was accepted")
	}
}

func TestCrossPlatformCoverageExtractionFailureSeams(t *testing.T) {
	valid := maliciousArchive(t, "manifest.json")
	if err := extractArchive(strings.NewReader("invalid"), t.TempDir()); err == nil {
		t.Fatal("invalid gzip was accepted")
	}

	tests := []struct {
		name  string
		setup func(*testing.T)
	}{
		{"reader", func(t *testing.T) {
			testseam.Swap(t, &newPayloadGzipReader, func(io.Reader) (io.ReadCloser, error) { return nil, errors.New("reader") })
		}},
		{"next", func(t *testing.T) {
			testseam.Swap(t, &nextPayloadEntry, func(*tar.Reader) (*tar.Header, error) { return nil, errors.New("next") })
		}},
		{"directory", func(t *testing.T) {
			testseam.Swap(t, &makePayloadDirectory, func(string, os.FileMode) error { return errors.New("mkdir") })
		}},
		{"open", func(t *testing.T) {
			testseam.Swap(t, &openPayloadOutput, func(string, int, os.FileMode) (io.WriteCloser, error) { return nil, errors.New("open") })
		}},
		{"copy", func(t *testing.T) {
			testseam.Swap(t, &copyPayloadEntry, func(io.Writer, io.Reader, int64) (int64, error) { return 0, errors.New("copy") })
		}},
		{"close", func(t *testing.T) {
			testseam.Swap(t, &closePayloadOutput, func(closer io.Closer) error {
				_ = closer.Close()
				return errors.New("close")
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.setup(t)
			if err := extractArchive(bytes.NewReader(valid), t.TempDir()); err == nil {
				t.Fatal("extraction failure was ignored")
			}
		})
	}

	tooMany := make([]string, maxFiles)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("ps/%032x", index)
	}
	if err := extractArchive(bytes.NewReader(maliciousArchive(t, tooMany...)), t.TempDir()); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("entry limit error = %v", err)
	}
	if err := extractArchive(bytes.NewReader(archiveWithHeader(t, &tar.Header{Name: "ps/00000000000000000000000000000000", Typeflag: tar.TypeSymlink})), t.TempDir()); err == nil {
		t.Fatal("non-regular entry was accepted")
	}
	t.Run("oversized", func(t *testing.T) {
		testseam.Swap(t, &nextPayloadEntry, func(*tar.Reader) (*tar.Header, error) {
			return &tar.Header{Name: "manifest.json", Typeflag: tar.TypeReg, Size: maxFileBytes + 1}, nil
		})
		if err := extractArchive(bytes.NewReader(valid), t.TempDir()); err == nil {
			t.Fatal("oversized entry was accepted")
		}
	})
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractArchive(bytes.NewReader(valid), root); err == nil {
		t.Fatal("existing output was overwritten")
	}
}

func TestCrossPlatformCoverageValidationFailures(t *testing.T) {
	if !validArchivePath("manifest.json") || !validArchivePath("x7k2m9p4q1w8.dylib") ||
		validArchivePath("other") || validArchivePath("ps/a/b") || validArchivePath("ps/short") ||
		validArchivePath("ps/zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz") || !validArchivePath("ps/00000000000000000000000000000000") {
		t.Fatal("archive path validation drifted")
	}

	tests := []struct {
		name   string
		goos   string
		mutate func(*testing.T, string)
	}{
		{"missing manifest", "linux", func(t *testing.T, root string) { os.Remove(filepath.Join(root, "manifest.json")) }},
		{"unsupported target", "plan9", func(*testing.T, string) {}},
		{"manifest mismatch", "linux", func(t *testing.T, root string) {
			rewritePayloadManifest(t, root, func(value *manifest) { value.PayloadVersion = "other" })
		}},
		{"manifest count", "linux", func(t *testing.T, root string) {
			rewritePayloadManifest(t, root, func(value *manifest) { value.PSFileCount = 1 })
		}},
		{"missing library", "linux", func(t *testing.T, root string) { os.Remove(filepath.Join(root, "libx7k2m9p4q1w8.so")) }},
		{"library checksum", "linux", func(t *testing.T, root string) {
			os.WriteFile(filepath.Join(root, "libx7k2m9p4q1w8.so"), []byte("bad"), 0o700)
		}},
		{"missing directory", "linux", func(t *testing.T, root string) { os.Rename(filepath.Join(root, "ps"), filepath.Join(root, "gone")) }},
		{"incomplete files", "linux", func(t *testing.T, root string) {
			entries, _ := os.ReadDir(filepath.Join(root, "ps"))
			os.Remove(filepath.Join(root, "ps", entries[0].Name()))
		}},
		{"invalid file", "linux", func(t *testing.T, root string) {
			entries, _ := os.ReadDir(filepath.Join(root, "ps"))
			os.Rename(filepath.Join(root, "ps", entries[0].Name()), filepath.Join(root, "ps", "invalid"))
		}},
		{"non regular file", "linux", func(t *testing.T, root string) {
			testseam.Swap(t, &regularPayloadFile, func(path string) bool {
				return !strings.Contains(path, string(filepath.Separator)+"ps"+string(filepath.Separator)) && isRegularFile(path)
			})
		}},
		{"payload checksum", "linux", func(t *testing.T, root string) {
			entries, _ := os.ReadDir(filepath.Join(root, "ps"))
			os.WriteFile(filepath.Join(root, "ps", entries[0].Name()), []byte("bad"), 0o600)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writePayloadFixture(t, "linux", "amd64")
			test.mutate(t, root)
			if err := validateRoot(root, test.goos, "amd64"); err == nil {
				t.Fatal("invalid payload root was accepted")
			}
		})
	}

	root := writePayloadFixture(t, "linux", "amd64")
	if _, err := hashFile(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing hash input was accepted")
	}
	if _, err := hashReader(errorReader{}); err == nil {
		t.Fatal("reader error was ignored")
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(root); err == nil {
		t.Fatal("invalid manifest was accepted")
	}

	t.Run("hash open", func(t *testing.T) {
		testseam.Swap(t, &openHashInput, func(string) (io.ReadCloser, error) { return nil, errors.New("open") })
		if _, err := hashFile("value"); err == nil {
			t.Fatal("hash open error was ignored")
		}
	})
	t.Run("validation hash", func(t *testing.T) {
		root := writePayloadFixture(t, "linux", "amd64")
		testseam.Swap(t, &openHashInput, func(path string) (io.ReadCloser, error) {
			if strings.Contains(path, string(filepath.Separator)+"ps"+string(filepath.Separator)) {
				return nil, errors.New("open")
			}
			return os.Open(path)
		})
		if err := validateRoot(root, "linux", "amd64"); err == nil {
			t.Fatal("payload hash error was ignored")
		}
	})
	t.Run("manifest read", func(t *testing.T) {
		root := writePayloadFixture(t, "linux", "amd64")
		testseam.Swap(t, &readManifestFile, func(string) ([]byte, error) { return nil, errors.New("read") })
		if _, err := readManifest(root); err == nil {
			t.Fatal("manifest read error was ignored")
		}
	})
}

func TestCrossPlatformCoveragePublishDirectoryFailures(t *testing.T) {
	t.Run("existing valid target", func(t *testing.T) {
		temporary := writePayloadFixture(t, "linux", "amd64")
		target := writePayloadFixture(t, "linux", "amd64")
		if err := publishDirectory(temporary, target, "linux", "amd64"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unsafe target", func(t *testing.T) {
		temporary := writePayloadFixture(t, "linux", "amd64")
		target := filepath.Join(t.TempDir(), "target")
		if err := os.WriteFile(target, []byte("unsafe"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := publishDirectory(temporary, target, "linux", "amd64"); err == nil {
			t.Fatal("unsafe target was accepted")
		}
	})
	t.Run("unsafe racing target", func(t *testing.T) {
		temporary := writePayloadFixture(t, "linux", "amd64")
		target := filepath.Join(t.TempDir(), "target")
		if err := os.WriteFile(target, []byte("unsafe"), 0o600); err != nil {
			t.Fatal(err)
		}
		calls := 0
		testseam.Swap(t, &lstatPayloadTarget, func(path string) (os.FileInfo, error) {
			calls++
			if calls == 1 {
				return nil, fs.ErrNotExist
			}
			return os.Lstat(path)
		})
		testseam.Swap(t, &renamePayloadDirectory, func(string, string) error {
			return errors.New("rename")
		})
		if err := publishDirectory(temporary, target, "linux", "amd64"); err == nil {
			t.Fatal("racing unsafe target was accepted")
		}
	})
	t.Run("remove", func(t *testing.T) {
		temporary := writePayloadFixture(t, "linux", "amd64")
		target := filepath.Join(t.TempDir(), "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &removePayloadDirectory, func(string) error { return errors.New("remove") })
		if err := publishDirectory(temporary, target, "linux", "amd64"); err == nil {
			t.Fatal("remove error was ignored")
		}
	})
	t.Run("final rename", func(t *testing.T) {
		temporary := writePayloadFixture(t, "linux", "amd64")
		target := filepath.Join(t.TempDir(), "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		calls := 0
		testseam.Swap(t, &renamePayloadDirectory, func(string, string) error {
			calls++
			return errors.New("rename")
		})
		if err := publishDirectory(temporary, target, "linux", "amd64"); err == nil || calls != 2 {
			t.Fatalf("final rename error = %v, calls = %d", err, calls)
		}
	})
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read") }

func rewritePayloadManifest(t *testing.T, root string, mutate func(*manifest)) {
	t.Helper()
	value, err := readManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&value)
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func archiveWithHeader(t *testing.T, header *tar.Header) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writePayloadFixture(t *testing.T, goos, goarch string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ps"), 0o755); err != nil {
		t.Fatal(err)
	}
	library, err := LibraryName(goos, goarch)
	if err != nil {
		t.Fatal(err)
	}
	libraryContent := []byte("library-" + goos + "-" + goarch)
	if err := os.WriteFile(filepath.Join(root, library), libraryContent, 0o700); err != nil {
		t.Fatal(err)
	}
	psHasher := sha256.New()
	for index := range 123 {
		name := fmt.Sprintf("%032x", index)
		content := []byte(fmt.Sprintf("payload-%03d", index))
		if err := os.WriteFile(filepath.Join(root, "ps", name), content, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		fmt.Fprintf(psHasher, "%s  ps/%s\n", hex.EncodeToString(digest[:]), name)
	}
	libraryDigest := sha256.Sum256(libraryContent)
	metadata := manifest{
		FormatVersion: 1, PayloadVersion: PayloadVersion, Target: goos + "/" + goarch,
		Library: library, LibrarySHA256: hex.EncodeToString(libraryDigest[:]),
		PSFileCount: 123, PSManifestSHA256: hex.EncodeToString(psHasher.Sum(nil)),
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func maliciousArchive(t *testing.T, names ...string) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range names {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
