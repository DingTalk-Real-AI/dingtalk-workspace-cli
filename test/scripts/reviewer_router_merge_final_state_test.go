// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const appMergeFinalStateHelper = `function validateAppMergeFinalState(
  pullRequest,
  expectedHeadSha,
  expectedAppLogin,
  expectedBaseRepository,
  expectedMergeSha,
) {
  const mergedBy = pullRequest.merged_by?.login?.toLowerCase();
  const mergeCommitSha = pullRequest.merge_commit_sha;
  const baseRepository =
    pullRequest.base?.repo?.full_name?.toLowerCase();
  const responseShaRequired = arguments.length >= 5;
  const responseShaMatches =
    !responseShaRequired ||
    (typeof expectedMergeSha === 'string' &&
      expectedMergeSha.length > 0 &&
      mergeCommitSha === expectedMergeSha);
  if (
    pullRequest.state !== 'closed' ||
    !pullRequest.merged_at ||
    pullRequest.head.sha !== expectedHeadSha ||
    pullRequest.base?.ref !== 'main' ||
    baseRepository !== expectedBaseRepository ||
    mergedBy !== expectedAppLogin ||
    typeof mergeCommitSha !== 'string' ||
    !mergeCommitSha ||
    !responseShaMatches
  ) {
    throw new Error(
      ` + "`merge result was not attributed exactly to ${expectedAppLogin}`" + `,
    );
  }
  return {mergedBy, mergeCommitSha};
}`

const successfulMergeResponseHelper = `function requireSuccessfulMergeResponse(mergeResult) {
  if (mergeResult?.merged !== true) {
    throw new Error(
      ` + "`GitHub returned a successful response without merging: ${mergeResult?.message || 'no explanation'}`" + `,
    );
  }
  if (
    typeof mergeResult.sha !== 'string' ||
    !mergeResult.sha
  ) {
    throw new Error(
      'GitHub returned a successful merge without a merge commit SHA.',
    );
  }
  return mergeResult.sha;
}`

const mergePreflightHelper = `function classifyMergePreflight(pullRequest) {
  if (
    pullRequest.mergeable === true &&
    pullRequest.mergeable_state === 'behind'
  ) {
    return 'behind';
  }
  if (
    pullRequest.mergeable === null &&
    pullRequest.mergeable_state === 'unknown'
  ) {
    return 'unknown';
  }
  return 'attempt';
}`

const mergeAttemptRecoveryHelper = `function classifyMergeAttemptRecovery(
  status,
  errorMessage,
  latestPull,
  expectedHeadSha,
  expectedAppLogin,
  expectedBaseRepository,
  responseMergeSha,
) {
  if (latestPull.merged_at) {
    const finalState = responseMergeSha === undefined
      ? validateAppMergeFinalState(
          latestPull,
          expectedHeadSha,
          expectedAppLogin,
          expectedBaseRepository,
        )
      : validateAppMergeFinalState(
          latestPull,
          expectedHeadSha,
          expectedAppLogin,
          expectedBaseRepository,
          responseMergeSha,
        );
    return {outcome: 'merged', finalState};
  }
  if (latestPull.state !== 'open') {
    return {outcome: 'closed'};
  }
  if (status === 404) {
    throw new Error(
      'merge endpoint returned 404 while the pull request remained open',
    );
  }
  if (status === 405 || status === 409) {
    return {outcome: 'not_ready'};
  }
  const baseRepository =
    latestPull.base?.repo?.full_name?.toLowerCase();
  if (
    status === 403 &&
    errorMessage === 'Resource not accessible by integration' &&
    latestPull.head.sha === expectedHeadSha &&
    latestPull.base?.ref === 'main' &&
    baseRepository === expectedBaseRepository &&
    latestPull.mergeable === true &&
    latestPull.mergeable_state === 'behind'
  ) {
    return {outcome: 'not_ready'};
  }
  throw new Error(` + "`unsupported merge recovery status ${status}`" + `);
}`

func TestReviewerRouterValidatesAppMergeFinalState(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to verify App merge final-state semantics")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	path := filepath.Join(root, ".github", "workflows", "reviewer-router.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var workflow any
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var scripts []string
	collectGitHubScripts(workflow, &scripts)
	checked := 0
	for _, script := range scripts {
		if strings.Contains(script, "function validateAppMergeFinalState(") {
			checked++
			if got := strings.Count(script, appMergeFinalStateHelper); got != 1 {
				t.Errorf("App merge final-state helper count = %d, want 1", got)
			}
			if got := strings.Count(script, successfulMergeResponseHelper); got != 1 {
				t.Errorf("successful merge-response helper count = %d, want 1", got)
			}
			if got := strings.Count(script, mergePreflightHelper); got != 1 {
				t.Errorf("merge-preflight helper count = %d, want 1", got)
			}
			if got := strings.Count(script, mergeAttemptRecoveryHelper); got != 1 {
				t.Errorf("merge-attempt recovery helper count = %d, want 1", got)
			}
			if got := strings.Count(script, "validateAppMergeFinalState("); got != 6 {
				t.Errorf("App merge final-state helper/calls = %d, want one helper plus five guarded final-state calls", got)
			}
			for _, marker := range []string{"currentPull,", "mergedPull,", "latestPull,"} {
				if !strings.Contains(script, marker) {
					t.Errorf("reconciliation script is missing final-state validation marker %q", marker)
				}
			}
		}
	}
	if checked != 1 {
		t.Fatalf("App merge final-state scripts checked = %d, want 1", checked)
	}

	verification := appMergeFinalStateHelper + "\n" + successfulMergeResponseHelper + "\n" + mergePreflightHelper + "\n" + mergeAttemptRecoveryHelper + `
const app = 'dingtalk-dws-reviewer-router[bot]';
const valid = {
  state: 'closed',
  merged_at: '2026-08-25T00:00:00Z',
  head: {sha: 'head-sha'},
  base: {
    ref: 'main',
    repo: {full_name: 'DingTalk-Real-AI/dingtalk-workspace-cli'},
  },
  merged_by: {login: 'DingTalk-DWS-Reviewer-Router[bot]'},
  merge_commit_sha: 'merge-sha',
};
const repository = 'dingtalk-real-ai/dingtalk-workspace-cli';
const observed = validateAppMergeFinalState(valid, 'head-sha', app, repository);
if (observed.mergedBy !== app || observed.mergeCommitSha !== 'merge-sha') {
  throw new Error('valid concurrent App merge did not preserve final identity');
}
validateAppMergeFinalState(valid, 'head-sha', app, repository, 'merge-sha');

const invalidCases = [
  ['open PR', {...valid, state: 'open'}],
  ['missing merged_at', {...valid, merged_at: null}],
  ['wrong head', {...valid, head: {sha: 'other-head'}}],
  ['wrong base ref', {...valid, base: {...valid.base, ref: 'release'}}],
  ['wrong base repository', {...valid, base: {...valid.base, repo: {full_name: 'other/repo'}}}],
  ['missing base repository', {...valid, base: {ref: 'main', repo: null}}],
  ['human merger', {...valid, merged_by: {login: 'haofeng0705'}}],
  ['missing merger', {...valid, merged_by: null}],
  ['empty merge SHA', {...valid, merge_commit_sha: ''}],
  ['missing merge SHA', {...valid, merge_commit_sha: null}],
];
for (const [name, pull] of invalidCases) {
  let rejected = false;
  try {
    validateAppMergeFinalState(pull, 'head-sha', app, repository);
  } catch {
    rejected = true;
  }
  if (!rejected) {
    throw new Error(name + ' was accepted');
  }
}
for (const responseSha of [undefined, '', 'other-merge', null]) {
  let rejected = false;
  try {
    validateAppMergeFinalState(valid, 'head-sha', app, repository, responseSha);
  } catch {
    rejected = true;
  }
  if (!rejected) {
    throw new Error('invalid response SHA was accepted: ' + responseSha);
  }
}

if (requireSuccessfulMergeResponse({merged: true, sha: 'merge-sha'}) !== 'merge-sha') {
  throw new Error('successful merge response did not return its SHA');
}
for (const response of [
  {merged: false, sha: 'merge-sha', message: 'not ready'},
  {merged: true},
  {merged: true, sha: undefined},
  {merged: true, sha: ''},
]) {
  let rejected = false;
  try {
    requireSuccessfulMergeResponse(response);
  } catch {
    rejected = true;
  }
  if (!rejected) {
    throw new Error('invalid successful merge response was accepted: ' + JSON.stringify(response));
  }
}

for (const [name, pull, expected] of [
  ['behind', {mergeable: true, mergeable_state: 'behind'}, 'behind'],
  ['unknown', {mergeable: null, mergeable_state: 'unknown'}, 'unknown'],
  ['clean', {mergeable: true, mergeable_state: 'clean'}, 'attempt'],
  ['dirty', {mergeable: false, mergeable_state: 'dirty'}, 'attempt'],
  ['null blocked', {mergeable: null, mergeable_state: 'blocked'}, 'attempt'],
  ['missing mergeable', {mergeable_state: 'unknown'}, 'attempt'],
]) {
  const actual = classifyMergePreflight(pull);
  if (actual !== expected) {
    throw new Error(name + ' merge preflight = ' + actual + ', want ' + expected);
  }
}

for (const status of [405, 409]) {
  const recovery = classifyMergeAttemptRecovery(
    status,
    'not ready',
    {...valid, state: 'open', merged_at: null, merge_commit_sha: null},
    'head-sha',
    app,
    repository,
    undefined,
  );
  if (recovery.outcome !== 'not_ready') {
    throw new Error(status + ' open PR did not remain retriable');
  }
}
const behind = {
  ...valid,
  state: 'open',
  merged_at: null,
  merge_commit_sha: null,
  mergeable: true,
  mergeable_state: 'behind',
};
const behindRecovery = classifyMergeAttemptRecovery(
  403,
  'Resource not accessible by integration',
  behind,
  'head-sha',
  app,
  repository,
  undefined,
);
if (behindRecovery.outcome !== 'not_ready') {
  throw new Error('exact 403 behind-main denial did not remain retriable');
}
for (const responseSha of [undefined, 'merge-sha']) {
  const recovery = classifyMergeAttemptRecovery(
    409,
    'revision changed',
    valid,
    'head-sha',
    app,
    repository,
    responseSha,
  );
  if (recovery.outcome !== 'merged' || recovery.finalState.mergeCommitSha !== 'merge-sha') {
    throw new Error('valid concurrent App merge was not accepted');
  }
}
for (const status of [403, 404, 405, 409]) {
  const recovery = classifyMergeAttemptRecovery(
    status,
    status === 403 ? 'Resource not accessible by integration' : 'closed',
    {...valid, merged_at: null, merge_commit_sha: null},
    'head-sha',
    app,
    repository,
    undefined,
  );
  if (recovery.outcome !== 'closed') {
    throw new Error(status + ' closed-unmerged PR did not remain safely closed');
  }
}
for (const [name, status, message, pull, responseSha] of [
  ['open 404', 404, 'not found', {...valid, state: 'open', merged_at: null}, undefined],
  ['unsupported status', 500, 'server error', {...valid, state: 'open', merged_at: null}, undefined],
  ['human concurrent merge', 409, 'revision changed', {...valid, merged_by: {login: 'haofeng0705'}}, undefined],
  ['mismatched successful response SHA', 404, 'not found', valid, 'other-merge'],
  ['different 403 message', 403, 'API rate limit exceeded', behind, undefined],
  ['clean 403 state', 403, 'Resource not accessible by integration', {...behind, mergeable_state: 'clean'}, undefined],
  ['unknown 403 mergeability', 403, 'Resource not accessible by integration', {...behind, mergeable: null, mergeable_state: 'unknown'}, undefined],
  ['changed 403 head', 403, 'Resource not accessible by integration', {...behind, head: {sha: 'other-head'}}, undefined],
  ['wrong 403 base', 403, 'Resource not accessible by integration', {...behind, base: {...behind.base, ref: 'release'}}, undefined],
]) {
  let rejected = false;
  try {
    classifyMergeAttemptRecovery(
      status,
      message,
      pull,
      'head-sha',
      app,
      repository,
      responseSha,
    );
  } catch {
    rejected = true;
  }
  if (!rejected) {
    throw new Error(name + ' was accepted');
  }
}
`
	command := exec.Command(node, "-e", verification)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("App merge final-state verification failed: %v\n%s", runErr, output)
	}
}
