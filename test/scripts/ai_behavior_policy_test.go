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

func TestAIBehaviorPolicyProtectsEnforcementInputs(t *testing.T) {
	t.Parallel()

	path, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "ai-behavior-check.yml"))
	if err != nil {
		t.Fatalf("Abs(ai-behavior-check.yml) error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	workflow := string(data)
	for _, want := range []string{
		"filename.startsWith('scripts/ci/')",
		"filename.startsWith('scripts/policy/')",
		"filename === 'test/fixtures/cli-interface-baseline.txt'",
		"previous_filename",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("AI behavior policy does not protect %q", want)
		}
	}
}

func TestAIBehaviorReviewedPublicPatchExecution(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("执行 AI Behavior 工作流回归测试需要 Node.js")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ai-behavior-check.yml"))
	if err != nil {
		t.Fatalf("读取 AI Behavior 工作流: %v", err)
	}
	const marker = "          script: |\n"
	_, body, found := strings.Cut(string(data), marker)
	if !found {
		t.Fatal("未找到工作流内嵌的 github-script")
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
	if strings.TrimSpace(script) == "" {
		t.Fatal("提取的 github-script 为空")
	}

	cases := []struct {
		name string
		pass bool
	}{
		{"reviewed_original", true},
		{"reviewed_rebased", true},
		{"ordinary_small", true},
		{"ordinary_over_limit", false},
		{"ordinary_protected", false},
		{"new_blob_same_path", false},
		{"changed_preimage", false},
		{"extra_file", false},
		{"changed_mode", false},
		{"symlink", false},
		{"submodule", false},
		{"rename", false},
		{"deletion", false},
		{"truncated_current_tree", false},
		{"truncated_reviewed_tree", false},
		{"base_not_ancestor", false},
		{"head_drift_before_files", false},
		{"head_drift_after_files", false},
		{"head_drift_after_trees", false},
		{"base_drift_after_trees", false},
		{"draft_drift_after_trees", false},
		{"author_drift_after_trees", false},
		{"ref_drift_after_trees", false},
		{"wrong_repository", false},
		{"wrong_pr", false},
		{"wrong_author", false},
		{"wrong_ref", false},
		{"wrong_base_ref", false},
		{"fork_head", false},
		{"draft", false},
		{"head_policy_injection", false},
		{"body_exception_injection", false},
		{"files_api_sha_mismatch", false},
		{"files_api_omission", false},
		{"files_api_duplicate", false},
		{"duplicate_tree_path", false},
		{"tree_api_error", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"script": script, "scenario": tc.name})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, node, "-e", aiBehaviorExecutionHarness)
			command.Stdin = strings.NewReader(string(input))
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("Node 工作流执行失败: %v\n%s", err, output)
			}
			var result struct {
				Statuses []struct {
					State string `json:"state"`
					SHA   string `json:"sha"`
				} `json:"statuses"`
				Failures    []string `json:"failures"`
				Thrown      string   `json:"thrown"`
				Head        string   `json:"head"`
				CommitReads []string `json:"commitReads"`
				PullReads   int      `json:"pullReads"`
			}
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("解码 Node 执行结果: %v\n%s", err, output)
			}
			if len(result.Statuses) == 0 {
				t.Fatalf("工作流未发布状态: %s", output)
			}
			final := result.Statuses[len(result.Statuses)-1]
			if final.SHA != result.Head {
				t.Fatalf("状态未绑定事件中的精确 head: %s", output)
			}
			if tc.pass {
				if final.State != "success" || len(result.Failures) != 0 || result.Thrown != "" {
					t.Fatalf("获准补丁被拒绝: %s", output)
				}
				if strings.HasPrefix(tc.name, "reviewed_") {
					for _, sha := range []string{
						"3f0fc94111b1f9cd34e47468980b1f1a723ca69a",
						"53de9d92c9b310798d7f42a385e162273330442b",
					} {
						if !strings.Contains(strings.Join(result.CommitReads, "\n"), sha) {
							t.Fatalf("未读取固定评审锚点 %s: %s", sha, output)
						}
					}
					if result.PullReads < 3 {
						t.Fatalf("完整补丁读取后未重新核对 PR 身份: %s", output)
					}
				}
			} else if final.State != "failure" && final.State != "error" {
				t.Fatalf("未经批准的候选未失败: %s", output)
			} else if len(result.Failures) == 0 && result.Thrown == "" {
				t.Fatalf("状态失败但工作流自身未失败: %s", output)
			}
		})
	}
}

// 直接执行工作流脚本，避免只检查 YAML 字符串而漏掉实际放行逻辑。
const aiBehaviorExecutionHarness = `
const input = JSON.parse(require('fs').readFileSync(0, 'utf8'));
const scenario = input.scenario;
const clone = value => JSON.parse(JSON.stringify(value));
const reviewedBase = '3f0fc94111b1f9cd34e47468980b1f1a723ca69a';
const reviewedHead = '53de9d92c9b310798d7f42a385e162273330442b';
const rebased = scenario === 'reviewed_rebased';
const base = rebased || scenario === 'changed_preimage' ? 'a'.repeat(40) : reviewedBase;
const head = rebased ? 'b'.repeat(40) : reviewedHead;
const blob = value => value.toString(16).padStart(40, '0');
const protectedPaths = [
  'scripts/policy/skill-command-check/main.go',
  'scripts/policy/skill-command-check/main_test.go',
];
const paths = ['.changes/aitable-public-reliability.md'];
for (let i = 0; i < 41; i++) paths.push('internal/aitableprotocol/reviewed-fixture-' + i + '.go');
paths.push(...protectedPaths);
const entry = (path, sha) => ({ path, sha, mode: '100644', type: 'blob' });
const reviewedBaseTree = paths.slice(1).map((path, i) => entry(path, blob(1000 + i)));
const reviewedHeadTree = paths.map((path, i) => entry(path, blob(2000 + i)));
reviewedBaseTree.push(entry('unchanged.txt', blob(3000)));
reviewedHeadTree.push(entry('unchanged.txt', blob(3000)));
const currentBaseTree = clone(reviewedBaseTree);
const currentHeadTree = clone(reviewedHeadTree);
if (rebased) {
  const governance = entry('.github/workflows/ai-behavior-check.yml', blob(4000));
  currentBaseTree.push(clone(governance));
  currentHeadTree.push(clone(governance));
}
let files = paths.map((filename, i) => ({
  filename, status: i === 0 ? 'added' : 'modified', sha: blob(2000 + i),
}));
const pull = {
  number: 1463, state: 'open', draft: false,
  user: { login: 'haofeng0705' },
  base: { sha: base, ref: 'main', repo: { full_name: 'DingTalk-Real-AI/dingtalk-workspace-cli' } },
  head: {
    sha: head, ref: 'codex/sync-aitable-public-fixes',
    repo: { full_name: 'DingTalk-Real-AI/dingtalk-workspace-cli' },
  },
  labels: [{ name: 'ai-generated' }], changed_files: 44,
};
const context = {
  eventName: 'pull_request_target', sha: base,
  repo: { owner: 'DingTalk-Real-AI', repo: 'dingtalk-workspace-cli' },
  issue: { number: 1463 }, payload: {},
};
switch (scenario) {
  case 'ordinary_small':
    pull.number = context.issue.number = 1464;
    files = files.filter(file => !protectedPaths.includes(file.filename)).slice(0, 2);
    break;
  case 'ordinary_over_limit': pull.number = context.issue.number = 1464; break;
  case 'ordinary_protected':
    pull.number = context.issue.number = 1464;
    files = files.filter(file => protectedPaths.includes(file.filename));
    break;
  case 'new_blob_same_path':
  case 'body_exception_injection':
    currentHeadTree[1].sha = files[1].sha = blob(5000);
    pull.body = JSON.stringify({ reviewed: { base, head, count: 44, protectedPaths } });
    break;
  case 'extra_file':
    currentHeadTree.push(entry('internal/aitableprotocol/unreviewed.go', blob(5000)));
    files.push({ filename: 'internal/aitableprotocol/unreviewed.go', status: 'added', sha: blob(5000) });
    break;
  case 'changed_preimage': currentBaseTree[0].sha = blob(5000); break;
  case 'changed_mode': currentHeadTree[1].mode = '100755'; break;
  case 'symlink': currentHeadTree[1].mode = '120000'; break;
  case 'submodule': currentHeadTree[1].mode = '160000'; currentHeadTree[1].type = 'commit'; break;
  case 'rename':
    currentHeadTree[1].path = files[1].filename = 'internal/aitableprotocol/renamed.go';
    files[1].previous_filename = paths[1]; files[1].status = 'renamed';
    break;
  case 'deletion': currentHeadTree.splice(1, 1); files[1].status = 'removed'; break;
  case 'wrong_repository': context.repo.repo = 'other-repository'; break;
  case 'wrong_pr': pull.number = context.issue.number = 1464; break;
  case 'wrong_author': pull.user.login = 'other-author'; break;
  case 'wrong_ref': pull.head.ref = 'other-head-ref'; break;
  case 'wrong_base_ref': pull.base.ref = 'other-base-ref'; break;
  case 'fork_head': pull.head.repo.full_name = 'other-owner/dingtalk-workspace-cli'; break;
  case 'draft': pull.draft = true; break;
  case 'head_policy_injection':
    currentHeadTree[0].path = files[0].filename = '.github/workflows/injected-exception.json';
    break;
  case 'files_api_sha_mismatch': files[1].sha = blob(5000); break;
  case 'files_api_omission': files.pop(); break;
  case 'files_api_duplicate': files[1] = clone(files[0]); break;
  case 'duplicate_tree_path': currentHeadTree.push(clone(currentHeadTree[1])); break;
}
pull.changed_files = files.length;
context.payload.pull_request = clone(pull);
const snapshots = new Map([
  [reviewedBase, reviewedBaseTree], [reviewedHead, reviewedHeadTree],
]);
// 同一 SHA 的响应不得被候选更改污染；变体使用独立当前 head。
if (!rebased && scenario !== 'reviewed_original') {
  pull.head.sha = 'c'.repeat(40);
  context.payload.pull_request.head.sha = pull.head.sha;
}
snapshots.set(base, currentBaseTree);
snapshots.set(pull.head.sha, currentHeadTree);
const statuses = [], failures = [], commitReads = [];
let pullReads = 0;
const github = {
  paginate: async (method, params) => {
    if (method !== github.rest.pulls.listFiles || params.pull_number !== context.issue.number) {
      throw new Error('Unexpected Files API request');
    }
    return clone(files);
  },
  rest: {
    pulls: {
      listFiles: async () => { throw new Error('Files API must be paginated'); },
      get: async () => {
        pullReads++;
        const current = clone(pull);
        if ((scenario === 'head_drift_before_files' && pullReads >= 1) ||
            (scenario === 'head_drift_after_files' && pullReads >= 2) ||
            (scenario === 'head_drift_after_trees' && pullReads >= 3)) {
          current.head.sha = 'd'.repeat(40);
        }
        if (scenario === 'base_drift_after_trees' && pullReads >= 3) current.base.sha = 'd'.repeat(40);
        if (scenario === 'draft_drift_after_trees' && pullReads >= 3) current.draft = true;
        if (scenario === 'author_drift_after_trees' && pullReads >= 3) current.user.login = 'other-author';
        if (scenario === 'ref_drift_after_trees' && pullReads >= 3) current.head.ref = 'other-head-ref';
        return { data: current };
      },
    },
    repos: {
      createCommitStatus: async status => { statuses.push(clone(status)); return { data: status }; },
      compareCommits: async params => {
        if (params.base !== base || params.head !== pull.head.sha) throw new Error('Wrong ancestry anchors');
        return { data: {
          merge_base_commit: { sha: scenario === 'base_not_ancestor' ? 'd'.repeat(40) : base },
          behind_by: scenario === 'base_not_ancestor' ? 1 : 0,
        } };
      },
    },
    git: {
      getCommit: async ({ commit_sha }) => {
        commitReads.push(commit_sha);
        if (!snapshots.has(commit_sha)) throw new Error('Unapproved commit lookup');
        return { data: { tree: { sha: commit_sha } } };
      },
      getTree: async ({ tree_sha, recursive }) => {
        if (recursive !== '1') throw new Error('Tree read must be complete and recursive');
        if (scenario === 'tree_api_error') throw new Error('Tree API unavailable');
        if (!snapshots.has(tree_sha)) throw new Error('Unknown tree lookup');
        const truncated =
          (scenario === 'truncated_current_tree' && tree_sha === pull.head.sha) ||
          (scenario === 'truncated_reviewed_tree' && tree_sha === reviewedHead);
        return { data: { truncated, tree: clone(snapshots.get(tree_sha)) } };
      },
    },
  },
};
const core = {
  setFailed: message => failures.push(String(message)),
  notice: () => {}, warning: () => {}, info: () => {},
};
const AsyncFunction = Object.getPrototypeOf(async function() {}).constructor;
(async () => {
  let thrown = '';
  try { await new AsyncFunction('github', 'context', 'core', input.script)(github, context, core); }
  catch (error) { thrown = String(error.message || error); }
  process.stdout.write(JSON.stringify({
    statuses, failures, thrown, head: context.payload.pull_request.head.sha, commitReads, pullReads,
  }));
})().catch(error => { process.stderr.write(String(error)); process.exitCode = 1; });
`
