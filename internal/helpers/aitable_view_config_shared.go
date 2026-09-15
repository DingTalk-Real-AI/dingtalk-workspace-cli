// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import "context"

// NormalizeAITableViewConfig is shared by the atomic and Shortcut entrypoints.
// It normalizes the caller-owned map, resolves typed entity filters and rejects
// config blocks owned by dedicated endpoints before any write.
func NormalizeAITableViewConfig(ctx context.Context, baseID, tableID string, config map[string]any) error {
	return normalizeAndValidateViewConfig(ctx, baseID, tableID, config)
}
