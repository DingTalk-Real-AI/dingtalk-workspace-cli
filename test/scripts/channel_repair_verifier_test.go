// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Execute the real workflow download/verification step with deliberately
// incompatible main and sealed verifiers. A historical immutable release must
// use its own verifier, and verifier failures must still stop channel repair.
func TestReleaseChannelRepairRunsSealedArtifactVerifier(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required to execute the channel repair step")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, step := range workflow.Jobs["repair-channel"].Steps {
		if step.Name == "Download and verify immutable GitHub Release assets" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("channel repair download/verification step is missing")
	}

	for _, test := range []struct {
		name     string
		version  string
		download int
		verify   int
	}{
		{name: "historical payload format", version: "v1.0.66"},
		{name: "current payload format", version: "v1.0.67-beta.1"},
		{name: "sealed verifier rejects artifacts", version: "v1.0.66", verify: 42},
		{name: "download failure stops verification", version: "v1.0.66", download: 43},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			for _, dir := range []string{bin, filepath.Join(root, "tooling", "scripts", "release"), filepath.Join(root, "release-source", "scripts", "release")} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			mustWriteFile(t, filepath.Join(bin, "gh"), []byte(`#!/bin/sh
set -eu
test "$1 $2 $3" = "release download $VERSION"
shift 3
while [ "$#" -gt 0 ]; do
  if [ "$1" = --dir ]; then
    shift
    dist="$1"
  fi
  shift
done
exit_code="${DOWNLOAD_EXIT:-0}"
[ "$exit_code" = 0 ] || exit "$exit_code"
printf '%s\n' "$VERSION" > "$dist/downloaded-version"
`), 0o755)
			mustWriteFile(t, filepath.Join(root, "tooling", "scripts", "release", "verify-release-artifacts.sh"), []byte("#!/bin/sh\necho 'wrong verifier: main expects a different runtime format' >&2\nexit 97\n"), 0o755)
			mustWriteFile(t, filepath.Join(root, "release-source", "scripts", "release", "verify-release-artifacts.sh"), []byte(`#!/bin/sh
set -eu
test "$1" = "$VERSION"
test "$DWS_PACKAGE_DIST_DIR" = "$GITHUB_WORKSPACE/dist"
test "$(cat "$DWS_PACKAGE_DIST_DIR/downloaded-version")" = "$VERSION"
printf '%s\n' "$VERSION" > "$GITHUB_WORKSPACE/sealed-verifier-called"
exit "$VERIFY_EXIT"
`), 0o755)
			cmd := exec.Command(bash, "-c", script)
			cmd.Dir = root
			cmd.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GITHUB_WORKSPACE="+root,
				"GITHUB_REPOSITORY=DingTalk-Real-AI/dingtalk-workspace-cli",
				"VERSION="+test.version,
				fmt.Sprintf("DOWNLOAD_EXIT=%d", test.download),
				fmt.Sprintf("VERIFY_EXIT=%d", test.verify),
			)
			output, err := cmd.CombinedOutput()
			wantExit := test.download
			if wantExit == 0 {
				wantExit = test.verify
			}
			gotExit := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					gotExit = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if gotExit != wantExit {
				t.Fatalf("repair step exit = %d, want %d:\n%s", gotExit, wantExit, output)
			}
			called, readErr := os.ReadFile(filepath.Join(root, "sealed-verifier-called"))
			if test.download != 0 {
				if !os.IsNotExist(readErr) {
					t.Fatalf("download failure ran the verifier: %q, %v", called, readErr)
				}
			} else if readErr != nil || strings.TrimSpace(string(called)) != test.version {
				t.Fatalf("sealed verifier was not called for %s: %q, %v", test.version, called, readErr)
			}
		})
	}
}
