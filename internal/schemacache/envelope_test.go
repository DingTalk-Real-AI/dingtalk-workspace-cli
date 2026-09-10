package schemacache

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

func testEnvelope() Envelope {
	return Envelope{
		Kind:                   KindMeta,
		Serializer:             SerializerProtobuf,
		Codec:                  CodecRaw,
		FormatVersion:          DTOFormatVersion,
		CatalogSnapshotVersion: 7,
		EncodedLength:          3,
		DecodedLength:          3,
		EditionSHA256:          sha256.Sum256([]byte("edition")),
		SourceSHA256:           sha256.Sum256([]byte("source")),
		SurfaceSHA256:          sha256.Sum256([]byte("surface")),
		BuildID:                sha256.Sum256([]byte("build")),
		EncodedSHA256:          sha256.Sum256([]byte("abc")),
	}
}

func TestCrossPlatformCoverageEnvelopeExactRoundTrip(t *testing.T) {
	want := testEnvelope()
	b, err := want.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != HeaderSize {
		t.Fatalf("header length = %d, want %d", len(b), HeaderSize)
	}
	got, err := ParseEnvelope(b)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", got, want)
	}
	for i, value := range b[200:208] {
		if value != 0 {
			t.Fatalf("reserved byte %d is non-zero", i)
		}
	}
}

func TestCrossPlatformCoverageEnvelopeRejectsEveryFixedFieldViolation(t *testing.T) {
	valid, err := testEnvelope().MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func([]byte){
		"magic":               func(b []byte) { b[0] ^= 1 },
		"version":             func(b []byte) { binary.BigEndian.PutUint16(b[8:10], 2) },
		"header size":         func(b []byte) { binary.BigEndian.PutUint16(b[10:12], 207) },
		"kind":                func(b []byte) { b[12] = 4 },
		"serializer":          func(b []byte) { b[13] = 1 },
		"codec":               func(b []byte) { b[14] = 1 },
		"flags":               func(b []byte) { b[15] = 1 },
		"DTO version":         func(b []byte) { binary.BigEndian.PutUint32(b[16:20], DTOFormatVersion+1) },
		"retired DTO version": func(b []byte) { binary.BigEndian.PutUint32(b[16:20], 1) },
		"public version":      func(b []byte) { binary.BigEndian.PutUint32(b[20:24], 0) },
		"raw lengths":         func(b []byte) { binary.BigEndian.PutUint64(b[32:40], 2) },
		"reserved":            func(b []byte) { b[207] = 1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			b := append([]byte(nil), valid...)
			mutate(b)
			if _, err := ParseEnvelope(b); !errors.Is(err, ErrInvalidArtifact) {
				t.Fatalf("ParseEnvelope error = %v, want ErrInvalidArtifact", err)
			}
		})
	}
	for _, size := range []int{0, HeaderSize - 1, HeaderSize + 1} {
		if _, err := ParseEnvelope(make([]byte, size)); !errors.Is(err, ErrInvalidArtifact) {
			t.Fatalf("size %d error = %v", size, err)
		}
	}
}

func TestCrossPlatformCoverageEnvelopeBoundsAndOverflow(t *testing.T) {
	tests := []struct {
		name   string
		kind   ArtifactKind
		length uint64
	}{
		{"meta over limit", KindMeta, MaxMetaFileSize - HeaderSize + 1},
		{"registry over limit", KindRegistry, MaxRegistryPayloadSize + 1},
		{"integer overflow", KindRegistry, math.MaxUint64},
		{"empty", KindMeta, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := testEnvelope()
			e.Kind = test.kind
			e.EncodedLength = test.length
			e.DecodedLength = test.length
			if _, err := e.MarshalBinary(); !errors.Is(err, ErrInvalidArtifact) {
				t.Fatalf("MarshalBinary error = %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageExpectedIdentityIsTrustAnchor(t *testing.T) {
	e := testEnvelope()
	expected := ArtifactExpectation{
		Kind: KindMeta, Serializer: SerializerProtobuf, Codec: CodecRaw,
		FormatVersion: DTOFormatVersion, EncodedLength: 3, DecodedLength: 3,
		EncodedSHA256: e.EncodedSHA256,
	}
	identity := ExpectedIdentity{
		CatalogSnapshotVersion: e.CatalogSnapshotVersion,
		EditionSHA256:          e.EditionSHA256, SourceSHA256: e.SourceSHA256,
		SurfaceSHA256: e.SurfaceSHA256, BuildID: e.BuildID,
	}
	if err := identity.Authenticate(e, expected); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Envelope){
		"edition": func(e *Envelope) { e.EditionSHA256[0] ^= 1 },
		"source":  func(e *Envelope) { e.SourceSHA256[0] ^= 1 },
		"surface": func(e *Envelope) { e.SurfaceSHA256[0] ^= 1 },
		"build":   func(e *Envelope) { e.BuildID[0] ^= 1 },
		"payload": func(e *Envelope) { e.EncodedSHA256[0] ^= 1 },
	} {
		t.Run(name, func(t *testing.T) {
			changed := e
			mutate(&changed)
			if err := identity.Authenticate(changed, expected); !errors.Is(err, ErrIdentityMismatch) {
				t.Fatalf("Authenticate error = %v", err)
			}
		})
	}
}

func FuzzParseEnvelope(f *testing.F) {
	valid, err := testEnvelope().MarshalBinary()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte("DWSSCHC1"))
	f.Fuzz(func(t *testing.T, input []byte) {
		e, err := ParseEnvelope(input)
		if err != nil {
			return
		}
		roundTrip, err := e.MarshalBinary()
		if err != nil {
			t.Fatalf("accepted envelope cannot marshal: %v", err)
		}
		if string(roundTrip) != string(input) {
			t.Fatal("accepted envelope did not round trip exactly")
		}
	})
}

func TestCrossPlatformCoverageMarshalEnvelopeMatchesBinary(t *testing.T) {
	want := testEnvelope()
	viaMethod, err := want.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	viaPackage, err := MarshalEnvelope(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(viaMethod) != string(viaPackage) {
		t.Fatal("MarshalEnvelope diverged from MarshalBinary")
	}
}

func TestCrossPlatformCoverageEnvelopeFromAndCheckedFileSize(t *testing.T) {
	e := testEnvelope()
	identity := ExpectedIdentity{
		CatalogSnapshotVersion: e.CatalogSnapshotVersion,
		EditionSHA256:          e.EditionSHA256, SourceSHA256: e.SourceSHA256,
		SurfaceSHA256: e.SurfaceSHA256, BuildID: e.BuildID,
	}
	expected := ArtifactExpectation{
		Kind: KindMeta, Serializer: SerializerProtobuf, Codec: CodecRaw,
		FormatVersion: DTOFormatVersion, EncodedLength: e.EncodedLength, DecodedLength: e.DecodedLength,
		EncodedSHA256: e.EncodedSHA256,
	}
	got, err := envelopeFrom(identity, expected)
	if err != nil {
		t.Fatal(err)
	}
	if got != e {
		t.Fatalf("envelopeFrom = %#v, want %#v", got, e)
	}
	size, err := checkedFileSize(e.EncodedLength)
	if err != nil || size != int64(HeaderSize)+int64(e.EncodedLength) {
		t.Fatalf("checkedFileSize = %d, %v", size, err)
	}
	if _, err := checkedFileSize(math.MaxUint64); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("overflow error = %v, want ErrInvalidArtifact", err)
	}
	zeroIdentity := identity
	zeroIdentity.BuildID = [32]byte{}
	if _, err := envelopeFrom(zeroIdentity, expected); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("incomplete identity error = %v, want ErrIdentityMismatch", err)
	}
	if _, err := envelopeFrom(identity, ArtifactExpectation{Kind: KindMeta, Serializer: 99}); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("invalid artifact error = %v, want ErrIdentityMismatch", err)
	}
}

func TestCrossPlatformCoveragePersistentBackendEnabled(t *testing.T) {
	for _, tt := range []struct {
		goos, goarch string
		want         bool
	}{
		{"darwin", "arm64", true},
		{"darwin", "amd64", true},
		{"linux", "amd64", true},
		{"linux", "arm64", true},
		{"windows", "amd64", true},
		{"windows", "arm64", true},
		{"windows", "386", false},
		{"linux", "386", false},
	} {
		if got := PersistentBackendEnabled(tt.goos, tt.goarch); got != tt.want {
			t.Fatalf("PersistentBackendEnabled(%s, %s) = %v, want %v", tt.goos, tt.goarch, got, tt.want)
		}
	}
}
