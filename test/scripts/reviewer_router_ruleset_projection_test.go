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

const strictUpdateRuleHelper = `function isStrictUpdateRule(rule) {
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
}`

const strictGraphQLUpdateRuleHelper = `function isStrictGraphQLUpdateRule(restRuleset, graphRuleset) {
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
}`

func TestReviewerRouterAcceptsGitHubStrictUpdateReadProjection(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to verify the ruleset projection helper")
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
	for _, script := range scripts {
		if !strings.Contains(script, "const writerRuleset = writerRulesets[0];") {
			continue
		}
		checked++
		if got := strings.Count(script, strictUpdateRuleHelper); got != 1 {
			t.Errorf("writer-ruleset script contains strict update helper %d times, want 1", got)
		}
		if got := strings.Count(script, strictGraphQLUpdateRuleHelper); got != 1 {
			t.Errorf("writer-ruleset script contains GraphQL strict update helper %d times, want 1", got)
		}
		if strings.Contains(
			script,
			"parameters?.update_allows_fetch_and_merge !== false",
		) {
			t.Error("writer-ruleset script rejects GitHub's omitted strict-false read projection")
		}
		if !strings.Contains(script, "!isStrictUpdateRule(writerRuleset.rules[0])") {
			t.Error("writer-ruleset script does not enforce the strict update helper")
		}
		for _, marker := range []string{
			"query ReviewerRouterWriterRule($rulesetID: ID!)",
			"rules(first: 2)",
			"updateAllowsFetchAndMerge",
			"{rulesetID: writerRuleset.node_id}",
			"!isStrictGraphQLUpdateRule(writerRuleset, graphWriterRuleset)",
		} {
			if !strings.Contains(script, marker) {
				t.Errorf("writer-ruleset script does not enforce GraphQL marker %q", marker)
			}
		}
	}
	if checked != 3 {
		t.Fatalf("writer-ruleset scripts checked = %d, want App enable, reconcile, and CI self-check", checked)
	}

	verification := strictUpdateRuleHelper + "\n" + strictGraphQLUpdateRuleHelper + `
const cases = [
  ['omitted parameters', {type: 'update'}, true],
  ['explicit false', {type: 'update', parameters: {update_allows_fetch_and_merge: false}}, true],
  ['null parameters', {type: 'update', parameters: null}, false],
  ['empty parameters', {type: 'update', parameters: {}}, false],
  ['boolean parameters', {type: 'update', parameters: false}, false],
  ['string parameters', {type: 'update', parameters: 'malformed'}, false],
  ['array parameters', {type: 'update', parameters: []}, false],
  ['explicit true', {type: 'update', parameters: {update_allows_fetch_and_merge: true}}, false],
  ['null field', {type: 'update', parameters: {update_allows_fetch_and_merge: null}}, false],
  ['string false', {type: 'update', parameters: {update_allows_fetch_and_merge: 'false'}}, false],
  ['numeric false', {type: 'update', parameters: {update_allows_fetch_and_merge: 0}}, false],
  ['extra parameter', {type: 'update', parameters: {update_allows_fetch_and_merge: false, future_exception: false}}, false],
  ['wrong rule type', {type: 'creation'}, false],
  ['null rule', null, false],
];
for (const [name, rule, want] of cases) {
  const got = isStrictUpdateRule(rule);
  if (got !== want) {
    throw new Error(name + ': got ' + got + ', want ' + want);
  }
}

const restRuleset = {id: 21363804, name: 'main-merge-writers'};
const graphRule = {
  type: 'UPDATE',
  parameters: {
    __typename: 'UpdateParameters',
    updateAllowsFetchAndMerge: false,
  },
};
const graphRuleset = {
  databaseId: 21363804,
  name: 'main-merge-writers',
  enforcement: 'ACTIVE',
  target: 'BRANCH',
  rules: {totalCount: 1, nodes: [graphRule]},
};
const graphCases = [
  ['valid GraphQL projection', restRuleset, graphRuleset, true],
  ['invalid REST id', {id: 0, name: 'main-merge-writers'}, graphRuleset, false],
  ['mismatched GraphQL id', restRuleset, {...graphRuleset, databaseId: 7}, false],
  ['mismatched name', restRuleset, {...graphRuleset, name: 'other'}, false],
  ['inactive ruleset', restRuleset, {...graphRuleset, enforcement: 'EVALUATE'}, false],
  ['wrong target', restRuleset, {...graphRuleset, target: 'TAG'}, false],
  ['extra rule', restRuleset, {...graphRuleset, rules: {totalCount: 2, nodes: [graphRule, graphRule]}}, false],
  ['wrong rule type', restRuleset, {...graphRuleset, rules: {totalCount: 1, nodes: [{...graphRule, type: 'CREATION'}]}}, false],
  ['wrong parameter type', restRuleset, {...graphRuleset, rules: {totalCount: 1, nodes: [{...graphRule, parameters: {...graphRule.parameters, __typename: 'PullRequestParameters'}}]}}, false],
  ['fetch and merge enabled', restRuleset, {...graphRuleset, rules: {totalCount: 1, nodes: [{...graphRule, parameters: {...graphRule.parameters, updateAllowsFetchAndMerge: true}}]}}, false],
  ['missing GraphQL node', restRuleset, null, false],
];
for (const [name, rest, graph, want] of graphCases) {
  const got = isStrictGraphQLUpdateRule(rest, graph);
  if (got !== want) {
    throw new Error(name + ': got ' + got + ', want ' + want);
  }
}
`
	command := exec.Command(node, "-e", verification)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("strict update projection verification failed: %v\n%s", runErr, output)
	}
}
