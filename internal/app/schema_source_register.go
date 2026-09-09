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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var registerSchemaRuntimeDeliveryOnce sync.Once

const (
	schemaCacheDisableEnv = "DWS_SCHEMA_CACHE_DISABLE"
	schemaCacheTestEnv    = "DWS_SCHEMA_CACHE_TEST"
)

var (
	schemaCacheGOOS   = runtime.GOOS
	schemaCacheGOARCH = runtime.GOARCH
)

// registerSchemaRuntimeDelivery installs the ResolveSchemaBuild root factory
// for production Catalog / ResolveMeta delivery. Called from NewRootCommand
// (and optionally cmd entrypoints). Intentionally NOT an init() side effect:
// importing app from package cli_test must not flip package-cli tests onto
// the assembly path.
//
// Schema identity is not produced at compile or release time. On supported
// platforms the persistent cache is enabled: a previously generated local
// identity is loaded when present, otherwise the first schema-consuming path
// generates identity from live declarations, publishes protobuf shards, and
// writes the sidecar for later processes.
func registerSchemaRuntimeDelivery() {
	registerSchemaRuntimeDeliveryOnce.Do(func() {
		cli.RegisterSchemaSourceRoot(func() *cobra.Command {
			return NewSchemaSourceRootCommand()
		})
	})
	applyProductionSchemaCache()
}

func applyProductionSchemaCache() {
	options, ok := productionSchemaCacheOptions()
	if !ok {
		_ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{})
		return
	}
	_ = cli.RegisterSchemaCacheOptions(options)
	if _, ready := cli.SchemaCacheFastPathIdentity(); ready {
		cli.PrewarmSchemaCache()
	}
}

func productionSchemaCacheOptions() (cli.SchemaCacheOptions, bool) {
	if testing.Testing() && strings.TrimSpace(os.Getenv(schemaCacheTestEnv)) == "" {
		return cli.SchemaCacheOptions{}, false
	}
	if !((schemaCacheGOOS == "darwin" && schemaCacheGOARCH == "arm64") || (schemaCacheGOOS == "linux" && schemaCacheGOARCH == "amd64")) {
		return cli.SchemaCacheOptions{}, false
	}
	hooks := edition.Get()
	editionName := "open"
	if hooks != nil && strings.TrimSpace(hooks.Name) != "" {
		editionName = hooks.Name
	}
	identity, _ := cli.TryLoadLocalSchemaCacheIdentity(editionName)
	return cli.SchemaCacheOptions{
		Enabled:       true,
		AllowGenerate: true,
		Edition:       editionName,
		Identity:      identity,
		GOOS:          schemaCacheGOOS,
		GOARCH:        schemaCacheGOARCH,
		RuntimeEligible: func() bool {
			if strings.TrimSpace(os.Getenv(schemaCacheDisableEnv)) != "" {
				return false
			}
			current := edition.Get()
			return current != nil && current.Name == editionName && current.RegisterExtraCommands == nil
		},
	}, true
}
