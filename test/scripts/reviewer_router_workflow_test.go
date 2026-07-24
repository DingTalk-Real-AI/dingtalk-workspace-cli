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
	Jobs        map[string]reviewerRouterJob     `yaml:"jobs"`
}

type reviewerRouterTrigger struct {
	Branches []string `yaml:"branches"`
	Types    []string `yaml:"types"`
}

type reviewerRouterJob struct {
	If    string               `yaml:"if"`
	Steps []reviewerRouterStep `yaml:"steps"`
}

type reviewerRouterStep struct {
	Uses string            `yaml:"uses"`
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

	if len(workflow.On) != 1 {
		t.Fatalf("workflow triggers = %v, want pull_request_target only", workflow.On)
	}
	trigger, ok := workflow.On["pull_request_target"]
	if !ok {
		t.Fatalf("workflow triggers = %v, want pull_request_target", workflow.On)
	}
	if wantBranches := []string{"main"}; !reflect.DeepEqual(trigger.Branches, wantBranches) {
		t.Fatalf("pull_request_target branches = %v, want %v", trigger.Branches, wantBranches)
	}
	wantTypes := []string{"opened", "synchronize", "reopened", "ready_for_review"}
	if !reflect.DeepEqual(trigger.Types, wantTypes) {
		t.Fatalf("pull_request_target types = %v, want %v", trigger.Types, wantTypes)
	}

	wantPermissions := map[string]string{
		"contents":      "write",
		"pull-requests": "write",
	}
	if !reflect.DeepEqual(workflow.Permissions, wantPermissions) {
		t.Fatalf("workflow permissions = %v, want exactly %v", workflow.Permissions, wantPermissions)
	}

	if len(workflow.Jobs) != 1 {
		t.Fatalf("workflow jobs = %v, want one isolated routing job", workflow.Jobs)
	}
	job, ok := workflow.Jobs["route"]
	if !ok {
		t.Fatalf("workflow jobs = %v, want route", workflow.Jobs)
	}
	if job.If != "github.event.pull_request.draft == false" {
		t.Fatalf("route.if = %q, want non-draft guard", job.If)
	}
	const githubScriptSHA = "actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b"
	if len(job.Steps) != 1 || job.Steps[0].Uses != githubScriptSHA {
		t.Fatalf("route steps = %#v, want one pinned base-owned github-script step", job.Steps)
	}

	script := job.Steps[0].With["script"]
	for _, want := range []string{
		"'sczheng189'",
		"'shangguanxuan633-lab'",
		"'audanye-sudo'",
		"'wxianfeng'",
		"github.rest.pulls.get",
		"currentPull.head.sha !== eventHeadSha",
		"currentPull.state !== 'open'",
		"currentPull.draft",
		"currentPull.base.ref !== 'main'",
		"getReadyEventPull('review request')",
		"getReadyEventPull('auto-merge enable')",
		"context.payload.action === 'synchronize'",
		"context.payload.sender?.login?.toLowerCase()",
		"reviewer.toLowerCase() !== author",
		"reviewer.toLowerCase() !== latestPusher",
		"pullRequest.requested_reviewers",
		"pullRequest.requested_teams",
		"github.rest.pulls.listReviews",
		"['APPROVED', 'CHANGES_REQUESTED', 'DISMISSED'].includes",
		"review.commit_id === headSha",
		"['APPROVED', 'CHANGES_REQUESTED'].includes(review.state)",
		"review.state === 'CHANGES_REQUESTED'",
		"staleChangeRequester",
		"state: 'open'",
		"loads.set(candidate, loads.get(candidate) + 1)",
		"loads.get(left) - loads.get(right)",
		"for (const reviewer of ranked)",
		"github.rest.pulls.requestReviewers",
		"trying the next candidate",
		"Reviewer routing hit an unexpected error",
		"enablePullRequestAutoMerge",
		"mergeMethod: MERGE",
		"core.warning",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("reviewer router script is missing contract marker %q", want)
		}
	}

	for _, forbidden := range []string{
		"actions/checkout",
		"['APPROVED', 'CHANGES_REQUESTED', 'COMMENTED']",
		"PeterGuy326",
		"core.setFailed",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("reviewer router must not contain %q", forbidden)
		}
	}
}
