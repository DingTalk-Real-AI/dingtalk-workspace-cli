package schemacache

import (
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	HeaderSize             = 208
	EnvelopeVersion        = uint16(1)
	DTOFormatVersion       = uint32(1)
	SerializerProtobuf     = uint8(2)
	CodecRaw               = uint8(0)
	MaxMetaFileSize        = uint64(4 << 20)
	MaxRegistryFileSize    = uint64(64 << 20)
	MaxProductRangeSize    = uint64(8 << 20)
	MaxRegistryPayloadSize = MaxRegistryFileSize - HeaderSize
)

var envelopeMagic = [8]byte{'D', 'W', 'S', 'S', 'C', 'H', 'C', '1'}

type ArtifactKind uint8

const (
	KindMeta     ArtifactKind = 1
	KindRegistry ArtifactKind = 2
)

// Envelope is the fixed, allocation-free portion of a cache artifact.
type Envelope struct {
	Kind                   ArtifactKind
	Serializer             uint8
	Codec                  uint8
	Flags                  uint8
	FormatVersion          uint32
	CatalogSnapshotVersion uint32
	EncodedLength          uint64
	DecodedLength          uint64
	EditionSHA256          [32]byte
	SourceSHA256           [32]byte
	SurfaceSHA256          [32]byte
	BuildID                [32]byte
	EncodedSHA256          [32]byte
}

// ExpectedIdentity contains only values pinned by the running binary. Header
// values are compared with this identity and are never accepted on their own.
type ExpectedIdentity struct {
	CatalogSnapshotVersion uint32
	EditionSHA256          [32]byte
	SourceSHA256           [32]byte
	SurfaceSHA256          [32]byte
	BuildID                [32]byte
}

// ArtifactExpectation is the binary-pinned identity of one exact artifact.
type ArtifactExpectation struct {
	Kind          ArtifactKind
	Serializer    uint8
	Codec         uint8
	FormatVersion uint32
	EncodedLength uint64
	DecodedLength uint64
	EncodedSHA256 [32]byte
}

// RangeDescriptor must come from an already authenticated Meta payload.
// Offset is relative to the Registry payload, after its fixed header.
type RangeDescriptor struct {
	Offset uint64
	Length uint64
	SHA256 [32]byte
}

func (e Envelope) MarshalBinary() ([]byte, error) {
	if err := e.validateShape(); err != nil {
		return nil, err
	}
	b := make([]byte, HeaderSize)
	copy(b[0:8], envelopeMagic[:])
	binary.BigEndian.PutUint16(b[8:10], EnvelopeVersion)
	binary.BigEndian.PutUint16(b[10:12], HeaderSize)
	b[12] = byte(e.Kind)
	b[13] = e.Serializer
	b[14] = e.Codec
	b[15] = e.Flags
	binary.BigEndian.PutUint32(b[16:20], e.FormatVersion)
	binary.BigEndian.PutUint32(b[20:24], e.CatalogSnapshotVersion)
	binary.BigEndian.PutUint64(b[24:32], e.EncodedLength)
	binary.BigEndian.PutUint64(b[32:40], e.DecodedLength)
	copy(b[40:72], e.EditionSHA256[:])
	copy(b[72:104], e.SourceSHA256[:])
	copy(b[104:136], e.SurfaceSHA256[:])
	copy(b[136:168], e.BuildID[:])
	copy(b[168:200], e.EncodedSHA256[:])
	return b, nil
}

func MarshalEnvelope(e Envelope) ([]byte, error) { return e.MarshalBinary() }

func ParseEnvelope(b []byte) (Envelope, error) {
	var e Envelope
	if len(b) != HeaderSize {
		return e, fmt.Errorf("%w: header is %d bytes, want %d", ErrInvalidArtifact, len(b), HeaderSize)
	}
	if subtle.ConstantTimeCompare(b[0:8], envelopeMagic[:]) != 1 {
		return e, fmt.Errorf("%w: bad magic", ErrInvalidArtifact)
	}
	if binary.BigEndian.Uint16(b[8:10]) != EnvelopeVersion {
		return e, fmt.Errorf("%w: unsupported envelope version", ErrInvalidArtifact)
	}
	if binary.BigEndian.Uint16(b[10:12]) != HeaderSize {
		return e, fmt.Errorf("%w: bad header size", ErrInvalidArtifact)
	}
	e.Kind = ArtifactKind(b[12])
	e.Serializer = b[13]
	e.Codec = b[14]
	e.Flags = b[15]
	e.FormatVersion = binary.BigEndian.Uint32(b[16:20])
	e.CatalogSnapshotVersion = binary.BigEndian.Uint32(b[20:24])
	e.EncodedLength = binary.BigEndian.Uint64(b[24:32])
	e.DecodedLength = binary.BigEndian.Uint64(b[32:40])
	copy(e.EditionSHA256[:], b[40:72])
	copy(e.SourceSHA256[:], b[72:104])
	copy(e.SurfaceSHA256[:], b[104:136])
	copy(e.BuildID[:], b[136:168])
	copy(e.EncodedSHA256[:], b[168:200])
	for _, v := range b[200:208] {
		if v != 0 {
			return Envelope{}, fmt.Errorf("%w: reserved bytes are non-zero", ErrInvalidArtifact)
		}
	}
	if err := e.validateShape(); err != nil {
		return Envelope{}, err
	}
	return e, nil
}

func (e Envelope) validateShape() error {
	if e.Kind != KindMeta && e.Kind != KindRegistry {
		return fmt.Errorf("%w: unknown artifact kind %d", ErrInvalidArtifact, e.Kind)
	}
	if e.Serializer != SerializerProtobuf {
		return fmt.Errorf("%w: unsupported serializer %d", ErrInvalidArtifact, e.Serializer)
	}
	if e.Codec != CodecRaw {
		return fmt.Errorf("%w: unsupported codec %d", ErrInvalidArtifact, e.Codec)
	}
	if e.Flags != 0 {
		return fmt.Errorf("%w: unsupported flags %#x", ErrInvalidArtifact, e.Flags)
	}
	if e.FormatVersion != DTOFormatVersion {
		return fmt.Errorf("%w: unsupported DTO version %d", ErrInvalidArtifact, e.FormatVersion)
	}
	if e.CatalogSnapshotVersion == 0 {
		return fmt.Errorf("%w: zero catalog snapshot version", ErrInvalidArtifact)
	}
	if e.EncodedLength != e.DecodedLength {
		return fmt.Errorf("%w: raw encoded and decoded lengths differ", ErrInvalidArtifact)
	}
	if e.EncodedLength > math.MaxUint64-HeaderSize {
		return fmt.Errorf("%w: artifact length overflows", ErrInvalidArtifact)
	}
	if e.EncodedLength == 0 {
		return fmt.Errorf("%w: empty payload", ErrInvalidArtifact)
	}
	max := MaxMetaFileSize
	if e.Kind == KindRegistry {
		max = MaxRegistryFileSize
		if e.EncodedLength > MaxRegistryPayloadSize {
			return fmt.Errorf("%w: registry payload exceeds limit", ErrInvalidArtifact)
		}
	}
	if HeaderSize+e.EncodedLength > max {
		return fmt.Errorf("%w: artifact exceeds limit", ErrInvalidArtifact)
	}
	return nil
}

func (i ExpectedIdentity) validate() error {
	if i.CatalogSnapshotVersion == 0 {
		return fmt.Errorf("%w: zero catalog snapshot version", ErrIdentityMismatch)
	}
	if isZeroDigest(i.EditionSHA256) || isZeroDigest(i.SourceSHA256) ||
		isZeroDigest(i.SurfaceSHA256) || isZeroDigest(i.BuildID) {
		return fmt.Errorf("%w: incomplete trusted identity", ErrIdentityMismatch)
	}
	return nil
}

func (a ArtifactExpectation) validate(kind ArtifactKind) error {
	if a.Kind != kind {
		return fmt.Errorf("%w: expected kind %d, want %d", ErrIdentityMismatch, a.Kind, kind)
	}
	e := Envelope{
		Kind:                   a.Kind,
		Serializer:             a.Serializer,
		Codec:                  a.Codec,
		FormatVersion:          a.FormatVersion,
		CatalogSnapshotVersion: 1,
		EncodedLength:          a.EncodedLength,
		DecodedLength:          a.DecodedLength,
	}
	if err := e.validateShape(); err != nil {
		return fmt.Errorf("%w: invalid trusted artifact expectation: %v", ErrIdentityMismatch, err)
	}
	if isZeroDigest(a.EncodedSHA256) {
		return fmt.Errorf("%w: zero trusted artifact digest", ErrIdentityMismatch)
	}
	return nil
}

func (e Envelope) authenticate(i ExpectedIdentity, a ArtifactExpectation) error {
	if err := i.validate(); err != nil {
		return err
	}
	if err := a.validate(e.Kind); err != nil {
		return err
	}
	if e.CatalogSnapshotVersion != i.CatalogSnapshotVersion ||
		e.Serializer != a.Serializer || e.Codec != a.Codec ||
		e.FormatVersion != a.FormatVersion || e.EncodedLength != a.EncodedLength ||
		e.DecodedLength != a.DecodedLength ||
		!digestEqual(e.EditionSHA256, i.EditionSHA256) ||
		!digestEqual(e.SourceSHA256, i.SourceSHA256) ||
		!digestEqual(e.SurfaceSHA256, i.SurfaceSHA256) ||
		!digestEqual(e.BuildID, i.BuildID) ||
		!digestEqual(e.EncodedSHA256, a.EncodedSHA256) {
		return ErrIdentityMismatch
	}
	return nil
}

// Authenticate compares every envelope identity field with binary-pinned
// trusted values. It does not authenticate payload bytes by itself.
func (i ExpectedIdentity) Authenticate(e Envelope, a ArtifactExpectation) error {
	if err := e.validateShape(); err != nil {
		return err
	}
	return e.authenticate(i, a)
}

func envelopeFrom(i ExpectedIdentity, a ArtifactExpectation) (Envelope, error) {
	if err := i.validate(); err != nil {
		return Envelope{}, err
	}
	if err := a.validate(a.Kind); err != nil {
		return Envelope{}, err
	}
	e := Envelope{
		Kind:                   a.Kind,
		Serializer:             a.Serializer,
		Codec:                  a.Codec,
		FormatVersion:          a.FormatVersion,
		CatalogSnapshotVersion: i.CatalogSnapshotVersion,
		EncodedLength:          a.EncodedLength,
		DecodedLength:          a.DecodedLength,
		EditionSHA256:          i.EditionSHA256,
		SourceSHA256:           i.SourceSHA256,
		SurfaceSHA256:          i.SurfaceSHA256,
		BuildID:                i.BuildID,
		EncodedSHA256:          a.EncodedSHA256,
	}
	return e, e.validateShape()
}

func checkedFileSize(payload uint64) (int64, error) {
	if payload > math.MaxInt64-HeaderSize {
		return 0, fmt.Errorf("%w: file size overflows int64", ErrInvalidArtifact)
	}
	return int64(HeaderSize + payload), nil
}

func digestEqual(a, b [32]byte) bool {
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func isZeroDigest(v [32]byte) bool {
	var zero [32]byte
	return subtle.ConstantTimeCompare(v[:], zero[:]) == 1
}
