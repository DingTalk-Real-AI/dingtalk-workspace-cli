//go:build (darwin && arm64) || (linux && amd64)

package schemacache

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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

func openTestCache(t *testing.T, ops unixIO) (*Cache, *Counters, ExpectedIdentity) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	oldUserCacheDir, oldPlatformIO := userCacheDir, platformIO
	userCacheDir = func() (string, error) { return base, nil }
	if ops == nil {
		ops = realUnixIO{}
	}
	platformIO = ops
	t.Cleanup(func() {
		userCacheDir, platformIO = oldUserCacheDir, oldPlatformIO
	})
	counters := &Counters{}
	cache, err := Open("official", WithCounters(counters))
	if err != nil {
		t.Fatalf("Open: %v (base %s)", err, base)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return cache, counters, testIdentity(t, "official")
}

func TestSecureReadPathsAndCounters(t *testing.T) {
	cache, counters, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("authenticated meta"))
	registry := testArtifact(KindRegistry, []byte("alpha-product-beta-product"))
	if err := cache.Publish(identity, registry, meta); err != nil {
		t.Fatal(err)
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
		t.Fatal("Meta/overview path read Registry payload")
	}

	opened, err := cache.OpenRegistry(identity, registry.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	afterOpen := counters.Snapshot()
	if afterOpen.RegistryReadOps != afterMeta.RegistryReadOps {
		t.Fatal("OpenRegistry hashed or read Registry payload")
	}
	rangeBytes := registry.Payload[6:13]
	gotRange, err := opened.ReadRange(RangeDescriptor{Offset: 6, Length: 7, SHA256: sha256.Sum256(rangeBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if string(gotRange) != string(rangeBytes) {
		t.Fatalf("range = %q", gotRange)
	}
	afterRange := counters.Snapshot()
	if delta := afterRange.RegistryReadBytes - afterOpen.RegistryReadBytes; delta != uint64(len(rangeBytes)) {
		t.Fatalf("targeted Registry read bytes = %d, want %d", delta, len(rangeBytes))
	}
	if err := opened.ValidateAggregate(); err != nil {
		t.Fatal(err)
	}
	if counters.Snapshot().RegistryReadBytes-afterRange.RegistryReadBytes != uint64(len(registry.Payload)) {
		t.Fatal("aggregate validation did not read the exact payload")
	}
}

func TestReadMetaRejectsPartialTrailingAndUntrustedSelfDigest(t *testing.T) {
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

	for name, bytes := range map[string][]byte{
		"partial":  original[:len(original)-1],
		"trailing": append(append([]byte(nil), original...), 0),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, bytes, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrInvalidArtifact) {
				t.Fatalf("ReadMeta error = %v", err)
			}
		})
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
		t.Fatalf("self-consistent forged artifact error = %v", err)
	}
}

func TestSecureFileAndDirectoryRejections(t *testing.T) {
	t.Run("invalid edition traversal", func(t *testing.T) {
		if _, err := Open("../escape"); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Open error = %v", err)
		}
	})

	for _, test := range []struct {
		name  string
		setup func(t *testing.T, cache *Cache, target string)
	}{
		{"symlink", func(t *testing.T, cache *Cache, target string) {
			other := filepath.Join(cache.Directory(), "other")
			if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(other, target); err != nil {
				t.Fatal(err)
			}
		}},
		{"directory", func(t *testing.T, _ *Cache, target string) {
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"wrong mode", func(t *testing.T, _ *Cache, target string) {
			if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"hardlink", func(t *testing.T, cache *Cache, target string) {
			other := filepath.Join(cache.Directory(), "linked")
			if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(other, target); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cache, _, identity := openTestCache(t, nil)
			target := filepath.Join(cache.Directory(), metaFileName)
			test.setup(t, cache, target)
			expected := testArtifact(KindMeta, []byte("x")).Expectation
			if _, err := cache.ReadMeta(identity, expected); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("ReadMeta error = %v", err)
			}
		})
	}
}

func TestSecureRootRejectsSymlinkAndWritableOwnedSuffix(t *testing.T) {
	for name, prepare := range map[string]func(*testing.T, string){
		"symlink": func(t *testing.T, base string) {
			destination := t.TempDir()
			if err := os.Symlink(destination, filepath.Join(base, "dws")); err != nil {
				t.Fatal(err)
			}
		},
		"writable": func(t *testing.T, base string) {
			path := filepath.Join(base, "dws")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o755); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			prepare(t, base)
			old := userCacheDir
			userCacheDir = func() (string, error) { return base, nil }
			t.Cleanup(func() { userCacheDir = old })
			if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("Open error = %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageCacheBootstrapsMissingUserCacheDirectory(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".cache", "Library/Caches"} {
		t.Run(suffix, func(t *testing.T) {
			base := filepath.Join(parent, suffix)
			fd, path, err := openCacheDirectory(base, "edition", &Counters{}, realUnixIO{})
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(fd)
			if path != filepath.Join(base, "dws/schema/edition/v1") {
				t.Fatalf("unexpected cache path: %s", path)
			}
			info, err := os.Stat(base)
			if err != nil || info.Mode().Perm() != 0o700 {
				t.Fatalf("new user cache directory must be private: info=%v err=%v", info, err)
			}
		})
	}
	unsafe := filepath.Join(parent, "writable")
	if err := os.Mkdir(unsafe, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0o777); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(unsafe, "missing")
	if fd, _, err := openCacheDirectory(base, "edition", &Counters{}, realUnixIO{}); !errors.Is(err, ErrUnsafePath) {
		if fd >= 0 {
			unix.Close(fd)
		}
		t.Fatalf("writable ancestor was accepted: %v", err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("created cache below an unsafe ancestor: %v", err)
	}
}

type shortPreadIO struct {
	unixIO
	mu           sync.Mutex
	payloadCalls int
}

func (s *shortPreadIO) pread(fd int, p []byte, offset int64) (int, error) {
	if offset >= HeaderSize {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.payloadCalls++
		if s.payloadCalls > 1 {
			return 0, nil
		}
		p = p[:max(1, len(p)/2)]
	}
	return s.unixIO.pread(fd, p, offset)
}

func TestReadMetaRejectsShortPread(t *testing.T) {
	ops := &shortPreadIO{unixIO: realUnixIO{}}
	cache, _, identity := openTestCache(t, ops)
	meta := testArtifact(KindMeta, []byte("long enough payload"))
	registry := testArtifact(KindRegistry, []byte("registry"))
	if err := cache.Publish(identity, registry, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("ReadMeta error = %v", err)
	}
}

type chmodDuringPreadIO struct {
	unixIO
	once sync.Once
}

func (c *chmodDuringPreadIO) pread(fd int, p []byte, offset int64) (int, error) {
	n, err := c.unixIO.pread(fd, p, offset)
	if offset >= HeaderSize {
		c.once.Do(func() { _ = unix.Fchmod(fd, 0o640) })
	}
	return n, err
}

func TestReadMetaRejectsFileChangedDuringRead(t *testing.T) {
	ops := &chmodDuringPreadIO{unixIO: realUnixIO{}}
	cache, _, identity := openTestCache(t, ops)
	meta := testArtifact(KindMeta, []byte("meta"))
	registry := testArtifact(KindRegistry, []byte("registry"))
	if err := cache.Publish(identity, registry, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("ReadMeta error = %v", err)
	}
}

type failWriteIO struct{ unixIO }

func (f failWriteIO) write(int, []byte) (int, error) { return 0, io.ErrShortWrite }

func TestAtomicFailureKeepsOldArtifactAndCleansTemp(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	oldMeta := testArtifact(KindMeta, []byte("old meta"))
	registry := testArtifact(KindRegistry, []byte("registry"))
	if err := cache.Publish(identity, registry, oldMeta); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cache.Directory(), metaFileName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cache.backend.(*unixCache).ops = failWriteIO{unixIO: realUnixIO{}}
	newMeta := testArtifact(KindMeta, []byte("new meta"))
	if err := cache.WriteArtifact(identity, newMeta); err == nil {
		t.Fatal("WriteArtifact unexpectedly succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, after) {
		t.Fatal("failed replacement changed old artifact")
	}
	entries, err := os.ReadDir(cache.Directory())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("staging file was not cleaned: %s", entry.Name())
		}
	}
}

type stagedFaultIO struct {
	unixIO
	mu      sync.Mutex
	stage   string
	staging map[int]bool
	renamed bool
}

func (f *stagedFaultIO) openat(dirfd int, path string, flags int, mode uint32) (int, error) {
	fd, err := f.unixIO.openat(dirfd, path, flags, mode)
	if err == nil && filepath.Ext(path) == ".tmp" {
		f.mu.Lock()
		if f.staging == nil {
			f.staging = make(map[int]bool)
		}
		f.staging[fd] = true
		f.mu.Unlock()
	}
	return fd, err
}

func (f *stagedFaultIO) isStaging(fd int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.staging[fd]
}

func (f *stagedFaultIO) write(fd int, p []byte) (int, error) {
	if f.stage == "write" && f.isStaging(fd) {
		return 0, io.ErrShortWrite
	}
	return f.unixIO.write(fd, p)
}

func (f *stagedFaultIO) fsync(fd int) error {
	if f.stage == "file sync" && f.isStaging(fd) {
		return errors.New("injected file sync failure")
	}
	f.mu.Lock()
	renamed := f.renamed
	f.mu.Unlock()
	if f.stage == "directory sync" && renamed && !f.isStaging(fd) {
		return errors.New("injected directory sync failure")
	}
	return f.unixIO.fsync(fd)
}

func (f *stagedFaultIO) close(fd int) error {
	if f.stage == "close" && f.isStaging(fd) {
		f.mu.Lock()
		delete(f.staging, fd)
		f.mu.Unlock()
		_ = f.unixIO.close(fd)
		return errors.New("injected close failure")
	}
	return f.unixIO.close(fd)
}

func (f *stagedFaultIO) renameat(olddirfd int, oldpath string, newdirfd int, newpath string) error {
	if f.stage == "rename" {
		return errors.New("injected rename failure")
	}
	if err := f.unixIO.renameat(olddirfd, oldpath, newdirfd, newpath); err != nil {
		return err
	}
	f.mu.Lock()
	f.renamed = true
	f.mu.Unlock()
	return nil
}

func TestAtomicWriterFaultMatrix(t *testing.T) {
	for _, stage := range []string{"write", "file sync", "close", "rename", "directory sync"} {
		t.Run(stage, func(t *testing.T) {
			cache, counters, identity := openTestCache(t, nil)
			oldMeta := testArtifact(KindMeta, []byte("old"))
			if err := cache.WriteArtifact(identity, oldMeta); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(cache.Directory(), metaFileName)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fault := &stagedFaultIO{unixIO: realUnixIO{}, stage: stage}
			cache.backend.(*unixCache).ops = fault
			beforeCounters := counters.Snapshot()
			if err := cache.WriteArtifact(identity, testArtifact(KindMeta, []byte("new"))); err == nil {
				t.Fatal("WriteArtifact unexpectedly succeeded")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if stage != "directory sync" && !slices.Equal(before, after) {
				t.Fatal("failure before rename replaced the old artifact")
			}
			entries, err := os.ReadDir(cache.Directory())
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if filepath.Ext(entry.Name()) == ".tmp" {
					t.Fatalf("staging file was not cleaned: %s", entry.Name())
				}
			}
			if stage != "directory sync" && counters.Snapshot().RemoveOps == beforeCounters.RemoveOps {
				t.Fatal("failed staging file did not execute bounded cleanup")
			}
		})
	}
}

type wrongOwnerIO struct {
	unixIO
	mu      sync.Mutex
	targets map[int]bool
}

func (w *wrongOwnerIO) openat(dirfd int, path string, flags int, mode uint32) (int, error) {
	fd, err := w.unixIO.openat(dirfd, path, flags, mode)
	if err == nil && path == metaFileName {
		w.mu.Lock()
		if w.targets == nil {
			w.targets = make(map[int]bool)
		}
		w.targets[fd] = true
		w.mu.Unlock()
	}
	return fd, err
}

func (w *wrongOwnerIO) fstat(fd int, stat *unix.Stat_t) error {
	if err := w.unixIO.fstat(fd, stat); err != nil {
		return err
	}
	w.mu.Lock()
	isTarget := w.targets[fd]
	w.mu.Unlock()
	if isTarget {
		stat.Uid++
	}
	return nil
}

func TestSecureReadRejectsWrongOwnerThroughIOSeam(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("meta"))
	if err := cache.WriteArtifact(identity, meta); err != nil {
		t.Fatal(err)
	}
	cache.backend.(*unixCache).ops = &wrongOwnerIO{unixIO: realUnixIO{}}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("ReadMeta error = %v", err)
	}
}

type recordingIO struct {
	unixIO
	mu      sync.Mutex
	renames []string
	failOn  string
}

func (r *recordingIO) renameat(olddirfd int, oldpath string, newdirfd int, newpath string) error {
	r.mu.Lock()
	r.renames = append(r.renames, newpath)
	r.mu.Unlock()
	if newpath == r.failOn {
		return errors.New("injected rename failure")
	}
	return r.unixIO.renameat(olddirfd, oldpath, newdirfd, newpath)
}

func TestPublishRegistryFirstMetaLastAndStopsOnFailure(t *testing.T) {
	for _, failRegistry := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail=%v", failRegistry), func(t *testing.T) {
			recorder := &recordingIO{unixIO: realUnixIO{}}
			if failRegistry {
				recorder.failOn = registryFileName
			}
			cache, _, identity := openTestCache(t, recorder)
			err := cache.Publish(identity, testArtifact(KindRegistry, []byte("registry")), testArtifact(KindMeta, []byte("meta")))
			if failRegistry {
				if err == nil {
					t.Fatal("Publish unexpectedly succeeded")
				}
				if !slices.Equal(recorder.renames, []string{registryFileName}) {
					t.Fatalf("renames = %v", recorder.renames)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(recorder.renames, []string{registryFileName, metaFileName}) {
				t.Fatalf("renames = %v", recorder.renames)
			}
		})
	}
}

func TestRegistryRangeBoundsAndDigest(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	registry := testArtifact(KindRegistry, []byte("registry"))
	meta := testArtifact(KindMeta, []byte("meta"))
	if err := cache.Publish(identity, registry, meta); err != nil {
		t.Fatal(err)
	}
	opened, err := cache.OpenRegistry(identity, registry.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	badDigest := sha256.Sum256([]byte("wrong"))
	tests := []RangeDescriptor{
		{Offset: 0, Length: 0, SHA256: badDigest},
		{Offset: 0, Length: MaxProductRangeSize + 1, SHA256: badDigest},
		{Offset: uint64(len(registry.Payload)), Length: 1, SHA256: badDigest},
		{Offset: 0, Length: 1, SHA256: badDigest},
	}
	for _, descriptor := range tests {
		if _, err := opened.ReadRange(descriptor); err == nil {
			t.Fatalf("ReadRange(%+v) unexpectedly succeeded", descriptor)
		}
	}
}

func TestRegistryRejectsChangedOpenFile(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	registry := testArtifact(KindRegistry, []byte("registry"))
	if err := cache.WriteArtifact(identity, registry); err != nil {
		t.Fatal(err)
	}
	opened, err := cache.OpenRegistry(identity, registry.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	backend := opened.backend.(*unixRegistry)
	if err := unix.Fchmod(backend.fd, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ReadRange(RangeDescriptor{
		Length: uint64(len(registry.Payload)), SHA256: sha256.Sum256(registry.Payload),
	}); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("ReadRange error = %v", err)
	}
}

func TestConcurrentAtomicReplacementNeverExposesPartialArtifact(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	oldMeta := testArtifact(KindMeta, []byte("old payload"))
	newMeta := testArtifact(KindMeta, []byte("new payload"))
	if err := cache.WriteArtifact(identity, oldMeta); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			artifact := oldMeta
			if i%2 != 0 {
				artifact = newMeta
			}
			if err := cache.WriteArtifact(identity, artifact); err != nil {
				errCh <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if payload, err := cache.ReadMeta(identity, oldMeta.Expectation); err == nil {
				if !slices.Equal(payload, oldMeta.Payload) {
					errCh <- errors.New("old expectation returned other bytes")
					return
				}
			} else if !errors.Is(err, ErrIdentityMismatch) && !errors.Is(err, ErrInvalidArtifact) && !errors.Is(err, ErrUnsafePath) {
				errCh <- fmt.Errorf("old read failed unexpectedly: %w", err)
				return
			}
			if payload, err := cache.ReadMeta(identity, newMeta.Expectation); err == nil {
				if !slices.Equal(payload, newMeta.Payload) {
					errCh <- errors.New("new expectation returned other bytes")
					return
				}
			} else if !errors.Is(err, ErrIdentityMismatch) && !errors.Is(err, ErrInvalidArtifact) && !errors.Is(err, ErrUnsafePath) {
				errCh <- fmt.Errorf("new read failed unexpectedly: %w", err)
				return
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestLockTimeoutIsDistinctAndLockRemainsUsable(t *testing.T) {
	cache, _, _ := openTestCache(t, nil)
	first, err := cache.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.AcquireLock(context.Background(), 20*time.Millisecond); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("second AcquireLock error = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	third, err := cache.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := third.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestCrossProcessFlockTimeout(t *testing.T) {
	cache, _, _ := openTestCache(t, nil)
	path := filepath.Join(cache.Directory(), lockFileName)
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	if _, err := cache.AcquireLock(context.Background(), 20*time.Millisecond); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("AcquireLock error = %v", err)
	}
}
