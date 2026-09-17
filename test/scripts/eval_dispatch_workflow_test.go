// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalDispatchWorkflowUsesRepositoryPermissionAndReviewedSHA(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "eval-dispatch.yml"))
	if err != nil {
		t.Fatalf("read eval-dispatch workflow: %v", err)
	}
	workflow := string(data)

	for _, want := range []string{
		"/collaborators/${COMMENTER}/permission",
		"eval_dispatch_guard.py permission",
		"PR_AUTHOR: ${{ github.event.issue.user.login }}",
		"EVAL_ALLOWLIST_PATH: .github/eval-allowlist.txt",
		"REVIEWED_SHA: ${{ steps.parse.outputs.reviewed_sha }}",
		"eval_dispatch_guard.py head",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("eval-dispatch workflow missing security contract %q", want)
		}
	}
	if strings.Contains(workflow, "author_association") {
		t.Error("eval-dispatch workflow must not authorize from author_association")
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "eval-allowlist.txt")); err != nil {
		t.Errorf("eval allowlist file missing on default branch: %v", err)
	}
}

func TestEvalDispatchWorkflowCanWritePRConversationComments(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "eval-dispatch.yml"))
	if err != nil {
		t.Fatalf("read eval-dispatch workflow: %v", err)
	}
	workflow := string(data)

	dispatchStart := strings.Index(workflow, "jobs:\n  dispatch:")
	if dispatchStart < 0 {
		t.Fatal("eval-dispatch workflow missing dispatch job")
	}
	stepsOffset := strings.Index(workflow[dispatchStart:], "\n    steps:")
	if stepsOffset < 0 {
		t.Fatal("eval-dispatch workflow missing dispatch steps")
	}
	permissions := workflow[dispatchStart : dispatchStart+stepsOffset]
	if !containsTrimmedLine(permissions, "pull-requests: write") {
		t.Error("dispatch job must grant write permission for PR conversation comments")
	}
	if containsTrimmedLine(permissions, "pull-requests: read") {
		t.Error("dispatch job still limits pull requests to read-only")
	}
	if containsTrimmedLine(permissions, "issues: write") {
		t.Error("dispatch job must keep a single write domain instead of granting issue-wide writes")
	}

	commentWrites := []struct {
		step     string
		method   string
		endpoint string
	}{
		{
			step:     "Reply usage on parse failure",
			method:   "POST",
			endpoint: "issues/${PR_NUMBER}/comments",
		},
		{
			step:     "Create dispatch placeholder",
			method:   "POST",
			endpoint: "issues/${PR_NUMBER}/comments",
		},
		{
			step:     "Finalize dispatch marker",
			method:   "PATCH",
			endpoint: "issues/comments/${DISPATCH_COMMENT_ID}",
		},
		{
			step:     "Mark dispatch preparation failure",
			method:   "PATCH",
			endpoint: "issues/comments/${DISPATCH_COMMENT_ID}",
		},
	}
	for _, write := range commentWrites {
		write := write
		t.Run(write.step, func(t *testing.T) {
			t.Parallel()
			step := evalDispatchWorkflowStep(t, workflow, write.step)
			if !strings.Contains(step, "gh api --method "+write.method) {
				t.Errorf("step %q must use gh api so GitHub's HTTP error message remains visible", write.step)
			}
			if !strings.Contains(step, write.endpoint) {
				t.Errorf("step %q missing comment endpoint %q", write.step, write.endpoint)
			}
			for _, forbidden := range []string{"curl --fail", "Authorization: Bearer", "2>/dev/null"} {
				if strings.Contains(step, forbidden) {
					t.Errorf("step %q hides or manually transports API diagnostics via %q", write.step, forbidden)
				}
			}
		})
	}
}

func TestEvalDispatchWorkflowPublishesArtifactBoundRequest(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "eval-dispatch.yml"))
	if err != nil {
		t.Fatalf("read eval-dispatch workflow: %v", err)
	}
	workflow := string(data)

	orderedSteps := []string{
		"- name: Create dispatch placeholder",
		"- name: Build dispatch request manifest",
		"- name: Upload dispatch request manifest",
		"- name: Finalize dispatch marker",
	}
	previous := -1
	for _, step := range orderedSteps {
		index := strings.Index(workflow, step)
		if index < 0 {
			t.Fatalf("eval-dispatch workflow missing step %q", step)
		}
		if index <= previous {
			t.Fatalf("eval-dispatch workflow step %q is out of order", step)
		}
		previous = index
	}

	placeholderStart := strings.Index(workflow, orderedSteps[0])
	manifestStart := strings.Index(workflow, orderedSteps[1])
	if strings.Contains(workflow[placeholderStart:manifestStart], "<!-- eval-dispatch:") {
		t.Fatal("dispatch placeholder must not expose a consumable marker before the manifest exists")
	}
	if count := strings.Count(workflow, "<!-- eval-dispatch:"); count != 1 {
		t.Fatalf("eval-dispatch workflow marker count = %d, want exactly one finalized marker", count)
	}

	for _, want := range []string{
		"REPOSITORY_ID: '1187709537'",
		"WORKFLOW_ID: '331725458'",
		"WORKFLOW_PATH: .github/workflows/eval-dispatch.yml",
		"RUN_ID: ${{ github.run_id }}",
		"RUN_ATTEMPT: ${{ github.run_attempt }}",
		"SOURCE_COMMENT_ID: ${{ github.event.comment.id }}",
		"DISPATCH_COMMENT_ID: ${{ steps.placeholder.outputs.comment_id }}",
		"idempotency_key: $idempotency_key",
		"actions/upload-artifact@v4",
		"name: eval-dispatch-request-${{ github.run_id }}-${{ github.run_attempt }}-${{ steps.placeholder.outputs.comment_id }}",
		"path: ${{ runner.temp }}/eval-dispatch-request.json",
		"if-no-files-found: error",
		"retention-days: 1",
		"overwrite: false",
		"ARTIFACT_ID: ${{ steps.artifact.outputs.artifact-id }}",
		"ARTIFACT_DIGEST: ${{ steps.artifact.outputs.artifact-digest }}",
		"<!-- eval-dispatch: ${marker_json} -->",
		"gh api --method PATCH",
		"issues/comments/${DISPATCH_COMMENT_ID}",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("eval-dispatch workflow missing artifact contract %q", want)
		}
	}

	for _, forbidden := range []string{
		"EVAL_TRIGGER_URL",
		"EVAL_TRIGGER_TOKEN",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("eval-dispatch workflow exposes retired direct-trigger detail %q", forbidden)
		}
	}
}

func evalDispatchWorkflowStep(t *testing.T, workflow, name string) string {
	t.Helper()

	marker := "      - name: " + name + "\n"
	start := strings.Index(workflow, marker)
	if start < 0 {
		t.Fatalf("eval-dispatch workflow missing step %q", name)
	}
	rest := workflow[start+len(marker):]
	if end := strings.Index(rest, "\n      - name: "); end >= 0 {
		return rest[:end]
	}
	return rest
}

func containsTrimmedLine(text, want string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func TestEvalPollValidatePython(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	cmd := exec.Command(
		"python3",
		"-B",
		filepath.Join(root, "scripts", "ci", "test_eval_poll_validate.py"),
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval poll validator tests failed: %v\n%s", err, output)
	}
}

func TestEvalDispatchRejectsLowRepositoryPermissions(t *testing.T) {
	t.Parallel()

	for _, permission := range []string{"read", "triage", "none"} {
		permission := permission
		t.Run(permission, func(t *testing.T) {
			t.Parallel()
			output, err := runEvalDispatchGuard(t, "permission", `{"permission":"`+permission+`"}`, "COMMENTER=low-privilege-user")
			if err == nil {
				t.Fatalf("permission %q unexpectedly allowed; output=%s", permission, output)
			}
			if !strings.Contains(output, "does not have write, maintain, or admin permission") {
				t.Fatalf("permission %q rejection = %q, want trusted-permission error", permission, output)
			}
		})
	}
}

func TestEvalDispatchAllowsTrustedRepositoryPermissions(t *testing.T) {
	t.Parallel()

	for _, permission := range []string{"write", "maintain", "admin"} {
		permission := permission
		t.Run(permission, func(t *testing.T) {
			t.Parallel()
			output, err := runEvalDispatchGuard(t, "permission", `{"permission":"`+permission+`"}`, "COMMENTER=maintainer")
			if err != nil {
				t.Fatalf("permission %q rejected: %v\n%s", permission, err, output)
			}
		})
	}
}

func TestEvalDispatchRejectsChangedPRHead(t *testing.T) {
	t.Parallel()

	const reviewedSHA = "1111111111111111111111111111111111111111"
	const currentSHA = "2222222222222222222222222222222222222222"
	pr := `{"number":934,"state":"open","head":{"sha":"` + currentSHA + `"}}`
	output, err := runEvalDispatchGuard(
		t,
		"head",
		pr,
		"EXPECTED_PR_NUMBER=934",
		"REVIEWED_SHA="+reviewedSHA,
	)
	if err == nil {
		t.Fatalf("changed PR head unexpectedly allowed; output=%s", output)
	}
	if !strings.Contains(output, "PR head changed after review") {
		t.Fatalf("changed-head rejection = %q, want explicit stale-review error", output)
	}
}

func TestEvalDispatchAcceptsReviewedCurrentPRHead(t *testing.T) {
	t.Parallel()

	const reviewedSHA = "1111111111111111111111111111111111111111"
	pr := `{"number":934,"state":"open","head":{"sha":"` + reviewedSHA + `"}}`
	output, err := runEvalDispatchGuard(
		t,
		"head",
		pr,
		"EXPECTED_PR_NUMBER=934",
		"REVIEWED_SHA="+reviewedSHA,
	)
	if err != nil {
		t.Fatalf("reviewed current PR head rejected: %v\n%s", err, output)
	}
	if !strings.Contains(output, "head_sha="+reviewedSHA) {
		t.Fatalf("head guard output = %q, want pinned head SHA", output)
	}
}

func TestEvalCommentRequiresExplicitReviewedSHA(t *testing.T) {
	t.Parallel()

	const reviewedSHA = "1111111111111111111111111111111111111111"
	output, err := runEvalCommentParser(t, "/eval drive sha="+reviewedSHA)
	if err != nil {
		t.Fatalf("explicit reviewed SHA rejected: %v\n%s", err, output)
	}
	for _, want := range []string{"products=drive", "reviewed_sha=" + reviewedSHA} {
		if !strings.Contains(output, want) {
			t.Errorf("parser output = %q, want %q", output, want)
		}
	}

	// sha 省略在解析层放行（空 reviewed_sha），由 guard 按评论者是否 PR 作者裁决
	output, err = runEvalCommentParser(t, "/eval drive")
	if err != nil {
		t.Fatalf("omitted reviewed SHA rejected at parse layer: %v\n%s", err, output)
	}
	if !strings.Contains(output, "reviewed_sha=\n") && !strings.HasSuffix(output, "reviewed_sha=") {
		t.Fatalf("parser output = %q, want empty reviewed_sha passthrough", output)
	}
}

func TestEvalDispatchAutoPinsOwnPRHeadWhenShaOmitted(t *testing.T) {
	t.Parallel()

	const currentSHA = "3333333333333333333333333333333333333333"
	pr := `{"number":934,"state":"open","user":{"login":"Internal-Contributor"},"head":{"sha":"` + currentSHA + `"}}`
	output, err := runEvalDispatchGuard(
		t,
		"head",
		pr,
		"EXPECTED_PR_NUMBER=934",
		"REVIEWED_SHA=",
		"COMMENTER=internal-contributor",
	)
	if err != nil {
		t.Fatalf("own-PR auto pin rejected: %v\n%s", err, output)
	}
	if !strings.Contains(output, "head_sha="+currentSHA) {
		t.Fatalf("guard output = %q, want auto-pinned current head", output)
	}
}

func TestEvalDispatchRequiresShaForOthersPRs(t *testing.T) {
	t.Parallel()

	const currentSHA = "3333333333333333333333333333333333333333"
	pr := `{"number":934,"state":"open","user":{"login":"someone-else"},"head":{"sha":"` + currentSHA + `"}}`
	output, err := runEvalDispatchGuard(
		t,
		"head",
		pr,
		"EXPECTED_PR_NUMBER=934",
		"REVIEWED_SHA=",
		"COMMENTER=internal-contributor",
	)
	if err == nil {
		t.Fatalf("other author's PR without sha unexpectedly allowed; output=%s", output)
	}
	if !strings.Contains(output, "requires an explicit reviewed sha") {
		t.Fatalf("missing-sha rejection = %q, want explicit-sha requirement", output)
	}
}

func TestEvalDispatchAllowsAllowlistedAuthorOnOwnPR(t *testing.T) {
	t.Parallel()

	allowlist := writeEvalAllowlist(t, "# 自助评测名单\nInternal-Contributor  # 内部贡献者\n")
	output, err := runEvalDispatchGuard(
		t,
		"permission",
		`{"message":"Not Found"}`,
		"COMMENTER=internal-contributor",
		"PR_AUTHOR=Internal-Contributor",
		"EVAL_ALLOWLIST_PATH="+allowlist,
	)
	if err != nil {
		t.Fatalf("allowlisted author rejected on own PR: %v\n%s", err, output)
	}
	if !strings.Contains(output, "authorized=allowlisted-author") {
		t.Fatalf("guard output = %q, want allowlisted-author authorization", output)
	}
}

func TestEvalDispatchRejectsAllowlistedUserOnOthersPR(t *testing.T) {
	t.Parallel()

	allowlist := writeEvalAllowlist(t, "internal-contributor\n")
	output, err := runEvalDispatchGuard(
		t,
		"permission",
		`{"message":"Not Found"}`,
		"COMMENTER=internal-contributor",
		"PR_AUTHOR=someone-else",
		"EVAL_ALLOWLIST_PATH="+allowlist,
	)
	if err == nil {
		t.Fatalf("allowlisted user unexpectedly dispatched another author's PR; output=%s", output)
	}
	if !strings.Contains(output, "self-service only") {
		t.Fatalf("cross-PR rejection = %q, want self-service-only error", output)
	}
}

func TestEvalDispatchRejectsUnlistedCommenterWithoutWrite(t *testing.T) {
	t.Parallel()

	allowlist := writeEvalAllowlist(t, "# 空名单\n")
	output, err := runEvalDispatchGuard(
		t,
		"permission",
		`{"permission":"read"}`,
		"COMMENTER=stranger",
		"PR_AUTHOR=stranger",
		"EVAL_ALLOWLIST_PATH="+allowlist,
	)
	if err == nil {
		t.Fatalf("unlisted commenter unexpectedly allowed; output=%s", output)
	}
	if !strings.Contains(output, "not an allowlisted PR author") {
		t.Fatalf("unlisted rejection = %q, want allowlist-aware error", output)
	}
}

func writeEvalAllowlist(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "eval-allowlist.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write allowlist fixture: %v", err)
	}
	return path
}

func runEvalDispatchGuard(t *testing.T, mode, input string, env ...string) (string, error) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	cmd := exec.Command("python3", filepath.Join(root, "scripts", "ci", "eval_dispatch_guard.py"), mode)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), env...)
	output, runErr := cmd.CombinedOutput()
	return string(output), runErr
}

func runEvalCommentParser(t *testing.T, comment string) (string, error) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "github-output")
	cmd := exec.Command("python3", filepath.Join(root, "scripts", "ci", "eval_comment_parse.py"))
	cmd.Env = append(
		os.Environ(),
		"COMMENT_BODY="+comment,
		"GITHUB_OUTPUT="+outputPath,
	)
	combined, runErr := cmd.CombinedOutput()
	fileOutput, readErr := os.ReadFile(outputPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read parser GitHub output: %v", readErr)
	}
	return string(combined) + string(fileOutput), runErr
}
