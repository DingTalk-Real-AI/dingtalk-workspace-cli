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
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
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

	identity, err := schemareader.ParseIdentity(schemareader.RawIdentity{
		Edition: schemaCacheEdition, SourceSHA256: schemaCacheSourceSHA256,
		SurfaceSHA256: schemaCacheSurfaceSHA256, BuildID: schemaCacheBuildID,
		MetaLength: schemaCacheMetaLength, MetaSHA256: schemaCacheMetaSHA256,
		RegistryLength: schemaCacheRegistryLength, RegistrySHA256: schemaCacheRegistrySHA256,
	})
	if err != nil {
		return cli.SchemaCacheOptions{}, false
	}
	editionName := identity.Edition

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
