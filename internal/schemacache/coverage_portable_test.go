package schemacache

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubBackend exercises Cache/Registry/Lock facades on platforms where the
// persistent unix backend is not compiled, including Windows coverage gate.
type stubBackend struct {
	closed   bool
	dir      string
	writeErr error
}

func (s *stubBackend) close() error {
	s.closed = true
	return nil
}

func (s *stubBackend) directory() string { return s.dir }

func (s *stubBackend) guard() error {
	if s.closed {
		return ErrClosed
	}
	return nil
}

func (s *stubBackend) readMeta(ExpectedIdentity, ArtifactExpectation) ([]byte, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	return []byte("meta"), nil
}

func (s *stubBackend) openRegistry(ExpectedIdentity, ArtifactExpectation) (registryBackend, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	return &stubRegistry{}, nil
}

func (s *stubBackend) openPayloads(ExpectedIdentity, ArtifactExpectation) (registryBackend, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	return &stubRegistry{}, nil
}

func (s *stubBackend) writeArtifact(ExpectedIdentity, Artifact) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	return s.guard()
}

func (s *stubBackend) acquire(context.Context, time.Duration) (lockBackend, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	return stubLock{}, nil
}

type stubRegistry struct{ closed bool }

func (s *stubRegistry) close() error {
	s.closed = true
	return nil
}

func (s *stubRegistry) readRange(RangeDescriptor) ([]byte, error) {
	if s.closed {
		return nil, ErrClosed
	}
	return []byte("range"), nil
}

func (s *stubRegistry) validateAggregate() error {
	if s.closed {
		return ErrClosed
	}
	return nil
}

type stubLock struct{}

func (stubLock) release() error { return nil }

func portableIdentity() ExpectedIdentity {
	return ExpectedIdentity{
		CatalogSnapshotVersion: 1,
		EditionSHA256:          sha256.Sum256([]byte("edition")),
		SourceSHA256:           sha256.Sum256([]byte("source")),
		SurfaceSHA256:          sha256.Sum256([]byte("surface")),
		BuildID:                sha256.Sum256([]byte("build")),
	}
}

func portableArtifact(kind ArtifactKind, payload []byte) Artifact {
	digest := sha256.Sum256(payload)
	return Artifact{
		Expectation: ArtifactExpectation{
			Kind:          kind,
			Serializer:    SerializerProtobuf,
			Codec:         CodecRaw,
			FormatVersion: DTOFormatVersion,
			EncodedLength: uint64(len(payload)),
			DecodedLength: uint64(len(payload)),
			EncodedSHA256: digest,
		},
		Payload: payload,
	}
}

func TestCrossPlatformCoverageCacheFacadeWithoutPersistentBackend(t *testing.T) {
	if _, err := Open("!!!invalid"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("invalid edition Open = %v", err)
	}
	if _, err := EditionSHA256("!!!invalid"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("invalid edition hash = %v", err)
	}
	digest, err := EditionSHA256("official")
	if err != nil || digest == ([32]byte{}) {
		t.Fatalf("EditionSHA256 = %x, %v", digest, err)
	}
	if WithCounters(nil) == nil || WithNoCreate() == nil {
		t.Fatal("option constructors")
	}

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

	backend := &stubBackend{dir: "stub-cache"}
	opened := &Cache{backend: backend}
	if opened.Directory() != "stub-cache" {
		t.Fatalf("Directory = %q", opened.Directory())
	}
	identity := portableIdentity()
	meta := portableArtifact(KindMeta, []byte("meta-bytes"))
	reg := portableArtifact(KindRegistry, []byte("registry-bytes"))
	payloads := portableArtifact(KindPayloads, []byte("payload-bytes"))
	if err := opened.Publish(identity, Artifact{Expectation: meta.Expectation, Payload: meta.Payload}, meta); err == nil {
		t.Fatal("publish with swapped kinds accepted")
	}
	if err := opened.Publish(identity, reg, meta, portableArtifact(KindMeta, []byte("nope"))); err == nil {
		t.Fatal("non-payload extra accepted")
	}
	zeroVersion := identity
	zeroVersion.CatalogSnapshotVersion = 0
	if err := opened.WriteArtifact(zeroVersion, meta); err == nil {
		t.Fatal("write with zero identity accepted")
	}
	badLen := meta
	badLen.Expectation.EncodedLength = 1
	if err := opened.WriteArtifact(identity, badLen); err == nil {
		t.Fatal("length mismatch accepted")
	}
	badDigest := meta
	badDigest.Expectation.EncodedSHA256 = sha256.Sum256([]byte("other"))
	if err := opened.WriteArtifact(identity, badDigest); err == nil {
		t.Fatal("digest mismatch accepted")
	}
	if _, err := opened.ReadMeta(zeroVersion, meta.Expectation); err == nil {
		t.Fatal("ReadMeta zero identity accepted")
	}
	if _, err := opened.ReadMeta(identity, reg.Expectation); err == nil {
		t.Fatal("ReadMeta kind mismatch accepted")
	}
	if _, err := opened.OpenRegistry(zeroVersion, reg.Expectation); err == nil {
		t.Fatal("OpenRegistry zero identity accepted")
	}
	if _, err := opened.OpenRegistry(identity, meta.Expectation); err == nil {
		t.Fatal("OpenRegistry kind mismatch accepted")
	}
	if _, err := opened.OpenPayloads(zeroVersion, payloads.Expectation); err == nil {
		t.Fatal("OpenPayloads zero identity accepted")
	}
	if _, err := opened.OpenPayloads(identity, meta.Expectation); err == nil {
		t.Fatal("OpenPayloads kind mismatch accepted")
	}
	mismatchLen := meta
	mismatchLen.Expectation.EncodedLength = uint64(len(mismatchLen.Payload) + 4)
	mismatchLen.Expectation.DecodedLength = mismatchLen.Expectation.EncodedLength
	if err := opened.WriteArtifact(identity, mismatchLen); err == nil {
		t.Fatal("matching encoded/decoded length still accepted a different payload")
	}
	mismatchExtra := payloads
	mismatchExtra.Expectation.EncodedLength = uint64(len(mismatchExtra.Payload) + 4)
	mismatchExtra.Expectation.DecodedLength = mismatchExtra.Expectation.EncodedLength
	if err := opened.Publish(identity, reg, meta, mismatchExtra); err == nil {
		t.Fatal("publish accepted a length-mismatched extra")
	}
	failing := &Cache{backend: &stubBackend{dir: "fail-write", writeErr: errors.New("forced write")}}
	if err := failing.Publish(identity, reg, meta, payloads); err == nil {
		t.Fatal("publish ignored write failure")
	}
	if err := opened.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ReadMeta(identity, meta.Expectation); err != nil {
		t.Fatal(err)
	}
	openedReg, err := opened.OpenRegistry(identity, reg.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openedReg.ReadRange(RangeDescriptor{}); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("empty range = %v", err)
	}
	if _, err := openedReg.ReadRange(RangeDescriptor{Length: 1, SHA256: sha256.Sum256([]byte("range"))}); err != nil {
		t.Fatal(err)
	}
	if err := openedReg.ValidateAggregate(); err != nil {
		t.Fatal(err)
	}
	if err := openedReg.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.OpenPayloads(identity, payloads.Expectation); err != nil {
		t.Fatal(err)
	}
	if err := opened.WriteArtifact(identity, meta); err != nil {
		t.Fatal(err)
	}
	held, err := opened.AcquireLock(nil, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed ReadMeta = %v", err)
	}
	if _, err := opened.OpenRegistry(identity, reg.Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed OpenRegistry = %v", err)
	}
	if _, err := opened.OpenPayloads(identity, payloads.Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed OpenPayloads = %v", err)
	}
	if err := opened.WriteArtifact(identity, meta); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := opened.AcquireLock(context.Background(), 0); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoveragePortableFileOpen(t *testing.T) {
	UseMemoryOpenForTest(t)
	if _, err := Open("open", WithNoCreate()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("noCreate missing = %v", err)
	}
	cache, err := Open("open", WithCounters(&Counters{}))
	if err != nil {
		t.Fatal(err)
	}
	identity := portableIdentity()
	identity.EditionSHA256 = sha256.Sum256([]byte("open"))
	meta := portableArtifact(KindMeta, []byte("meta-bytes"))
	reg := portableArtifact(KindRegistry, []byte("registry-bytes"))
	payloads := portableArtifact(KindPayloads, []byte("payload-bytes"))
	if err := cache.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err != nil {
		t.Fatal(err)
	}
	wrong := identity
	wrong.SourceSHA256 = sha256.Sum256([]byte("wrong-source"))
	if _, err := cache.ReadMeta(wrong, meta.Expectation); err == nil {
		t.Fatal("source mismatch accepted")
	}
	wrongEdition := identity
	wrongEdition.EditionSHA256 = sha256.Sum256([]byte("other-edition"))
	if _, err := cache.ReadMeta(wrongEdition, meta.Expectation); err == nil {
		t.Fatal("edition mismatch accepted")
	}
	if err := cache.WriteArtifact(wrongEdition, meta); err == nil {
		t.Fatal("write edition mismatch accepted")
	}
	openedReg, err := cache.OpenRegistry(identity, reg.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	rangeDigest := sha256.Sum256([]byte("registry-bytes")[:1])
	if _, err := openedReg.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: rangeDigest}); err != nil {
		t.Fatal(err)
	}
	if _, err := openedReg.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: sha256.Sum256([]byte("nope"))}); err == nil {
		t.Fatal("range digest mismatch accepted")
	}
	if _, err := openedReg.ReadRange(RangeDescriptor{Offset: 100, Length: 1, SHA256: rangeDigest}); err == nil {
		t.Fatal("out of range accepted")
	}
	if err := openedReg.ValidateAggregate(); err != nil {
		t.Fatal(err)
	}
	openedPayloads, err := cache.OpenPayloads(identity, payloads.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest := sha256.Sum256([]byte("payload-bytes")[:1])
	if _, err := openedPayloads.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: payloadDigest}); err != nil {
		t.Fatal(err)
	}
	if err := openedPayloads.ValidateAggregate(); err != nil {
		t.Fatal(err)
	}
	if err := openedPayloads.Close(); err != nil {
		t.Fatal(err)
	}
	held, err := cache.AcquireLock(context.Background(), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.AcquireLock(context.Background(), 0); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("busy lock = %v", err)
	}
	if _, err := cache.AcquireLock(context.Background(), time.Millisecond); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("timer lock = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.AcquireLock(cancelled, time.Second); err == nil {
		t.Fatal("cancelled lock succeeded")
	}
	timed, timeoutCancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer timeoutCancel()
	if _, err := cache.AcquireLock(timed, 50*time.Millisecond); err == nil {
		t.Fatal("timeout lock succeeded")
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.AcquireLock(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	dir := cache.Directory()
	metaPath := filepath.Join(dir, metaFileName)
	metaBody, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	flipped := append([]byte{}, metaBody...)
	flipped[len(flipped)-1] ^= 0xff
	if err := os.WriteFile(metaPath, flipped, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("payload digest mismatch accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, metaFileName), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("short meta accepted")
	}
	junk := make([]byte, HeaderSize)
	if err := os.WriteFile(filepath.Join(dir, metaFileName), junk, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("junk header accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, registryFileName), junk, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.OpenRegistry(identity, reg.Expectation); err == nil {
		t.Fatal("junk registry header opened")
	}
	if err := os.WriteFile(filepath.Join(dir, registryFileName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	regHandle, err := cache.OpenRegistry(identity, reg.Expectation)
	if err == nil {
		t.Fatal("corrupt registry opened")
	}
	_ = regHandle
	if err := cache.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, metaFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("missing meta accepted")
	}
	if err := cache.Publish(identity, reg, meta, payloads); err != nil {
		t.Fatal(err)
	}
	openedReg, err = cache.OpenRegistry(identity, reg.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	regPath := filepath.Join(dir, registryFileName)
	regBody, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	regFlipped := append([]byte{}, regBody...)
	regFlipped[len(regFlipped)-1] ^= 0xff
	if err := os.WriteFile(regPath, regFlipped, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := openedReg.ValidateAggregate(); err == nil {
		t.Fatal("aggregate digest mismatch accepted")
	}
	if err := os.Remove(regPath); err != nil {
		t.Fatal(err)
	}
	if err := openedReg.ValidateAggregate(); err == nil {
		t.Fatal("missing shard aggregate accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, registryFileName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := openedReg.ValidateAggregate(); err == nil {
		t.Fatal("short aggregate accepted")
	}
	if _, err := openedReg.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: rangeDigest}); err == nil {
		t.Fatal("short range read accepted")
	}
	if err := openedReg.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := openedReg.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: rangeDigest}); err == nil {
		t.Fatal("closed range read succeeded")
	}
	if err := os.Remove(filepath.Join(dir, payloadFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.OpenPayloads(identity, payloads.Expectation); err == nil {
		t.Fatal("missing payloads opened")
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.OpenRegistry(identity, reg.Expectation); err == nil {
		t.Fatal("closed OpenRegistry succeeded")
	}
	if _, err := cache.ReadMeta(identity, meta.Expectation); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed portable ReadMeta = %v", err)
	}
	if _, err := cache.AcquireLock(context.Background(), time.Millisecond); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed lock = %v", err)
	}
}
