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

package contract

import (
	"encoding/json"
	"strings"
)

// FieldProvenance records how one final field was selected. Value is raw JSON
// so provenance can describe strings, booleans and structured extension
// values without weakening the resolved ToolSpec itself.
type FieldProvenance struct {
	Value                json.RawMessage            `json:"value,omitempty"`
	Source               string                     `json:"source"`
	SourceRef            string                     `json:"source_ref,omitempty"`
	Precedence           string                     `json:"precedence,omitempty"`
	Resolution           string                     `json:"resolution"`
	ReviewReason         string                     `json:"review_reason,omitempty"`
	Candidates           []FieldCandidateProvenance `json:"candidates,omitempty"`
	OverriddenCandidates []FieldCandidateProvenance `json:"overridden_candidates,omitempty"`
}

// FieldCandidateProvenance retains one winning or non-winning source value.
type FieldCandidateProvenance struct {
	Value        json.RawMessage `json:"value,omitempty"`
	Source       string          `json:"source"`
	SourceRef    string          `json:"source_ref,omitempty"`
	Precedence   string          `json:"precedence,omitempty"`
	ReviewReason string          `json:"review_reason,omitempty"`
	Selected     *bool           `json:"selected,omitempty"`
}

// ResolvedFieldProvenance builds a single-winner provenance record for a
// ContractFinal / ProductDecl pass-through field.
func ResolvedFieldProvenance(value any, source, sourceRef, precedence, resolution, reviewReason string) FieldProvenance {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = json.RawMessage("null")
	}
	selected := true
	return FieldProvenance{
		Value:        append(json.RawMessage(nil), raw...),
		Source:       strings.TrimSpace(source),
		SourceRef:    strings.TrimSpace(sourceRef),
		Precedence:   strings.TrimSpace(precedence),
		Resolution:   strings.TrimSpace(resolution),
		ReviewReason: strings.TrimSpace(reviewReason),
		Candidates: []FieldCandidateProvenance{{
			Value:        append(json.RawMessage(nil), raw...),
			Source:       strings.TrimSpace(source),
			SourceRef:    strings.TrimSpace(sourceRef),
			Precedence:   strings.TrimSpace(precedence),
			ReviewReason: strings.TrimSpace(reviewReason),
			Selected:     &selected,
		}},
	}
}
