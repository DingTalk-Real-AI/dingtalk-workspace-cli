// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

'use strict';

const SKIP_WORKFLOW_PATTERN =
  /\[(?:skip ci|ci skip|no ci|skip actions|actions skip)\]|\bskip-checks\s*:\s*true\b/i;

function reviewedAppSlug(value) {
  const appSlug = typeof value === 'string' ? value.trim() : '';
  if (!appSlug || appSlug !== appSlug.toLowerCase() || appSlug === 'github-actions') {
    return '';
  }
  return appSlug;
}

function safeMergeMetadata(pullNumber) {
  if (!Number.isSafeInteger(pullNumber) || pullNumber <= 0) {
    throw new Error('pull number must be a positive safe integer');
  }
  return {
    headline: `Merge pull request #${pullNumber}`,
    body: `Merged by the dedicated Reviewer Router GitHub App for PR #${pullNumber}.`,
  };
}

function mergeTexts(pullRequest) {
  return [
    pullRequest?.title,
    pullRequest?.auto_merge?.commit_title,
    pullRequest?.auto_merge?.commit_message,
  ].filter(value => typeof value === 'string');
}

function hasWorkflowSkipDirective(pullRequest) {
  return mergeTexts(pullRequest).some(value => SKIP_WORKFLOW_PATTERN.test(value));
}

function requiresManualWorkflowMerge(files) {
  if (!Array.isArray(files)) {
    throw new Error('pull request files must be an array');
  }
  return files.some(file => {
    if (
      file === null ||
      typeof file !== 'object' ||
      Array.isArray(file) ||
      typeof file.filename !== 'string' ||
      !file.filename
    ) {
      throw new Error('pull request file entry must contain a non-empty filename');
    }
    return file.filename.startsWith('.github/workflows/');
  });
}

function classifyAutoMergeRequest({
  pullRequest,
  expectedAppOwner,
  safeCommitHeadline,
  safeCommitBody,
}) {
  if (!pullRequest?.auto_merge) {
    return 'none';
  }
  const enabledBy = pullRequest.auto_merge.enabled_by?.login?.toLowerCase();
  if (
    !hasWorkflowSkipDirective(pullRequest) &&
    enabledBy === expectedAppOwner &&
    pullRequest.auto_merge.commit_title === safeCommitHeadline &&
    pullRequest.auto_merge.commit_message === safeCommitBody
  ) {
    return 'safe_app';
  }
  return 'unsafe';
}

function classifyMergeDefaults(repository) {
  if (
    repository === null ||
    typeof repository !== 'object' ||
    Array.isArray(repository)
  ) {
    return 'invalid';
  }
  const hasTitle = Object.prototype.hasOwnProperty.call(
    repository,
    'merge_commit_title',
  );
  const hasMessage = Object.prototype.hasOwnProperty.call(
    repository,
    'merge_commit_message',
  );
  if (!hasTitle && !hasMessage) {
    return 'omitted';
  }
  if (!hasTitle || !hasMessage) {
    return 'invalid';
  }
  if (
    repository.merge_commit_title === 'MERGE_MESSAGE' &&
    ['PR_TITLE', 'BLANK'].includes(repository.merge_commit_message)
  ) {
    return 'reviewed';
  }
  return 'invalid';
}

function isStrictUpdateRule(rule) {
  if (rule?.type !== 'update') {
    return false;
  }
  if (!Object.prototype.hasOwnProperty.call(rule, 'parameters')) {
    return true;
  }
  const parameters = rule.parameters;
  if (
    parameters === null ||
    typeof parameters !== 'object' ||
    Array.isArray(parameters)
  ) {
    return false;
  }
  const parameterKeys = Object.keys(parameters);
  return (
    parameterKeys.length === 1 &&
    parameterKeys[0] === 'update_allows_fetch_and_merge' &&
    parameters.update_allows_fetch_and_merge === false
  );
}

function isStrictGraphQLUpdateRule(restRuleset, graphRuleset) {
  const restRulesetID = Number(restRuleset?.id);
  const graphRulesetID = Number(graphRuleset?.databaseId);
  const graphRules = graphRuleset?.rules;
  const graphRule = graphRules?.nodes?.[0];
  return (
    Number.isSafeInteger(restRulesetID) &&
    restRulesetID > 0 &&
    graphRulesetID === restRulesetID &&
    graphRuleset.name === restRuleset.name &&
    graphRuleset.enforcement === 'ACTIVE' &&
    graphRuleset.target === 'BRANCH' &&
    graphRules?.totalCount === 1 &&
    graphRules.nodes?.length === 1 &&
    graphRule?.type === 'UPDATE' &&
    graphRule.parameters?.__typename === 'UpdateParameters' &&
    graphRule.parameters.updateAllowsFetchAndMerge === false
  );
}

async function verifyBuiltInWriterBoundary({github, owner, repo}) {
  const {data: repository} = await github.rest.repos.get({owner, repo});
  const mergeDefaultsProjection = classifyMergeDefaults(repository);
  if (mergeDefaultsProjection === 'invalid') {
    throw new Error(
      'Repository merge-message defaults are malformed or changed from their reviewed values.',
    );
  }

  const repositorySource = `${owner}/${repo}`.toLowerCase();
  const appliedRules = await github.paginate(
    'GET /repos/{owner}/{repo}/rules/branches/{branch}',
    {owner, repo, branch: 'main', per_page: 100},
  );
  const applicableRulesetIDs = [
    ...new Set(
      appliedRules
        .filter(rule =>
          rule.ruleset_source_type === 'Repository' &&
          rule.ruleset_source?.toLowerCase() === repositorySource &&
          Number.isSafeInteger(Number(rule.ruleset_id)) &&
          Number(rule.ruleset_id) > 0,
        )
        .map(rule => Number(rule.ruleset_id)),
    ),
  ];
  const activeMainRulesets = [];
  for (const rulesetID of applicableRulesetIDs) {
    const {data: ruleset} = await github.request(
      'GET /repos/{owner}/{repo}/rulesets/{ruleset_id}',
      {owner, repo, ruleset_id: rulesetID},
    );
    if (
      ruleset.enforcement !== 'active' ||
      ruleset.target !== 'branch' ||
      ruleset.source_type !== 'Repository' ||
      ruleset.source?.toLowerCase() !== repositorySource
    ) {
      throw new Error(
        `Applicable repository ruleset ${ruleset.name || rulesetID} is not an active branch ruleset owned by this repository.`,
      );
    }
    activeMainRulesets.push(ruleset);
  }

  const writerRulesetName = 'main-merge-writers';
  const writerRulesets = activeMainRulesets.filter(
    ruleset => ruleset.name === writerRulesetName,
  );
  if (writerRulesets.length !== 1) {
    throw new Error(
      `Expected exactly one active ${writerRulesetName} ruleset on main; found ${writerRulesets.length}.`,
    );
  }
  const writerRuleset = writerRulesets[0];
  const writerIncludes = writerRuleset.conditions?.ref_name?.include || [];
  const writerExcludes = writerRuleset.conditions?.ref_name?.exclude || [];
  if (
    writerIncludes.length !== 1 ||
    writerIncludes[0] !== 'refs/heads/main' ||
    writerExcludes.length !== 0 ||
    typeof writerRuleset.node_id !== 'string' ||
    !writerRuleset.node_id ||
    writerRuleset.rules?.length !== 1 ||
    !isStrictUpdateRule(writerRuleset.rules[0]) ||
    writerRuleset.current_user_can_bypass !== 'never'
  ) {
    throw new Error(
      `${writerRulesetName} must target only refs/heads/main, contain only the strict update rule, and deny this built-in Actions identity any bypass.`,
    );
  }
  const {node: graphWriterRuleset} = await github.graphql(
    `query ReviewerRouterWriterRule($rulesetID: ID!) {
      node(id: $rulesetID) {
        ... on RepositoryRuleset {
          databaseId
          name
          enforcement
          target
          rules(first: 2) {
            totalCount
            nodes {
              type
              parameters {
                __typename
                ... on UpdateParameters {
                  updateAllowsFetchAndMerge
                }
              }
            }
          }
        }
      }
    }`,
    {rulesetID: writerRuleset.node_id},
  );
  if (!isStrictGraphQLUpdateRule(writerRuleset, graphWriterRuleset)) {
    throw new Error(
      `${writerRulesetName} must expose one strict UPDATE rule with updateAllowsFetchAndMerge=false through GraphQL.`,
    );
  }
  return {mergeDefaultsProjection, writerRuleset};
}

module.exports = {
  classifyAutoMergeRequest,
  classifyMergeDefaults,
  hasWorkflowSkipDirective,
  isStrictGraphQLUpdateRule,
  isStrictUpdateRule,
  requiresManualWorkflowMerge,
  reviewedAppSlug,
  safeMergeMetadata,
  verifyBuiltInWriterBoundary,
};
