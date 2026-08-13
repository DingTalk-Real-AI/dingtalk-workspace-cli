// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

type workflowFixture struct {
	Version     int                                    `json:"version"`
	Description string                                 `json:"description,omitempty"`
	Cases       []cli.ToolSearchWorkflowEvaluationCase `json:"cases"`
}

type comparisonOutput struct {
	Diagnostic      cli.ToolSearchDiagnosticComparison   `json:"diagnostic"`
	Shadows         []cli.ToolSearchDiagnosticComparison `json:"shadows"`
	AgentPlanningAB *cli.ToolSearchAgentPlanningABReport `json:"agent_planning_ab,omitempty"`
	AgentAB         *cli.ToolSearchAgentABReport         `json:"agent_ab,omitempty"`
	Independent     *cli.ToolSearchIndependentReport     `json:"independent,omitempty"`
}

type independentFixture struct {
	Version     string                          `json:"version"`
	State       string                          `json:"state"`
	Description string                          `json:"description,omitempty"`
	Cases       []cli.ToolSearchIndependentCase `json:"cases"`
}

func main() {
	var workflowsPath string
	var agentResultsPath string
	var directPlansPath string
	var searchPlansPath string
	var agentModel string
	var outputPath string
	var independentQrelsPath string
	flag.StringVar(&workflowsPath, "workflows", "scripts/testdata/tool_search_workflows.json", "reviewed diagnostic workflow fixture")
	flag.StringVar(&agentResultsPath, "agent-results", "", "optional paired Agent A/B result JSON")
	flag.StringVar(&directPlansPath, "agent-plans-direct", "", "optional direct_schema Agent planning output JSON")
	flag.StringVar(&searchPlansPath, "agent-plans-search", "", "optional search_inspect Agent planning output JSON")
	flag.StringVar(&agentModel, "agent-model", "", "model identifier for paired planning outputs")
	flag.StringVar(&outputPath, "output", "", "output JSON path; stdout when empty")
	flag.StringVar(&independentQrelsPath, "independent-qrels", "", "optional independently authored tool-search-qrels.v1 JSON")
	flag.Parse()

	// Runtime and CI use the same declaration-owned Catalog assembly. The
	// comparison generator never reads or writes a repository Catalog JSON.
	cli.RegisterSchemaSourceRoot(func() *cobra.Command { return app.NewSchemaSourceRootCommand() })

	workflows, err := readWorkflows(workflowsPath)
	if err != nil {
		fatal(err)
	}
	diagnostic, err := cli.BuildDeliveryToolSearchDiagnosticComparison(context.Background(), workflows)
	if err != nil {
		fatal(err)
	}
	actionShadow, err := cli.BuildDeliveryToolSearchDiagnosticComparisonForAlgorithm(context.Background(), workflows, cli.ToolSearchLexicalBM25Action)
	if err != nil {
		fatal(err)
	}
	shadow, err := cli.BuildDeliveryToolSearchDiagnosticComparisonForAlgorithm(context.Background(), workflows, cli.ToolSearchLexicalTFIDF)
	if err != nil {
		fatal(err)
	}
	result := comparisonOutput{Diagnostic: diagnostic, Shadows: []cli.ToolSearchDiagnosticComparison{actionShadow, shadow}}
	if independentQrelsPath != "" {
		fixture, readErr := readIndependentQrels(independentQrelsPath)
		if readErr != nil {
			fatal(readErr)
		}
		independent, evaluationErr := cli.BuildDeliveryToolSearchIndependentEvaluation(context.Background(), fixture.Cases)
		if evaluationErr != nil {
			fatal(evaluationErr)
		}
		result.Independent = &independent
	}
	if directPlansPath != "" || searchPlansPath != "" || agentModel != "" {
		if directPlansPath == "" || searchPlansPath == "" || agentModel == "" {
			fatal(fmt.Errorf("-agent-plans-direct, -agent-plans-search and -agent-model must be provided together"))
		}
		directPlans, readErr := readAgentPlans(directPlansPath)
		if readErr != nil {
			fatal(readErr)
		}
		searchPlans, readErr := readAgentPlans(searchPlansPath)
		if readErr != nil {
			fatal(readErr)
		}
		planningReport, scoreErr := cli.ScoreToolSearchAgentPlanningAB(agentModel, workflows, directPlans, searchPlans)
		if scoreErr != nil {
			fatal(scoreErr)
		}
		result.AgentPlanningAB = &planningReport
	}
	if agentResultsPath != "" {
		agentInput, readErr := readAgentResults(agentResultsPath)
		if readErr != nil {
			fatal(readErr)
		}
		if agentInput.Catalog != diagnostic.Catalog {
			fatal(fmt.Errorf("Agent A/B Catalog hashes do not match the embedded binary"))
		}
		agentReport, aggregateErr := cli.AggregateToolSearchAgentAB(agentInput)
		if aggregateErr != nil {
			fatal(aggregateErr)
		}
		diagnostic.AgentABStatus = "completed"
		result.Diagnostic = diagnostic
		result.AgentAB = &agentReport
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err)
	}
	encoded = append(encoded, '\n')
	if outputPath == "" {
		if _, err := os.Stdout.Write(encoded); err != nil {
			fatal(err)
		}
		return
	}
	if err := writeAtomically(outputPath, encoded); err != nil {
		fatal(err)
	}
}

func readIndependentQrels(path string) (independentFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return independentFixture{}, fmt.Errorf("read independent qrels: %w", err)
	}
	var fixture independentFixture
	if err := decodeStrict(data, &fixture); err != nil {
		return independentFixture{}, fmt.Errorf("decode independent qrels: %w", err)
	}
	if fixture.Version != "tool-search-qrels.v1" || len(fixture.Cases) == 0 {
		return independentFixture{}, fmt.Errorf("independent qrels must be tool-search-qrels.v1 and non-empty")
	}
	return fixture, nil
}

func readWorkflows(path string) ([]cli.ToolSearchWorkflowEvaluationCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflows: %w", err)
	}
	var fixture workflowFixture
	if err := decodeStrict(data, &fixture); err != nil {
		return nil, fmt.Errorf("decode workflows: %w", err)
	}
	if fixture.Version != 1 || len(fixture.Cases) == 0 {
		return nil, fmt.Errorf("workflow fixture must be version 1 and non-empty")
	}
	return fixture.Cases, nil
}

func readAgentResults(path string) (cli.ToolSearchAgentABInput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cli.ToolSearchAgentABInput{}, fmt.Errorf("read Agent A/B results: %w", err)
	}
	var input cli.ToolSearchAgentABInput
	if err := decodeStrict(data, &input); err != nil {
		return cli.ToolSearchAgentABInput{}, fmt.Errorf("decode Agent A/B results: %w", err)
	}
	return input, nil
}

func readAgentPlans(path string) (cli.ToolSearchAgentPlanOutput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cli.ToolSearchAgentPlanOutput{}, fmt.Errorf("read Agent plans: %w", err)
	}
	var output cli.ToolSearchAgentPlanOutput
	if err := decodeStrict(data, &output); err != nil {
		return cli.ToolSearchAgentPlanOutput{}, fmt.Errorf("decode Agent plans: %w", err)
	}
	return output, nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func writeAtomically(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".tool-search-comparison-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
