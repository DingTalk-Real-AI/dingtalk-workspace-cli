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

type admittedMergeClassifierResult struct {
	Outputs  map[string]string `json:"outputs"`
	Warnings []string          `json:"warnings"`
}

func TestAdmittedMergeClassifierFailsClosed(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the admitted-merge classifier")
	}
	classifier := readWorkflowGitHubScript(t, ".github/workflows/ci.yml", "lint", "Classify revision scope")

	tests := []struct {
		name     string
		scenario string
		admitted string
	}{
		{name: "exact evidence is reused", scenario: "success", admitted: "true"},
		{name: "association index lag recovers on retry", scenario: "association-index-lag", admitted: "true"},
		{name: "cold association index recovers via closed-PR discovery", scenario: "association-index-cold", admitted: "true"},
		{name: "unresolvable merged PR recomputes full suite", scenario: "merged-pr-unresolvable", admitted: "false"},
		{name: "comparison API failure recomputes full suite", scenario: "compare-error", admitted: "false"},
		{name: "predecessor checks API failure recomputes full suite", scenario: "predecessor-check-error", admitted: "false"},
		{name: "failed predecessor admission recomputes full suite", scenario: "predecessor-check-failure", admitted: "false"},
		{name: "AI status for another PR recomputes full suite", scenario: "ai-target-mismatch", admitted: "false"},
		{name: "post-merge CI result recomputes full suite", scenario: "stale-ci", admitted: "false"},
		{name: "duplicate artifact recomputes full suite", scenario: "duplicate-artifact", admitted: "false"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := runAdmittedMergeClassifier(t, node, classifier, test.scenario)
			if got := result.Outputs["admitted_merge"]; got != test.admitted {
				t.Fatalf("admitted_merge = %q, want %q; warnings=%v", got, test.admitted, result.Warnings)
			}
			if test.admitted == "true" {
				for key, want := range map[string]string{
					"full_suite":                     "false",
					"release_sensitive":              "false",
					"interface_sensitive":            "false",
					"mcp_sensitive":                  "false",
					"edition_sensitive":              "false",
					"platform_sensitive":             "false",
					"admitted_merge_head_sha":        strings.Repeat("c", 40),
					"admitted_merge_run_id":          "100",
					"admitted_merge_artifact_id":     "300",
					"admitted_merge_artifact_digest": "sha256:" + strings.Repeat("d", 64),
				} {
					if got := result.Outputs[key]; got != want {
						t.Errorf("%s = %q, want %q", key, got, want)
					}
				}
				return
			}

			for _, key := range []string{
				"changelog_only",
				"docs_only",
			} {
				if got := result.Outputs[key]; got != "false" {
					t.Errorf("fail-closed %s = %q, want false", key, got)
				}
			}
			for _, key := range []string{
				"full_suite",
				"release_sensitive",
				"interface_sensitive",
				"mcp_sensitive",
				"edition_sensitive",
				"platform_sensitive",
			} {
				if got := result.Outputs[key]; got != "true" {
					t.Errorf("fail-closed %s = %q, want true", key, got)
				}
			}
			for _, key := range []string{
				"admitted_merge_head_sha",
				"admitted_merge_run_id",
				"admitted_merge_artifact_id",
				"admitted_merge_artifact_digest",
			} {
				if got := result.Outputs[key]; got != "" {
					t.Errorf("failed admission leaked %s = %q", key, got)
				}
			}
			if len(result.Warnings) == 0 {
				t.Error("fail-closed scenario must explain why reuse was rejected")
			}
		})
	}
}

func readWorkflowGitHubScript(t *testing.T, relativePath, jobName, stepName string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Uses string `yaml:"uses"`
				With struct {
					Script string `yaml:"script"`
				} `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse %s: %v", relativePath, err)
	}
	job, ok := workflow.Jobs[jobName]
	if !ok {
		t.Fatalf("workflow %s has no job %q", relativePath, jobName)
	}
	for _, step := range job.Steps {
		if step.Name == stepName && strings.HasPrefix(step.Uses, "actions/github-script@") {
			if step.With.Script == "" {
				t.Fatalf("workflow step %q has no script", stepName)
			}
			return step.With.Script
		}
	}
	t.Fatalf("workflow %s job %s has no github-script step %q", relativePath, jobName, stepName)
	return ""
}

func runAdmittedMergeClassifier(t *testing.T, node, classifier, scenario string) admittedMergeClassifierResult {
	t.Helper()
	const harnessPrefix = `
const scenario = process.argv[2];
// Keep the production discovery retry loop fast inside the harness; the
// production defaults stay in the classifier script itself.
process.env.DWS_ADMITTED_MERGE_RETRY_INTERVAL_MS = '1';
process.env.DWS_ADMITTED_MERGE_RETRY_ATTEMPTS = '5';
let associationCalls = 0;
const before = 'b'.repeat(40);
const after = 'a'.repeat(40);
const head = 'c'.repeat(40);
const mergedAt = '2026-09-03T12:00:00Z';
const completedAt = '2026-09-03T11:59:00Z';
const lateAt = '2026-09-03T12:01:00Z';
const repository = 'DingTalk-Real-AI/dingtalk-workspace-cli';
const headRepository = 'example/dingtalk-workspace-cli';
const headBranch = 'feature/admitted-merge';
const ciRunID = 100;
const aiRunID = 200;
const pullNumber = 1280;
const requiredContexts = [
  'Lint', 'Test', 'Coverage', 'Policy', 'Edition',
  'Interface Integrity', 'AI Behavior', 'CLI Smoke', 'Mock MCP',
];
const fullCoverageContexts = [
  'Coverage (current: app)', 'Coverage (current: cli)',
  'Coverage (current: generators)', 'Coverage (current: helpers)',
  'Coverage (current: remaining)', 'Coverage (supporting)',
];
const checks = [...requiredContexts, ...fullCoverageContexts].map((name, index) => ({
  id: 1000 + index,
  name,
  head_sha: head,
  app: {slug: 'github-actions'},
  status: 'completed',
  conclusion: 'success',
  completed_at: completedAt,
  details_url: name === 'AI Behavior'
    ? 'https://github.com/' + repository + '/actions/runs/' + aiRunID + '/job/2001'
    : 'https://github.com/' + repository + '/actions/runs/' + ciRunID + '/job/' + (1000 + index),
}));
const pull = {
  number: pullNumber,
  state: 'closed',
  merged_at: mergedAt,
  base: {ref: 'main', sha: before, repo: {full_name: repository}},
  head: {ref: headBranch, sha: head, repo: {full_name: headRepository}},
  merge_commit_sha: after,
};
const ciRun = {
  id: ciRunID,
  name: 'CI',
  workflow_id: 10,
  path: '.github/workflows/ci.yml',
  event: 'pull_request',
  head_sha: head,
  head_branch: headBranch,
  head_repository: {full_name: headRepository},
  repository: {full_name: repository},
  status: 'completed',
  conclusion: 'success',
  updated_at: scenario === 'stale-ci' ? lateAt : completedAt,
};
const aiRun = {
  id: aiRunID,
  name: 'Code Admission — AI Behavior',
  path: '.github/workflows/ai-behavior-check.yml',
  event: 'pull_request_target',
  head_sha: head,
  head_branch: headBranch,
  head_repository: {full_name: headRepository},
  repository: {full_name: repository},
  status: 'completed',
  conclusion: 'success',
  updated_at: completedAt,
};
const artifact = {
  id: 300,
  name: 'coverage-report',
  expired: false,
  size_in_bytes: 4096,
  digest: 'sha256:' + 'd'.repeat(64),
  updated_at: completedAt,
  workflow_run: {id: ciRunID, head_sha: head},
};
const endpoints = {
  repos: {
    compareCommitsWithBasehead: async () => {
      if (scenario === 'compare-error') throw new Error('comparison unavailable');
      return {data: {
        status: 'ahead', merge_base_commit: {sha: before},
        behind_by: 0, ahead_by: 1, total_commits: 1,
        files: scenario.startsWith('predecessor-')
          ? [{filename: 'CHANGELOG.md', status: 'modified'}]
          : [{filename: 'internal/app/root.go', status: 'modified'}],
      }};
    },
    getCommit: async ({ref}) => ({data: ref === after
      ? {sha: after, parents: scenario.startsWith('predecessor-')
          ? [{sha: before}]
          : [{sha: before}, {sha: head}], commit: {tree: {sha: 'tree'}}}
      : {sha: head, commit: {tree: {sha: 'tree'}}}
    }),
    getContent: async () => ({data: {type: 'file', sha: 'e'.repeat(40)}}),
    listCommitStatusesForRef: async () => ({data: [{
      id: 500,
      context: 'AI Behavior',
      state: 'success',
      target_url: scenario === 'ai-target-mismatch'
        ? 'https://github.com/' + repository + '/pull/1279'
        : 'https://github.com/' + repository + '/pull/' + pullNumber,
      updated_at: completedAt,
    }]}),
  },
  pulls: {
    listPullRequestsAssociatedWithCommit: async () => {
      if (scenario === 'association-index-lag') {
        associationCalls += 1;
        return {data: associationCalls > 2 ? [pull] : []};
      }
      if (scenario === 'association-index-cold' || scenario === 'merged-pr-unresolvable') {
        return {data: []};
      }
      return {data: [pull]};
    },
    list: async () => ({data: scenario === 'association-index-cold' ? [pull] : []}),
  },
  checks: {
    listForRef: async ({ref}) => {
      if (ref === before && scenario === 'predecessor-check-error') {
        throw new Error('checks unavailable');
      }
      if (ref === before) {
        return {data: {check_runs: requiredContexts.map((name, index) => ({
          id: 2000 + index,
          name,
          head_sha: before,
          app: {slug: 'github-actions'},
          status: 'completed',
          conclusion: scenario === 'predecessor-check-failure' && name === 'Coverage'
            ? 'failure'
            : 'success',
        }))}};
      }
      return {data: {check_runs: checks}};
    },
  },
  actions: {
    getWorkflow: async () => ({data: {id: 10, name: 'CI', path: '.github/workflows/ci.yml', state: 'active'}}),
    getWorkflowRun: async () => ({data: aiRun}),
    listWorkflowRuns: async () => ({data: {workflow_runs: [ciRun]}}),
    listWorkflowRunArtifacts: async () => ({data: {artifacts:
      scenario === 'duplicate-artifact' ? [artifact, {...artifact, id: 301}] : [artifact],
    }}),
  },
};
const unwrapPage = (response) => {
  const data = response?.data;
  if (Array.isArray(data)) return data;
  for (const key of ['check_runs', 'workflow_runs', 'artifacts']) {
    if (Array.isArray(data?.[key])) return data[key];
  }
  throw new Error('mock endpoint did not return a pageable collection');
};
const github = {
  rest: endpoints,
  paginate: async (endpoint, args) => unwrapPage(await endpoint(args)),
};
const outputs = {};
const warnings = [];
const summary = {
  addHeading() { return this; },
  addRaw() { return this; },
  async write() { return this; },
};
const core = {
  setOutput(name, value) { outputs[name] = String(value); },
  warning(value) { warnings.push(String(value)); },
  summary,
};
const context = {
  eventName: 'push',
  ref: 'refs/heads/main',
  sha: after,
  repo: {owner: 'DingTalk-Real-AI', repo: 'dingtalk-workspace-cli'},
  payload: {
    before,
    after,
    ref: 'refs/heads/main',
    created: false,
    deleted: false,
    forced: false,
  },
};
(async () => {
`
	const harnessSuffix = `
  process.stdout.write(JSON.stringify({outputs, warnings}));
})().catch((error) => {
  process.stderr.write(error.stack || String(error));
  process.exit(1);
});
`
	source := harnessPrefix + classifier + harnessSuffix
	scriptPath := filepath.Join(t.TempDir(), "admitted-merge-classifier.js")
	if err := os.WriteFile(scriptPath, []byte(source), 0o600); err != nil {
		t.Fatalf("write classifier harness: %v", err)
	}
	command := exec.Command(node, scriptPath, scenario)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("execute classifier scenario %q: %v\n%s", scenario, err, output)
	}
	var result admittedMergeClassifierResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode classifier scenario %q: %v\n%s", scenario, err, output)
	}
	if result.Outputs == nil {
		t.Fatalf("classifier scenario %q produced no outputs", scenario)
	}
	return result
}

func TestAIBehaviorStatusBindsPullRequestURL(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ai-behavior-check.yml"))
	if err != nil {
		t.Fatalf("read AI Behavior workflow: %v", err)
	}
	workflow := string(data)
	for _, want := range []string{
		"const statusTargetUrl = context.eventName === 'push'",
		"`https://github.com/${context.repo.owner}/${context.repo.repo}/pull/${context.issue.number}`",
		"target_url: statusTargetUrl",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("AI Behavior status is missing PR binding %q", want)
		}
	}
}
