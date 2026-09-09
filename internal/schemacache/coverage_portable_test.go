package schemacache

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

// stubBackend exercises Cache/Registry/Lock facades on platforms where the
// persistent unix backend is not compiled, including Windows coverage gate.
type stubBackend struct {
	closed bool
	dir    string
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

func (s *stubBackend) writeArtifact(ExpectedIdentity, Artifact) error { return s.guard() }

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
