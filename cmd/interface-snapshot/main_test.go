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

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRunGenerateCapturesActualRootOffline(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"generate"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run(generate) exit=%d stderr=%s", exitCode, stderr.String())
	}
	snapshot, err := interfacesnapshot.Read(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		t.Fatalf("decode generated snapshot: %v", err)
	}

	commands := make(map[string]interfacesnapshot.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.Path] = command
	}
	for _, path := range []string{"dws", "dws chat", "dws dev app create"} {
		if _, ok := commands[path]; !ok {
			t.Errorf("actual root snapshot is missing %q", path)
		}
	}
	for _, path := range []string{"dws completion", "dws help"} {
		if _, ok := commands[path]; ok {
			t.Errorf("framework-noise path %q leaked into snapshot", path)
		}
	}

	create := commands["dws dev app create"]
	if !hasFlag(create.LocalFlags, "name", "string") {
		t.Errorf("dev app create local flags do not contain --name string: %#v", create.LocalFlags)
	}
	if !hasFlag(create.InheritedFlags, "profile", "string") {
		t.Errorf("dev app create inherited flags do not contain --profile string: %#v", create.InheritedFlags)
	}
}

func TestCrossPlatformCoverageRunGenerateRejectsHelpRenderingFailure(t *testing.T) {
	testseam.Swap(t, &newRootCommand, func() *cobra.Command {
		root := &cobra.Command{Use: "dws"}
		root.SetHelpFunc(func(command *cobra.Command, _ []string) {
			_, _ = io.WriteString(command.ErrOrStderr(), "injected help failure")
		})
		return root
	})

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"generate"}, &stdout, &stderr); exitCode != 2 {
		t.Fatalf("run(generate) exit=%d stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "injected help failure") {
		t.Fatalf("run(generate) stderr=%q", stderr.String())
	}
}

func TestCrossPlatformCoverageRunCompareUsesBothSnapshotInputsAndExitCode(t *testing.T) {
	current := commandSnapshot("dws")
	mergeBase := commandSnapshot("dws")
	stable := commandSnapshot("dws", "dws legacy")

	dir := t.TempDir()
	currentPath := writeSnapshot(t, dir, "current.json", current)
	mergeBasePath := writeSnapshot(t, dir, "base.json", mergeBase)
	stablePath := writeSnapshot(t, dir, "stable.json", stable)

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"compare",
		"--current", currentPath,
		"--base", mergeBasePath,
		"--stable", stablePath,
	}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("run(compare) exit=%d, want 1; stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"reference": "main"`)) ||
		!bytes.Contains(stdout.Bytes(), []byte(`"reference": "stable"`)) ||
		!bytes.Contains(stdout.Bytes(), []byte(`"kind": "command_removed"`)) {
		t.Fatalf("comparison report does not contain both references and the blocking change:\n%s", stdout.String())
	}
}

func TestCrossPlatformCoverageRunCompareEnforcesBaseOwnedFlagMigrationLifecycle(t *testing.T) {
	dir := t.TempDir()
	before := flagMigrationSnapshot(false)
	after := flagMigrationSnapshot(true)
	currentPath := writeSnapshot(t, dir, "current.json", after)
	basePath := writeSnapshot(t, dir, "base.json", before)
	stablePath := writeSnapshot(t, dir, "stable.json", before)
	approvedPath := writeManifest(t, dir, "approved.json", flagMigrationManifestJSON("pending"))
	candidatePath := writeManifest(t, dir, "candidate.json", flagMigrationManifestJSON("consumed"))

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"compare",
		"--current", currentPath,
		"--base", basePath,
		"--stable", stablePath,
		"--approved-flag-migrations", approvedPath,
		"--candidate-flag-migrations", candidatePath,
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exact base-owned migration exit=%d stderr=%s", exitCode, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"compatible": true`)) {
		t.Fatalf("exact migration report is not compatible:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	emptyApproved := writeManifest(t, dir, "empty-approved.json", `{"version":1,"migrations":[]}`)
	exitCode = run([]string{
		"compare",
		"--current", currentPath,
		"--base", basePath,
		"--stable", stablePath,
		"--approved-flag-migrations", emptyApproved,
		"--candidate-flag-migrations", candidatePath,
	}, &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "must start pending") {
		t.Fatalf("candidate self-approval exit=%d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
}

func TestCrossPlatformCoverageRunCompareRequiresBothFlagMigrationInputs(t *testing.T) {
	dir := t.TempDir()
	currentPath := writeSnapshot(t, dir, "current.json", commandSnapshot("dws"))
	basePath := writeSnapshot(t, dir, "base.json", commandSnapshot("dws"))
	approvedPath := writeManifest(t, dir, "approved.json", `{"version":1,"migrations":[]}`)

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"compare",
		"--current", currentPath,
		"--base", basePath,
		"--approved-flag-migrations", approvedPath,
	}, &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "must be provided together") {
		t.Fatalf("one-sided migration input exit=%d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
}

func TestCrossPlatformCoverageRunPrintsUsageForMissingAndUnknownCommands(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr []string
	}{
		{
			name:       "missing command",
			args:       nil,
			wantStderr: []string{"usage:", "interface-snapshot generate", "--approved-flag-migrations"},
		},
		{
			name:       "unknown command",
			args:       []string{"unknown"},
			wantStderr: []string{`unknown command "unknown"`, "usage:", "interface-snapshot compare"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 2 {
				t.Fatalf("run(%v) exit=%d, want 2", test.args, exitCode)
			}
			if stdout.Len() != 0 {
				t.Fatalf("run(%v) unexpectedly wrote stdout: %s", test.args, stdout.String())
			}
			for _, want := range test.wantStderr {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("run(%v) stderr missing %q:\n%s", test.args, want, stderr.String())
				}
			}
		})
	}
}

func TestCrossPlatformCoverageRunRejectsInvalidSubcommandArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "generate unknown flag", args: []string{"generate", "--unknown"}, want: "flag provided but not defined"},
		{name: "generate positional", args: []string{"generate", "unexpected"}, want: "generate accepts no positional arguments"},
		{name: "compare unknown flag", args: []string{"compare", "--unknown"}, want: "flag provided but not defined"},
		{name: "compare positional", args: []string{"compare", "unexpected"}, want: "compare accepts no positional arguments"},
		{name: "compare missing current", args: []string{"compare", "--base", "base.json"}, want: "compare requires --current"},
		{name: "compare missing reference", args: []string{"compare", "--current", "current.json"}, want: "compare requires --base, --stable, or both"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 2 {
				t.Fatalf("run(%v) exit=%d, want 2", test.args, exitCode)
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("run(%v) stderr missing %q:\n%s", test.args, test.want, stderr.String())
			}
		})
	}
}

func TestCrossPlatformCoverageRunGenerateRejectsUnsafeOutputPath(t *testing.T) {
	outputDirectory := t.TempDir()
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"generate", "--output", outputDirectory}, &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "create snapshot") {
		t.Fatalf("directory output exit=%d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
}

func TestCrossPlatformCoverageRunGenerateReportsTemporaryDirectoryFailure(t *testing.T) {
	missingTempRoot := filepath.Join(t.TempDir(), "missing")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, missingTempRoot)
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"generate"}, &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "create isolated home") {
		t.Fatalf("invalid temporary root exit=%d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
}

func TestCrossPlatformCoverageRunCompareReportsSnapshotReadFailures(t *testing.T) {
	dir := t.TempDir()
	validPath := writeSnapshot(t, dir, "valid.json", commandSnapshot("dws"))
	missingPath := filepath.Join(dir, "missing.json")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "current",
			args: []string{"compare", "--current", missingPath, "--base", validPath},
			want: "read current snapshot",
		},
		{
			name: "main",
			args: []string{"compare", "--current", validPath, "--base", missingPath},
			want: "read main/development baseline snapshot",
		},
		{
			name: "stable",
			args: []string{"compare", "--current", validPath, "--stable", missingPath},
			want: "read stable snapshot",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 2 {
				t.Fatalf("run(compare) exit=%d, want 2", exitCode)
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr missing %q:\n%s", test.want, stderr.String())
			}
		})
	}
}

func TestCrossPlatformCoverageRunCompareReportsEachManifestReadFailure(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := writeSnapshot(t, dir, "snapshot.json", commandSnapshot("dws"))
	validManifest := writeManifest(t, dir, "valid-manifest.json", `{"version":1,"migrations":[]}`)
	invalidManifest := writeManifest(t, dir, "invalid-manifest.json", `{`)
	tests := []struct {
		name      string
		approved  string
		candidate string
		want      string
	}{
		{
			name:      "approved manifest",
			approved:  invalidManifest,
			candidate: validManifest,
			want:      "read approved flag migrations",
		},
		{
			name:      "candidate manifest",
			approved:  validManifest,
			candidate: invalidManifest,
			want:      "read candidate flag migrations",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := run([]string{
				"compare",
				"--current", snapshotPath,
				"--base", snapshotPath,
				"--approved-flag-migrations", test.approved,
				"--candidate-flag-migrations", test.candidate,
			}, &stdout, &stderr)
			if exitCode != 2 || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("%s exit=%d stdout=%s stderr=%s", test.name, exitCode, stdout.String(), stderr.String())
			}
		})
	}
}

func TestCrossPlatformCoverageRunCompareReportsOutputFailure(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := writeSnapshot(t, dir, "snapshot.json", commandSnapshot("dws"))
	var stderr bytes.Buffer
	exitCode := run([]string{
		"compare",
		"--current", snapshotPath,
		"--base", snapshotPath,
	}, failingWriter{}, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "write comparison report") {
		t.Fatalf("comparison output failure exit=%d stderr=%s", exitCode, stderr.String())
	}
}

func TestCrossPlatformCoverageReadHelpersRejectMissingAndInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "missing.json")
	invalidPath := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte(`{`), 0o600); err != nil {
		t.Fatalf("write invalid fixture: %v", err)
	}

	if _, err := readSnapshot(missingPath); err == nil {
		t.Fatal("readSnapshot(missing) unexpectedly succeeded")
	}
	if _, err := readSnapshot(invalidPath); err == nil {
		t.Fatal("readSnapshot(invalid) unexpectedly succeeded")
	}
	if _, err := readFlagMigrationManifest(missingPath); err == nil {
		t.Fatal("readFlagMigrationManifest(missing) unexpectedly succeeded")
	}
	if _, err := readFlagMigrationManifest(invalidPath); err == nil {
		t.Fatal("readFlagMigrationManifest(invalid) unexpectedly succeeded")
	}
}

func TestCrossPlatformCoverageValidateHelpRenderingReportsResolveError(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	err := validateHelpRendering(root, commandSnapshot("dws missing"))
	if err == nil || !strings.Contains(err.Error(), `resolve "dws missing" before help rendering`) {
		t.Fatalf("validateHelpRendering resolve error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateHelpRenderingReportsTemplateError(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.SetHelpTemplate(`{{index .Commands 99}}`)
	err := validateHelpRendering(root, commandSnapshot("dws"))
	if err == nil || !strings.Contains(err.Error(), `render "dws" help`) {
		t.Fatalf("validateHelpRendering template error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateHelpRenderingRecoversTemplatePanic(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.SetHelpTemplate("{{")
	err := validateHelpRendering(root, commandSnapshot("dws"))
	if err == nil || !strings.Contains(err.Error(), `render "dws" help`) {
		t.Fatalf("validateHelpRendering template panic = %v", err)
	}
}

func TestCrossPlatformCoverageValidateHelpRenderingRejectsCustomHelpStderr(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.SetHelpFunc(func(command *cobra.Command, _ []string) {
		_, _ = io.WriteString(command.ErrOrStderr(), "injected help failure")
	})
	err := validateHelpRendering(root, commandSnapshot("dws"))
	if err == nil || !strings.Contains(err.Error(), "injected help failure") {
		t.Fatalf("validateHelpRendering custom stderr = %v", err)
	}
}

func TestCrossPlatformCoverageValidateHelpRenderingRejectsEmptyOutput(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.SetHelpFunc(func(*cobra.Command, []string) {})
	err := validateHelpRendering(root, commandSnapshot("dws"))
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("validateHelpRendering empty output = %v", err)
	}
}

func TestCrossPlatformCoverageValidateHelpRenderingAcceptsNormalOutput(t *testing.T) {
	root := &cobra.Command{Use: "dws", Short: "root command"}
	if err := validateHelpRendering(root, commandSnapshot("dws")); err != nil {
		t.Fatalf("validateHelpRendering normal output: %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("injected write failure")
}

var _ io.Writer = failingWriter{}

func commandSnapshot(paths ...string) interfacesnapshot.Snapshot {
	commands := make([]interfacesnapshot.Command, 0, len(paths))
	for _, path := range paths {
		commands = append(commands, interfacesnapshot.Command{
			Path:           path,
			Aliases:        []string{},
			LocalFlags:     []interfacesnapshot.Flag{},
			InheritedFlags: []interfacesnapshot.Flag{},
		})
	}
	return interfacesnapshot.Snapshot{
		SchemaVersion: interfacesnapshot.SchemaVersion,
		Rules: interfacesnapshot.Rules{
			ExcludedCommandSubtrees: []string{},
			ExcludedFlags:           []string{},
		},
		Commands: commands,
	}
}

func writeSnapshot(t *testing.T, dir, name string, snapshot interfacesnapshot.Snapshot) string {
	t.Helper()
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	if err := interfacesnapshot.Write(file, snapshot); err != nil {
		file.Close()
		t.Fatalf("write %s: %v", path, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
	return path
}

func writeManifest(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func flagMigrationSnapshot(after bool) interfacesnapshot.Snapshot {
	legacy := interfacesnapshot.Flag{
		Name:      "legacy-id",
		Shorthand: "l",
		Type:      "string",
		Default:   "",
		NoOpt:     "auto",
		Required:  true,
	}
	flags := []interfacesnapshot.Flag{legacy}
	if after {
		legacy.Required = false
		legacy.Hidden = true
		legacy.AliasOf = "message-id"
		flags = []interfacesnapshot.Flag{
			legacy,
			{Name: "message-id", Type: "string", Default: "", Required: true},
		}
	}
	return interfacesnapshot.Snapshot{
		SchemaVersion: interfacesnapshot.SchemaVersion,
		Rules: interfacesnapshot.Rules{
			ExcludedCommandSubtrees: []string{},
			ExcludedFlags:           []string{},
		},
		Commands: []interfacesnapshot.Command{
			{Path: "dws", Runnable: true, Aliases: []string{}, LocalFlags: []interfacesnapshot.Flag{}, InheritedFlags: []interfacesnapshot.Flag{}},
			{Path: "dws chat send", Runnable: true, Aliases: []string{}, LocalFlags: flags, InheritedFlags: []interfacesnapshot.Flag{}},
		},
	}
}

func flagMigrationManifestJSON(state string) string {
	return strings.Replace(`{
  "version": 1,
  "migrations": [{
    "command": "dws chat send",
    "legacy": {
      "name": "legacy-id",
      "before": {"present": true, "type": "string", "required": true, "shorthand": "l", "no_opt": "auto", "scope": "local"},
      "after": {"present": true, "type": "string", "hidden": true, "shorthand": "l", "no_opt": "auto", "scope": "local", "alias_of": "message-id"}
    },
    "canonical": {
      "name": "message-id",
      "before": {"present": false},
      "after": {"present": true, "type": "string", "required": true, "scope": "local"}
    },
    "state": "STATE",
    "reason": "reviewed exact migration"
  }]
}`, "STATE", state, 1)
}

func hasFlag(flags []interfacesnapshot.Flag, name, flagType string) bool {
	for _, flag := range flags {
		if flag.Name == name && flag.Type == flagType {
			return true
		}
	}
	return false
}
