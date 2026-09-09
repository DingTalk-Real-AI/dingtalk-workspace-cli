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

func TestReviewerRouterReconcilesBlockedCandidates(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute Reviewer Router reconciliation")
	}
	path := filepath.Join("..", "..", ".github", "workflows", "reviewer-router.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var workflow reviewerRouterWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var body string
	for _, step := range workflow.Jobs["reconcile"].Steps {
		script := step.With["script"]
		if !strings.Contains(script, "const mergePulls =") {
			continue
		}
		start := strings.Index(script, "const skipWorkflowPattern =")
		if start < 0 || body != "" {
			t.Fatal("expected one reconciliation body after the initial ruleset check")
		}
		body = script[start:]
	}
	if body == "" {
		t.Fatal("reconciliation body not found")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	// 直接执行 YAML 中的协调循环；规则内容由 required-rulesets 测试验证，
	// 此处替换外部 API 和规则校验，检查调用顺序及最终合并结果。
	const verification = `
const assert = require('node:assert/strict');
const body = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
const AsyncFunction = Object.getPrototypeOf(async function() {}).constructor;
const run = new AsyncFunction('github', 'core', 'owner', 'repo',
  'expectedAppOwner', 'repositorySource', 'assertReviewerRouterRulesetBoundary', body);
const owner = 'DingTalk-Real-AI';
const repo = 'dingtalk-workspace-cli';
const app = 'dingtalk-dws-reviewer-router[bot]';
const repository = (owner + '/' + repo).toLowerCase();
const pull = {
  number: 1284, node_id: 'pr-node', title: 'fix: ordinary command',
  state: 'open', draft: false, merged_at: null,
  head: {sha: 'reviewed-head'},
  base: {sha: 'reviewed-base', ref: 'main', repo: {full_name: owner + '/' + repo}},
  mergeable: true, mergeable_state: 'blocked',
  auto_merge: {
    enabled_by: {login: app},
    commit_title: 'Merge pull request #1284',
    commit_message: 'Merged by the dedicated Reviewer Router GitHub App for PR #1284.',
  },
};
const denied = {status: 403, message: 'Resource not accessible by integration'};
const cases = [
  {name: 'App writer restriction: approved and green', attempts: 1, merged: 1},
  {name: 'required approval denied by GitHub', error: {status: 405, message: 'Review required'}, attempts: 1, notReady: 1},
  {name: 'required checks denied by GitHub', error: denied, attempts: 1, notReady: 1},
  {name: 'conflict', patch: {mergeable: false, mergeable_state: 'dirty'}, notReady: 1},
  {name: 'blocked without mergeability', patch: {mergeable: null}, notReady: 1},
  {name: 'unknown', patch: {mergeable: null, mergeable_state: 'unknown'}, notReady: 1},
  {name: 'behind main', patch: {mergeable_state: 'behind'}, notReady: 1},
  {name: 'ruleset boundary fails', boundaryError: true, failed: 1},
  {name: 'final head changed', final: {head: {sha: 'new-head'}}, skipped: 1},
  {name: 'final intent disabled', final: {auto_merge: null}, skipped: 1},
  {name: 'final draft', final: {draft: true}, skipped: 1},
  {name: 'unrelated permission failure', error: {...denied, message: 'Forbidden'}, attempts: 1, failed: 1},
  {name: '403 head changed', error: denied, after: {head: {sha: 'new-head'}}, attempts: 1, failed: 1},
  {name: '403 base repository changed', error: denied, after: {base: {...pull.base, repo: {full_name: 'other/repo'}}}, attempts: 1, failed: 1},
  {name: '403 mergeability unavailable', error: denied, after: {mergeable: null, mergeable_state: 'unknown'}, attempts: 1, failed: 1},
  {name: 'successful response with wrong merger', after: {merged_by: {login: 'other-bot'}}, attempts: 1, failed: 1},
];
(async () => {
  for (const c of cases) {
    const candidate = {...pull, ...c.patch};
    const finalPull = {...candidate, ...c.final};
    const calls = [];
    const messages = [];
    const errors = [];
    const failures = [];
    let lists = 0, reads = 0, attempts = 0;
    const github = {rest: {pulls: {
      list: Symbol('list'), listFiles: Symbol('files'),
      get: async ({pull_number}) => {
        assert.equal(pull_number, pull.number);
        calls.push('get');
        reads++;
        if (reads === 1) return {data: candidate};
        if (reads === 2) return {data: finalPull};
        assert.equal(attempts, 1);
        const after = c.error ? finalPull : {...finalPull,
          state: 'closed', merged_at: '2026-09-04T00:00:00Z',
          merged_by: {login: app}, merge_commit_sha: 'merge-sha'};
        return {data: {...after, ...c.after}};
      },
      merge: async (request) => {
        calls.push('merge');
        attempts++;
        assert.deepEqual(calls, ['get', 'files', 'boundary', 'get', 'merge']);
        assert.deepEqual(request, {
          owner, repo, pull_number: pull.number, sha: 'reviewed-head',
          merge_method: 'merge', commit_title: pull.auto_merge.commit_title,
          commit_message: pull.auto_merge.commit_message,
        });
        if (c.error) throw Object.assign(new Error(c.error.message), {status: c.error.status});
        return {data: {merged: true, sha: 'merge-sha'}};
      },
    }}};
    github.paginate = async (endpoint) => {
      if (endpoint === github.rest.pulls.list) return ++lists === 1 ? [] : [candidate];
      assert.equal(endpoint, github.rest.pulls.listFiles);
      calls.push('files');
      return [{filename: 'internal/helpers/aitable.go'}];
    };
    const core = {
      info: message => messages.push(message), notice: message => messages.push(message),
      error: message => errors.push(message), setFailed: message => failures.push(message),
    };
    await run(github, core, owner, repo, app, repository, async () => {
      calls.push('boundary');
      if (c.boundaryError) throw new Error('App can bypass required checks');
    });
    assert.equal(attempts, c.attempts || 0, c.name + ': merge endpoint calls');
    assert.equal(errors.length, c.failed || 0, c.name + ': candidate failures');
    assert.equal(failures.length, c.failed || 0, c.name + ': workflow failures');
    assert.equal(messages.at(-1),
      'Reconciliation summary: migrated=0, merged=' + (c.merged || 0) +
      ', not_ready=' + (c.notReady || 0) + ', skipped=' + (c.skipped || 0) +
      ', failed=' + (c.failed || 0) + '.', c.name);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
`
	command := exec.Command(node, "-e", verification)
	command.Stdin = strings.NewReader(string(encoded))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("reconciliation execution failed: %v\n%s", err, output)
	}
}
