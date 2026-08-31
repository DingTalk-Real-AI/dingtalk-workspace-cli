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

const mergeDefaultsProjectionHelper = `function classifyMergeDefaults(repository) {
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
}`

const privilegedMergeDefaultsGuard = `if (mergeDefaultsProjection !== 'reviewed') {
  throw new Error(
    'Dedicated Reviewer Router App cannot verify the reviewed repository merge-message defaults.',
  );
}`

func TestReviewerRouterMergeDefaultsProjection(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to verify merge-default projection semantics")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	var scripts []string
	for _, relativePath := range []string{
		filepath.Join(".github", "workflows", "ci.yml"),
		filepath.Join(".github", "workflows", "reviewer-router.yml"),
	} {
		path := filepath.Join(root, relativePath)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		var workflow any
		if unmarshalErr := yaml.Unmarshal(data, &workflow); unmarshalErr != nil {
			t.Fatalf("parse %s: %v", path, unmarshalErr)
		}
		collectGitHubScripts(workflow, &scripts)
	}

	checked := 0
	readOnlyChecks := 0
	privilegedChecks := 0
	for _, script := range scripts {
		if strings.Count(script, mergeDefaultsProjectionHelper) != 1 {
			continue
		}
		checked++
		for _, marker := range []string{
			"github.rest.repos.get",
			"const mergeDefaultsProjection = classifyMergeDefaults(repository);",
		} {
			if !strings.Contains(script, marker) {
				t.Errorf("merge-default script does not enforce marker %q", marker)
			}
		}
		if strings.Contains(script, "MINTED_REVIEWER_ROUTER_APP_SLUG") {
			privilegedChecks++
			for _, marker := range []string{
				privilegedMergeDefaultsGuard,
			} {
				if !strings.Contains(script, marker) {
					t.Errorf("privileged merge-default script missing marker %q", marker)
				}
			}
			guardIndex := strings.Index(script, privilegedMergeDefaultsGuard)
			guardEndIndex := guardIndex + len(privilegedMergeDefaultsGuard)
			firstMutationIndex := len(script)
			for _, mutation := range []string{
				"disablePullRequestAutoMerge",
				"enablePullRequestAutoMerge",
			} {
				if index := strings.Index(script, mutation); index >= 0 && index < firstMutationIndex {
					firstMutationIndex = index
				}
			}
			if guardIndex < 0 || firstMutationIndex == len(script) || guardEndIndex >= firstMutationIndex {
				t.Error("privileged merge-default validation must precede every auto-merge mutation")
			}
			continue
		}
		readOnlyChecks++
		for _, marker := range []string{
			"mergeDefaultsProjection === 'invalid'",
			"mergeDefaultsProjection === 'omitted'",
			"Read-only CI cannot observe repository merge-message defaults; exact validation is delegated to the dedicated App.",
		} {
			if !strings.Contains(script, marker) {
				t.Errorf("read-only merge-default script missing marker %q", marker)
			}
		}
		if strings.Contains(script, "mergeDefaultsProjection !== 'reviewed'") {
			t.Error("read-only CI must distinguish exact dual omission from malformed projections")
		}
	}
	if checked != 3 || readOnlyChecks != 1 || privilegedChecks != 2 {
		t.Fatalf(
			"merge-default scripts checked=%d read-only=%d privileged=%d, want 3/1/2",
			checked,
			readOnlyChecks,
			privilegedChecks,
		)
	}

	verification := mergeDefaultsProjectionHelper + `
const cases = [
  ['reviewed PR title', {merge_commit_title: 'MERGE_MESSAGE', merge_commit_message: 'PR_TITLE'}, 'reviewed'],
  ['reviewed blank body', {merge_commit_title: 'MERGE_MESSAGE', merge_commit_message: 'BLANK'}, 'reviewed'],
  ['dual omission', {}, 'omitted'],
  ['null repository', null, 'invalid'],
  ['array repository', [], 'invalid'],
  ['title only', {merge_commit_title: 'MERGE_MESSAGE'}, 'invalid'],
  ['message only', {merge_commit_message: 'PR_TITLE'}, 'invalid'],
  ['own undefined title', {merge_commit_title: undefined, merge_commit_message: 'PR_TITLE'}, 'invalid'],
  ['own undefined message', {merge_commit_title: 'MERGE_MESSAGE', merge_commit_message: undefined}, 'invalid'],
  ['null title', {merge_commit_title: null, merge_commit_message: 'PR_TITLE'}, 'invalid'],
  ['null message', {merge_commit_title: 'MERGE_MESSAGE', merge_commit_message: null}, 'invalid'],
  ['unsafe title', {merge_commit_title: 'PR_TITLE', merge_commit_message: 'PR_TITLE'}, 'invalid'],
  ['unsafe body', {merge_commit_title: 'MERGE_MESSAGE', merge_commit_message: 'PR_BODY'}, 'invalid'],
];
for (const [name, repository, want] of cases) {
  const got = classifyMergeDefaults(repository);
  if (got !== want) {
    throw new Error(name + ': got ' + got + ', want ' + want);
  }
}
`
	command := exec.Command(node, "-e", verification)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("merge-default projection verification failed: %v\n%s", runErr, output)
	}
}
