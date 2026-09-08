// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package schemareader composes authenticated cache I/O with the shared typed
// Schema decoder. It owns no declarations, process globals, repair or rendering
// policy; the CLI consumes these immutable identities.
package schemareader

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

const CatalogSnapshotVersion = 1

// ErrInvalidIdentity is the stable boundary classification for a linker-pinned
// identity that is present but incomplete or malformed.
var ErrInvalidIdentity = errors.New("invalid_schema_identity")

type Identity struct {
	Edition                string
	CatalogSnapshotVersion uint32
	SourceSHA256           [sha256.Size]byte
	SurfaceSHA256          [sha256.Size]byte
	BuildID                [sha256.Size]byte
	Meta                   schemacache.ArtifactExpectation
	Registry               schemacache.ArtifactExpectation
	Payload                schemacache.ArtifactExpectation
	// PayloadIndex pins the payload file's self-describing index region (its
	// 4-byte length prefix plus the SchemaPayloadIndex proto), so command
	// lookups can authenticate it without reading Meta.
	PayloadIndexLength uint64
	PayloadIndexSHA256 [sha256.Size]byte
}

// RawIdentity contains only linker-pinned values, never file or environment data.
type RawIdentity struct {
	Edition, SourceSHA256, SurfaceSHA256, BuildID                        string
	MetaLength, MetaSHA256, RegistryLength, RegistrySHA256               string
	PayloadLength, PayloadSHA256, PayloadIndexLength, PayloadIndexSHA256 string
}

// ParseOptionalIdentity distinguishes an intentionally disabled build (every
// linker field is empty) from a partially injected or malformed release. A
// broken sealed identity must never be treated as an ordinary cache miss.
func ParseOptionalIdentity(raw RawIdentity) (*Identity, error) {
	values := []string{
		raw.Edition, raw.SourceSHA256, raw.SurfaceSHA256, raw.BuildID,
		raw.MetaLength, raw.MetaSHA256, raw.RegistryLength, raw.RegistrySHA256,
		raw.PayloadLength, raw.PayloadSHA256, raw.PayloadIndexLength, raw.PayloadIndexSHA256,
	}
	present := 0
	for _, value := range values {
		if value != "" {
			present++
		}
	}
	if present == 0 {
		return nil, nil
	}
	if present != len(values) {
		return nil, fmt.Errorf("%w: partial Schema cache identity: got %d of %d fields", ErrInvalidIdentity, present, len(values))
	}
	identity, err := ParseIdentity(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidIdentity, err)
	}
	return &identity, nil
}

func ParseIdentity(raw RawIdentity) (Identity, error) {
	editionName := raw.Edition
	source, sourceOK := parseSchemaCacheLowerHex(raw.SourceSHA256)
	surface, surfaceOK := parseSchemaCacheLowerHex(raw.SurfaceSHA256)
	buildID, buildOK := parseSchemaCacheLowerHex(raw.BuildID)
	metaHash, metaHashOK := parseSchemaCacheLowerHex(raw.MetaSHA256)
	registryHash, registryHashOK := parseSchemaCacheLowerHex(raw.RegistrySHA256)
	payloadHash, payloadHashOK := parseSchemaCacheLowerHex(raw.PayloadSHA256)
	indexHash, indexHashOK := parseSchemaCacheLowerHex(raw.PayloadIndexSHA256)
	metaLength, metaLengthOK := parseSchemaCachePositiveDecimal(raw.MetaLength)
	registryLength, registryLengthOK := parseSchemaCachePositiveDecimal(raw.RegistryLength)
	payloadLength, payloadLengthOK := parseSchemaCachePositiveDecimal(raw.PayloadLength)
	indexLength, indexLengthOK := parseSchemaCachePositiveDecimal(raw.PayloadIndexLength)
	if editionName == "" || !sourceOK || !surfaceOK || !buildOK || !metaHashOK || !registryHashOK || !payloadHashOK || !indexHashOK || !metaLengthOK || !registryLengthOK || !payloadLengthOK || !indexLengthOK {
		return Identity{}, fmt.Errorf("incomplete Schema cache identity")
	}
	identity := Identity{
		Edition: editionName, CatalogSnapshotVersion: CatalogSnapshotVersion,
		SourceSHA256: source, SurfaceSHA256: surface, BuildID: buildID,
		Meta:               schemaCacheArtifactExpectation(schemacache.KindMeta, metaLength, metaHash),
		Registry:           schemaCacheArtifactExpectation(schemacache.KindRegistry, registryLength, registryHash),
		Payload:            schemaCacheArtifactExpectation(schemacache.KindPayloads, payloadLength, payloadHash),
		PayloadIndexLength: indexLength,
		PayloadIndexSHA256: indexHash,
	}
	return identity, identity.Validate()
}

func (i Identity) Validate() error {
	if i.CatalogSnapshotVersion != CatalogSnapshotVersion {
		return fmt.Errorf("unsupported Schema catalog snapshot version %d", i.CatalogSnapshotVersion)
	}
	editionDigest, err := schemacache.EditionSHA256(i.Edition)
	if err != nil {
		return err
	}
	identity := i.ExpectedIdentity()
	if editionDigest != identity.EditionSHA256 {
		return fmt.Errorf("Schema cache edition identity mismatch")
	}
	for _, expectation := range []schemacache.ArtifactExpectation{i.Meta, i.Registry, i.Payload} {
		envelope := schemacache.Envelope{
			Kind: expectation.Kind, Serializer: expectation.Serializer, Codec: expectation.Codec,
			FormatVersion: expectation.FormatVersion, CatalogSnapshotVersion: identity.CatalogSnapshotVersion,
			EncodedLength: expectation.EncodedLength, DecodedLength: expectation.DecodedLength,
			EditionSHA256: identity.EditionSHA256, SourceSHA256: identity.SourceSHA256,
			SurfaceSHA256: identity.SurfaceSHA256, BuildID: identity.BuildID, EncodedSHA256: expectation.EncodedSHA256,
		}
		if err := identity.Authenticate(envelope, expectation); err != nil {
			return fmt.Errorf("invalid Schema cache identity: %w", err)
		}
	}
	if i.Meta.Kind != schemacache.KindMeta || i.Registry.Kind != schemacache.KindRegistry || i.Payload.Kind != schemacache.KindPayloads {
		return fmt.Errorf("invalid Schema cache artifact kinds")
	}
	return nil
}
func (identity Identity) ExpectedIdentity() schemacache.ExpectedIdentity {
	edition, _ := schemacache.EditionSHA256(identity.Edition)
	return schemacache.ExpectedIdentity{
		CatalogSnapshotVersion: identity.CatalogSnapshotVersion,
		EditionSHA256:          edition, SourceSHA256: identity.SourceSHA256,
		SurfaceSHA256: identity.SurfaceSHA256, BuildID: identity.BuildID,
	}
}

func schemaCacheArtifactExpectation(kind schemacache.ArtifactKind, length uint64, digest [sha256.Size]byte) schemacache.ArtifactExpectation {
	return schemacache.ArtifactExpectation{
		Kind: kind, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
		FormatVersion: schemacache.DTOFormatVersion, EncodedLength: length, DecodedLength: length, EncodedSHA256: digest,
	}
}

func parseSchemaCacheLowerHex(raw string) ([sha256.Size]byte, bool) {
	var result [sha256.Size]byte
	if len(raw) != sha256.Size*2 {
		return result, false
	}
	for _, value := range raw {
		if !((value >= '0' && value <= '9') || (value >= 'a' && value <= 'f')) {
			return result, false
		}
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != len(result) {
		return result, false
	}
	copy(result[:], decoded)
	return result, true
}

func parseSchemaCachePositiveDecimal(raw string) (uint64, bool) {
	if raw == "" || raw[0] < '1' || raw[0] > '9' {
		return 0, false
	}
	for _, value := range raw[1:] {
		if value < '0' || value > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	return parsed, err == nil && parsed > 0
}
