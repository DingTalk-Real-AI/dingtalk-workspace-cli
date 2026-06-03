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

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
)

type fakeLoader struct{ cat ir.Catalog }

func (f fakeLoader) Load(context.Context) (ir.Catalog, error) { return f.cat, nil }

func auditTestCatalog() ir.Catalog {
	return ir.Catalog{Products: []ir.CanonicalProduct{{
		ID: "minutes",
		Tools: []ir.ToolDescriptor{{
			RPCName:     "export",
			Title:       "导出听记",
			Description: "导出听记纪要为本地文件",
			Sensitive:   true,
		}},
	}}}
}

// End-to-end wiring test: a finished invocation must produce a fully-populated
// audit event in BOTH the local file and the organization's forward sink, with
// every obtainable field set (only token-derived actor/org are absent in a
// test env with no login).
func TestEmitAudit_PopulatesAllObtainableFields(t *testing.T) {
	var forwarded []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		forwarded = b.Bytes()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv(audit.EnvEnabled, "true")
	t.Setenv(audit.EnvForwardURL, srv.URL)
	t.Setenv(audit.EnvForwardRedact, "none") // org's own sink: ship verbatim
	t.Setenv(audit.EnvNLIntent, "把上周的战略会听记导出到桌面")
	t.Setenv(envDWSChannel, "openclaw") // which agent/channel is driving dws

	var file bytes.Buffer
	r := &runtimeRunner{
		loader:    fakeLoader{cat: auditTestCatalog()},
		auditSink: audit.BuildSink(&file),
	}

	inv := executor.Invocation{
		CanonicalProduct: "minutes",
		Tool:             "export",
		CanonicalPath:    "minutes export",
		Params: map[string]any{
			"minuteId": "m-77",
			"name":     "Q2 战略会",
			"output":   "/Users/x/Desktop/q2.md",
		},
	}

	r.emitAudit(context.Background(), "trace-xyz", "https://gw.internal/mcp", inv, true, "", time.Now())

	// --- local file: full record ---
	var local audit.Event
	if err := json.Unmarshal(bytes.TrimSpace(file.Bytes()), &local); err != nil {
		t.Fatalf("local audit not valid JSON: %v\n%s", err, file.String())
	}
	checks := map[string]struct{ got, want string }{
		"trace_id":        {local.TraceID, "trace-xyz"},
		"module":          {local.Module, "minutes"},
		"command":         {local.Command, "minutes"},
		"subcommand":      {local.Subcommand, "export"},
		"subcommand_desc": {local.SubcommandDesc, "导出听记纪要为本地文件"},
		"target.id":       {local.Target.ID, "m-77"},
		"target.name":     {local.Target.Name, "Q2 战略会"},
		"intent.nl":       {local.Intent.NLInput, "把上周的战略会听记导出到桌面"},
		"outcome":         {local.Outcome, "ok"},
		"flow.localpath":  {local.Flow.LocalPath, "/Users/x/Desktop/q2.md"},
		"flow.api":        {local.Flow.API, "export"},
	}
	for field, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", field, c.got, c.want)
		}
	}
	if local.Target.Sensitivity != audit.SensitivityConfidential {
		t.Errorf("sensitivity = %q, want confidential (tool marked Sensitive)", local.Target.Sensitivity)
	}
	if local.Flow.Direction != audit.DirectionLocalExport {
		t.Errorf("direction = %q, want local-export (output path present)", local.Flow.Direction)
	}
	if local.Intent.Provenance != audit.ProvenanceAgent {
		t.Errorf("NL provenance must be agent, got %q", local.Intent.Provenance)
	}
	if local.Device.OS == "" {
		t.Error("device.os should always be set")
	}
	// client.cli_version is the compiled-in version (trustworthy, dws-managed).
	if local.Client.CLIVersion != version {
		t.Errorf("client.cli_version = %q, want %q", local.Client.CLIVersion, version)
	}
	// client.channel comes from DWS_CHANNEL (which agent/channel is calling).
	if local.Client.Channel != "openclaw" {
		t.Errorf("client.channel = %q, want %q", local.Client.Channel, "openclaw")
	}

	// --- forward sink received the same trace ---
	if len(forwarded) == 0 {
		t.Fatal("forwarder received nothing")
	}
	var fwd audit.Event
	if err := json.Unmarshal(forwarded, &fwd); err != nil {
		t.Fatalf("forwarded audit not valid JSON: %v", err)
	}
	if fwd.TraceID != "trace-xyz" || fwd.Subcommand != "export" {
		t.Errorf("forwarded event lost fields: %+v", fwd)
	}
}

// A read-only command must classify as DirectionRead with no peer-id leakage.
func TestEmitAudit_ReadVerbClassification(t *testing.T) {
	t.Setenv(audit.EnvEnabled, "true")
	var file bytes.Buffer
	r := &runtimeRunner{
		loader:    fakeLoader{cat: ir.Catalog{}},
		auditSink: audit.BuildSink(&file),
	}
	inv := executor.Invocation{
		CanonicalProduct: "doc",
		Tool:             "list",
		CanonicalPath:    "doc list",
		Params:           map[string]any{"spaceId": "s-1"},
	}
	r.emitAudit(context.Background(), "t2", "https://gw/mcp", inv, true, "", time.Now())

	var ev audit.Event
	_ = json.Unmarshal(bytes.TrimSpace(file.Bytes()), &ev)
	if ev.Flow.Direction != audit.DirectionRead {
		t.Errorf("list should be read, got %q", ev.Flow.Direction)
	}
}
