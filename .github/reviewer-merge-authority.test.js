// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

'use strict';

const assert = require('node:assert/strict');
const {
  classifyAutoMergeRequest,
  classifyMergeDefaults,
  hasWorkflowSkipDirective,
  isStrictGraphQLUpdateRule,
  isStrictUpdateRule,
  requiresManualWorkflowMerge,
  reviewedAppSlug,
  safeMergeMetadata,
  verifyBuiltInWriterBoundary,
} = require('./reviewer-merge-authority.js');

const pullNumber = 123;
const expectedAppOwner = 'dingtalk-dws-reviewer-router[bot]';
const {headline, body} = safeMergeMetadata(pullNumber);

assert.equal(reviewedAppSlug(' dingtalk-dws-reviewer-router '), 'dingtalk-dws-reviewer-router');
assert.equal(reviewedAppSlug('DingTalk-DWS-Reviewer-Router'), '');
assert.equal(reviewedAppSlug('github-actions'), '');
assert.equal(reviewedAppSlug(''), '');
assert.throws(() => safeMergeMetadata(0), /positive safe integer/);

const exactAppPull = {
  title: 'ci: harden merge authority',
  auto_merge: {
    enabled_by: {login: expectedAppOwner},
    commit_title: headline,
    commit_message: body,
  },
};
assert.equal(
  classifyAutoMergeRequest({
    pullRequest: exactAppPull,
    expectedAppOwner,
    safeCommitHeadline: headline,
    safeCommitBody: body,
  }),
  'safe_app',
);
assert.equal(
  classifyAutoMergeRequest({
    pullRequest: {...exactAppPull, auto_merge: null},
    expectedAppOwner,
    safeCommitHeadline: headline,
    safeCommitBody: body,
  }),
  'none',
);
for (const unsafePull of [
  {
    ...exactAppPull,
    auto_merge: {...exactAppPull.auto_merge, enabled_by: {login: 'github-actions[bot]'}},
  },
  {
    ...exactAppPull,
    auto_merge: {...exactAppPull.auto_merge, enabled_by: {login: 'maintainer'}},
  },
  {
    ...exactAppPull,
    auto_merge: {...exactAppPull.auto_merge, commit_title: 'unsafe title'},
  },
  {
    ...exactAppPull,
    auto_merge: {...exactAppPull.auto_merge, commit_message: 'unsafe body'},
  },
  {...exactAppPull, title: 'ci: bypass [skip ci]'},
]) {
  assert.equal(
    classifyAutoMergeRequest({
      pullRequest: unsafePull,
      expectedAppOwner,
      safeCommitHeadline: headline,
      safeCommitBody: body,
    }),
    'unsafe',
  );
}
assert.equal(hasWorkflowSkipDirective({title: 'normal title'}), false);
assert.equal(hasWorkflowSkipDirective({title: 'unsafe [actions skip]'}), true);
assert.equal(
  hasWorkflowSkipDirective({title: 'normal', auto_merge: {commit_message: 'skip-checks: true'}}),
  true,
);
assert.equal(
  requiresManualWorkflowMerge([
    {filename: '.github/reviewer-router.js'},
    {filename: 'internal/app/app.go'},
  ]),
  false,
);
assert.equal(
  requiresManualWorkflowMerge([
    {filename: 'docs/ci-pr-gates.md'},
    {filename: '.github/workflows/ci.yml'},
  ]),
  true,
);
assert.throws(
  () => requiresManualWorkflowMerge([{filename: ''}]),
  /non-empty filename/,
);

assert.equal(classifyMergeDefaults({}), 'omitted');
assert.equal(
  classifyMergeDefaults({merge_commit_title: 'MERGE_MESSAGE', merge_commit_message: 'PR_TITLE'}),
  'reviewed',
);
assert.equal(classifyMergeDefaults({merge_commit_title: 'MERGE_MESSAGE'}), 'invalid');
assert.equal(classifyMergeDefaults(null), 'invalid');

assert.equal(isStrictUpdateRule({type: 'update'}), true);
assert.equal(
  isStrictUpdateRule({
    type: 'update',
    parameters: {update_allows_fetch_and_merge: false},
  }),
  true,
);
assert.equal(
  isStrictUpdateRule({
    type: 'update',
    parameters: {update_allows_fetch_and_merge: true},
  }),
  false,
);
assert.equal(isStrictUpdateRule({type: 'deletion'}), false);

const restRuleset = {id: 42, name: 'main-merge-writers'};
const graphRuleset = {
  databaseId: 42,
  name: 'main-merge-writers',
  enforcement: 'ACTIVE',
  target: 'BRANCH',
  rules: {
    totalCount: 1,
    nodes: [
      {
        type: 'UPDATE',
        parameters: {
          __typename: 'UpdateParameters',
          updateAllowsFetchAndMerge: false,
        },
      },
    ],
  },
};
assert.equal(isStrictGraphQLUpdateRule(restRuleset, graphRuleset), true);
assert.equal(
  isStrictGraphQLUpdateRule(restRuleset, {
    ...graphRuleset,
    rules: {
      ...graphRuleset.rules,
      nodes: [
        {
          ...graphRuleset.rules.nodes[0],
          parameters: {
            ...graphRuleset.rules.nodes[0].parameters,
            updateAllowsFetchAndMerge: true,
          },
        },
      ],
    },
  }),
  false,
);

async function testWriterBoundary() {
  const owner = 'DingTalk-Real-AI';
  const repo = 'dingtalk-workspace-cli';
  const repositorySource = `${owner}/${repo}`;
  const writerRuleset = {
    ...restRuleset,
    node_id: 'RULE_NODE',
    enforcement: 'active',
    target: 'branch',
    source_type: 'Repository',
    source: repositorySource,
    conditions: {
      ref_name: {include: ['refs/heads/main'], exclude: []},
    },
    rules: [{type: 'update'}],
    current_user_can_bypass: 'never',
  };
  function fakeGitHub(ruleset = writerRuleset) {
    return {
      rest: {
        repos: {
          get: async () => ({data: {}}),
        },
      },
      paginate: async () => [
        {
          ruleset_id: 42,
          ruleset_source_type: 'Repository',
          ruleset_source: repositorySource,
        },
      ],
      request: async () => ({data: ruleset}),
      graphql: async () => ({node: graphRuleset}),
    };
  }

  assert.deepEqual(
    await verifyBuiltInWriterBoundary({github: fakeGitHub(), owner, repo}),
    {mergeDefaultsProjection: 'omitted', writerRuleset},
  );
  await assert.rejects(
    verifyBuiltInWriterBoundary({
      github: fakeGitHub({
        ...writerRuleset,
        current_user_can_bypass: 'pull_requests_only',
      }),
      owner,
      repo,
    }),
    /deny this built-in Actions identity any bypass/,
  );
}

testWriterBoundary()
  .then(() => console.log('reviewer merge authority tests passed'))
  .catch(error => {
    console.error(error);
    process.exitCode = 1;
  });
