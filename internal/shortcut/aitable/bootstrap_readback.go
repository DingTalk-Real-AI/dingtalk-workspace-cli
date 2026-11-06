// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"fmt"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var bootstrapReadbackWait = func(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// verifyCreatedTableEventually only retries an explicit empty table collection
// for an already-created ID. Errors, malformed results and wrong IDs stop;
// creation is never repeated and the caller retains its recovery checkpoint.
// The matching table is returned so callers can preserve server navigation
// without another read or a second URL implementation.
func verifyCreatedTableEventually(rt *shortcut.RuntimeContext, baseID, tableID string) (map[string]any, error) {
	// Requests and backoff share one readback deadline. Do not multiply the
	// transport's full timeout by every eventual-consistency probe.
	original := rt.Command().Context()
	ctx, cancel := context.WithTimeout(original, 5*time.Second)
	defer cancel()
	rt.Command().SetContext(ctx)
	defer rt.Command().SetContext(original)
	delays := [...]time.Duration{250 * time.Millisecond, time.Second, 2 * time.Second}
	for attempt := 0; ; attempt++ {
		if err := rt.Command().Context().Err(); err != nil {
			return nil, err
		}
		detail, err := rt.CallMCPData(serverMain, "get_tables", map[string]any{"baseId": baseID, "tableIds": []string{tableID}})
		if err != nil {
			return nil, err
		}
		tables, found := findNamedObjectList(detail, "tables", "tableList")
		if !found {
			return nil, fmt.Errorf("get_tables response is missing the tables collection for created tableId %s", tableID)
		}
		for _, table := range tables {
			if stringValue(table, "tableId", "sheetId", "id") == tableID {
				return table, nil
			}
		}
		if len(tables) != 0 || attempt == len(delays) {
			return nil, fmt.Errorf("get_tables does not identify created tableId %s after %d reads; resume verification without recreating the table", tableID, attempt+1)
		}
		if err := bootstrapReadbackWait(rt.Command().Context(), delays[attempt]); err != nil {
			return nil, err
		}
	}
}
