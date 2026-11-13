// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoveragePlanRecordCandidatesMatchOneToOne(t *testing.T) {
	for _, tc := range []struct {
		name, tool, records, actual string
		wantErr                     bool
	}{
		{"same values missing candidate", "create_records", `[{"cells":{"F":"same"}},{"cells":{"F":"same"}}]`, `[{"recordId":"R1","cells":{"F":"same"}},{"recordId":"OLD","cells":{"F":"same"}}]`, true},
		{"same values complete unordered", "create_records", `[{"cells":{"F":"same"}},{"cells":{"F":"same"}}]`, `[{"recordId":"OLD","cells":{"F":"same"}},{"recordId":"R2","cells":{"F":"same"}},{"recordId":"R1","cells":{"F":"same"}}]`, false},
		{"duplicate candidate row", "create_records", `[{"cells":{"F":"a"}},{"cells":{"F":"b"}}]`, `[{"recordId":"R1","cells":{"F":"a"}},{"recordId":"R1","cells":{"F":"b"}},{"recordId":"R2","cells":{"F":"b"}}]`, true},
		{"wrong cells", "create_records", `[{"cells":{"F":"a"}},{"cells":{"F":"b"}}]`, `[{"recordId":"R1","cells":{"F":"a"}},{"recordId":"R2","cells":{"F":"other"}}]`, true},
		{"empty cells", "create_records", `[{"cells":{}},{"cells":{"F":"b"}}]`, `[{"recordId":"R1","cells":{}},{"recordId":"R2","cells":{"F":"b"}}]`, true},
		{"overlapping assertions need reassignment", "create_records", `[{"cells":{"F":"a"}},{"cells":{"F":"a","N":1}}]`, `[{"recordId":"R1","cells":{"F":"a","N":1}},{"recordId":"R2","cells":{"F":"a","N":2}}]`, false},
		{"update IDs bind values", "update_records", `[{"recordId":"R1","cells":{"F":"a"}},{"recordId":"R2","cells":{"F":"b"}}]`, `[{"recordId":"R2","cells":{"F":"a"}},{"recordId":"R1","cells":{"F":"b"}}]`, true},
		{"update IDs complete unordered", "update_records", `[{"recordId":"R1","cells":{"F":"a"}},{"recordId":"R2","cells":{"F":"b"}}]`, `[{"recordId":"R2","cells":{"F":"b"}},{"recordId":"R1","cells":{"F":"a"}}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := buildPlanStepState{
				Arguments: boundaryObject(t, `{"tableId":"t","records":`+tc.records+`}`),
				Receipt:   boundaryObject(t, `{"data":{"newRecordIds":["R1","R2"]}}`),
			}
			err := verifyBuildPlanIntent(nil, &buildPlanState{BaseID: "b"}, buildPlanStep{Tool: tc.tool}, &current,
				boundaryObject(t, `{"data":{"baseId":"b","tableId":"t","records":`+tc.actual+`}}`))
			if (err != nil) != tc.wantErr {
				t.Fatalf("candidate identity/value verification: %v, want error=%v", err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoveragePlanUnknownCreationCandidateResumesReadOnly(t *testing.T) {
	for _, chart := range []bool{false, true} {
		t.Run(fmt.Sprintf("chart=%t", chart), func(t *testing.T) {
			planPath, checkpoint := setupBuildPlan(t)
			plan, _, err := readBuildPlan(planPath)
			if err != nil {
				t.Fatal(err)
			}
			writer, reader, key, id := "create_dashboard", "get_dashboard", "dashboardId", "d"
			candidate := map[string]any{"baseId": "b", "dashboardId": "d", "mutationSubmitted": true, "deliveryStatus": "unknown", "uid": "private-identity"}
			read := `{"status":"success","data":{"baseId":"b","dashboardId":"d","dashboardName":"看板"}}`
			if chart {
				writer, reader, key, id = "create_chart", "get_chart", "chartId", "c"
				candidate["chartId"] = "c"
				plan.Steps[0].Tool = writer
				plan.Steps[0].Arguments = boundaryObject(t, `{"baseId":"b","dashboardId":"d","config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}`)
				plan.Steps[0].Verify = buildPlanReadback{Tool: reader, Arguments: boundaryObject(t, `{"baseId":"b","dashboardId":"d","chartId":{"$ref":"board.chartId"}}`), Expected: boundaryObject(t, `{"data":{"chartId":{"$ref":"board.chartId"}}}`)}
				writeBoundaryJSON(t, planPath, plan)
				read = `{"status":"success","data":{"baseId":"b","dashboardId":"d","chartId":"c","config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}}`
			}
			errorReceipt, _ := json.Marshal(map[string]any{"status": "error", "error": map[string]any{
				"code": strings.ToUpper(writer) + "_OPERATION_ERROR", "message": "delivery unknown; retain the candidate", "retryable": false, "details": candidate,
			}})
			writes, reads := 0, 0
			caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
				switch tool {
				case writer:
					writes++
					return string(errorReceipt), nil
				case reader:
					reads++
					if reads == 1 {
						return "", errors.New("read temporarily unavailable")
					}
					return read, nil
				case "get_dashboard":
					return `{"data":{"baseId":"b","dashboardId":"d","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`, nil
				default:
					t.Fatalf("unexpected tool: %s", tool)
					return "", nil
				}
			}}
			if _, err = runAITableCompositeCLI(t, caller, "+build-plan", "--plan", planPath, "--checkpoint", checkpoint, "--yes"); err == nil {
				t.Fatal("unavailable readback claimed completion")
			}
			raw, err := os.ReadFile(checkpoint)
			if err != nil {
				t.Fatal(err)
			}
			var saved buildPlanState
			if err = json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Steps[0].Resources[key] != id || saved.Steps[0].Receipt["status"] != "error" || strings.Contains(string(raw), "private-identity") {
				t.Fatalf("lost or unsafe candidate evidence: %s", raw)
			}
			for attempt := 0; attempt < 2; attempt++ {
				out, err := runAITableCompositeCLI(t, caller, "+build-plan-verify", "--plan", planPath, "--checkpoint", checkpoint)
				if err != nil || !strings.Contains(out, `"status": "verified"`) || writes != 1 {
					t.Fatalf("read-only resume replayed or failed: writes=%d, out=%s, err=%v", writes, out, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageUnknownBatchNeverRetainsPlannedStatus(t *testing.T) {
	for _, transportFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("transport=%t", transportFailure), func(t *testing.T) {
			unknown := upsertByKeyStep{text: `{"status":"error","error":{"code":"BATCH_CREATE_CHARTS_OPERATION_ERROR","message":"delivery unknown sentinel","retryable":false,"details":{"mutationSubmitted":true,"deliveryStatus":"unknown"}}}`}
			if transportFailure {
				unknown = upsertByKeyStep{err: errors.New("transport unknown sentinel")}
			}
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
				{text: `{"data":{"baseId":"b","dashboardId":"d","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`}, unknown,
			}}
			_, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "b", "--dashboard-id", "d", "--charts", `[{"config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`, "--server-batch", "--yes")
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Retryable || typed.ExecutionStarted == nil || !*typed.ExecutionStarted {
				t.Fatalf("unknown write lost effect evidence: %v", err)
			}
			result := typed.Details["result"].(map[string]any)
			items := result["items"].([]map[string]any)
			if result["writeSubmissions"] != 1 || items[0]["status"] != "unknown" || len(caller.calls) != 2 || !strings.Contains(fmt.Sprint(typed.Cause), "unknown sentinel") {
				t.Fatalf("unknown batch status/cause/replay: %+v, cause=%v", result, typed.Cause)
			}
		})
	}
}

func TestCrossPlatformCoveragePlanCreationCandidatesRejectUnboundEvidence(t *testing.T) {
	for _, name := range []string{"unsupported tool", "untyped error", "no execution evidence", "pre-write refusal", "not submitted", "not unknown", "wrong base", "missing dashboard", "non-string dashboard", "padded dashboard", "missing chart", "non-string chart", "padded chart", "wrong parent", "valid dashboard", "valid chart"} {
		t.Run(name, func(t *testing.T) {
			tool := "create_chart"
			details := map[string]any{"baseId": "b", "dashboardId": "d", "chartId": "c", "mutationSubmitted": true, "deliveryStatus": "unknown", "uid": "private"}
			options := []apperrors.Option{apperrors.WithDetails(details), apperrors.WithExecutionStarted(true)}
			switch name {
			case "unsupported tool":
				tool = "create_table"
			case "no execution evidence":
				options = options[:1]
			case "pre-write refusal":
				options[1] = apperrors.WithExecutionStarted(false)
			case "not submitted":
				details["mutationSubmitted"] = false
			case "not unknown":
				details["deliveryStatus"] = "accepted"
			case "wrong base":
				details["baseId"] = "other"
			case "missing dashboard":
				delete(details, "dashboardId")
			case "non-string dashboard":
				details["dashboardId"] = 7
			case "padded dashboard":
				details["dashboardId"] = " d "
			case "missing chart":
				delete(details, "chartId")
			case "non-string chart":
				details["chartId"] = false
			case "padded chart":
				details["chartId"] = " c "
			case "wrong parent":
				details["dashboardId"] = "other"
			case "valid dashboard":
				tool = "create_dashboard"
			}
			var cause error = apperrors.NewAPI("delivery unknown", options...)
			if name == "untyped error" {
				cause = errors.New("candidate chartId=c in prose is not evidence")
			}
			candidate := buildPlanCreationCandidate(tool, "b", map[string]any{"dashboardId": "d"}, cause)
			if strings.HasPrefix(name, "valid ") {
				if candidate["dashboardId"] != "d" || candidate["uid"] != nil || (tool == "create_chart" && candidate["chartId"] != "c") {
					t.Fatalf("safe candidate lost or diagnostics leaked: %+v", candidate)
				}
			} else if candidate != nil {
				t.Fatalf("unbound creation evidence accepted: %+v", candidate)
			}
		})
	}
}
