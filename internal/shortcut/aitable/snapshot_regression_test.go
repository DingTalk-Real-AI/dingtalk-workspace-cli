// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageViewGetPreservesConfiguration(t *testing.T) {
	config := map[string]any{
		"viewId": "v", "viewName": "任务", "viewType": "Grid",
		"filter":  map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "gt", "operands": []any{"f", float64(3)}}}},
		"sort":    []any{map[string]any{"fieldId": "f", "desc": true}},
		"group":   []any{map[string]any{"fieldId": "f"}},
		"columns": []any{map[string]any{"fieldId": "f", "width": float64(120)}},
		"custom":  map[string]any{"futureOption": false},
	}
	views, err := viewGetProject(map[string]any{"views": []any{config}})
	if err != nil || len(views) != 1 || !reflect.DeepEqual(views[0], config) {
		t.Fatalf("configuration lost: %v %v", views, err)
	}
	views[0]["viewName"] = "changed"
	if config["viewName"] != "任务" {
		t.Fatal("projection mutates source row")
	}
}

func TestCrossPlatformCoverageBootstrapWaitsForCreatedTableWithoutRewriting(t *testing.T) {
	for _, command := range []string{"+base-bootstrap", "+table-bootstrap"} {
		for _, scenario := range []string{"visible later", "always empty", "permission error", "wrong id", "malformed"} {
			t.Run(command+"/"+scenario, func(t *testing.T) {
				testseam.Swap(t, &bootstrapReadbackWait, func(context.Context, time.Duration) error { return nil })
				steps := []upsertByKeyStep{}
				args := []string{"--base-id", "b", "--name", "任务", "--fields", `[]`, "--yes"}
				if command == "+base-bootstrap" {
					steps = append(steps, upsertByKeyStep{text: `{"baseId":"b"}`}, upsertByKeyStep{text: `{"baseId":"b"}`})
					args = []string{"--name", "测试", "--tables", marshalBootstrapTables(t, nil), "--yes"}
				}
				steps = append(steps, upsertByKeyStep{text: `{"tableId":"t"}`})
				reads := 1
				switch scenario {
				case "visible later":
					reads = 2
					steps = append(steps, upsertByKeyStep{text: `{"tables":[]}`}, upsertByKeyStep{text: `{"tables":[{"tableId":"t"}]}`}, upsertByKeyStep{text: `{"fields":[{"fieldId":"f","fieldName":"名称","name":"名称","type":"text"}]}`})
				case "always empty":
					reads = 4
					for i := 0; i < reads; i++ {
						steps = append(steps, upsertByKeyStep{text: `{"tables":[]}`})
					}
				case "permission error":
					steps = append(steps, upsertByKeyStep{err: errors.New("permission denied")})
				case "wrong id":
					steps = append(steps, upsertByKeyStep{text: `{"tables":[{"tableId":"other","description":"t"}]}`})
				case "malformed":
					steps = append(steps, upsertByKeyStep{text: `{"summary":"t"}`})
				}
				caller := &upsertByKeyCaller{steps: steps}
				_, err := runAITableCompositeCLI(t, caller, command, args...)
				if (err == nil) != (scenario == "visible later") {
					t.Fatalf("err=%v calls=%v", err, caller.calls)
				}
				creates, actualReads := 0, 0
				for _, call := range caller.calls {
					if call.tool == "create_table" {
						creates++
					}
					if call.tool == "get_tables" {
						actualReads++
						if !reflect.DeepEqual(call.args["tableIds"], []string{"t"}) {
							t.Fatalf("wrong target: %v", call.args)
						}
					}
				}
				if creates != 1 || actualReads != reads {
					t.Fatalf("creates=%d reads=%d want 1/%d", creates, actualReads, reads)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageBootstrapReadbackWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bootstrapReadbackWait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestCrossPlatformCoverageDatasourceUpdateRejectsIncompleteReadback(t *testing.T) {
	caller := &datasourceCoverageCaller{}
	err := runDatasourceShortcutCLI(t, caller, "+datasource-update", "--base-id", "b", "--table-id", "t", "--auto")
	if err == nil || !strings.Contains(err.Error(), "source-config") || len(caller.argLog) != 1 || caller.toolLog[0] != "get_datasource_config" {
		t.Fatalf("err=%v calls=%v", err, caller.argLog)
	}
}
