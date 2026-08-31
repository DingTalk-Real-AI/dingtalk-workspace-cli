// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// runRecordCreateCLI executes `dws aitable record create` with the given args
// against an installed fake MCP caller and returns captured output + error.
func runRecordCreateCLI(t *testing.T, caller *aitableTestCaller, args ...string) (string, error) {
	t.Helper()
	out := installAitableDeps(t, caller)
	base := []string{"record", "create", "--base-id", "base-sub", "--table-id", "table-sub"}
	full := append(base, args...)
	os.Args = append([]string{"dws", "aitable"}, full...)

	command := newAitableCommand()
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs(full)
	err := command.Execute()
	return out.String(), err
}

func TestCrossPlatformCoverageRecordCreateSubRecordDispatch(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{`{"data":{"recordIds":["r1"],"hierarchyFieldId":"fldH","parentRecordId":"recP"}}`}}
	_, err := runRecordCreateCLI(t, caller,
		"--parent-record-id", "recP",
		"--records", `[{"cells":{"fldText":"child"}}]`,
		"--view-id", "viewH",
		"--client-token", "11111111-2222-4333-8444-555555555555",
	)
	if err != nil {
		t.Fatalf("record create --parent-record-id returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.tool != "create_sub_records" {
		t.Fatalf("tool = %q, want create_sub_records", call.tool)
	}
	for key, want := range map[string]any{
		"baseId":         "base-sub",
		"tableId":        "table-sub",
		"parentRecordId": "recP",
		"viewId":         "viewH",
		"clientToken":    "11111111-2222-4333-8444-555555555555",
	} {
		if got := call.args[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
	records, ok := call.args["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("records = %#v, want one entry", call.args["records"])
	}
}

func TestCrossPlatformCoverageRecordCreatePlainModeUnchanged(t *testing.T) {
	caller := &aitableTestCaller{}
	_, err := runRecordCreateCLI(t, caller, "--records", `[{"cells":{"fldText":"row"}}]`)
	if err != nil {
		t.Fatalf("record create returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.tool != "create_records" {
		t.Fatalf("tool = %q, want create_records", call.tool)
	}
	for _, key := range []string{"parentRecordId", "viewId", "clientToken"} {
		if got, present := call.args[key]; present {
			t.Fatalf("plain mode must not send %s, got %#v", key, got)
		}
	}
}

func TestCrossPlatformCoverageRecordCreateSubRecordCellsShortcut(t *testing.T) {
	caller := &aitableTestCaller{}
	_, err := runRecordCreateCLI(t, caller,
		"--parent-record-id", "recP",
		"--cells", `{"fldText":"child"}`,
	)
	if err != nil {
		t.Fatalf("record create --cells returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "create_sub_records" {
		t.Fatalf("calls = %#v, want single create_sub_records", caller.calls)
	}
	if caller.calls[0].args["parentRecordId"] != "recP" {
		t.Fatalf("parentRecordId = %#v", caller.calls[0].args["parentRecordId"])
	}
}

func TestCrossPlatformCoverageRecordCreateSubRecordModeGuards(t *testing.T) {
	// --view-id / --client-token without --parent-record-id must fail before
	// any MCP call instead of silently creating flat records.
	for _, flag := range []string{"view-id", "client-token"} {
		caller := &aitableTestCaller{}
		_, err := runRecordCreateCLI(t, caller, "--records", `[{"cells":{}}]`, "--"+flag, "v1")
		if err == nil || !strings.Contains(err.Error(), "--"+flag+" requires --parent-record-id") {
			t.Fatalf("--%s guard error = %v, want parent-record-id requirement", flag, err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("--%s guard still issued %d MCP calls", flag, len(caller.calls))
		}
	}

	// Sub-record mode keeps the server's 100-record cap client-side.
	many := bytes.Buffer{}
	many.WriteString("[")
	for i := 0; i < 101; i++ {
		if i > 0 {
			many.WriteString(",")
		}
		many.WriteString(`{"cells":{}}`)
	}
	many.WriteString("]")
	caller := &aitableTestCaller{}
	_, err := runRecordCreateCLI(t, caller, "--parent-record-id", "recP", "--records", many.String())
	if err == nil || !strings.Contains(err.Error(), "max 100") {
		t.Fatalf("101-record sub create error = %v, want max 100", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("oversized sub create issued %d MCP calls", len(caller.calls))
	}
}
