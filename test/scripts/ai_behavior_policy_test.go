// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAIBehaviorAdmissionIgnoresLabelEvents(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ai-behavior-check.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "types: [opened, synchronize, reopened, ready_for_review, converted_to_draft]") {
		t.Fatal("admission must follow revision/Draft events without label-driven reruns")
	}
}

func TestAIBehaviorAdmissionExecution(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("AI Behavior workflow execution tests require Node.js")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ai-behavior-check.yml"))
	if err != nil {
		t.Fatal(err)
	}
	_, body, found := strings.Cut(string(data), "          script: |\n")
	if !found {
		t.Fatal("workflow github-script not found")
	}
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		if !strings.HasPrefix(line, "            ") {
			break
		}
		lines = append(lines, strings.TrimPrefix(line, "            "))
	}
	script := strings.Join(lines, "\n")
	cases := []struct {
		name      string
		state     string
		pullReads int
	}{
		{"ready_unlabeled", "success", 2},
		{"ready_labeled", "success", 2},
		{"large_unlabeled", "success", 2},
		{"large_labeled", "success", 2},
		{"protected_unlabeled", "success", 2},
		{"protected_labeled", "success", 2},
		{"renamed_protected_labeled", "success", 2},
		{"draft_unlabeled", "failure", 1},
		{"draft_labeled", "failure", 1},
		{"head_drift_before", "error", 1},
		{"head_drift_after", "error", 2},
		{"base_drift_before", "error", 1},
		{"base_drift_after", "error", 2},
		{"draft_drift_before", "error", 1},
		{"draft_drift_after", "error", 2},
		{"closed_before", "error", 1},
		{"closed_after", "error", 2},
		{"api_error_before", "error", 1},
		{"api_error_after", "error", 2},
		{"main_push", "success", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"script": script, "scenario": tc.name})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, node, "-e", aiBehaviorAdmissionHarness)
			command.Stdin = strings.NewReader(string(input))
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("execute workflow: %v\n%s", err, output)
			}
			var result struct {
				Statuses []struct {
					State, SHA, Context string
					TargetURL           string `json:"target_url"`
				}
				Failures                []string
				Thrown, Head, TargetURL string
				PullReads, ContentReads int
			}
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("decode workflow result: %v\n%s", err, output)
			}
			if len(result.Statuses) != 2 || result.Statuses[0].State != "pending" {
				t.Fatalf("expected pending followed by one terminal status: %s", output)
			}
			for _, status := range result.Statuses {
				if status.SHA != result.Head || status.Context != "AI Behavior" || status.TargetURL != result.TargetURL {
					t.Fatalf("status is not bound to the exact event head and URL: %s", output)
				}
			}
			if final := result.Statuses[1].State; final != tc.state {
				t.Fatalf("final state = %s, want %s: %s", final, tc.state, output)
			}
			if result.PullReads != tc.pullReads || result.ContentReads != 0 {
				t.Fatalf("unexpected API reads (want %d PR reads, no patch reads): %s", tc.pullReads, output)
			}
			switch tc.state {
			case "success":
				if len(result.Failures) != 0 || result.Thrown != "" {
					t.Fatalf("Ready PR/main push was rejected: %s", output)
				}
			case "failure":
				if len(result.Failures) != 1 || !strings.Contains(result.Failures[0], "Draft pull requests") || result.Thrown != "" {
					t.Fatalf("Draft must explicitly fail admission: %s", output)
				}
			case "error":
				if result.Thrown == "" {
					t.Fatalf("API/revision races must fail the action: %s", output)
				}
			}
		})
	}
}

const aiBehaviorAdmissionHarness = `
const fs = require('fs');
const { script, scenario } = JSON.parse(fs.readFileSync(0, 'utf8'));
const clone = value => JSON.parse(JSON.stringify(value));
const pull = {
  number: 2000, state: 'open', draft: scenario.startsWith('draft_') && !scenario.startsWith('draft_drift_'),
  base: { sha: 'a'.repeat(40), ref: 'main' },
  head: { sha: 'b'.repeat(40) },
  labels: scenario.endsWith('_unlabeled') ? [] : [{ name: 'ai-generated' }],
  changed_files: 1,
};
const files = scenario.startsWith('large_')
  ? Array.from({ length: 114 }, (_, i) => ({ filename: 'internal/cli/file-' + i + '.go' }))
  : scenario.startsWith('renamed_protected_')
    ? [{ filename: 'internal/cli/moved.go', previous_filename: 'scripts/policy/gate.go', status: 'renamed' }]
    : scenario.startsWith('protected_')
      ? ['.github/workflows/ci.yml', 'scripts/ci/gate.sh', 'scripts/policy/gate.go',
         'scripts/release/release.sh', 'test/fixtures/cli-interface-baseline.txt',
         '.goreleaser.yaml', 'Makefile'].map(filename => ({ filename }))
      : [{ filename: 'internal/cli/file.go' }];
pull.changed_files = files.length;
const push = scenario === 'main_push';
const context = {
  eventName: push ? 'push' : 'pull_request_target',
  sha: 'c'.repeat(40),
  repo: { owner: 'DingTalk-Real-AI', repo: 'dingtalk-workspace-cli' },
  issue: { number: pull.number },
  payload: push ? {} : { pull_request: clone(pull) },
};
const head = push ? context.sha : pull.head.sha;
const targetUrl = 'https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/' +
  (push ? 'commit/' + head : 'pull/' + pull.number);
const statuses = [], failures = [];
let pullReads = 0, contentReads = 0;
const unexpectedContentRead = async () => {
  contentReads++;
  throw new Error('Admission must not inspect PR files or approved patch trees');
};
const github = {
  paginate: unexpectedContentRead,
  rest: {
    pulls: {
      listFiles: unexpectedContentRead,
      get: async () => {
        pullReads++;
        const phase = scenario.endsWith('_before') ? 1 : 2;
        if (scenario.startsWith('api_error_') && pullReads >= phase) throw new Error('PR API unavailable');
        const current = clone(pull);
        if (pullReads >= phase) {
          if (scenario.startsWith('head_drift_')) current.head.sha = 'd'.repeat(40);
          if (scenario.startsWith('base_drift_')) current.base.sha = 'd'.repeat(40);
          if (scenario.startsWith('draft_drift_')) current.draft = true;
          if (scenario.startsWith('closed_')) current.state = 'closed';
        }
        return { data: current };
      },
    },
    repos: {
      createCommitStatus: async status => { statuses.push(clone(status)); },
      compareCommits: unexpectedContentRead,
    },
    git: { getCommit: unexpectedContentRead, getTree: unexpectedContentRead },
  },
};
const core = {
  setFailed: message => failures.push(String(message)),
  notice: () => {},
};
const AsyncFunction = Object.getPrototypeOf(async function() {}).constructor;
(async () => {
  let thrown = '';
  try { await new AsyncFunction('github', 'context', 'core', script)(github, context, core); }
  catch (error) { thrown = String(error.message || error); }
  process.stdout.write(JSON.stringify({ statuses, failures, thrown, head, targetUrl, pullReads, contentReads }));
})().catch(error => { process.stderr.write(String(error)); process.exitCode = 1; });
`
