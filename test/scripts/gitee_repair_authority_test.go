// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGiteeRepairAuthorityRequiresCompleteReleaseAssetSet(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the repair authority workflow script")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				With struct {
					Script string `yaml:"script"`
				} `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	script := ""
	for _, step := range workflow.Jobs["repair-channel"].Steps {
		if step.Name == "Verify immutable release authority" {
			script = step.With.Script
		}
	}
	if script == "" {
		t.Fatal("repair-channel authority script was not found")
	}

	for _, test := range []struct {
		name    string
		version string
		mutate  func([]string) []string
		wantErr bool
	}{
		{name: "stable 64 complete", version: "v1.0.64"},
		{name: "stable 65 complete", version: "v1.0.65"},
		{
			name: "missing OpenHarmony", version: "v1.0.65", wantErr: true,
			mutate: func(assets []string) []string {
				var remaining []string
				for _, asset := range assets {
					if asset != "dws-openharmony-arm64.tar.gz" {
						remaining = append(remaining, asset)
					}
				}
				return remaining
			},
		},
		{
			name: "missing checksums", version: "v1.0.65", wantErr: true,
			mutate: func(assets []string) []string { return assets[:len(assets)-1] },
		},
		{
			name: "duplicate replaces required asset", version: "v1.0.65", wantErr: true,
			mutate: func(assets []string) []string {
				assets[len(assets)-1] = assets[0]
				return assets
			},
		},
		{
			name: "unknown replaces required asset", version: "v1.0.65", wantErr: true,
			mutate: func(assets []string) []string {
				assets[len(assets)-1] = "dws-unknown.tar.gz"
				return assets
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			names := append([]string(nil), requiredGiteeAssets...)
			if test.mutate != nil {
				names = test.mutate(names)
			}
			var assets []map[string]string
			for _, name := range names {
				assets = append(assets, map[string]string{"name": name})
			}
			fixture, err := json.Marshal(map[string]any{
				"tag_name": test.version, "draft": false, "immutable": true,
				"prerelease": false, "assets": assets,
			})
			if err != nil {
				t.Fatal(err)
			}
			command := exec.Command(node, "--input-type=module", "-e", giteeRepairAuthorityHarness)
			command.Env = append(os.Environ(),
				"REPAIR_AUTHORITY_SCRIPT="+script,
				"REPAIR_AUTHORITY_RELEASE="+string(fixture),
			)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("repair authority fixture failed: %v\n%s", err, output)
			}
			var result struct {
				Failures []string          `json:"failures"`
				Outputs  map[string]string `json:"outputs"`
			}
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("decode repair authority result: %v\n%s", err, output)
			}
			if test.wantErr {
				if len(result.Failures) != 1 || !strings.Contains(result.Failures[0], "exact release asset set") || len(result.Outputs) != 0 {
					t.Fatalf("invalid release was not rejected before authority outputs: %+v", result)
				}
				return
			}
			if len(result.Failures) != 0 || result.Outputs["commit_sha"] != strings.Repeat("b", 40) || result.Outputs["tag_object"] != strings.Repeat("a", 40) || result.Outputs["oss_mirror"] != "deferred" {
				t.Fatalf("complete stable release did not retain sealed authority: %+v", result)
			}
		})
	}
}

const giteeRepairAuthorityHarness = `
const release = JSON.parse(process.env.REPAIR_AUTHORITY_RELEASE);
const version = release.tag_name;
process.env.VERSION = version;
const commit = "b".repeat(40);
const failures = [];
const outputs = {};
const context = {
  repo: {owner: "DingTalk-Real-AI", repo: "dingtalk-workspace-cli"},
  payload: {repository: {default_branch: "main"}},
};
const github = {rest: {
  git: {
    async getRef({ref}) {
      if (ref === "tags/withdrawn/" + version) {
        const error = new Error("not withdrawn");
        error.status = 404;
        throw error;
      }
      if (ref !== "tags/" + version) throw new Error("unexpected ref " + ref);
      return {data: {object: {type: "tag", sha: "a".repeat(40)}}};
    },
    async getTag() {
      return {data: {tag: version, object: {type: "commit", sha: commit}, message: "OSS-Mirror: deferred"}};
    },
  },
  repos: {
    async getBranch() { return {data: {commit: {sha: "c".repeat(40)}}}; },
    async compareCommitsWithBasehead() { return {data: {status: "ahead"}}; },
    async getReleaseByTag() { return {data: release}; },
  },
}};
const core = {
  setFailed(message) { failures.push(message); },
  setOutput(name, value) { outputs[name] = value; },
};
const AsyncFunction = Object.getPrototypeOf(async function() {}).constructor;
await new AsyncFunction("github", "context", "core", process.env.REPAIR_AUTHORITY_SCRIPT)(github, context, core);
console.log(JSON.stringify({failures, outputs}));
`
