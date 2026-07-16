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
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
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

func hasFlag(flags []interfacesnapshot.Flag, name, flagType string) bool {
	for _, flag := range flags {
		if flag.Name == name && flag.Type == flagType {
			return true
		}
	}
	return false
}
