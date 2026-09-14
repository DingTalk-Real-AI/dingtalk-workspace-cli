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

const stableMergedPRIdentityHelper = `function isStableMergedPRIdentity(
  currentPull,
  pullNumber,
  headSha,
  mergeCommitSha,
) {
  return (
    currentPull?.number === pullNumber &&
    currentPull.state === 'closed' &&
    currentPull.merged === true &&
    typeof currentPull.merged_at === 'string' &&
    currentPull.merged_at.length > 0 &&
    currentPull.base?.ref === 'main' &&
    currentPull.head?.sha === headSha &&
    currentPull.merge_commit_sha === mergeCommitSha
  );
}`

func TestCoverageBaselineRepairUsesStableMergedPRIdentity(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to verify merged-PR repair identity semantics")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	workflowPath := filepath.Join(
		root,
		".github",
		"workflows",
		"coverage-baseline-repair.yml",
	)
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}
	workflow := string(data)

	var parsedWorkflow any
	if err := yaml.Unmarshal(data, &parsedWorkflow); err != nil {
		t.Fatalf("parse %s: %v", workflowPath, err)
	}
	var scripts []string
	collectGitHubScripts(parsedWorkflow, &scripts)
	helperCount := 0
	checkCount := 0
	for _, script := range scripts {
		helperCount += strings.Count(script, stableMergedPRIdentityHelper)
		checkCount += strings.Count(script, "!isStableMergedPRIdentity(")
	}
	if helperCount != 2 {
		t.Fatalf("stable merged-PR identity helper occurrences = %d, want dispatcher and producer", helperCount)
	}
	if checkCount != 2 {
		t.Fatalf("stable merged-PR identity checks = %d, want dispatcher and producer", checkCount)
	}
	for _, forbidden := range []string{
		"currentPull.base.sha",
		"const baseSha =",
		"base_sha",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("merged-PR repair must not bind mutable Ref SHA via %q", forbidden)
		}
	}
	for _, want := range []string{
		"const baseRef = eventPull?.base?.ref;",
		"baseRef !== 'main'",
		"const headSha = eventPull?.head?.sha;",
		"const headSha = payload.head_sha;",
		"head_sha: headSha",
		"currentPull.head?.sha === headSha",
		"currentPull.merge_commit_sha === mergeCommitSha",
		"await requireMainContainment(mergeCommitSha)",
		"await requireMainContainment(targetSha)",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("merged-PR repair missing stable identity guard %q", want)
		}
	}

	verification := stableMergedPRIdentityHelper + `
const pullNumber = 1077;
const headSha = '1'.repeat(40);
const mergeCommitSha = '2'.repeat(40);
const validPull = {
  number: pullNumber,
  state: 'closed',
  merged: true,
  merged_at: '2026-08-25T04:55:30Z',
  base: {ref: 'main', sha: '3'.repeat(40)},
  head: {sha: headSha},
  merge_commit_sha: mergeCommitSha,
};
const cases = [
  ['base ref advanced after event', {...validPull, base: {...validPull.base, sha: '4'.repeat(40)}}, true],
  ['base sha omitted by projection', {...validPull, base: {ref: 'main'}}, true],
  ['wrong pull number', {...validPull, number: pullNumber + 1}, false],
  ['open pull request', {...validPull, state: 'open'}, false],
  ['not merged', {...validPull, merged: false}, false],
  ['missing merged at', {...validPull, merged_at: null}, false],
  ['empty merged at', {...validPull, merged_at: ''}, false],
  ['non-string merged at', {...validPull, merged_at: 1}, false],
  ['wrong base ref', {...validPull, base: {...validPull.base, ref: 'release'}}, false],
  ['missing base ref', {...validPull, base: {}}, false],
  ['missing head', {...validPull, head: null}, false],
  ['wrong head sha', {...validPull, head: {sha: '5'.repeat(40)}}, false],
  ['wrong merge sha', {...validPull, merge_commit_sha: '6'.repeat(40)}, false],
  ['null pull', null, false],
];
for (const [name, currentPull, want] of cases) {
  const got = isStableMergedPRIdentity(
    currentPull,
    pullNumber,
    headSha,
    mergeCommitSha,
  );
  if (got !== want) {
    throw new Error(name + ': got ' + got + ', want ' + want);
  }
}
`
	command := exec.Command(node, "-e", verification)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("stable merged-PR identity verification failed: %v\n%s", runErr, output)
	}
}
