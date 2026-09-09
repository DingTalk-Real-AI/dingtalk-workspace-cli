// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type failFastWorkflowJob struct {
	Name        string `yaml:"name"`
	Needs       any    `yaml:"needs"`
	If          string `yaml:"if"`
	RunsOn      string `yaml:"runs-on"`
	Timeout     int    `yaml:"timeout-minutes"`
	Permissions map[string]string
	Steps       []struct {
		Name string `yaml:"name"`
		Env  map[string]string
		Run  string `yaml:"run"`
	} `yaml:"steps"`
}

func failFastNeeds(needs any) []string {
	switch value := needs.(type) {
	case string:
		return []string{value}
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

// TestCIFailFastTripwiresWatchEverySubstantiveJob pins the fail-fast
// cancellation contract: every substantive pull-request admission job has one
// dedicated tripwire that cancels the whole run the moment that job fails, so
// already-doomed siblings stop consuming hosted runners and concurrency
// slots. The watched set is derived from the live job graph, so adding a new
// substantive job without a tripwire fails here, and a tripwire left behind
// by a removed or exempted job fails as stale. Protected-main pushes keep the
// complete failure picture and the coverage-cache producer, so tripwires are
// scoped to pull-request events and the exempt set is reviewed explicitly.
func TestCIFailFastTripwiresWatchEverySubstantiveJob(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	var workflow struct {
		Jobs map[string]failFastWorkflowJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse CI workflow: %v", err)
	}

	// lint failure skips every downstream job through `needs`, and
	// coverage-main-metadata only runs on protected-main pushes, which are
	// deliberately exempt from fail-fast cancellation.
	exempt := map[string]string{
		"lint":                   "its failure skips every dependent job already",
		"coverage-main-metadata": "push-only job; tripwires are pull-request scoped",
	}
	requiredContexts := map[string]bool{
		"Lint": true, "Test": true, "Coverage": true, "Policy": true,
		"Edition": true, "Interface Integrity": true, "AI Behavior": true,
		"CLI Smoke": true, "Mock MCP": true,
	}

	substantive := make(map[string]bool)
	tripwires := make(map[string]string)
	for jobID := range workflow.Jobs {
		if strings.HasPrefix(jobID, "fail-fast-") {
			tripwires[jobID] = strings.TrimPrefix(jobID, "fail-fast-")
			continue
		}
		if _, skipped := exempt[jobID]; skipped {
			continue
		}
		substantive[jobID] = true
	}

	for jobID := range substantive {
		tripwireID := "fail-fast-" + jobID
		tripwire, ok := workflow.Jobs[tripwireID]
		if !ok {
			t.Errorf("substantive job %q has no %q tripwire; a first failure must cancel doomed siblings", jobID, tripwireID)
			continue
		}
		needs := failFastNeeds(tripwire.Needs)
		if len(needs) != 2 || needs[0] != "lint" || needs[1] != jobID {
			t.Errorf("%s needs = %v, want exactly [lint %s] so the tripwire fires the moment that job fails while lint stays the Draft-gated entry point", tripwireID, needs, jobID)
		}
		if tripwire.If != "failure() && needs.lint.result == 'success' && github.event_name == 'pull_request'" {
			t.Errorf("%s if = %q, want pull-request-scoped failure trigger guarded by a successful lint classification so protected-main pushes run to completion", tripwireID, tripwire.If)
		}
		if tripwire.RunsOn != "ubuntu-latest" {
			t.Errorf("%s runs-on = %q, want ubuntu-latest", tripwireID, tripwire.RunsOn)
		}
		if tripwire.Timeout != 5 {
			t.Errorf("%s timeout-minutes = %d, want 5", tripwireID, tripwire.Timeout)
		}
		if tripwire.Permissions["actions"] != "write" {
			t.Errorf("%s must grant actions: write to cancel the run", tripwireID)
		}
		if len(tripwire.Steps) != 1 {
			t.Fatalf("%s steps = %d, want exactly one cancellation step", tripwireID, len(tripwire.Steps))
		}
		step := tripwire.Steps[0]
		if !strings.Contains(step.Run, `gh run cancel "$GITHUB_RUN_ID" -R "$GITHUB_REPOSITORY"`) {
			t.Errorf("%s step must cancel the current run through gh run cancel with an explicit -R repository binding; the job has no checkout, so gh cannot infer the base repo", tripwireID)
		}
		if step.Env["GH_TOKEN"] != "${{ github.token }}" {
			t.Errorf("%s step must authenticate gh with the job GITHUB_TOKEN", tripwireID)
		}
		if requiredContexts[tripwire.Name] {
			t.Errorf("%s name %q collides with a required ruleset context", tripwireID, tripwire.Name)
		}
		if !strings.HasPrefix(tripwire.Name, "Fail fast (") || !strings.HasSuffix(tripwire.Name, ")") {
			t.Errorf("%s name = %q, want the reviewed \"Fail fast (<watched job>)\" shape", tripwireID, tripwire.Name)
		}
	}

	for tripwireID, watched := range tripwires {
		if _, ok := substantive[watched]; !ok {
			if reason, isExempt := exempt[watched]; isExempt {
				t.Errorf("stale tripwire %s watches exempt job %q: %s", tripwireID, watched, reason)
				continue
			}
			t.Errorf("stale tripwire %s watches unknown job %q", tripwireID, watched)
		}
	}
}
