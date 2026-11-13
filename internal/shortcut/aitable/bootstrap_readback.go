// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
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

// A created table was seen flipping between readable and not found for about
// 50s in a live QwenWork run (2026-11-04), so one table gets a 60s budget.
var createdTableReadbackDelays = [...]time.Duration{
	250 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second,
	8 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second,
}

// readCreatedTableEventually reads back the table and fields of an ID that
// create_table just returned. Creation is never repeated; the caller keeps its
// recovery checkpoint when the readback fails. A nil table means the table
// identity itself was not confirmed.
func readCreatedTableEventually(rt *shortcut.RuntimeContext, baseID, tableID string) (map[string]any, []map[string]any, error) {
	// One budget covers both reads, requests included, so a slow transport
	// cannot multiply the wait per probe.
	original := rt.Command().Context()
	ctx, cancel := context.WithTimeout(original, 60*time.Second)
	defer cancel()
	rt.Command().SetContext(ctx)
	defer rt.Command().SetContext(original)

	// get_tables wraps the sheet service's not-found text for this exact table
	// as GET_TABLES_ERROR, a code it also uses for unrelated failures.
	tableMissing := func(err error) bool {
		return strings.Contains(err.Error(), "fail to find the resource Sheet identified by "+tableID)
	}
	// Once get_tables has confirmed the ID, get_fields' ResourceNotFound can
	// only mean the field schema is not readable yet.
	fieldsMissing := func(err error) bool {
		var cliErr *helpers.CLIError
		return tableMissing(err) || errors.As(err, &cliErr) && cliErr.ServerCode == "InvalidRequest.ResourceNotFound"
	}

	var table map[string]any
	err := retryCreatedTableRead(ctx, tableID, tableMissing, func() (bool, error) {
		detail, err := rt.CallMCPData(serverMain, "get_tables", map[string]any{"baseId": baseID, "tableIds": []string{tableID}})
		if err != nil {
			return false, err
		}
		tables, found := findNamedObjectList(detail, "tables", "tableList")
		if !found {
			return false, fmt.Errorf("get_tables response is missing the tables collection for created tableId %s", tableID)
		}
		for _, candidate := range tables {
			if stringValue(candidate, "tableId", "sheetId", "id") == tableID {
				table = candidate
				return true, nil
			}
		}
		if len(tables) != 0 {
			return false, fmt.Errorf("get_tables does not identify created tableId %s; resume verification without recreating the table", tableID)
		}
		return false, nil
	})
	if err != nil {
		return nil, nil, err
	}
	var fields []map[string]any
	err = retryCreatedTableRead(ctx, tableID, fieldsMissing, func() (bool, error) {
		data, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": baseID, "tableId": tableID})
		if err != nil {
			return false, err
		}
		list, found := findNamedObjectList(data, "fields", "fieldList")
		if !found {
			return false, fmt.Errorf("field read-back for table %s is missing the fields collection", tableID)
		}
		fields = list
		return true, nil
	})
	return table, fields, err
}

// retryCreatedTableRead repeats a read-only probe while the created table is
// empty or reported missing; any other error stops immediately.
func retryCreatedTableRead(ctx context.Context, tableID string, missing func(error) bool, read func() (bool, error)) error {
	var lastErr error
	for attempt := 0; attempt <= len(createdTableReadbackDelays); attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		done, err := read()
		if done || (err != nil && !missing(err)) {
			return err
		}
		lastErr = err
		if attempt == len(createdTableReadbackDelays) {
			break
		}
		if waitErr := bootstrapReadbackWait(ctx, createdTableReadbackDelays[attempt]); waitErr != nil {
			if lastErr != nil {
				return lastErr
			}
			return waitErr
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("created tableId %s is still not readable after %d reads; resume verification without recreating the table", tableID, len(createdTableReadbackDelays)+1)
}
