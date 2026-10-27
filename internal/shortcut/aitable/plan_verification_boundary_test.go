// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"testing"
)

func boundaryObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestCrossPlatformCoveragePlanVerificationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, tool, args, resources, receipt, read string
		wantErr                                    bool
	}{
		{name: "no args", args: `{}`, resources: `{}`, read: `{}`, wantErr: true},
		{name: "wrong base", args: `{"baseId":"b"}`, resources: `{}`, read: `{"baseId":"other"}`, wantErr: true},
		{name: "wrong table", args: `{"tableId":"t"}`, resources: `{}`, read: `{"tableId":"other"}`, wantErr: true},
		{name: "dashboard wrong name", tool: "create_dashboard", args: `{"config":{"name":"expected"}}`, resources: `{"dashboardId":"d"}`, read: `{"dashboardId":"d","name":"other"}`, wantErr: true},
		{name: "dashboard wrong config", tool: "create_dashboard", args: `{"config":{"theme":"expected"}}`, resources: `{"dashboardId":"d"}`, read: `{"dashboardId":"d","theme":"other"}`, wantErr: true},
		{name: "dashboard complete", tool: "create_dashboard", args: `{"config":{"theme":"expected"}}`, resources: `{"dashboardId":"d"}`, read: `{"dashboardId":"d","theme":"expected"}`},
		{name: "no table identity", tool: "create_table", args: `{"fields":[]}`, resources: `{}`, read: `{}`, wantErr: true},
		{name: "table mismatch", tool: "create_table", args: `{"tableName":"expected","fields":[]}`, resources: `{"tableId":"t"}`, read: `{"tables":[{"tableId":"t","name":"other"}]}`, wantErr: true},
		{name: "no fields", tool: "create_fields", args: `{"tableId":"t","fields":[]}`, resources: `{}`, read: `{}`, wantErr: true},
		{name: "no field receipts", tool: "create_fields", args: `{"tableId":"t","fields":[{"fieldName":"F","type":"text"}]}`, resources: `{}`, read: `{"fields":[]}`, receipt: `{}`, wantErr: true},
		{name: "wrong field type", tool: "create_fields", args: `{"tableId":"t","fields":[{"fieldName":"F","type":"text"}]}`, resources: `{}`, read: `{"fields":[{"fieldId":"F","fieldName":"F","type":"number"}]}`, receipt: `{"results":[{"fieldId":"F","fieldName":"F","success":true}]}`, wantErr: true},
		{name: "view absent", tool: "create_view", args: `{"tableId":"t"}`, resources: `{"viewId":"v"}`, read: `{"views":[]}`, wantErr: true},
		{name: "view duplicate", tool: "create_view", args: `{"tableId":"t"}`, resources: `{"viewId":"v"}`, read: `{"views":[{"viewId":"v"},{"viewId":"v"}]}`, wantErr: true},
		{name: "view scope", tool: "create_view", args: `{"tableId":"t"}`, resources: `{"viewId":"v"}`, read: `{"views":[{"viewId":"v","tableId":"other"}]}`, wantErr: true},
		{name: "view no list", tool: "update_view", args: `{"tableId":"t","viewId":"v"}`, resources: `{}`, read: `{}`, wantErr: true},
		{name: "view description", tool: "update_view", args: `{"baseId":"b","tableId":"t","viewId":"v","viewDescription":"desc"}`, resources: `{}`, read: `{"views":[{"viewId":"v","description":"desc"}]}`},
		{name: "view wrong name", tool: "update_view", args: `{"tableId":"t","viewId":"v","viewName":"name"}`, resources: `{}`, read: `{"views":[{"viewId":"v","name":"other"}]}`, wantErr: true},
		{name: "view invalid config", tool: "update_view", args: `{"tableId":"t","viewId":"v","config":false}`, resources: `{}`, read: `{"views":[{"viewId":"v"}]}`, wantErr: true},
		{name: "no records", tool: "update_records", args: `{"tableId":"t","records":[]}`, resources: `{}`, read: `{}`, wantErr: true},
		{name: "invalid row", tool: "update_records", args: `{"tableId":"t","records":[false]}`, resources: `{}`, read: `{"records":[]}`, wantErr: true},
		{name: "no created IDs", tool: "create_records", args: `{"tableId":"t","records":[{"cells":{"F":"v"}}]}`, resources: `{}`, receipt: `{}`, read: `{"records":[]}`, wantErr: true},
		{name: "empty record", tool: "update_records", args: `{"tableId":"t","records":[{"recordId":"R","cells":{}}]}`, resources: `{}`, read: `{"records":[{"recordId":"R"}]}`, wantErr: true},
		{name: "record values wrong", tool: "update_records", args: `{"tableId":"t","records":[{"recordId":"R","cells":{"F":"v"}}]}`, resources: `{}`, read: `{"records":[{"recordId":"unrelated","cells":{"F":"v"}},{"recordId":"R","cells":{"F":"wrong"}}]}`, wantErr: true},
		{name: "record IDs bind values", tool: "update_records", args: `{"tableId":"t","records":[{"recordId":"R","cells":{"F":"v"}},{"recordId":"R2","cells":{"F":"v2"}}]}`, resources: `{}`, read: `{"records":[{"recordId":"R","cells":{"F":"v"}},{"recordId":"R2","cells":{"F":"v2"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := buildPlanStepState{Arguments: boundaryObject(t, tc.args), Resources: boundaryObject(t, tc.resources)}
			if tc.receipt != "" {
				current.Receipt = boundaryObject(t, tc.receipt)
			}
			err := verifyBuildPlanIntent(nil, &buildPlanState{BaseID: "b"}, buildPlanStep{Tool: tc.tool}, &current, boundaryObject(t, tc.read))
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected verification: %v", err)
			}
		})
	}
}
func TestCrossPlatformCoveragePlanCreatedTableFields(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		callErr      error
		wantErr      bool
	}{
		{name: "dependency fails", callErr: errors.New("timeout"), wantErr: true},
		{name: "wrong field base", fields: `{"baseId":"other"}`, wantErr: true},
		{name: "wrong field table", fields: `{"tableId":"other"}`, wantErr: true},
		{name: "duplicate names", fields: `{"fields":[{"fieldId":"F","fieldName":"F","type":"text"},{"fieldId":"F2","fieldName":"F","type":"text"}]}`, wantErr: true},
		{name: "complete with unnamed extras", fields: `{"fields":[{"fieldId":"F","fieldName":"F","type":"text"},{"fieldId":"other","type":"text"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.fields, err: tc.callErr}}}
			rt := boundaryRuntime(t, ChartsCreate, nil, nil, caller)
			current := buildPlanStepState{Arguments: boundaryObject(t, `{"tableName":"table","fields":[{"fieldName":"F","type":"text"}]}`), Resources: map[string]any{"tableId": "t"}}
			err := verifyBuildPlanIntent(rt, &buildPlanState{BaseID: "b"}, buildPlanStep{Tool: "create_table"}, &current, boundaryObject(t, `{"tables":[{"tableId":"t","tableName":"table"}]}`))
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
			if !tc.wantErr && current.Resources["fieldsByName"] == nil {
				t.Fatal("verified field aliases not retained")
			}
		})
	}
}
func TestCrossPlatformCoveragePlanRecordReconciliationBoundaries(t *testing.T) {
	for _, name := range []string{"missing token", "cached", "read error", "partial IDs"} {
		t.Run(name, func(t *testing.T) {
			current := buildPlanStepState{Arguments: boundaryObject(t, `{"tableId":"t","clientToken":"token","records":[{"cells":{"F":"v"}}]}`)}
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"data":{"baseId":"b","tableId":"t","clientToken":"token","state":"applied","recordIds":[]}}`}}}
			if name == "missing token" {
				delete(current.Arguments, "clientToken")
			}
			if name == "cached" {
				current.Reconciliation = boundaryObject(t, `{"data":{"baseId":"b","tableId":"t","clientToken":"token","state":"applied","recordIds":["R"]}}`)
			}
			if name == "read error" {
				caller.steps[0].err = errors.New("timeout")
			}
			rt := boundaryRuntime(t, ChartsCreate, nil, nil, caller)
			err := reconcileBuildPlanRecords(rt, &buildPlanState{BaseID: "b"}, buildPlanStep{ID: "rows"}, &current)
			if name == "cached" {
				if err != nil || len(caller.calls) != 0 {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("partial reconciliation accepted")
			}
		})
	}
}

func TestCrossPlatformCoveragePlanExtraDuplicateFieldNames(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"fields":[{"fieldId":"F","fieldName":"F","type":"text"},{"fieldId":"X","fieldName":"extra","type":"text"},{"fieldId":"X2","fieldName":"extra","type":"text"}]}`}}}
	rt := boundaryRuntime(t, ChartsCreate, nil, nil, caller)
	current := buildPlanStepState{Arguments: boundaryObject(t, `{"tableName":"table","fields":[{"fieldName":"F","type":"text"}]}`), Resources: map[string]any{"tableId": "t"}}
	if err := verifyBuildPlanIntent(rt, &buildPlanState{BaseID: "b"}, buildPlanStep{Tool: "create_table"}, &current, boundaryObject(t, `{"tables":[{"tableId":"t","tableName":"table"}]}`)); err == nil {
		t.Fatal("ambiguous extra fields bound")
	}
	current = buildPlanStepState{Arguments: boundaryObject(t, `{"records":[{"cells":{"F":"v"}}]}`), Receipt: boundaryObject(t, `{"newRecordIds":["R"]}`)}
	if err := reconcileBuildPlanRecords(rt, &buildPlanState{BaseID: "b"}, buildPlanStep{}, &current); err != nil {
		t.Fatal("full original IDs not accepted", err)
	}
}
