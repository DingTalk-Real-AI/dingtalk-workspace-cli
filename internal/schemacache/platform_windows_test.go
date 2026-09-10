//go:build windows && (amd64 || arm64)

package schemacache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func testIdentity(t *testing.T, edition string) ExpectedIdentity {
	t.Helper()
	editionDigest, err := EditionSHA256(edition)
	if err != nil {
		t.Fatal(err)
	}
	return ExpectedIdentity{
		CatalogSnapshotVersion: 9,
		EditionSHA256:          editionDigest,
		SourceSHA256:           sha256.Sum256([]byte("source")),
		SurfaceSHA256:          sha256.Sum256([]byte("surface")),
		BuildID:                sha256.Sum256([]byte("build")),
	}
}

func testArtifact(kind ArtifactKind, payload []byte) Artifact {
	return Artifact{
		Expectation: ArtifactExpectation{
			Kind: kind, Serializer: SerializerProtobuf, Codec: CodecRaw,
			FormatVersion: DTOFormatVersion, EncodedLength: uint64(len(payload)),
			DecodedLength: uint64(len(payload)), EncodedSHA256: sha256.Sum256(payload),
		},
		Payload: append([]byte(nil), payload...),
	}
}

func privateTestBase(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp("", "dws-schemacache-win-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	resolved, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(resolved)
}

func openTestCache(t *testing.T, ops windowsIO) (*Cache, *Counters, ExpectedIdentity) {
	t.Helper()
	base := privateTestBase(t)
	oldUser, oldIO, oldProgram := userCacheDir, platformIO, programDataDir
	userCacheDir = func() (string, error) { return base, nil }
	programDataDir = func() string { return "" }
	if ops == nil {
		ops = realWindowsIO{}
	}
	platformIO = ops
	t.Cleanup(func() {
		userCacheDir, platformIO, programDataDir = oldUser, oldIO, oldProgram
	})
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	counters := &Counters{}
	cache, err := Open("official", WithCounters(counters))
	if err != nil {
		t.Fatalf("Open: %v (base %s)", err, base)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return cache, counters, testIdentity(t, "official")
}

func TestCrossPlatformCoverageWindowsOpenPublishReadAndCounters(t *testing.T) {
	cache, counters, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("authenticated meta"))
	registry := testArtifact(KindRegistry, []byte("alpha-product-beta-product"))
	payloads := testArtifact(KindPayloads, []byte("payload-shard-bytes"))
	if err := cache.Publish(identity, registry, meta, payloads); err != nil {
		t.Fatal(err)
	}
	if cache.Directory() == "" {
		t.Fatal("empty cache directory")
	}

	before := counters.Snapshot()
	gotMeta, err := cache.ReadMeta(identity, meta.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotMeta) != string(meta.Payload) {
		t.Fatalf("Meta = %q", gotMeta)
	}
	afterMeta := counters.Snapshot()
	if afterMeta.RegistryReadOps != before.RegistryReadOps || afterMeta.RegistryReadBytes != before.RegistryReadBytes {
		t.Fatal("Meta path read Registry payload")
	}

	opened, err := cache.OpenRegistry(identity, registry.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	afterOpen := counters.Snapshot()
	if afterOpen.RegistryReadOps != afterMeta.RegistryReadOps {
		t.Fatal("OpenRegistry hashed Registry payload")
	}
	rangeBytes := registry.Payload[6:13]
	gotRange, err := opened.ReadRange(RangeDescriptor{Offset: 6, Length: 7, SHA256: sha256.Sum256(rangeBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if string(gotRange) != string(rangeBytes) {
		t.Fatalf("range = %q", gotRange)
	}
	if err := opened.ValidateAggregate(); err != nil {
		t.Fatal(err)
	}

	openedPayloads, err := cache.OpenPayloads(identity, payloads.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	defer openedPayloads.Close()
	gotPayload, err := openedPayloads.ReadRange(RangeDescriptor{Offset: 0, Length: uint64(len(payloads.Payload)), SHA256: payloads.Expectation.EncodedSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if string(gotPayload) != string(payloads.Payload) {
		t.Fatalf("payload range = %q", gotPayload)
	}
	if err := openedPayloads.ValidateAggregate(); err != nil {
		t.Fatal(err)
	}
	held, err := cache.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageWindowsTamperAndIdentityMiss(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("trusted"))
	registry := testArtifact(KindRegistry, []byte("registry"))
	if err := cache.Publish(identity, registry, meta); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cache.Directory(), metaFileName)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original[:len(original)-1], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("partial Meta = %v", err)
	}
	if err := os.WriteFile(path, append(append([]byte{}, original...), 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("trailing Meta = %v", err)
	}

	forgedPayload := []byte("forged!")
	forged := testArtifact(KindMeta, forgedPayload)
	forgedHeader, err := envelopeFrom(identity, forged.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	headerBytes, err := forgedHeader.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(headerBytes, forgedPayload...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("forged Meta = %v", err)
	}

	wrong := identity
	wrong.SourceSHA256 = sha256.Sum256([]byte("other"))
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(wrong, meta.Expectation); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("identity miss = %v", err)
	}
	wrongEdition := identity
	wrongEdition.EditionSHA256 = sha256.Sum256([]byte("other-edition"))
	if _, err := cache.ReadMeta(wrongEdition, meta.Expectation); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("edition miss = %v", err)
	}
	if _, err := cache.OpenRegistry(wrongEdition, registry.Expectation); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("registry edition miss = %v", err)
	}
	if _, err := cache.OpenPayloads(wrongEdition, testArtifact(KindPayloads, []byte("p")).Expectation); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("payload edition miss = %v", err)
	}
	if err := cache.WriteArtifact(wrongEdition, meta); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("write edition miss = %v", err)
	}
}

func TestCrossPlatformCoverageWindowsDisableMissAndUnsafePaths(t *testing.T) {
	if _, err := Open("../escape"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("traversal Open = %v", err)
	}
	oldUser := userCacheDir
	userCacheDir = func() (string, error) { return "", errors.New("no cache dir") }
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	programDataDir = func() string { return "" }
	t.Cleanup(func() { userCacheDir = oldUser })
	if _, err := Open("official"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("missing user cache = %v", err)
	}

	base := privateTestBase(t)
	userCacheDir = func() (string, error) { return base, nil }
	if _, err := Open("official", WithNoCreate()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("noCreate miss = %v", err)
	}

	t.Setenv("DWS_SCHEMA_CACHE_DIR", "relative/cache")
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("relative override = %v", err)
	}

	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("x"))
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing meta = %v", err)
	}
	if _, err := cache.OpenRegistry(identity, testArtifact(KindRegistry, []byte("r")).Expectation); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing registry = %v", err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed read = %v", err)
	}
	if _, err := cache.OpenRegistry(identity, testArtifact(KindRegistry, []byte("r")).Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed registry = %v", err)
	}
	if _, err := cache.OpenPayloads(identity, testArtifact(KindPayloads, []byte("p")).Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed payloads = %v", err)
	}
	if err := cache.WriteArtifact(identity, meta); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed write = %v", err)
	}
	if _, err := cache.AcquireLock(context.Background(), time.Millisecond); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed lock = %v", err)
	}
}

func TestCrossPlatformCoverageWindowsSystemAndOverridePaths(t *testing.T) {
	if got := func() string {
		old := currentGOOS
		currentGOOS = "linux"
		defer func() { currentGOOS = old }()
		return systemSchemaCacheBase()
	}(); got != "" {
		t.Fatalf("non-windows system base = %q", got)
	}
	programDataDir = func() string { return "" }
	if got := systemSchemaCacheBase(); got != "" {
		t.Fatalf("empty ProgramData = %q", got)
	}
	programDataDir = func() string { return "relative" }
	if got := systemSchemaCacheBase(); got != "" {
		t.Fatalf("relative ProgramData = %q", got)
	}
	shared := privateTestBase(t)
	programDataDir = func() string { return shared }
	if got := systemSchemaCacheBase(); got != filepath.Join(shared, "dws") {
		t.Fatalf("system base = %q", got)
	}

	override := privateTestBase(t)
	t.Setenv("DWS_SCHEMA_CACHE_DIR", override)
	userCacheDir = func() (string, error) { return privateTestBase(t), nil }
	platformIO = realWindowsIO{}
	cache, err := Open("official")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	digest := sha256.Sum256([]byte("official"))
	want := filepath.Join(override, "dws", "schema", hex.EncodeToString(digest[:]), "v1")
	if cache.Directory() != want {
		t.Fatalf("override directory = %s want %s", cache.Directory(), want)
	}

	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	sharedRoot := privateTestBase(t)
	programDataDir = func() string { return sharedRoot }
	systemBase := filepath.Join(sharedRoot, "dws")
	if err := os.MkdirAll(filepath.Join(systemBase, "dws", "schema", hex.EncodeToString(digest[:]), "v1"), 0o700); err != nil {
		t.Fatal(err)
	}
	userCacheDir = func() (string, error) { return privateTestBase(t), nil }
	sharedCache, err := Open("official", WithNoCreate())
	if err != nil {
		t.Fatalf("shared Open: %v", err)
	}
	_ = sharedCache.Close()
}

func TestCrossPlatformCoverageWindowsReparseAndRegularFileRejection(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	target := filepath.Join(cache.Directory(), metaFileName)
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, testArtifact(KindMeta, []byte("x")).Expectation); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("directory artifact = %v", err)
	}

	cache2, _, identity2 := openTestCache(t, nil)
	link := filepath.Join(cache2.Directory(), metaFileName)
	other := filepath.Join(cache2.Directory(), "other")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	if _, err := cache2.ReadMeta(identity2, testArtifact(KindMeta, []byte("x")).Expectation); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("reparse artifact = %v", err)
	}

	base := privateTestBase(t)
	if err := os.Symlink(privateTestBase(t), filepath.Join(base, "dws")); err != nil {
		t.Skipf("directory symlink not available: %v", err)
	}
	oldUser, oldIO := userCacheDir, platformIO
	userCacheDir = func() (string, error) { return base, nil }
	platformIO = realWindowsIO{}
	t.Cleanup(func() { userCacheDir, platformIO = oldUser, oldIO })
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	programDataDir = func() string { return "" }
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("reparse ancestry = %v", err)
	}
}

func TestCrossPlatformCoverageWindowsLockTimeoutAndClosedRegistry(t *testing.T) {
	cache, _, _ := openTestCache(t, nil)
	held, err := cache.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.AcquireLock(context.Background(), 0); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("busy lock = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.AcquireLock(cancelled, time.Second); err == nil {
		t.Fatal("cancelled lock succeeded")
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	cache2, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("meta-lock"))
	reg := testArtifact(KindRegistry, []byte("registry-lock-bytes"))
	if err := cache2.Publish(identity, reg, meta); err != nil {
		t.Fatal(err)
	}
	opened, err := cache2.OpenRegistry(identity, reg.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ReadRange(RangeDescriptor{Offset: 100, Length: 1, SHA256: sha256.Sum256([]byte("x"))}); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("out of range = %v", err)
	}
	if _, err := opened.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: sha256.Sum256([]byte("nope"))}); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("range digest = %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: sha256.Sum256([]byte("r"))}); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed range = %v", err)
	}
	if err := opened.ValidateAggregate(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed aggregate = %v", err)
	}
}

type wrapIO struct {
	windowsIO
	attrFn     func(string) (uint32, error)
	mkdirFn    func(string) error
	openFn     func(string, uint32, uint32, uint32, uint32) (windows.Handle, error)
	infoFn     func(windows.Handle) (windows.ByHandleFileInformation, error)
	readFn     func(windows.Handle, []byte, int64) (int, error)
	writeFn    func(windows.Handle, []byte) (int, error)
	flushFn    func(windows.Handle) error
	closeFn    func(windows.Handle) error
	renameFn   func(string, string) error
	removeFn   func(string) error
	lockFn     func(windows.Handle) error
	unlockFn   func(windows.Handle) error
	restrictFn func(string) error
	randomFn   func([]byte) (int, error)
}

func (w wrapIO) attributes(path string) (uint32, error) {
	if w.attrFn != nil {
		return w.attrFn(path)
	}
	return w.windowsIO.attributes(path)
}
func (w wrapIO) mkdir(path string) error {
	if w.mkdirFn != nil {
		return w.mkdirFn(path)
	}
	return w.windowsIO.mkdir(path)
}
func (w wrapIO) open(path string, access, share, disposition, flags uint32) (windows.Handle, error) {
	if w.openFn != nil {
		return w.openFn(path, access, share, disposition, flags)
	}
	return w.windowsIO.open(path, access, share, disposition, flags)
}
func (w wrapIO) info(h windows.Handle) (windows.ByHandleFileInformation, error) {
	if w.infoFn != nil {
		return w.infoFn(h)
	}
	return w.windowsIO.info(h)
}
func (w wrapIO) readAt(h windows.Handle, p []byte, offset int64) (int, error) {
	if w.readFn != nil {
		return w.readFn(h, p, offset)
	}
	return w.windowsIO.readAt(h, p, offset)
}
func (w wrapIO) write(h windows.Handle, p []byte) (int, error) {
	if w.writeFn != nil {
		return w.writeFn(h, p)
	}
	return w.windowsIO.write(h, p)
}
func (w wrapIO) flush(h windows.Handle) error {
	if w.flushFn != nil {
		return w.flushFn(h)
	}
	return w.windowsIO.flush(h)
}
func (w wrapIO) close(h windows.Handle) error {
	if w.closeFn != nil {
		return w.closeFn(h)
	}
	return w.windowsIO.close(h)
}
func (w wrapIO) rename(oldpath, newpath string) error {
	if w.renameFn != nil {
		return w.renameFn(oldpath, newpath)
	}
	return w.windowsIO.rename(oldpath, newpath)
}
func (w wrapIO) lock(h windows.Handle) error {
	if w.lockFn != nil {
		return w.lockFn(h)
	}
	return w.windowsIO.lock(h)
}
func (w wrapIO) restrictACL(path string) error {
	if w.restrictFn != nil {
		return w.restrictFn(path)
	}
	return w.windowsIO.restrictACL(path)
}
func (w wrapIO) unlock(h windows.Handle) error {
	if w.unlockFn != nil {
		return w.unlockFn(h)
	}
	return w.windowsIO.unlock(h)
}
func (w wrapIO) remove(path string) error {
	if w.removeFn != nil {
		return w.removeFn(path)
	}
	return w.windowsIO.remove(path)
}
func (w wrapIO) random(p []byte) (int, error) {
	if w.randomFn != nil {
		return w.randomFn(p)
	}
	return w.windowsIO.random(p)
}

func TestCrossPlatformCoverageWindowsInjectedFaults(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("fault-meta-payload"))
	reg := testArtifact(KindRegistry, []byte("fault-registry-payload"))
	if err := cache.Publish(identity, reg, meta); err != nil {
		t.Fatal(err)
	}
	uc := cache.backend.(*windowsCache)

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, infoFn: func(h windows.Handle) (windows.ByHandleFileInformation, error) {
		return windows.ByHandleFileInformation{}, errors.New("forced info")
	}}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("info failure accepted")
	}

	calls := 0
	uc.ops = wrapIO{windowsIO: realWindowsIO{}, infoFn: func(h windows.Handle) (windows.ByHandleFileInformation, error) {
		info, err := realWindowsIO{}.info(h)
		if err != nil {
			return info, err
		}
		calls++
		if calls >= 2 {
			info.FileSizeLow++
		}
		return info, nil
	}}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("file-changed accepted")
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, readFn: func(windows.Handle, []byte, int64) (int, error) {
		return 0, io.ErrUnexpectedEOF
	}}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("short read = %v", err)
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, writeFn: func(windows.Handle, []byte) (int, error) {
		return 0, errors.New("forced write")
	}}
	if err := cache.WriteArtifact(identity, meta); err == nil {
		t.Fatal("write failure accepted")
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, flushFn: func(windows.Handle) error {
		return errors.New("forced flush")
	}}
	if err := cache.WriteArtifact(identity, meta); err == nil {
		t.Fatal("flush failure accepted")
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, renameFn: func(string, string) error {
		return errors.New("forced rename")
	}}
	if err := cache.WriteArtifact(identity, meta); err == nil {
		t.Fatal("rename failure accepted")
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, renameFn: func(oldpath, newpath string) error {
		if filepath.Base(newpath) == metaFileName && strings.HasSuffix(oldpath, ".tmp") {
			return errors.New("forced dest rename")
		}
		return realWindowsIO{}.rename(oldpath, newpath)
	}}
	if err := cache.WriteArtifact(identity, meta); err == nil {
		t.Fatal("dest rename failure accepted")
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, restrictFn: func(string) error {
		return errors.New("forced acl")
	}}
	if err := cache.WriteArtifact(identity, meta); err == nil {
		t.Fatal("acl failure accepted")
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, randomFn: func([]byte) (int, error) {
		return 0, errors.New("forced random")
	}}
	if err := cache.WriteArtifact(identity, meta); err == nil {
		t.Fatal("random failure accepted")
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, lockFn: func(windows.Handle) error {
		return errors.New("forced lock")
	}}
	if _, err := cache.AcquireLock(context.Background(), time.Millisecond); err == nil {
		t.Fatal("lock failure accepted")
	}

	uc.ops = wrapIO{windowsIO: realWindowsIO{}, lockFn: func(windows.Handle) error {
		return windows.ERROR_LOCK_VIOLATION
	}}
	if _, err := cache.AcquireLock(context.Background(), 0); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("lock busy = %v", err)
	}

	var infoCalls int
	uc.ops = wrapIO{windowsIO: realWindowsIO{}, infoFn: func(h windows.Handle) (windows.ByHandleFileInformation, error) {
		info, err := realWindowsIO{}.info(h)
		if err != nil {
			return info, err
		}
		infoCalls++
		if infoCalls >= 2 {
			return windows.ByHandleFileInformation{}, errors.New("open info")
		}
		return info, nil
	}}
	if _, err := cache.OpenRegistry(identity, reg.Expectation); err == nil {
		t.Fatal("registry info failure accepted")
	}

	base := privateTestBase(t)
	userCacheDir = func() (string, error) { return filepath.Join(base, "missing"), nil }
	platformIO = wrapIO{windowsIO: realWindowsIO{}, mkdirFn: func(string) error {
		return errors.New("forced mkdir")
	}}
	programDataDir = func() string { return "" }
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("mkdir failure = %v", err)
	}

	platformIO = wrapIO{windowsIO: realWindowsIO{}, attrFn: func(path string) (uint32, error) {
		if filepath.Base(path) == "dws" {
			return windows.FILE_ATTRIBUTE_REPARSE_POINT | windows.FILE_ATTRIBUTE_DIRECTORY, nil
		}
		return realWindowsIO{}.attributes(path)
	}}
	userCacheDir = func() (string, error) { return privateTestBase(t), nil }
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("injected reparse = %v", err)
	}

	platformIO = wrapIO{windowsIO: realWindowsIO{}, attrFn: func(path string) (uint32, error) {
		if filepath.Base(path) == "dws" {
			return windows.FILE_ATTRIBUTE_ARCHIVE, nil
		}
		return realWindowsIO{}.attributes(path)
	}}
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("file-as-dir = %v", err)
	}
}

func TestCrossPlatformCoverageWindowsBootstrapAndSecureOpenName(t *testing.T) {
	parent := privateTestBase(t)
	missing := filepath.Join(parent, "Local")
	userCacheDir = func() (string, error) { return missing, nil }
	platformIO = realWindowsIO{}
	programDataDir = func() string { return "" }
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	cache, err := Open("official")
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if _, err := os.Stat(missing); err != nil {
		t.Fatalf("bootstrap missing user cache: %v", err)
	}

	uc := cache.backend.(*windowsCache)
	if _, _, err := uc.secureOpen("..\\escape", windows.GENERIC_READ, secureShareRead, windows.OPEN_EXISTING); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("escape name = %v", err)
	}

	if _, err := openCacheDirectory("C:relative", "edition", &Counters{}, realWindowsIO{}, true, false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("drive-relative = %v", err)
	}
	if _, err := openCacheDirectory(`C:\ok\..\nope`, "edition", &Counters{}, realWindowsIO{}, true, false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("unclean = %v", err)
	}

	info := windows.ByHandleFileInformation{FileAttributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, NumberOfLinks: 1}
	if err := validateCacheFile(info, false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("reparse file = %v", err)
	}
	info = windows.ByHandleFileInformation{FileAttributes: windows.FILE_ATTRIBUTE_DIRECTORY, NumberOfLinks: 1}
	if err := validateCacheFile(info, false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("dir file = %v", err)
	}
	info = windows.ByHandleFileInformation{NumberOfLinks: 2}
	if err := validateCacheFile(info, false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("hardlink file = %v", err)
	}
	if !isNotFound(windows.ERROR_PATH_NOT_FOUND) || isNotFound(errors.New("other")) {
		t.Fatal("isNotFound")
	}
	a := fileState{volume: 1, indexH: 2, indexL: 3, size: 4, attrs: 5, nlink: 1}
	b := a
	if !sameFileState(a, b) {
		t.Fatal("same state")
	}
	b.size++
	if sameFileState(a, b) {
		t.Fatal("different state")
	}
	_ = fileStateFrom(windows.ByHandleFileInformation{FileSizeLow: 4, NumberOfLinks: 1})
}

func TestCrossPlatformCoverageWindowsShardOpenFaultsAndLockRelease(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("shard-meta"))
	reg := testArtifact(KindRegistry, []byte("shard-registry-bytes"))
	payloads := testArtifact(KindPayloads, []byte("shard-payload-bytes"))
	if err := cache.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	uc := cache.backend.(*windowsCache)
	if err := os.WriteFile(filepath.Join(uc.path, registryFileName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.OpenRegistry(identity, reg.Expectation); err == nil {
		t.Fatal("short registry opened")
	}
	junk := make([]byte, HeaderSize)
	if err := os.WriteFile(filepath.Join(uc.path, payloadFileName), junk, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.OpenPayloads(identity, payloads.Expectation); err == nil {
		t.Fatal("junk payloads opened")
	}

	if err := cache.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	opened, err := cache.OpenRegistry(identity, reg.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	wr := opened.backend.(*windowsRegistry)
	wr.ops = wrapIO{windowsIO: realWindowsIO{}, infoFn: func(windows.Handle) (windows.ByHandleFileInformation, error) {
		return windows.ByHandleFileInformation{}, errors.New("range info")
	}}
	if _, err := opened.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: sha256.Sum256(reg.Payload[:1])}); err == nil {
		t.Fatal("range info failure accepted")
	}
	if err := opened.ValidateAggregate(); err == nil {
		t.Fatal("aggregate info failure accepted")
	}
	_ = opened.Close()

	held, err := cache.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	wl := held.backend.(*windowsLock)
	wl.ops = wrapIO{windowsIO: realWindowsIO{}, unlockFn: func(windows.Handle) error {
		return errors.New("forced unlock")
	}}
	if err := held.Release(); err == nil {
		t.Fatal("unlock failure accepted")
	}

	held2, err := cache.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	wl2 := held2.backend.(*windowsLock)
	wl2.ops = wrapIO{windowsIO: realWindowsIO{}, closeFn: func(windows.Handle) error {
		_ = realWindowsIO{}.close(wl2.fd)
		return errors.New("forced close")
	}}
	if err := held2.Release(); err == nil {
		t.Fatal("close failure accepted")
	}
}

func TestCrossPlatformCoverageWindowsCorruptRegistryAggregate(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("agg-meta"))
	reg := testArtifact(KindRegistry, []byte("aggregate-registry-payload"))
	if err := cache.Publish(identity, reg, meta); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cache.Directory(), registryFileName)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body[len(body)-1] ^= 0xff
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := cache.OpenRegistry(identity, reg.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if err := opened.ValidateAggregate(); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("corrupt aggregate = %v", err)
	}
}

func TestCrossPlatformCoverageWindowsConcurrentLocalLock(t *testing.T) {
	cache, _, _ := openTestCache(t, nil)
	held, err := cache.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	errCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		_, err := cache.AcquireLock(context.Background(), 20*time.Millisecond)
		errCh <- err
	}()
	wg.Wait()
	if err := <-errCh; !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("contended lock = %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageWindowsReplaceWhileReaderOpen(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("replace-open-meta"))
	reg := testArtifact(KindRegistry, []byte("replace-open-registry"))
	payloads := testArtifact(KindPayloads, []byte("replace-open-payloads"))
	if err := cache.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	handle, err := cache.OpenPayloads(identity, payloads.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	nextMeta := testArtifact(KindMeta, []byte("replace-open-meta-2"))
	nextReg := testArtifact(KindRegistry, []byte("replace-open-registry-2"))
	nextPayloads := testArtifact(KindPayloads, []byte("replace-open-payloads-2"))
	if err := cache.Publish(identity, nextReg, nextMeta, nextPayloads); err != nil {
		t.Fatalf("replace while reader open: %v", err)
	}
	reopened, err := cache.OpenPayloads(identity, nextPayloads.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.ReadRange(RangeDescriptor{Offset: 0, Length: uint64(len(nextPayloads.Payload)), SHA256: nextPayloads.Expectation.EncodedSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(nextPayloads.Payload) {
		t.Fatalf("replaced payload = %q", got)
	}
}

func TestCrossPlatformCoverageWindowsAncestryAttrFaults(t *testing.T) {
	base := privateTestBase(t)
	platformIO = wrapIO{windowsIO: realWindowsIO{}, attrFn: func(path string) (uint32, error) {
		if path == filepath.VolumeName(base)+`\` {
			return 0, errors.New("volume attrs")
		}
		return realWindowsIO{}.attributes(path)
	}}
	userCacheDir = func() (string, error) { return base, nil }
	programDataDir = func() string { return "" }
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("volume attr = %v", err)
	}

	if err := validateAttrsDirectory(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY, false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("ancestry reparse = %v", err)
	}
	if err := validateAncestryPath(`Z:\missing-drive-path-dws`, &Counters{}, wrapIO{windowsIO: realWindowsIO{}, attrFn: func(string) (uint32, error) {
		return 0, windows.ERROR_PATH_NOT_FOUND
	}}); err == nil {
		t.Fatal("missing ancestry accepted")
	}
}
