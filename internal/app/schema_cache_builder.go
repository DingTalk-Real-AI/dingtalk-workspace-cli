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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const (
	schemaCacheBuilderArgument  = "--_dws-schema-builder=1"
	schemaCacheBuilderTimeout   = 30 * time.Second
	maxSchemaCacheBuilderStderr = 64 << 10
)

// RunSchemaCacheBuilder handles only the private declaration-builder process.
// It must run before normal root construction so plugin discovery is impossible.
func RunSchemaCacheBuilder(args []string, output io.Writer) (bool, int) {
	_ = output
	if len(args) != 1 || args[0] != schemaCacheBuilderArgument {
		return false, 0
	}
	resolved, err := cli.ResolveSchemaBuild(NewSchemaSourceRootCommand(context.Background()))
	if err != nil {
		return true, writeSchemaCacheBuilderError(output, err)
	}
	artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
	if err != nil {
		return true, writeSchemaCacheBuilderError(output, err)
	}
	editionName := "open"
	if hooks := edition.Get(); hooks != nil && strings.TrimSpace(hooks.Name) != "" {
		editionName = hooks.Name
	}
	identity, err := cli.IdentityFromArtifacts(editionName, artifacts)
	if err != nil {
		return true, writeSchemaCacheBuilderError(output, err)
	}
	if err := cli.WriteSchemaCacheBuildResult(output, cli.SchemaCacheBuildResult{
		Artifacts: artifacts,
		Identity:  identity,
	}); err != nil {
		return true, 1
	}
	return true, 0
}

func writeSchemaCacheBuilderError(output io.Writer, err error) int {
	_, _ = fmt.Fprintf(os.Stderr, "Schema cache builder: %v\n", err)
	return 1
}

type cappedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if remaining > len(data) {
			remaining = len(data)
		}
		_, _ = b.Buffer.Write(data[:remaining])
	}
	if len(data) > remaining {
		b.truncated = true
	}
	return len(data), nil
}

func buildSchemaCacheInChild(ctx context.Context) (cli.SchemaCacheBuildResult, error) {
	executable, err := os.Executable()
	if err != nil {
		return cli.SchemaCacheBuildResult{}, fmt.Errorf("resolve CLI executable: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, schemaCacheBuilderTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, schemaCacheBuilderArgument)
	cmd.Env = cli.SchemaAssemblyEnvironmentSnapshot()
	if workingDir := cli.SchemaAssemblyWorkingDirectory(); workingDir != "" {
		cmd.Dir = workingDir
	}
	if len(cmd.Env) == 0 {
		return cli.SchemaCacheBuildResult{}, fmt.Errorf("Schema assembly environment snapshot is empty")
	}
	var stdout cappedBuffer
	stdout.limit = 64 << 20
	var stderr cappedBuffer
	stderr.limit = maxSchemaCacheBuilderStderr
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if stdout.truncated {
		return cli.SchemaCacheBuildResult{}, fmt.Errorf("Schema cache builder response exceeded output limit")
	}
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if stderr.truncated {
			message += " (stderr truncated)"
		}
		if message != "" {
			return cli.SchemaCacheBuildResult{}, fmt.Errorf("Schema cache builder: %w: %s", runErr, message)
		}
		return cli.SchemaCacheBuildResult{}, fmt.Errorf("Schema cache builder: %w", runErr)
	}
	result, err := cli.ReadSchemaCacheBuildResult(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		return cli.SchemaCacheBuildResult{}, err
	}
	return result, nil
}
