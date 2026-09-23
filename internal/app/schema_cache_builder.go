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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const (
	schemaCacheBuilderArgument = "--_dws-schema-builder=1"
)

var maxSchemaCacheBuilderStderr = 64 << 10

var (
	schemaCacheBuilderTimeout       = cli.DefaultSchemaCacheBuilderTimeout
	schemaCacheBuilderCommand       = exec.CommandContext
	schemaCacheBuilderExecutable    = os.Executable
	schemaCacheBuilderEnvironment   = cli.SchemaAssemblyEnvironmentSnapshot
	schemaCacheBuilderWorkingDir    = cli.SchemaAssemblyWorkingDirectory
	// Keep in sync with cli.maxSchemaCacheBuilderResponse (see its comment).
	schemaCacheBuilderResponseLimit = 192 << 20
	schemaCacheBuilderAssemble      = buildSchemaCacheResult
	schemaCacheResolve              = cli.ResolveSchemaBuild
	schemaCacheBuildArtifacts       = cli.BuildSchemaCacheArtifacts
	schemaCacheIdentity             = cli.IdentityFromArtifacts
	schemaCacheReadResult           = cli.ReadSchemaCacheBuildResult
)

// RunSchemaCacheBuilder handles only the private declaration-builder process.
// It must run before normal root construction so plugin discovery is impossible.
func RunSchemaCacheBuilder(args []string, output io.Writer) (bool, int) {
	if len(args) != 1 || args[0] != schemaCacheBuilderArgument {
		return false, 0
	}
	result, err := schemaCacheBuilderAssemble(context.Background())
	if err != nil {
		return true, writeSchemaCacheBuilderError(err)
	}
	if err := cli.WriteSchemaCacheBuildResult(output, result); err != nil {
		return true, 1
	}
	return true, 0
}

func buildSchemaCacheInProcessForTest(ctx context.Context) (cli.SchemaCacheBuildResult, error) {
	return buildSchemaCacheResult(ctx)
}

func buildSchemaCacheResult(ctx context.Context) (cli.SchemaCacheBuildResult, error) {
	resolved, err := schemaCacheResolve(NewSchemaSourceRootCommand(ctx))
	if err != nil {
		return cli.SchemaCacheBuildResult{}, err
	}
	artifacts, err := schemaCacheBuildArtifacts(resolved)
	if err != nil {
		return cli.SchemaCacheBuildResult{}, err
	}
	editionName := "open"
	if hooks := edition.Get(); hooks != nil && strings.TrimSpace(hooks.Name) != "" {
		editionName = hooks.Name
	}
	identity, err := schemaCacheIdentity(editionName, artifacts)
	if err != nil {
		return cli.SchemaCacheBuildResult{}, err
	}
	return cli.SchemaCacheBuildResult{Artifacts: artifacts, Identity: identity}, nil
}

func writeSchemaCacheBuilderError(err error) int {
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

func validateSchemaCacheBuilderProcessOutput(stdout, stderr *cappedBuffer, runErr error) error {
	if stdout.truncated {
		return fmt.Errorf("Schema cache builder response exceeded output limit")
	}
	if runErr == nil {
		return nil
	}
	message := strings.TrimSpace(stderr.String())
	if stderr.truncated {
		message += " (stderr truncated)"
	}
	if message != "" {
		return fmt.Errorf("Schema cache builder: %w: %s", runErr, message)
	}
	return fmt.Errorf("Schema cache builder: %w", runErr)
}

func buildSchemaCacheInChild(ctx context.Context) (cli.SchemaCacheBuildResult, error) {
	executable, err := schemaCacheBuilderExecutable()
	if err != nil {
		return cli.SchemaCacheBuildResult{}, fmt.Errorf("resolve CLI executable: %w", err)
	}
	// Ensure child execution is bounded by the builder timeout even if the
	// caller passes a context without an explicit deadline.
	ctx, cancel := context.WithTimeout(ctx, schemaCacheBuilderTimeout)
	defer cancel()
	cmd := schemaCacheBuilderCommand(ctx, executable, schemaCacheBuilderArgument)
	cmd.Env = schemaCacheBuilderEnvironment()
	if workingDir := schemaCacheBuilderWorkingDir(); workingDir != "" {
		cmd.Dir = workingDir
	}
	if len(cmd.Env) == 0 {
		return cli.SchemaCacheBuildResult{}, fmt.Errorf("Schema assembly environment snapshot is empty")
	}
	var stdout cappedBuffer
	stdout.limit = schemaCacheBuilderResponseLimit
	var stderr cappedBuffer
	stderr.limit = maxSchemaCacheBuilderStderr
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if err := validateSchemaCacheBuilderProcessOutput(&stdout, &stderr, runErr); err != nil {
		return cli.SchemaCacheBuildResult{}, err
	}
	result, err := schemaCacheReadResult(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		return cli.SchemaCacheBuildResult{}, err
	}
	return result, nil
}
