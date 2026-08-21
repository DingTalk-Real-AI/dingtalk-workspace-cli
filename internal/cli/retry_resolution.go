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
	"reflect"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

type toolCallRetryResolution struct {
	idempotency string
	policy      *contract.RetryPolicySpec
	argumentKey string
	conflict    bool
}

func retryInterfaceRefKey(productID, rpcName string) InterfaceRefKey {
	return InterfaceRefKey{
		ProductID: strings.TrimSpace(productID),
		RPCName:   strings.TrimSpace(rpcName),
	}
}

func buildToolCallRetryLookup(loaded loadedSchemaCatalog) map[InterfaceRefKey]toolCallRetryResolution {
	if len(loaded.Registry.Products) == 0 {
		return buildToolCallRetryLookupFromSnapshot(loaded.Snapshot.Tools)
	}
	lookup := make(map[InterfaceRefKey]toolCallRetryResolution)
	for _, product := range loaded.Registry.Products {
		for _, tool := range product.Tools {
			if tool.Interface.Mode != contract.InterfaceModeMCP || tool.Interface.Ref == nil {
				continue
			}
			resolution := toolCallRetryResolution{idempotency: tool.Safety.Idempotency, policy: cloneRetryPolicy(tool.RetryPolicy)}
			if tool.RetryPolicy != nil {
				for _, parameter := range tool.Parameters {
					if parameter.Name == tool.RetryPolicy.KeyParameter {
						resolution.argumentKey = parameter.Property
						break
					}
				}
			}
			mergeToolCallRetryResolution(lookup, retryInterfaceRefKey(tool.Interface.Ref.ProductID, tool.Interface.Ref.RPCName), resolution)
		}
	}
	return lookup
}

func buildToolCallRetryLookupFromSnapshot(tools map[string]map[string]any) map[InterfaceRefKey]toolCallRetryResolution {
	lookup := make(map[InterfaceRefKey]toolCallRetryResolution)
	for _, tool := range tools {
		ref, _ := tool["interface_ref"].(map[string]any)
		productID, rpcName := schemaString(ref["product_id"]), schemaString(ref["rpc_name"])
		if schemaString(tool["interface_mode"]) != contract.InterfaceModeMCP || productID == "" || rpcName == "" {
			continue
		}
		policy := retryPolicyFromSchemaValue(tool["retry_policy"])
		resolution := toolCallRetryResolution{idempotency: schemaString(tool["idempotency"]), policy: policy}
		if policy != nil {
			parameters, _ := tool["parameters"].(map[string]any)
			parameter, _ := parameters[policy.KeyParameter].(map[string]any)
			resolution.argumentKey = schemaString(parameter["property"])
		}
		mergeToolCallRetryResolution(lookup, retryInterfaceRefKey(productID, rpcName), resolution)
	}
	return lookup
}

func mergeToolCallRetryResolution(lookup map[InterfaceRefKey]toolCallRetryResolution, key InterfaceRefKey, candidate toolCallRetryResolution) {
	current, exists := lookup[key]
	if !exists {
		lookup[key] = candidate
		return
	}
	if current.conflict || current.idempotency != candidate.idempotency || current.argumentKey != candidate.argumentKey || !reflect.DeepEqual(current.policy, candidate.policy) {
		lookup[key] = toolCallRetryResolution{conflict: true}
	}
}

func cloneRetryPolicy(in *contract.RetryPolicySpec) *contract.RetryPolicySpec {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func retryPolicyFromSchemaValue(value any) *contract.RetryPolicySpec {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var policy contract.RetryPolicySpec
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil
	}
	normalized, err := contract.NormalizeRetryPolicySpec(&policy, "<snapshot>")
	if err != nil {
		return nil
	}
	return normalized
}

// ResolveToolCallRetry resolves transport retry safety from an exact reviewed
// MCP interface identity and actual RPC arguments. Missing or conflicting
// interface mappings fail closed; no command, flag, or property name is inferred.
func ResolveToolCallRetry(productID, rpcName string, arguments map[string]any) contract.RetryDecision {
	_ = deliverySchemaCatalog()
	panicIfMetaIndexUnusable(runtimeDeliverySchemaMetaIndexErr)
	resolution, ok := toolCallRetryByInterface[retryInterfaceRefKey(productID, rpcName)]
	if !ok {
		return contract.RetryDecision{EffectiveIdempotency: "unknown", Reason: "interface_not_declared"}
	}
	if resolution.conflict {
		return contract.RetryDecision{EffectiveIdempotency: "unknown", Reason: "interface_retry_policy_conflict"}
	}
	return contract.ResolveRetryDecision(resolution.idempotency, resolution.policy, arguments, resolution.argumentKey)
}
