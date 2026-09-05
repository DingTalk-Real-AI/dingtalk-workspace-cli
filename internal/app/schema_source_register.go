// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var registerSchemaRuntimeDeliveryOnce sync.Once

// Release builds inject all fields together. Empty is the safe development
// default: malformed or partial values leave persistent Schema delivery off.
var (
	schemaCacheEdition        string
	schemaCacheSourceSHA256   string
	schemaCacheSurfaceSHA256  string
	schemaCacheBuildID        string
	schemaCacheMetaLength     string
	schemaCacheMetaSHA256     string
	schemaCacheRegistryLength string
	schemaCacheRegistrySHA256 string
	schemaCacheGOOS           = runtime.GOOS
	schemaCacheGOARCH         = runtime.GOARCH
)

const schemaCacheDisableEnv = "DWS_SCHEMA_CACHE_DISABLE"

// registerSchemaRuntimeDelivery installs the ResolveSchemaBuild root factory
// for production Catalog / ResolveMeta delivery. Called from NewRootCommand
// (and optionally cmd entrypoints). Intentionally NOT an init() side effect:
// importing app from package cli_test must not flip package-cli tests onto
// the assembly path.
func registerSchemaRuntimeDelivery() {
	registerSchemaRuntimeDeliveryOnce.Do(func() {
		cli.RegisterSchemaSourceRoot(func() *cobra.Command {
			return NewSchemaSourceRootCommand()
		})
		options, ok := productionSchemaCacheOptions()
		if !ok {
			_ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{})
			return
		}
		_ = cli.RegisterSchemaCacheOptions(options)
	})
}

func productionSchemaCacheOptions() (cli.SchemaCacheOptions, bool) {
	if !((schemaCacheGOOS == "darwin" && schemaCacheGOARCH == "arm64") || (schemaCacheGOOS == "linux" && schemaCacheGOARCH == "amd64")) {
		return cli.SchemaCacheOptions{}, false
	}
	editionName := schemaCacheEdition
	source, sourceOK := parseSchemaCacheLowerHex(schemaCacheSourceSHA256)
	surface, surfaceOK := parseSchemaCacheLowerHex(schemaCacheSurfaceSHA256)
	buildID, buildOK := parseSchemaCacheLowerHex(schemaCacheBuildID)
	metaHash, metaHashOK := parseSchemaCacheLowerHex(schemaCacheMetaSHA256)
	registryHash, registryHashOK := parseSchemaCacheLowerHex(schemaCacheRegistrySHA256)
	metaLength, metaLengthOK := parseSchemaCachePositiveDecimal(schemaCacheMetaLength)
	registryLength, registryLengthOK := parseSchemaCachePositiveDecimal(schemaCacheRegistryLength)
	if editionName == "" || !sourceOK || !surfaceOK || !buildOK || !metaHashOK || !registryHashOK || !metaLengthOK || !registryLengthOK {
		return cli.SchemaCacheOptions{}, false
	}
	identity := cli.SchemaCacheIdentity{
		Edition: editionName, CatalogSnapshotVersion: cli.SchemaCatalogSnapshotVersion,
		SourceSHA256: source, SurfaceSHA256: surface, BuildID: buildID,
		Meta:     schemaCacheArtifactExpectation(schemacache.KindMeta, metaLength, metaHash),
		Registry: schemaCacheArtifactExpectation(schemacache.KindRegistry, registryLength, registryHash),
	}
	return cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: schemaCacheGOOS, GOARCH: schemaCacheGOARCH,
		RuntimeEligible: func() bool {
			if strings.TrimSpace(os.Getenv(schemaCacheDisableEnv)) != "" {
				return false
			}
			hooks := edition.Get()
			return hooks != nil && hooks.Name == editionName && hooks.RegisterExtraCommands == nil
		},
	}, true
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
