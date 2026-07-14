// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

const selectionRevisionID = "selection"

type hintDirFile struct {
	Version  int                       `json:"version"`
	Source   hintDirSource             `json:"source"`
	Products map[string]hintDirProduct `json:"products,omitempty"`
	Tools    map[string]hintDirTool    `json:"tools,omitempty"`
}

type hintDirSource struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Reviewed bool   `json:"reviewed,omitempty"`
}

type hintDirProduct struct {
	AgentSummary string   `json:"agent_summary"`
	UseWhen      []string `json:"use_when"`
	AvoidWhen    []string `json:"avoid_when"`
	Reviewed     bool     `json:"reviewed"`
	ReviewReason string   `json:"review_reason"`
	SourceRefs   []string `json:"source_refs"`
}

type hintDirTool struct {
	AgentSummary string                               `json:"agent_summary,omitempty"`
	UseWhen      []string                             `json:"use_when,omitempty"`
	AvoidWhen    []string                             `json:"avoid_when,omitempty"`
	Examples     []string                             `json:"examples,omitempty"`
	Reviewed     bool                                 `json:"reviewed,omitempty"`
	ReviewReason string                               `json:"review_reason,omitempty"`
	SourceRefs   []string                             `json:"source_refs,omitempty"`
	CLIPath      string                               `json:"cli_path,omitempty"`
	Parameters   map[string]ManualSchemaParameterHint `json:"parameters,omitempty"`
}

func loadManualSchemaHintsFromHintDirs(metadataFS fs.FS, metadataGlob string, selectionFS fs.FS, selectionGlob string) (ManualSchemaHintSnapshot, error) {
	commands, err := loadParameterCommandsFromMetadata(metadataFS, metadataGlob)
	if err != nil {
		return ManualSchemaHintSnapshot{}, err
	}
	agentHints, err := loadAgentHintsFromSelection(selectionFS, selectionGlob)
	if err != nil {
		return ManualSchemaHintSnapshot{}, err
	}
	snapshot := ManualSchemaHintSnapshot{
		Schema:     manualSchemaHintSchemaRef,
		Version:    manualSchemaHintVersion,
		Commands:   commands,
		AgentHints: agentHints,
	}
	if err := ValidateManualAgentHintSet(snapshot.AgentHints, nil, nil); err != nil {
		return ManualSchemaHintSnapshot{}, fmt.Errorf("validate selection Agent hints: %w", err)
	}
	return snapshot, nil
}

func loadParameterCommandsFromMetadata(metadataFS fs.FS, globPattern string) ([]ManualSchemaCommandHint, error) {
	files, err := fs.Glob(metadataFS, globPattern)
	if err != nil {
		return nil, fmt.Errorf("list metadata hints: %w", err)
	}
	sort.Strings(files)
	commands := make([]ManualSchemaCommandHint, 0)
	for _, name := range files {
		data, err := fs.ReadFile(metadataFS, name)
		if err != nil {
			return nil, fmt.Errorf("read metadata hint %s: %w", name, err)
		}
		var file hintDirFile
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("decode metadata hint %s: %w", name, err)
		}
		canonicals := make([]string, 0, len(file.Tools))
		for canonical := range file.Tools {
			canonicals = append(canonicals, canonical)
		}
		sort.Strings(canonicals)
		for _, canonical := range canonicals {
			tool := file.Tools[canonical]
			if len(tool.Parameters) == 0 {
				continue
			}
			cliPath := strings.TrimSpace(tool.CLIPath)
			if cliPath == "" {
				return nil, fmt.Errorf("metadata tool %s has parameters but missing cli_path", canonical)
			}
			reason := strings.TrimSpace(tool.ReviewReason)
			if reason == "" {
				reason = "reviewed parameter overrides from schema_hints/metadata"
			}
			commands = append(commands, ManualSchemaCommandHint{
				CLIPath:       cliPath,
				CanonicalPath: canonical,
				Reason:        reason,
				Reviewed:      true,
				Parameters:    tool.Parameters,
			})
		}
	}
	return commands, nil
}

// LoadAgentHintsFromSelectionForValidation loads selection HintFiles from an
// FS rooted at schema_hints/selection (so glob *.json).
func LoadAgentHintsFromSelectionForValidation(selectionFS fs.FS) (ManualAgentHintSet, error) {
	return loadAgentHintsFromSelection(selectionFS, "*.json")
}

func loadAgentHintsFromSelection(selectionFS fs.FS, globPattern string) (ManualAgentHintSet, error) {
	files, err := fs.Glob(selectionFS, globPattern)
	if err != nil {
		return ManualAgentHintSet{}, fmt.Errorf("list selection hints: %w", err)
	}
	sort.Strings(files)
	hints := ManualAgentHintSet{
		Revisions: map[string]ManualAgentHintRevision{
			selectionRevisionID: {
				GeneratedBy: "human",
				Reason:      "reviewed Agent selection prose under internal/cli/schema_hints/selection",
			},
		},
		Products: map[string]ManualAgentProductHint{},
		Tools:    map[string]ManualAgentToolHint{},
	}
	for _, name := range files {
		data, err := fs.ReadFile(selectionFS, name)
		if err != nil {
			return ManualAgentHintSet{}, fmt.Errorf("read selection hint %s: %w", name, err)
		}
		var file hintDirFile
		if err := json.Unmarshal(data, &file); err != nil {
			return ManualAgentHintSet{}, fmt.Errorf("decode selection hint %s: %w", name, err)
		}
		productIDs := make([]string, 0, len(file.Products))
		for productID := range file.Products {
			productIDs = append(productIDs, productID)
		}
		sort.Strings(productIDs)
		for _, productID := range productIDs {
			product := file.Products[productID]
			reason := strings.TrimSpace(product.ReviewReason)
			if reason == "" {
				reason = "reviewed product selection"
			}
			evidence := append([]string{}, product.SourceRefs...)
			if len(evidence) == 0 {
				evidence = []string{"schema_hints/selection/" + filepath.Base(name)}
			}
			hints.Products[productID] = ManualAgentProductHint{
				AgentSummary: product.AgentSummary,
				UseWhen:      product.UseWhen,
				AvoidWhen:    product.AvoidWhen,
				Reviewed:     true,
				Revision:     selectionRevisionID,
				Reason:       reason,
				Evidence:     evidence,
			}
		}
		canonicals := make([]string, 0, len(file.Tools))
		for canonical := range file.Tools {
			canonicals = append(canonicals, canonical)
		}
		sort.Strings(canonicals)
		for _, canonical := range canonicals {
			tool := file.Tools[canonical]
			reason := strings.TrimSpace(tool.ReviewReason)
			if reason == "" {
				reason = "reviewed tool selection"
			}
			evidence := append([]string{}, tool.SourceRefs...)
			if len(evidence) == 0 {
				evidence = []string{"schema_hints/selection/" + filepath.Base(name)}
			}
			hints.Tools[canonical] = ManualAgentToolHint{
				AgentSummary: tool.AgentSummary,
				UseWhen:      tool.UseWhen,
				AvoidWhen:    tool.AvoidWhen,
				Examples:     tool.Examples,
				Reviewed:     true,
				Revision:     selectionRevisionID,
				Reason:       reason,
				Evidence:     evidence,
			}
		}
	}
	return hints, nil
}
