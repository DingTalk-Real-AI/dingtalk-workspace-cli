// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import "fmt"

// RecordClientTokenHeader carries the persisted creation intent outside the
// published record_upsert business schema. It is not a tracing identifier.
const RecordClientTokenHeader = "X-Dingtalk-Record-Client-Token"

// RecordUpsertWireParams keeps the legacy tool arguments unchanged while moving
// the existing CLI token to invocation-scoped transport metadata. Never mutate
// params: audit and recovery must retain the original intent.
func RecordUpsertWireParams(product, tool string, params map[string]any) (map[string]any, string, error) {
	if (product != "aitable" && product != "aitable-helper") || tool != "record_upsert" {
		return params, "", nil
	}
	raw, exists := params["clientToken"]
	if !exists {
		return params, "", nil
	}
	token, ok := raw.(string)
	if !ok {
		return nil, "", fmt.Errorf("clientToken must be a UUID v4 string")
	}
	if err := ValidateClientToken(token); err != nil {
		return nil, "", err
	}
	wire := make(map[string]any, len(params))
	for key, value := range params {
		if key != "clientToken" {
			wire[key] = value
		}
	}
	return wire, token, nil
}
