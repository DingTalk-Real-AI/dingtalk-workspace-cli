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

const reviewerRouterRequiredRulesetHelpers = `function hasExactStringSet(values, expected) {
  return (
    Array.isArray(values) &&
    values.length === expected.length &&
    new Set(values).size === expected.length &&
    expected.every(value => values.includes(value))
  );
}
function hasExactRequiredStatusChecks(checks) {
  if (
    !Array.isArray(checks) ||
    checks.length !== requiredCheckContexts.length
  ) {
    return false;
  }
  const contexts = checks.map(check => check?.context);
  return (
    new Set(contexts).size === requiredCheckContexts.length &&
    checks.every(
      check =>
        requiredCheckContexts.includes(check?.context) &&
        check?.integration_id === requiredCheckIntegrationID,
    )
  );
}
function hasExactMainRulesetScope(ruleset) {
  const includes = ruleset?.conditions?.ref_name?.include;
  const excludes = ruleset?.conditions?.ref_name?.exclude;
  return (
    ruleset?.enforcement === 'active' &&
    ruleset?.target === 'branch' &&
    ruleset?.source_type === 'Repository' &&
    ruleset?.source?.toLowerCase() === repositorySource &&
    Array.isArray(includes) &&
    includes.length === 1 &&
    includes[0] === 'refs/heads/main' &&
    Array.isArray(excludes) &&
    excludes.length === 0
  );
}
function isExactMainProtectionRuleset(ruleset) {
  if (
    !hasExactMainRulesetScope(ruleset) ||
    !Array.isArray(ruleset.rules) ||
    ruleset.rules.length !== 3 ||
    !hasExactStringSet(
      ruleset.rules.map(rule => rule?.type),
      ['deletion', 'non_fast_forward', 'pull_request'],
    )
  ) {
    return false;
  }
  const pullRequestRule = ruleset.rules.find(
    rule => rule.type === 'pull_request',
  );
  const parameters = pullRequestRule?.parameters;
  return (
    parameters?.required_approving_review_count === 1 &&
    parameters.require_last_push_approval === true &&
    parameters.require_extra_approval_for_unattributed_changes === true &&
    parameters.dismiss_stale_reviews_on_push === false &&
    parameters.require_code_owner_review === false &&
    parameters.required_review_thread_resolution === false &&
    hasExactStringSet(
      parameters.allowed_merge_methods,
      ['merge', 'squash', 'rebase'],
    ) &&
    parameters.dismissal_restriction?.enabled === false &&
    Array.isArray(parameters.dismissal_restriction.allowed_actors) &&
    parameters.dismissal_restriction.allowed_actors.length === 0 &&
    Array.isArray(parameters.required_reviewers) &&
    parameters.required_reviewers.length === 0
  );
}
function isExactMainQualityRuleset(ruleset) {
  if (
    !hasExactMainRulesetScope(ruleset) ||
    !Array.isArray(ruleset.rules) ||
    ruleset.rules.length !== 1 ||
    ruleset.rules[0]?.type !== 'required_status_checks'
  ) {
    return false;
  }
  const parameters = ruleset.rules[0].parameters;
  return (
    parameters?.strict_required_status_checks_policy === true &&
    parameters.do_not_enforce_on_create === false &&
    hasExactRequiredStatusChecks(
      parameters.required_status_checks,
    )
  );
}`

func TestReviewerRouterRequiresExactApprovalAndQualityRulesets(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to verify approval and quality ruleset semantics")
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
		if !strings.Contains(script, "function isExactMainProtectionRuleset(") {
			continue
		}
		checked++
		if got := strings.Count(script, reviewerRouterRequiredRulesetHelpers); got != 1 {
			t.Errorf("approval/quality ruleset helper count = %d, want 1", got)
		}
		for _, marker := range []string{
			"const requiredCheckIntegrationID = 15368;",
			"protectionRulesets.length !== 1",
			"protectionRulesets[0].current_user_can_bypass !== 'never'",
			"!isExactMainProtectionRuleset(protectionRulesets[0])",
			"qualityRulesets.length !== 1",
			"qualityRulesets[0].current_user_can_bypass !== 'never'",
			"!isExactMainQualityRuleset(qualityRulesets[0])",
			"exact non-bypassable main-protection approval ruleset",
			"exact non-bypassable main-quality ruleset with nine strict GitHub Actions checks",
		} {
			if !strings.Contains(script, marker) {
				t.Errorf("reconciliation script is missing live ruleset contract marker %q", marker)
			}
		}
	}
	if checked != 1 {
		t.Fatalf("approval/quality ruleset scripts checked = %d, want 1 privileged reconcile script", checked)
	}

	verification := `
const repositorySource = 'dingtalk-real-ai/dingtalk-workspace-cli';
const requiredCheckContexts = [
  'Lint',
  'Test',
  'Coverage',
  'Policy',
  'Edition',
  'Interface Integrity',
  'AI Behavior',
  'CLI Smoke',
  'Mock MCP',
];
const requiredCheckIntegrationID = 15368;
` + reviewerRouterRequiredRulesetHelpers + `
const clone = value => JSON.parse(JSON.stringify(value));
const scope = {
  enforcement: 'active',
  target: 'branch',
  source_type: 'Repository',
  source: 'DingTalk-Real-AI/dingtalk-workspace-cli',
  conditions: {ref_name: {include: ['refs/heads/main'], exclude: []}},
  current_user_can_bypass: 'never',
};
const protection = {
  ...scope,
  name: 'main-protection',
  rules: [
    {type: 'deletion'},
    {type: 'non_fast_forward'},
    {
      type: 'pull_request',
      parameters: {
        allowed_merge_methods: ['merge', 'squash', 'rebase'],
        dismiss_stale_reviews_on_push: false,
        dismissal_restriction: {allowed_actors: [], enabled: false},
        require_code_owner_review: false,
        require_extra_approval_for_unattributed_changes: true,
        require_last_push_approval: true,
        required_approving_review_count: 1,
        required_review_thread_resolution: false,
        required_reviewers: [],
      },
    },
  ],
};
const quality = {
  ...scope,
  name: 'main-quality',
  rules: [{
    type: 'required_status_checks',
    parameters: {
      do_not_enforce_on_create: false,
      strict_required_status_checks_policy: true,
      required_status_checks: requiredCheckContexts.map(context => ({
        context,
        integration_id: requiredCheckIntegrationID,
      })),
    },
  }],
};
if (!isExactMainProtectionRuleset(protection)) {
  throw new Error('valid main-protection ruleset was rejected');
}
if (!isExactMainQualityRuleset(quality)) {
  throw new Error('valid main-quality ruleset was rejected');
}

const invalidProtection = [];
let candidate = clone(protection);
candidate.enforcement = 'disabled';
invalidProtection.push(['disabled', candidate]);
candidate = clone(protection);
candidate.conditions.ref_name.include = ['~ALL'];
invalidProtection.push(['wrong scope', candidate]);
candidate = clone(protection);
candidate.rules = candidate.rules.filter(rule => rule.type !== 'pull_request');
invalidProtection.push(['missing pull-request rule', candidate]);
for (const [name, key, value] of [
  ['missing approval', 'required_approving_review_count', 0],
  ['latest-push approval disabled', 'require_last_push_approval', false],
  ['unattributed approval disabled', 'require_extra_approval_for_unattributed_changes', false],
]) {
  candidate = clone(protection);
  candidate.rules.find(rule => rule.type === 'pull_request').parameters[key] = value;
  invalidProtection.push([name, candidate]);
}
for (const [name, value] of invalidProtection) {
  if (isExactMainProtectionRuleset(value)) {
    throw new Error(name + ' main-protection ruleset was accepted');
  }
}

const invalidQuality = [];
candidate = clone(quality);
candidate.enforcement = 'disabled';
invalidQuality.push(['disabled', candidate]);
candidate = clone(quality);
candidate.rules = [];
invalidQuality.push(['missing status-check rule', candidate]);
candidate = clone(quality);
candidate.rules[0].parameters.strict_required_status_checks_policy = false;
invalidQuality.push(['non-strict', candidate]);
candidate = clone(quality);
candidate.rules[0].parameters.do_not_enforce_on_create = true;
invalidQuality.push(['create bypass', candidate]);
candidate = clone(quality);
candidate.rules[0].parameters.required_status_checks.pop();
invalidQuality.push(['missing check', candidate]);
candidate = clone(quality);
candidate.rules[0].parameters.required_status_checks[8].context = 'Lint';
invalidQuality.push(['duplicate check', candidate]);
candidate = clone(quality);
candidate.rules[0].parameters.required_status_checks.push({
  context: 'Unexpected',
  integration_id: requiredCheckIntegrationID,
});
invalidQuality.push(['extra check', candidate]);
candidate = clone(quality);
delete candidate.rules[0].parameters.required_status_checks[0].integration_id;
invalidQuality.push(['missing integration', candidate]);
candidate = clone(quality);
candidate.rules[0].parameters.required_status_checks[0].integration_id = 1;
invalidQuality.push(['wrong integration', candidate]);
candidate = clone(quality);
candidate.rules[0].parameters.required_status_checks[8] = {
  context: 'Lint',
  integration_id: 1,
};
invalidQuality.push(['duplicate context from another integration', candidate]);
for (const [name, value] of invalidQuality) {
  if (isExactMainQualityRuleset(value)) {
    throw new Error(name + ' main-quality ruleset was accepted');
  }
}
`
	command := exec.Command(node, "-e", verification)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("approval/quality ruleset verification failed: %v\n%s", runErr, output)
	}
}
