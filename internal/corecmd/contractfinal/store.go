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

package contractfinal

import (
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
)

var contractFinalByCommand sync.Map // *cobra.Command → *contract.ContractFinalPayload

// RegisterRuntimeContractFinal annotates dws.schema.contract then stores the
// typed final Schema overlay. This is the atomic annotate+store implementation.
//
// Ownership lives under the command framework (this package). All callers —
// products and corecmd.AttachContract alike — call this function directly.
func RegisterRuntimeContractFinal(cmd *cobra.Command, payload contract.ContractFinalPayload) {
	if cmd == nil {
		return
	}
	runtimeannotate.AnnotateRuntimeContract(cmd)
	p := cloneContractFinalPayload(payload)
	contractFinalByCommand.Store(cmd, &p)
}

// RuntimeContractFinal returns the registered final Schema overlay (read-only).
func RuntimeContractFinal(cmd *cobra.Command) (contract.ContractFinalPayload, bool) {
	if cmd == nil {
		return contract.ContractFinalPayload{}, false
	}
	raw, ok := contractFinalByCommand.Load(cmd)
	if !ok {
		return contract.ContractFinalPayload{}, false
	}
	p, ok := raw.(*contract.ContractFinalPayload)
	if !ok || p == nil {
		return contract.ContractFinalPayload{}, false
	}
	return cloneContractFinalPayload(*p), true
}

func cloneContractFinalPayload(in contract.ContractFinalPayload) contract.ContractFinalPayload {
	out := in
	out.Positionals = cloneSlice(in.Positionals)
	out.Parameters = cloneSlice(in.Parameters)
	for i := range out.Parameters {
		out.Parameters[i].Enum = cloneSlice(in.Parameters[i].Enum)
		if in.Parameters[i].Required != nil {
			required := *in.Parameters[i].Required
			out.Parameters[i].Required = &required
		}
	}
	if in.Safety != nil {
		value := *in.Safety
		out.Safety = &value
	}
	if in.DryRun != nil {
		value := *in.DryRun
		out.DryRun = &value
	}
	if in.Result != nil {
		value := *in.Result
		value.Outcomes = cloneSlice(in.Result.Outcomes)
		value.DataSchema = cloneSlice(in.Result.DataSchema)
		value.SensitivePaths = cloneSlice(in.Result.SensitivePaths)
		if in.Result.NDJSON != nil {
			ndjson := *in.Result.NDJSON
			ndjson.RecordSchema = cloneSlice(in.Result.NDJSON.RecordSchema)
			value.NDJSON = &ndjson
		}
		if in.Result.Pagination != nil {
			pagination := *in.Result.Pagination
			value.Pagination = &pagination
		}
		out.Result = &value
	}
	if in.Interface != nil {
		value := *in.Interface
		if in.Interface.Ref != nil {
			ref := *in.Interface.Ref
			value.Ref = &ref
		}
		out.Interface = &value
	}
	if in.Selection != nil {
		value := *in.Selection
		value.UseWhen = cloneSlice(in.Selection.UseWhen)
		value.AvoidWhen = cloneSlice(in.Selection.AvoidWhen)
		value.Prerequisites = cloneSlice(in.Selection.Prerequisites)
		value.Tips = cloneSlice(in.Selection.Tips)
		value.WorkflowRefs = cloneSlice(in.Selection.WorkflowRefs)
		value.Examples = cloneSlice(in.Selection.Examples)
		value.SourceRefs = cloneSlice(in.Selection.SourceRefs)
		value.ExampleDispositions = cloneSlice(in.Selection.ExampleDispositions)
		for i := range value.ExampleDispositions {
			if in.Selection.ExampleDispositions[i].Index != nil {
				index := *in.Selection.ExampleDispositions[i].Index
				value.ExampleDispositions[i].Index = &index
			}
		}
		if in.Selection.Reviewed != nil {
			reviewed := *in.Selection.Reviewed
			value.Reviewed = &reviewed
		}
		out.Selection = &value
	}
	if in.Identity != nil {
		value := *in.Identity
		value.Aliases = cloneSlice(in.Identity.Aliases)
		out.Identity = &value
	}
	return out
}

func cloneSlice[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

// HasRuntimeContractFinal reports whether the leaf has a registered final overlay.
func HasRuntimeContractFinal(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	_, ok := contractFinalByCommand.Load(cmd)
	return ok
}

// ResolveRuntimeSafety finds the live ContractFinal safety declaration for an
// invocation identity. declared distinguishes a matched declaration whose
// safety is unavailable or conflicting from a legacy invocation with no unified
// declaration context. Repeated equivalent command-tree registrations are
// accepted; conflicting matches fail closed with ok=false.
func ResolveRuntimeSafety(canonicalPath, cliPath string) (safety contract.SafetySpec, declared, ok bool) {
	canonicalPath = strings.TrimSpace(canonicalPath)
	cliPath = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cliPath), "dws "))

	var resolved contract.SafetySpec
	contractFinalByCommand.Range(func(_, raw any) bool {
		payload, valid := raw.(*contract.ContractFinalPayload)
		if !valid || payload == nil || payload.Identity == nil ||
			!runtimeIdentityMatches(*payload.Identity, canonicalPath, cliPath) {
			return true
		}
		declared = true
		if payload.Safety == nil {
			ok = false
			return false
		}
		candidate := normalizedRuntimeSafety(*payload.Safety)
		if !ok {
			resolved = candidate
			ok = true
			return true
		}
		if resolved != candidate {
			ok = false
			return false
		}
		return true
	})
	return resolved, declared, ok
}

func runtimeIdentityMatches(identity contract.ToolIdentitySpec, canonicalPath, cliPath string) bool {
	if canonicalPath != "" {
		for _, value := range []string{identity.CanonicalPath, identity.Path} {
			if strings.TrimSpace(value) == canonicalPath {
				return true
			}
		}
	}
	if cliPath == "" {
		return false
	}
	for _, value := range []string{identity.PrimaryCLIPath, identity.CLIPath} {
		if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "dws ")) == cliPath {
			return true
		}
	}
	return false
}

func normalizedRuntimeSafety(safety contract.SafetySpec) contract.SafetySpec {
	safety.Effect = strings.TrimSpace(safety.Effect)
	safety.EffectSource = strings.TrimSpace(safety.EffectSource)
	safety.Risk = strings.TrimSpace(safety.Risk)
	safety.Confirmation = strings.TrimSpace(safety.Confirmation)
	safety.Idempotency = strings.TrimSpace(safety.Idempotency)
	return safety
}
