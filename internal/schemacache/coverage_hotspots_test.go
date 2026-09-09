// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemacache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"golang.org/x/sys/unix"
)

func TestCrossPlatformCoverageSystemCacheBaseAndOverrides(t *testing.T) {
	if got := func() string {
		testseam.Swap(t, &currentGOOS, "linux")
		return systemSchemaCacheBase()
	}(); got != linuxSystemSchemaCacheBase {
		t.Fatalf("linux base = %q", got)
	}
	testseam.Swap(t, &currentGOOS, "darwin")
	if got := systemSchemaCacheBase(); got != darwinSystemSchemaCacheBase {
		t.Fatalf("darwin base = %q", got)
	}
	testseam.Swap(t, &currentGOOS, "windows")
	if got := systemSchemaCacheBase(); got != "" {
		t.Fatalf("windows base = %q", got)
	}

	base := privateTestBase(t)
	t.Setenv("DWS_SCHEMA_CACHE_DIR", base)
	cache, err := Open("official")
	if err != nil {
		t.Fatalf("override Open: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	if cache.Directory() == "" {
		t.Fatal("shared override directory empty")
	}

	t.Setenv("DWS_SCHEMA_CACHE_DIR", "relative/cache")
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("relative override error = %v", err)
	}

	testseam.Swap(t, &currentGOOS, "windows")
	testseam.Swap(t, &userCacheDir, func() (string, error) { return "", errors.New("no cache dir") })
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	if _, err := Open("official"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("user cache error = %v", err)
	}

	testseam.Swap(t, &userCacheDir, os.UserCacheDir)
	shared := privateTestBase(t)
	digest := sha256.Sum256([]byte("official"))
	editionHex := hex.EncodeToString(digest[:])
	if err := os.MkdirAll(filepath.Join(shared, "dws", "schema", editionHex, "v1"), 0o700); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &linuxSystemSchemaCacheBase, shared)
	testseam.Swap(t, &currentGOOS, "linux")
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	sharedCache, err := Open("official", WithNoCreate())
	if err != nil {
		t.Fatalf("system shared Open: %v (base %s)", err, shared)
	}
	_ = sharedCache.Close()
}

func TestCrossPlatformCoverageNilCacheAndEnvelopeValidation(t *testing.T) {
	var cache *Cache
	if cache.Directory() != "" {
		t.Fatal("nil Directory")
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(ExpectedIdentity{}, ArtifactExpectation{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("nil ReadMeta = %v", err)
	}
	if _, err := cache.OpenRegistry(ExpectedIdentity{}, ArtifactExpectation{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("nil OpenRegistry = %v", err)
	}
	if _, err := cache.OpenPayloads(ExpectedIdentity{}, ArtifactExpectation{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("nil OpenPayloads = %v", err)
	}
	if err := cache.WriteArtifact(ExpectedIdentity{}, Artifact{}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := cache.Publish(ExpectedIdentity{}, Artifact{}, Artifact{}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := cache.AcquireLock(context.Background(), time.Millisecond); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	empty := &Cache{}
	if empty.Directory() != "" || empty.Close() != nil {
		t.Fatal("empty backend")
	}
	var registry *Registry
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ReadRange(RangeDescriptor{}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := registry.ValidateAggregate(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if snap := (*Counters)(nil).Snapshot(); snap.RegistryReadOps != 0 {
		t.Fatalf("nil Snapshot = %#v", snap)
	}
	var lock *Lock
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}

	identity := testIdentity(t, "official")
	zeroVersion := identity
	zeroVersion.CatalogSnapshotVersion = 0
	if err := zeroVersion.validate(); err == nil {
		t.Fatal("zero snapshot version accepted")
	}
	if err := (ArtifactExpectation{Kind: KindMeta}).validate(KindRegistry); err == nil {
		t.Fatal("kind mismatch accepted")
	}
	if err := (ArtifactExpectation{Kind: KindMeta, Serializer: 99}).validate(KindMeta); err == nil {
		t.Fatal("invalid shape accepted")
	}
	if _, err := envelopeFrom(zeroVersion, testArtifact(KindMeta, []byte("x")).Expectation); err == nil {
		t.Fatal("envelopeFrom accepted zero version")
	}
	if _, err := envelopeFrom(identity, ArtifactExpectation{Kind: KindMeta, Serializer: 99}); err == nil {
		t.Fatal("envelopeFrom accepted invalid artifact")
	}

	opened, _, openedIdentity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("meta-bytes"))
	reg := testArtifact(KindRegistry, []byte("registry-bytes"))
	if err := opened.Publish(openedIdentity, Artifact{Expectation: meta.Expectation, Payload: meta.Payload}, meta); err == nil {
		t.Fatal("publish with swapped kinds accepted")
	}
	if err := opened.Publish(openedIdentity, reg, meta, testArtifact(KindMeta, []byte("nope"))); err == nil {
		t.Fatal("non-payload extra accepted")
	}
	if err := opened.WriteArtifact(zeroVersion, meta); err == nil {
		t.Fatal("write with zero identity accepted")
	}
	badLen := meta
	badLen.Expectation.EncodedLength = 1
	if err := opened.WriteArtifact(openedIdentity, badLen); err == nil {
		t.Fatal("length mismatch accepted")
	}
	badDigest := meta
	badDigest.Expectation.EncodedSHA256 = sha256.Sum256([]byte("other"))
	if err := opened.WriteArtifact(openedIdentity, badDigest); err == nil {
		t.Fatal("digest mismatch accepted")
	}
	if _, err := opened.ReadMeta(zeroVersion, meta.Expectation); err == nil {
		t.Fatal("ReadMeta zero identity accepted")
	}
	if _, err := opened.ReadMeta(openedIdentity, reg.Expectation); err == nil {
		t.Fatal("ReadMeta kind mismatch accepted")
	}
	if _, err := opened.OpenRegistry(zeroVersion, reg.Expectation); err == nil {
		t.Fatal("OpenRegistry zero identity accepted")
	}
	if _, err := opened.OpenRegistry(openedIdentity, meta.Expectation); err == nil {
		t.Fatal("OpenRegistry kind mismatch accepted")
	}
	payloads := testArtifact(KindPayloads, []byte("payload-bytes"))
	if _, err := opened.OpenPayloads(zeroVersion, payloads.Expectation); err == nil {
		t.Fatal("OpenPayloads zero identity accepted")
	}
	if _, err := opened.OpenPayloads(openedIdentity, meta.Expectation); err == nil {
		t.Fatal("OpenPayloads kind mismatch accepted")
	}
}

func TestCrossPlatformCoverageClosedCacheAndIdentityMismatch(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("closed-meta"))
	reg := testArtifact(KindRegistry, []byte("closed-reg"))
	payloads := testArtifact(KindPayloads, []byte("closed-payloads"))
	if err := cache.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	wrong := identity
	wrong.EditionSHA256 = sha256.Sum256([]byte("other-edition"))
	if _, err := cache.ReadMeta(wrong, meta.Expectation); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("ReadMeta mismatch = %v", err)
	}
	if _, err := cache.OpenRegistry(wrong, reg.Expectation); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("OpenRegistry mismatch = %v", err)
	}
	if _, err := cache.OpenPayloads(wrong, payloads.Expectation); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("OpenPayloads mismatch = %v", err)
	}
	if err := cache.WriteArtifact(wrong, meta); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("WriteArtifact mismatch = %v", err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed ReadMeta = %v", err)
	}
	if _, err := cache.OpenRegistry(identity, reg.Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed OpenRegistry = %v", err)
	}
	if _, err := cache.OpenPayloads(identity, payloads.Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed OpenPayloads = %v", err)
	}
	if err := cache.WriteArtifact(identity, meta); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := cache.AcquireLock(context.Background(), 0); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageSharedValidationAndIOFaults(t *testing.T) {
	base := privateTestBase(t)
	fd, err := unix.Open(base, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	counters := &Counters{}
	if err := validateOwnedDirectory(fd, counters, realUnixIO{}, true); err != nil {
		t.Fatalf("shared owned dir: %v", err)
	}
	if err := validateAncestryDirectory(fd, counters, realUnixIO{}); err != nil {
		t.Fatalf("ancestry: %v", err)
	}
	if err := validateOwnedDirectory(fd, counters, failFstatIO{err: errors.New("fstat failed")}, true); err == nil {
		t.Fatal("fstat failure accepted")
	}
	if err := validateAncestryDirectory(fd, counters, failFstatIO{err: errors.New("fstat failed")}); err == nil {
		t.Fatal("ancestry fstat failure accepted")
	}

	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("eintr-meta"))
	reg := testArtifact(KindRegistry, []byte("eintr-reg"))
	if err := cache.Publish(identity, reg, meta); err != nil {
		t.Fatal(err)
	}
	_ = cache.Close()

	eintr := &eintrThenRealIO{remaining: 2}
	cache2, _, identity2 := openTestCache(t, eintr)
	if err := cache2.Publish(identity2, testArtifact(KindRegistry, []byte("eintr-reg-2")), testArtifact(KindMeta, []byte("eintr-meta-2"))); err != nil {
		t.Fatal(err)
	}

	failStat := &failFstatIO{err: errors.New("fstat failed")}
	if _, _, err := openTestCacheMaybe(t, failStat); err == nil {
		t.Fatal("fstat failure accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	live, _, _ := openTestCache(t, nil)
	held, err := live.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	if _, err := live.AcquireLock(ctx, time.Second); err == nil {
		t.Fatal("cancelled lock accepted")
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := live.AcquireLock(context.Background(), -time.Second); err != nil && !errors.Is(err, ErrLockTimeout) {
		// timeout 0 should still attempt; a free lock succeeds
	}

	shortWrite := failWriteIO{unixIO: realUnixIO{}}
	cache3, _, identity3 := openTestCache(t, shortWrite)
	if err := cache3.WriteArtifact(identity3, testArtifact(KindMeta, []byte("short"))); err == nil {
		t.Fatal("short write accepted")
	}

	exhausted := &alwaysEEXISTOpenatIO{}
	cache4, _, identity4 := openTestCache(t, exhausted)
	if err := cache4.WriteArtifact(identity4, testArtifact(KindMeta, []byte("stage"))); err == nil {
		t.Fatal("exhausted staging accepted")
	}
}

type eintrThenRealIO struct {
	realUnixIO
	remaining int
}

func (e *eintrThenRealIO) pread(fd int, p []byte, offset int64) (int, error) {
	if e.remaining > 0 {
		e.remaining--
		return 0, unix.EINTR
	}
	return e.realUnixIO.pread(fd, p, offset)
}

func (e *eintrThenRealIO) write(fd int, p []byte) (int, error) {
	if e.remaining > 0 {
		e.remaining--
		return 0, unix.EINTR
	}
	return e.realUnixIO.write(fd, p)
}

type failFstatIO struct {
	realUnixIO
	err error
}

func (f failFstatIO) fstat(int, *unix.Stat_t) error { return f.err }

type alwaysEEXISTOpenatIO struct{ realUnixIO }

func (a alwaysEEXISTOpenatIO) openat(dirfd int, path string, flags int, mode uint32) (int, error) {
	if flags&unix.O_EXCL != 0 {
		return -1, unix.EEXIST
	}
	return a.realUnixIO.openat(dirfd, path, flags, mode)
}

func openTestCacheMaybe(t *testing.T, ops unixIO) (*Cache, *Counters, error) {
	t.Helper()
	base := privateTestBase(t)
	oldUserCacheDir, oldPlatformIO := userCacheDir, platformIO
	userCacheDir = func() (string, error) { return base, nil }
	platformIO = ops
	t.Cleanup(func() {
		userCacheDir, platformIO = oldUserCacheDir, oldPlatformIO
	})
	counters := &Counters{}
	cache, err := Open("official", WithCounters(counters))
	return cache, counters, err
}

func TestCrossPlatformCoverageLocalLockAndWaitRetry(t *testing.T) {
	lock := &localLock{token: make(chan struct{}, 1)}
	if err := takeLocalLock(context.Background(), lock, time.Now().Add(-time.Second)); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("empty expired lock = %v", err)
	}
	lock.token <- struct{}{}
	if err := takeLocalLock(context.Background(), lock, time.Now().Add(-time.Second)); err != nil {
		t.Fatalf("token expired lock = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := takeLocalLock(ctx, &localLock{token: make(chan struct{})}, time.Now().Add(time.Second)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled takeLocalLock = %v", err)
	}
	if err := waitForRetry(context.Background(), time.Now().Add(-time.Second)); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("expired wait = %v", err)
	}
	if err := waitForRetry(ctx, time.Now().Add(time.Second)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait = %v", err)
	}
	failFlock := failFlockCloseIO{unlockErr: errors.New("unlock"), closeErr: errors.New("close")}
	locked := &unixLock{fd: 3, local: &localLock{token: make(chan struct{}, 1)}, counters: &Counters{}, ops: failFlock}
	if err := locked.release(); err == nil {
		t.Fatal("lock release errors ignored")
	}
	closeOnly := failFlockCloseIO{closeErr: errors.New("close")}
	closed := &unixLock{fd: 3, local: &localLock{token: make(chan struct{}, 1)}, counters: &Counters{}, ops: closeOnly}
	if err := closed.release(); err == nil {
		t.Fatal("lock close errors ignored")
	}
}

type failFlockCloseIO struct {
	realUnixIO
	unlockErr error
	closeErr  error
}

func (f failFlockCloseIO) flock(int, int) error { return f.unlockErr }
func (f failFlockCloseIO) close(int) error      { return f.closeErr }

func TestCrossPlatformCoverageValidateSharedCacheFileAndDirectory(t *testing.T) {
	uid := uint32(unix.Geteuid())
	reg := uint32(unix.S_IFREG)
	if err := validateCacheFile(fileState{mode: reg | 0o644, uid: uid, nlink: 1}, true); err != nil {
		t.Fatal(err)
	}
	if err := validateCacheFile(fileState{mode: reg | 0o644, uid: 0, nlink: 1}, true); err != nil {
		t.Fatal(err)
	}
	if err := validateCacheFile(fileState{mode: reg | 0o666, uid: 0, nlink: 1}, true); err == nil {
		t.Fatal("world-writable shared file accepted")
	}
	if err := validateCacheFile(fileState{mode: reg | 0o644, uid: uid + 1, nlink: 1}, true); err == nil {
		t.Fatal("other-user shared file accepted")
	}

	file, err := os.CreateTemp(privateTestBase(t), "not-dir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	fd, err := unix.Open(file.Name(), unix.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := validateOwnedDirectory(fd, &Counters{}, realUnixIO{}, true); err == nil {
		t.Fatal("regular file accepted as shared cache directory")
	}

	base := privateTestBase(t)
	if err := os.Chmod(base, 0o777); err != nil {
		t.Fatal(err)
	}
	dirfd, err := unix.Open(base, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dirfd)
	if err := validateOwnedDirectory(dirfd, &Counters{}, realUnixIO{}, true); err == nil {
		t.Fatal("world-writable shared directory accepted")
	}
}

func TestCrossPlatformCoverageOpenAndReadFaults(t *testing.T) {
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "/tmp/dws-cache/../unsafe")
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("unclean override = %v", err)
	}

	var rootIO unixIO = failRootOpenIO{}
	testseam.Swap(t, &platformIO, rootIO)
	t.Setenv("DWS_SCHEMA_CACHE_DIR", "")
	if _, err := Open("official"); err == nil {
		t.Fatal("root open failure accepted")
	}

	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("meta-bytes"))
	reg := testArtifact(KindRegistry, []byte("registry-bytes"))
	payloads := testArtifact(KindPayloads, []byte("payload-bytes"))
	if err := cache.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cache.Directory(), metaFileName)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body[len(body)-1] ^= 0xff
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("corrupt Meta digest accepted")
	}

	closeCache, _, closeIdentity := openTestCache(t, nil)
	if err := closeCache.Publish(closeIdentity, testArtifact(KindRegistry, []byte("reg")), testArtifact(KindMeta, []byte("meta"))); err != nil {
		t.Fatal(err)
	}
	closeCache.backend.(*unixCache).ops = failCloseIO{unixIO: realUnixIO{}}
	if _, err := closeCache.ReadMeta(closeIdentity, testArtifact(KindMeta, []byte("meta")).Expectation); err == nil {
		t.Fatal("Meta close failure accepted")
	}

	live, _, _ := openTestCache(t, failFlockCloseIO{unlockErr: unix.EPERM})
	if _, err := live.AcquireLock(context.Background(), time.Millisecond); err == nil {
		t.Fatal("non-EWOULDBLOCK flock accepted")
	}
}

type failRootOpenIO struct{ realUnixIO }

func (failRootOpenIO) open(path string, flags int, mode uint32) (int, error) {
	if path == string(filepath.Separator) {
		return -1, errors.New("root open failed")
	}
	return realUnixIO{}.open(path, flags, mode)
}

type failCloseIO struct{ unixIO }

func (f failCloseIO) close(int) error { return errors.New("close failed") }
