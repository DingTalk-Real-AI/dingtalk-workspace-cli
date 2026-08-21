// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type reviewerRouterWorkflow struct {
	On          map[string]reviewerRouterTrigger `yaml:"on"`
	Permissions map[string]string                `yaml:"permissions"`
	Concurrency map[string]string                `yaml:"concurrency"`
	Jobs        map[string]reviewerRouterJob     `yaml:"jobs"`
}

type reviewerRouterTrigger struct {
	Branches []string `yaml:"branches"`
	Types    []string `yaml:"types"`
}

type reviewerRouterJob struct {
	If          string               `yaml:"if"`
	Permissions map[string]string    `yaml:"permissions"`
	Steps       []reviewerRouterStep `yaml:"steps"`
}

type reviewerRouterStep struct {
	ID   string            `yaml:"id"`
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Env  map[string]string `yaml:"env"`
	With map[string]string `yaml:"with"`
}

func TestReviewerRouterWorkflowContract(t *testing.T) {
	t.Parallel()

	path, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "reviewer-router.yml"))
	if err != nil {
		t.Fatalf("Abs(reviewer-router.yml) error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	var workflow reviewerRouterWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("yaml.Unmarshal(%s) error = %v", path, err)
	}
	policyPath := filepath.Join(filepath.Dir(filepath.Dir(path)), "reviewer-routing.js")
	policy, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", policyPath, err)
	}
	for _, want := range []string{
		"'wxianfeng'",
		"'typefield'",
		"'haofeng0705'",
		"'hlzjsong'",
		"resolveReviewRouting",
		"unknown_paths",
	} {
		if !strings.Contains(string(policy), want) {
			t.Errorf("reviewer routing policy is missing contract marker %q", want)
		}
	}

	if len(workflow.On) != 3 {
		t.Fatalf("workflow triggers = %v, want pull_request_target plus protected-main reconciliation", workflow.On)
	}
	trigger, ok := workflow.On["pull_request_target"]
	if !ok {
		t.Fatalf("workflow triggers = %v, want pull_request_target", workflow.On)
	}
	if wantBranches := []string{"main"}; !reflect.DeepEqual(trigger.Branches, wantBranches) {
		t.Fatalf("pull_request_target branches = %v, want %v", trigger.Branches, wantBranches)
	}
	wantTypes := []string{"opened", "synchronize", "reopened", "ready_for_review", "edited", "auto_merge_enabled"}
	if !reflect.DeepEqual(trigger.Types, wantTypes) {
		t.Fatalf("pull_request_target types = %v, want %v", trigger.Types, wantTypes)
	}
	if _, ok := workflow.On["workflow_dispatch"]; !ok {
		t.Fatalf("workflow triggers = %v, want workflow_dispatch reconciliation", workflow.On)
	}
	push, ok := workflow.On["push"]
	if !ok || !reflect.DeepEqual(push.Branches, []string{"main"}) {
		t.Fatalf("push trigger = %v, want protected main only", push)
	}

	wantPermissions := map[string]string{
		"contents":      "read",
		"pull-requests": "write",
	}
	if !reflect.DeepEqual(workflow.Permissions, wantPermissions) {
		t.Fatalf("workflow permissions = %v, want exactly %v", workflow.Permissions, wantPermissions)
	}
	wantConcurrency := map[string]string{
		"group":              "reviewer-router-${{ github.event_name == 'pull_request_target' && format('pr-{0}', github.event.pull_request.number) || 'reconcile-main' }}",
		"cancel-in-progress": "${{ github.event_name == 'pull_request_target' }}",
	}
	if !reflect.DeepEqual(workflow.Concurrency, wantConcurrency) {
		t.Fatalf("workflow concurrency = %v, want serialized reconciliation and cancellable PR events %v", workflow.Concurrency, wantConcurrency)
	}

	if len(workflow.Jobs) != 3 {
		t.Fatalf("workflow jobs = %v, want event routing plus controlled reconciliation", workflow.Jobs)
	}
	job, ok := workflow.Jobs["route"]
	if !ok {
		t.Fatalf("workflow jobs = %v, want route", workflow.Jobs)
	}
	if job.If != "github.event_name == 'pull_request_target' && github.event.pull_request.draft == false" {
		t.Fatalf("route.if = %q, want non-draft guard", job.If)
	}
	const checkoutSHA = "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683"
	const githubScriptSHA = "actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b"
	const appTokenSHA = "actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1"
	if len(job.Steps) != 2 ||
		job.Steps[0].Uses != checkoutSHA ||
		job.Steps[0].With["ref"] != "${{ github.event.pull_request.base.sha }}" ||
		job.Steps[0].With["persist-credentials"] != "false" ||
		job.Steps[1].Uses != githubScriptSHA {
		t.Fatalf("route steps = %#v, want trusted base checkout followed by routing only", job.Steps)
	}

	autoMergeJob, ok := workflow.Jobs["manage-auto-merge"]
	if !ok {
		t.Fatalf("workflow jobs = %v, want manage-auto-merge", workflow.Jobs)
	}
	if autoMergeJob.If != "github.event_name == 'pull_request_target' && github.event.pull_request.draft == false" {
		t.Fatalf("manage-auto-merge.if = %q, want exact ready event guard", autoMergeJob.If)
	}
	wantAutoPermissions := map[string]string{
		"contents":      "write",
		"pull-requests": "write",
	}
	if !reflect.DeepEqual(autoMergeJob.Permissions, wantAutoPermissions) {
		t.Fatalf("manage-auto-merge permissions = %v, want exactly %v", autoMergeJob.Permissions, wantAutoPermissions)
	}
	if len(autoMergeJob.Steps) != 3 ||
		autoMergeJob.Steps[0].Uses != githubScriptSHA ||
		autoMergeJob.Steps[1].ID != "reviewer-router-token" ||
		autoMergeJob.Steps[1].Uses != appTokenSHA ||
		autoMergeJob.Steps[2].Uses != githubScriptSHA {
		t.Fatalf("manage-auto-merge steps = %#v, want unsafe-owner cleanup before dedicated App enable", autoMergeJob.Steps)
	}

	appToken := autoMergeJob.Steps[1]
	if len(appToken.With) != 6 {
		t.Fatalf("reviewer-router token inputs = %v, want exactly the reviewed repository scope and two permissions", appToken.With)
	}
	for key, want := range map[string]string{
		"client-id":                "${{ vars.REVIEWER_ROUTER_APP_CLIENT_ID }}",
		"private-key":              "${{ secrets.REVIEWER_ROUTER_APP_PRIVATE_KEY }}",
		"owner":                    "${{ github.repository_owner }}",
		"repositories":             "${{ github.event.repository.name }}",
		"permission-contents":      "write",
		"permission-pull-requests": "write",
	} {
		if got := appToken.With[key]; got != want {
			t.Errorf("reviewer-router token input %q = %q, want %q", key, got, want)
		}
	}
	if _, ok := appToken.With["skip-token-revoke"]; ok {
		t.Error("reviewer-router App token must be revoked automatically after the job")
	}

	routingScript := job.Steps[1].With["script"]
	for _, want := range []string{
		"REVIEWER_POOL",
		"resolveReviewRouting",
		"github.rest.pulls.get",
		"github.rest.pulls.listFiles",
		"currentPull.head.sha !== eventHeadSha",
		"currentPull.base.sha !== eventBaseSha",
		"currentPull.state !== 'open'",
		"currentPull.draft",
		"currentPull.base.ref !== 'main'",
		"getReadyEventPull('review request')",
		"context.payload.action === 'synchronize'",
		"context.payload.sender?.login?.toLowerCase()",
		"reviewer.toLowerCase() !== author",
		"reviewer.toLowerCase() !== latestPusher",
		"pullRequest.requested_reviewers",
		"existingRequestedReviewers",
		"github.rest.pulls.listReviews",
		"['APPROVED', 'CHANGES_REQUESTED', 'DISMISSED'].includes",
		"currentHeadReviewers",
		"review.commit_id === headSha",
		"['APPROVED', 'CHANGES_REQUESTED'].includes(review.state)",
		"review.state === 'CHANGES_REQUESTED'",
		"staleChangeRequester",
		"state: 'open'",
		"loads.set(candidate, loads.get(candidate) + 1)",
		"loads.get(left) - loads.get(right)",
		"routing.requiredReviewers",
		"reviewerCandidates",
		"requestReviewersWithFallback",
		"satisfiedReviewers",
		"github.rest.pulls.requestReviewers",
		"trying the next candidate",
		"Reviewer routing hit an unexpected error",
		"core.warning",
	} {
		if !strings.Contains(routingScript, want) {
			t.Errorf("reviewer router script is missing contract marker %q", want)
		}
	}
	for _, forbidden := range []string{
		"enablePullRequestAutoMerge",
		"disablePullRequestAutoMerge",
		"mergeMethod: MERGE",
	} {
		if strings.Contains(routingScript, forbidden) {
			t.Errorf("built-in routing token must not control auto-merge through %q", forbidden)
		}
	}

	cleanupStep := autoMergeJob.Steps[0]
	if _, ok := cleanupStep.With["github-token"]; ok {
		t.Error("unsafe-owner cleanup must use only the job-scoped built-in token")
	}
	cleanupScript := cleanupStep.With["script"]
	for _, want := range []string{
		"currentPull.head.sha !== eventHeadSha",
		"currentPull.base.sha !== eventBaseSha",
		"currentPull.state !== 'open'",
		"currentPull.draft",
		"currentPull.base.ref !== 'main'",
		"const unsafeOwner = 'github-actions[bot]'",
		"const skipWorkflowPattern =",
		"currentPull.title",
		"currentPull.auto_merge?.commit_title",
		"currentPull.auto_merge?.commit_message",
		"const skipRequested =",
		"enabledBy !== unsafeOwner",
		"disablePullRequestAutoMerge",
		"cleanedPull.auto_merge",
		"Cleared workflow-skipping auto-merge metadata",
		"throw new Error",
	} {
		if !strings.Contains(cleanupScript, want) {
			t.Errorf("unsafe-owner cleanup is missing contract marker %q", want)
		}
	}
	if strings.Contains(cleanupScript, "enablePullRequestAutoMerge") {
		t.Error("built-in cleanup token must never enable auto-merge")
	}
	if got := strings.Count(cleanupScript, "const skipWorkflowPattern ="); got != 1 {
		t.Errorf("unsafe-request cleanup skip-workflow pattern declarations = %d, want exactly 1", got)
	}

	autoMergeStep := autoMergeJob.Steps[2]
	if got, want := autoMergeStep.With["github-token"], "${{ steps.reviewer-router-token.outputs.token }}"; got != want {
		t.Fatalf("auto-merge github-token = %q, want %q", got, want)
	}
	if got, want := autoMergeStep.Env["MINTED_REVIEWER_ROUTER_APP_SLUG"], "${{ steps.reviewer-router-token.outputs.app-slug }}"; got != want {
		t.Fatalf("minted auto-merge App slug = %q, want %q", got, want)
	}
	if got, want := autoMergeStep.Env["REVIEWER_ROUTER_APP_SLUG"], "${{ vars.REVIEWER_ROUTER_APP_SLUG }}"; got != want {
		t.Fatalf("configured auto-merge App slug = %q, want %q", got, want)
	}
	autoMergeScript := autoMergeStep.With["script"]
	for _, want := range []string{
		"github.rest.pulls.get",
		"currentPull.head.sha !== eventHeadSha",
		"currentPull.base.sha !== eventBaseSha",
		"currentPull.state !== 'open'",
		"currentPull.draft",
		"currentPull.base.ref !== 'main'",
		"getReadyEventPull('auto-merge enable')",
		"process.env.MINTED_REVIEWER_ROUTER_APP_SLUG?.trim()",
		"process.env.REVIEWER_ROUTER_APP_SLUG?.trim()",
		"appSlug === 'github-actions'",
		"mintedAppSlug !== appSlug",
		"const expectedAppOwner = `${appSlug}[bot]`",
		"GET /repos/{owner}/{repo}/rules/branches/{branch}",
		".map(rule => Number(rule.ruleset_id))",
		"Number.isSafeInteger(rulesetID) && rulesetID > 0",
		"GET /repos/{owner}/{repo}/rulesets/{ruleset_id}",
		"ruleset.enforcement !== 'active'",
		"ruleset.target !== 'branch'",
		"ruleset.name === 'main-merge-writers'",
		"writerRuleset.source_type !== 'Repository'",
		"writerRuleset.source?.toLowerCase() !== repositorySource",
		"writerIncludes[0] !== 'refs/heads/main'",
		"writerExcludes.length !== 0",
		"writerRuleset.rules?.length !== 1",
		"writerRuleset.rules[0].type !== 'update'",
		"update_allows_fetch_and_merge !== false",
		"current_user_can_bypass !== 'pull_requests_only'",
		"ruleset.current_user_can_bypass !== 'never'",
		"Reviewer Router App must never bypass main ruleset",
		"const skipWorkflowPattern =",
		"const safeCommitHeadline = `Merge pull request #${pullNumber}`",
		"const safeCommitBody =",
		"workflow-skip cleanup",
		"already has the reviewed ${expectedAppOwner} auto-merge request",
		"preserving it",
		"non-dedicated owner cleanup",
		"unsafe dedicated-App metadata cleanup",
		"getReadyEventPull('post-owner cleanup')",
		"enablePullRequestAutoMerge",
		"mergeMethod: MERGE",
		"expectedHeadOid: $expectedHeadOid",
		"commitHeadline: $commitHeadline",
		"commitBody: $commitBody",
		"expectedHeadOid: eventHeadSha",
		"const exactRevision =",
		"enabledBy === expectedAppOwner",
		"disablePullRequestAutoMerge",
		"unsafe-merge-message cleanup",
		"enabledPull.auto_merge?.commit_title !== safeCommitHeadline",
		"enabledPull.auto_merge?.commit_message !== safeCommitBody",
		"if (revertedPull.auto_merge)",
		"enabledBy !== expectedAppOwner",
	} {
		if !strings.Contains(autoMergeScript, want) {
			t.Errorf("dedicated auto-merge script is missing contract marker %q", want)
		}
	}
	if got := strings.Count(autoMergeScript, "const skipWorkflowPattern ="); got != 1 {
		t.Errorf("dedicated auto-merge script skip-workflow pattern declarations = %d, want exactly 1", got)
	}
	for _, want := range []string{
		"if (!exactRevision && enabledBy === expectedAppOwner)",
		"disableAutoMerge(enabledPull, 'stale-enable cleanup')",
		"existingOwner === expectedAppOwner &&",
		"disableAutoMerge(currentPull, 'workflow-skip cleanup')",
		"currentPull.auto_merge.commit_title === safeCommitHeadline",
		"currentPull.auto_merge.commit_message === safeCommitBody",
		"disableAutoMerge(enabledPull, 'unexpected owner cleanup')",
	} {
		if !strings.Contains(autoMergeScript, want) {
			t.Errorf("dedicated App stale-state cleanup is missing %q", want)
		}
	}
	for _, forbidden := range []string{"core.warning", "catch ("} {
		if strings.Contains(autoMergeScript, forbidden) {
			t.Errorf("dedicated auto-merge must fail closed instead of handling errors through %q", forbidden)
		}
	}
	for _, forbidden := range []string{
		"rule.ruleset_source_type",
		"rule.ruleset_source?.toLowerCase()",
		"ruleset.source_type !== 'Repository'",
		"ruleset.source?.toLowerCase() !== repositorySource",
	} {
		if strings.Contains(autoMergeScript, forbidden) {
			t.Errorf("dedicated App must inspect every applicable main ruleset, not prefilter through %q", forbidden)
		}
	}

	reconcile, ok := workflow.Jobs["reconcile"]
	if !ok {
		t.Fatalf("workflow jobs = %v, want reconcile", workflow.Jobs)
	}
	if reconcile.If != "(github.event_name == 'push' || github.event_name == 'workflow_dispatch') && github.ref == 'refs/heads/main' && github.repository == 'DingTalk-Real-AI/dingtalk-workspace-cli'" {
		t.Fatalf("reconcile.if = %q, want exact protected-main manual guard", reconcile.If)
	}
	if len(reconcile.Steps) != 2 ||
		reconcile.Steps[0].ID != "reviewer-router-reconcile-token" ||
		reconcile.Steps[0].Uses != appTokenSHA ||
		reconcile.Steps[1].Uses != githubScriptSHA {
		t.Fatalf("reconcile steps = %#v, want scoped App token followed by pinned migration script", reconcile.Steps)
	}
	if len(reconcile.Permissions) != 0 {
		t.Fatalf("reconcile built-in token permissions = %v, want none", reconcile.Permissions)
	}
	if len(reconcile.Steps[0].With) != 6 {
		t.Fatalf("reconcile token inputs = %v, want exactly the reviewed repository scope and two permissions", reconcile.Steps[0].With)
	}
	for key, want := range map[string]string{
		"client-id":                "${{ vars.REVIEWER_ROUTER_APP_CLIENT_ID }}",
		"private-key":              "${{ secrets.REVIEWER_ROUTER_APP_PRIVATE_KEY }}",
		"owner":                    "${{ github.repository_owner }}",
		"repositories":             "${{ github.event.repository.name }}",
		"permission-contents":      "write",
		"permission-pull-requests": "write",
	} {
		if got := reconcile.Steps[0].With[key]; got != want {
			t.Errorf("reconcile token input %q = %q, want %q", key, got, want)
		}
	}
	if _, ok := reconcile.Steps[0].With["skip-token-revoke"]; ok {
		t.Error("reconcile App token must be revoked automatically after the job")
	}
	if got, want := reconcile.Steps[1].With["github-token"], "${{ steps.reviewer-router-reconcile-token.outputs.token }}"; got != want {
		t.Fatalf("reconcile github-token = %q, want %q", got, want)
	}
	if got, want := reconcile.Steps[1].Env["MINTED_REVIEWER_ROUTER_APP_SLUG"], "${{ steps.reviewer-router-reconcile-token.outputs.app-slug }}"; got != want {
		t.Fatalf("minted reconcile App slug = %q, want %q", got, want)
	}
	if got, want := reconcile.Steps[1].Env["REVIEWER_ROUTER_APP_SLUG"], "${{ vars.REVIEWER_ROUTER_APP_SLUG }}"; got != want {
		t.Fatalf("configured reconcile App slug = %q, want %q", got, want)
	}
	reconcileScript := reconcile.Steps[1].With["script"]
	for _, want := range []string{
		"github.paginate(github.rest.pulls.list",
		"state: 'open'",
		"base: 'main'",
		"candidate.draft",
		"const candidateOwner =",
		"const candidateSkipRequested =",
		"const candidateHasSafeAppRequest =",
		"candidateOwner === expectedAppOwner",
		"!candidate.auto_merge",
		"candidateHasSafeAppRequest",
		"currentPull.head.sha !== expectedHeadSha",
		"currentPull.base.sha !== expectedBaseSha",
		"currentPull.state !== 'open'",
		"currentPull.draft",
		"currentPull.base.ref !== 'main'",
		"const currentHasSafeAppRequest =",
		"currentOwner === expectedAppOwner",
		"currentHasSafeAppRequest",
		"async function disableAutoMerge(pullNumber, pullRequestId, phase)",
		"disablePullRequestAutoMerge",
		"if (revertedPull.auto_merge)",
		"return revertedPull",
		"replace non-dedicated auto-merge owner",
		"repair dedicated-App merge metadata",
		"enablePullRequestAutoMerge",
		"mergeMethod: MERGE",
		"const expectedAppOwner = `${appSlug}[bot]`",
		"GET /repos/{owner}/{repo}/rules/branches/{branch}",
		".map(rule => Number(rule.ruleset_id))",
		"Number.isSafeInteger(rulesetID) && rulesetID > 0",
		"GET /repos/{owner}/{repo}/rulesets/{ruleset_id}",
		"ruleset.enforcement !== 'active'",
		"ruleset.target !== 'branch'",
		"ruleset.name === 'main-merge-writers'",
		"writerRuleset.source_type !== 'Repository'",
		"writerRuleset.source?.toLowerCase() !== repositorySource",
		"writerIncludes[0] !== 'refs/heads/main'",
		"writerExcludes.length !== 0",
		"writerRuleset.rules?.length !== 1",
		"writerRuleset.rules[0].type !== 'update'",
		"update_allows_fetch_and_merge !== false",
		"current_user_can_bypass !== 'pull_requests_only'",
		"ruleset.current_user_can_bypass !== 'never'",
		"Reviewer Router App must never bypass main ruleset",
		"const skipWorkflowPattern =",
		"const skipRequested =",
		"workflow-skip directive found in merge metadata; left manual-merge only",
		"const safeCommitHeadline = `Merge pull request #${candidate.number}`",
		"const safeCommitBody =",
		"expectedHeadOid: $expectedHeadOid",
		"commitHeadline: $commitHeadline",
		"commitBody: $commitBody",
		"expectedHeadOid: expectedHeadSha",
		"enabledPull.auto_merge?.commit_title !== safeCommitHeadline",
		"enabledPull.auto_merge?.commit_message !== safeCommitBody",
		"const failures = []",
		"catch (error)",
		"failures.push(failure)",
		"core.setFailed",
		"revisionChanged && enabledBy === expectedAppOwner",
		"enabledBy !== expectedAppOwner",
		"unexpected owner cleanup",
		"unsafe merge-message cleanup",
	} {
		if !strings.Contains(reconcileScript, want) {
			t.Errorf("reconciliation script is missing contract marker %q", want)
		}
	}
	if disableIndex, enableIndex := strings.Index(reconcileScript, "disablePullRequestAutoMerge"), strings.Index(reconcileScript, "enablePullRequestAutoMerge"); disableIndex < 0 || enableIndex <= disableIndex {
		t.Error("legacy reconciliation must disable the exact unsafe owner before App-token enable")
	}
	if got := strings.Count(reconcileScript, "const skipWorkflowPattern ="); got != 1 {
		t.Errorf("reconciliation script skip-workflow pattern declarations = %d, want exactly 1", got)
	}
	if strings.Contains(reconcileScript, "const unsafeOwner") {
		t.Error("reconciliation must converge every non-App owner, not only the legacy built-in owner")
	}
	for _, forbidden := range []string{
		"rule.ruleset_source_type",
		"rule.ruleset_source?.toLowerCase()",
		"ruleset.source_type !== 'Repository'",
		"ruleset.source?.toLowerCase() !== repositorySource",
	} {
		if strings.Contains(reconcileScript, forbidden) {
			t.Errorf("reconciliation App must inspect every applicable main ruleset, not prefilter through %q", forbidden)
		}
	}

	for _, forbidden := range []string{
		"['APPROVED', 'CHANGES_REQUESTED', 'COMMENTED']",
		"PeterGuy326",
		"secrets.HOMEBREW_PR_TOKEN",
		"secrets.RELEASE_GOVERNANCE_TOKEN",
		"secrets.GITHUB_TOKEN",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("reviewer router must not contain %q", forbidden)
		}
	}
}

func TestCodeAdmissionEnforcesReviewerRouterWriterBoundary(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("ReadFile(ci.yml) error = %v", err)
	}
	workflow := string(data)
	if !strings.Contains(workflow, "pull_request:\n    types: [opened, synchronize, reopened, ready_for_review, edited, auto_merge_enabled, auto_merge_disabled]") {
		t.Error("CI must rerun admission when a draft becomes ready or merge metadata changes")
	}
	start := strings.Index(workflow, "\n  test:\n")
	end := strings.Index(workflow, "\n  test-darwin:\n")
	if start < 0 || end <= start {
		t.Fatal("CI workflow missing Test aggregate boundaries")
	}
	testJob := workflow[start:end]
	for _, want := range []string{
		"contents: read",
		"pull-requests: read",
		"name: Verify auto-merge identity",
		"if: github.event_name == 'pull_request' && github.event.pull_request.draft == false",
		"REVIEWER_ROUTER_APP_SLUG: ${{ vars.REVIEWER_ROUTER_APP_SLUG }}",
		"process.env.REVIEWER_ROUTER_APP_SLUG?.trim()",
		"const expectedAppOwner = `${appSlug}[bot]`",
		"github.rest.pulls.get",
		"github.rest.repos.get",
		"repository.merge_commit_title !== 'MERGE_MESSAGE'",
		"!['PR_TITLE', 'BLANK'].includes(repository.merge_commit_message)",
		"const writerRulesetName = 'main-merge-writers'",
		"GET /repos/{owner}/{repo}/rules/branches/{branch}",
		"rule.ruleset_source_type === 'Repository'",
		"rule.ruleset_source?.toLowerCase() === repositorySource",
		"GET /repos/{owner}/{repo}/rulesets/{ruleset_id}",
		"ruleset.enforcement !== 'active'",
		"ruleset.target !== 'branch'",
		"ruleset.source_type !== 'Repository'",
		"ruleset.source?.toLowerCase() !== repositorySource",
		"writerIncludes[0] !== 'refs/heads/main'",
		"writerExcludes.length !== 0",
		"writerRuleset.rules?.length !== 1",
		"writerRuleset.rules[0].type !== 'update'",
		"update_allows_fetch_and_merge !== false",
		"writerRuleset.current_user_can_bypass !== 'never'",
		"deny this built-in Actions identity any bypass",
		"const skipWorkflowPattern =",
		"currentPull.title",
		"currentPull.auto_merge?.commit_title",
		"currentPull.auto_merge?.commit_message",
		"merge metadata contains a GitHub workflow-skip directive",
		"currentPull.head.sha !== eventHeadSha",
		"currentPull.base.sha !== eventBaseSha",
		"currentPull.state !== 'open'",
		"currentPull.draft",
		"currentPull.base.ref !== 'main'",
		"currentPull.auto_merge",
		"enabled_by?.login",
		"enabledBy === expectedAppOwner",
		"currentPull.auto_merge.commit_title === safeCommitHeadline",
		"currentPull.auto_merge.commit_message === safeCommitBody",
		"must be null or owned by ${expectedAppOwner}",
		"const maxAttempts = 6",
		"setTimeout(resolve, 5000)",
		"core.setFailed",
	} {
		if !strings.Contains(testJob, want) {
			t.Errorf("Test aggregate missing auto-merge identity contract %q", want)
		}
	}
	if !strings.Contains(testJob, "Null and non-built-in merge identities emit either the protected-main") ||
		!strings.Contains(testJob, "main-merge-writers never lets it update main") {
		t.Error("auto-merge identity comment must name the push/repair paths and built-in writer self-check")
	}
	for _, forbidden := range []string{
		"REVIEWER_ROUTER_APP_PRIVATE_KEY",
		"HOMEBREW_PR_TOKEN",
		"RELEASE_GOVERNANCE_TOKEN",
		"can emit protected-main workflow events",
		"pull-requests: write",
		"contents: write",
		"actions: write",
	} {
		if strings.Contains(testJob, forbidden) {
			t.Errorf("read-only admission identity check must not consume %q", forbidden)
		}
	}
}
