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

package compat

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

// buildOutputTransform compiles a CLIOutputFormat into a payload-shaping
// function applied to executor.Result.Response before the formatter runs.
// See discovery-schema-v3 §2.5.
//
// Apply order (deterministic): drop → rename → columns.
// Columns filter only takes effect under --format=table (the formatter
// consults it via the "_columns" marker key this function injects).
// Returns nil when the spec is empty so callers can skip wiring.
func buildOutputTransform(spec market.CLIOutputFormat) func(map[string]any) map[string]any {
	if len(spec.Drop) == 0 && len(spec.Rename) == 0 && len(spec.Columns) == 0 {
		return nil
	}
	dropped := append([]string(nil), spec.Drop...)
	renamed := make(map[string]string, len(spec.Rename))
	for k, v := range spec.Rename {
		renamed[k] = v
	}
	columns := append([]string(nil), spec.Columns...)

	return func(resp map[string]any) map[string]any {
		if resp == nil {
			return resp
		}
		applyDrop(resp, dropped)
		applyRename(resp, renamed)
		if len(columns) > 0 {
			resp["_columns"] = append([]string(nil), columns...)
		}
		return resp
	}
}

// applyDrop removes the named keys at the top level and one level of nested
// object. Missing keys are silently ignored. Keys with "." are treated as a
// two-part path (parent.child).
func applyDrop(m map[string]any, keys []string) {
	for _, key := range keys {
		if key == "" {
			continue
		}
		delete(m, key)
	}
	for _, v := range m {
		if inner, ok := v.(map[string]any); ok {
			for _, key := range keys {
				if key == "" {
					continue
				}
				delete(inner, key)
			}
		}
	}
}

// applyRename moves fields from src key to dst key at top level and one level
// of nested object. Collisions overwrite silently. Missing src keys are
// no-ops.
func applyRename(m map[string]any, mapping map[string]string) {
	if len(mapping) == 0 {
		return
	}
	// First pass: top level.
	for src, dst := range mapping {
		if src == "" || dst == "" || src == dst {
			continue
		}
		if v, ok := m[src]; ok {
			m[dst] = v
			delete(m, src)
		}
	}
	// Second pass: one level nested.
	for _, v := range m {
		inner, ok := v.(map[string]any)
		if !ok {
			continue
		}
		for src, dst := range mapping {
			if src == "" || dst == "" || src == dst {
				continue
			}
			if val, ok := inner[src]; ok {
				inner[dst] = val
				delete(inner, src)
			}
		}
	}
}
