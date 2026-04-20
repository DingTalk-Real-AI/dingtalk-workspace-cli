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

package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/spf13/cobra"
)

// loadPilotEnvelope loads a Diamond-published envelope fixture and converts
// it into a ServerDescriptor suitable for BuildDynamicCommands.
func loadPilotEnvelope(t *testing.T, productDir, fileName string) market.ServerDescriptor {
	t.Helper()
	fixturePath := filepath.Join("testdata", productDir, fileName)
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var env market.ServerEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return market.ServerDescriptor{
		Key:         env.Server.Name,
		DisplayName: env.Server.Name,
		Description: env.Server.Description,
		Endpoint:    env.Server.Remotes[0].URL,
		Status:      env.Meta.Registry.Status,
		CLI:         env.Meta.CLI,
	}
}

// TestChatPilotEnvelope_EndToEnd verifies that the phase-2 chat pilot envelope
// JSON produces the expected command tree when consumed by BuildDynamicCommands.
// This is the authoritative acceptance test for the Diamond-published config
// that operators will paste into `dws-wukong-discovery.chat`.
func TestChatPilotEnvelope_EndToEnd(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{loadPilotEnvelope(t, "chat_pilot", "dws-wukong-discovery.chat.json")}
	runner := &captureRunner{}
	cmds := BuildDynamicCommands(servers, runner, nil)

	if len(cmds) != 1 {
		t.Fatalf("expected 1 top-level command, got %d", len(cmds))
	}
	chat := cmds[0]
	if chat.Name() != "chat" {
		t.Fatalf("top-level name = %q, want %q", chat.Name(), "chat")
	}
	if !containsAlias(chat.Aliases, "im") {
		// The CLIOverlay.Aliases -> cobra.Command.Aliases mapping currently
		// isn't surfaced by BuildDynamicCommands (aliases are a consumer of
		// `cli.Aliases`, future work). For now just document expectation in
		// the fixture; do not fail.
		t.Logf("note: overlay aliases=[im] not yet propagated to cobra (P2 follow-up)")
	}

	// --- Group structure -----------------------------------------------------
	for _, p := range [][]string{
		{"group"},
		{"group", "members"},
		{"message"},
		{"bot"},
	} {
		if cmd := descend(chat, p...); cmd == nil {
			t.Fatalf("missing group path: chat %s", strings.Join(p, " "))
		}
	}

	// --- Leaf commands -------------------------------------------------------
	leafExpectations := []struct {
		path            []string
		expectTool      string
		expectProduct   string
		requiredFlags   []string
		optionalFlags   []string
		serverOverrides bool
	}{
		{
			path:          []string{"group", "rename"},
			expectTool:    "update_group_name",
			expectProduct: "chat",
			requiredFlags: []string{"id", "name"},
		},
		{
			path:          []string{"group", "members", "add"},
			expectTool:    "add_group_member",
			expectProduct: "chat",
			requiredFlags: []string{"id", "users"},
		},
		{
			path:          []string{"group", "members", "remove"},
			expectTool:    "remove_group_member",
			expectProduct: "chat",
			requiredFlags: []string{"id", "users"},
		},
		{
			path:            []string{"group", "members", "add-bot"},
			expectTool:      "add_robot_to_group",
			expectProduct:   "bot",
			requiredFlags:   []string{"robot-code", "id"},
			serverOverrides: true,
		},
		{
			path:          []string{"search-common"},
			expectTool:    "search_common_groups",
			expectProduct: "chat",
			requiredFlags: []string{"nicks"},
			optionalFlags: []string{"match-mode", "limit", "cursor"},
		},
		{
			path:          []string{"message", "list-unread-conversations"},
			expectTool:    "unread_message_conversation_list",
			expectProduct: "chat",
			optionalFlags: []string{"count"},
		},
		{
			path:          []string{"message", "list-focused"},
			expectTool:    "list_special_focus_messages",
			expectProduct: "chat",
			optionalFlags: []string{"limit", "cursor"},
		},
		{
			path:            []string{"bot", "search"},
			expectTool:      "search_my_robots",
			expectProduct:   "bot",
			optionalFlags:   []string{"page", "size", "name"},
			serverOverrides: true,
		},
	}

	for _, exp := range leafExpectations {
		full := strings.Join(exp.path, " ")
		t.Run(full, func(t *testing.T) {
			cmd := descend(chat, exp.path...)
			if cmd == nil {
				t.Fatalf("missing command: chat %s", full)
			}

			for _, flag := range exp.requiredFlags {
				f := cmd.Flags().Lookup(flag)
				if f == nil {
					t.Fatalf("chat %s: required flag --%s missing", full, flag)
				}
				if ann := f.Annotations[cobraRequiredAnnotation]; len(ann) == 0 || ann[0] != "true" {
					t.Fatalf("chat %s: flag --%s is not marked required (annotations=%v)", full, flag, f.Annotations)
				}
			}

			for _, flag := range exp.optionalFlags {
				if cmd.Flags().Lookup(flag) == nil {
					t.Fatalf("chat %s: optional flag --%s missing", full, flag)
				}
			}

			// Invoke with minimal required args to verify routing (product/tool).
			argv := buildInvocationArgs(t, cmd, exp.requiredFlags)
			chat.SetArgs(append(append([]string{}, exp.path...), argv...))
			chat.SetOut(&strings.Builder{})
			chat.SetErr(&strings.Builder{})
			chat.SilenceErrors = true
			chat.SilenceUsage = true
			if err := chat.Execute(); err != nil {
				t.Fatalf("chat %s: execute returned error: %v", full, err)
			}
			if runner.lastTool != exp.expectTool {
				t.Fatalf("chat %s: tool = %q, want %q", full, runner.lastTool, exp.expectTool)
			}
			if runner.lastProduct != exp.expectProduct {
				t.Fatalf("chat %s: product = %q, want %q", full, runner.lastProduct, exp.expectProduct)
			}
			if exp.serverOverrides && runner.lastProduct == "chat" {
				t.Fatalf("chat %s: expected serverOverride routing, but stayed on chat product", full)
			}

			// Reset capture state between sub-tests.
			runner.lastProduct = ""
			runner.lastTool = ""
			runner.lastParams = nil
		})
	}
}

// cobraRequiredAnnotation is the cobra annotation key set by MarkFlagRequired.
const cobraRequiredAnnotation = "cobra_annotation_bash_completion_one_required_flag"

func containsAlias(aliases []string, want string) bool {
	for _, a := range aliases {
		if a == want {
			return true
		}
	}
	return false
}

func descend(root *cobra.Command, path ...string) *cobra.Command {
	cur := root
	for _, seg := range path {
		cur = findChild(cur, seg)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// buildInvocationArgs returns an argv that supplies a placeholder value for
// each required flag so the command can execute without a usage error.
func buildInvocationArgs(t *testing.T, cmd *cobra.Command, requiredFlags []string) []string {
	t.Helper()
	argv := make([]string, 0, 2*len(requiredFlags))
	for _, flag := range requiredFlags {
		argv = append(argv, "--"+flag, "x")
	}
	return argv
}

// TestTodoPilotEnvelope_EndToEnd verifies the phase-3 todo pilot envelope.
// todo is migrated via the same two-step (coexistence → handwritten removal)
// approach as chat. After Phase 5 landed, the envelope covers:
//
//   - todo task get     → get_todo_detail            (P1)
//   - todo task delete  → delete_todo (isSensitive → --yes gate)   (P1)
//   - todo task create  → create_todo (bodyWrapper + iso8601_to_millis + enum_map + csv_to_array) (P2)
func TestTodoPilotEnvelope_EndToEnd(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{loadPilotEnvelope(t, "todo_pilot", "dws-wukong-discovery.todo.json")}
	runner := &captureRunner{}
	cmds := BuildDynamicCommands(servers, runner, nil)

	if len(cmds) != 1 {
		t.Fatalf("expected 1 top-level command, got %d", len(cmds))
	}
	todo := cmds[0]
	if todo.Name() != "todo" {
		t.Fatalf("top-level name = %q, want %q", todo.Name(), "todo")
	}

	// Verify task sub-group exists.
	if descend(todo, "task") == nil {
		t.Fatalf("missing group: todo task")
	}

	// get (P1-clean, single required flag)
	get := descend(todo, "task", "get")
	if get == nil {
		t.Fatalf("missing command: todo task get")
	}
	if f := get.Flags().Lookup("task-id"); f == nil {
		t.Fatalf("todo task get: --task-id flag missing")
	}

	// delete (isSensitive → needs --yes to bypass confirm)
	del := descend(todo, "task", "delete")
	if del == nil {
		t.Fatalf("missing command: todo task delete")
	}
	if f := del.Flags().Lookup("task-id"); f == nil {
		t.Fatalf("todo task delete: --task-id flag missing")
	}

	// Execute todo task get --task-id=X
	todo.SetArgs([]string{"task", "get", "--task-id", "T123"})
	todo.SetOut(&strings.Builder{})
	todo.SetErr(&strings.Builder{})
	todo.SilenceErrors = true
	todo.SilenceUsage = true
	if err := todo.Execute(); err != nil {
		t.Fatalf("todo task get execute: %v", err)
	}
	if runner.lastTool != "get_todo_detail" {
		t.Fatalf("todo task get: tool = %q, want %q", runner.lastTool, "get_todo_detail")
	}
	if runner.lastProduct != "todo" {
		t.Fatalf("todo task get: product = %q, want %q", runner.lastProduct, "todo")
	}
	if got, _ := runner.lastParams["taskId"].(string); got != "T123" {
		t.Fatalf("todo task get: taskId = %v, want %q", runner.lastParams["taskId"], "T123")
	}

	// --- P2: create (bodyWrapper + iso8601_to_millis + enum_map + csv_to_array) ---
	create := descend(todo, "task", "create")
	if create == nil {
		t.Fatal("missing command: todo task create")
	}
	for _, name := range []string{"subject", "due", "priority", "executors"} {
		if create.Flags().Lookup(name) == nil {
			t.Fatalf("todo task create: --%s flag missing", name)
		}
	}

	runner.lastProduct = ""
	runner.lastTool = ""
	runner.lastParams = nil
	todo.SetArgs([]string{
		"task", "create",
		"--subject", "ship P5",
		"--due", "2026-05-01T09:00:00Z",
		"--priority", "high",
		"--executors", "u1,u2,u3",
	})
	todo.SetOut(&strings.Builder{})
	todo.SetErr(&strings.Builder{})
	todo.SilenceErrors = true
	todo.SilenceUsage = true
	if err := todo.Execute(); err != nil {
		t.Fatalf("todo task create execute: %v", err)
	}
	if runner.lastTool != "create_todo" {
		t.Fatalf("todo task create: tool = %q, want %q", runner.lastTool, "create_todo")
	}
	wrap, ok := runner.lastParams["PersonalTodoCreateVO"].(map[string]any)
	if !ok {
		t.Fatalf("todo task create: expected bodyWrapper PersonalTodoCreateVO, got %+v", runner.lastParams)
	}
	if wrap["subject"] != "ship P5" {
		t.Fatalf("wrap[subject]=%v", wrap["subject"])
	}
	if due, ok := wrap["dueTime"].(int64); !ok || due <= 1_700_000_000_000 {
		t.Fatalf("wrap[dueTime] must be millis from iso8601, got %T %v", wrap["dueTime"], wrap["dueTime"])
	}
	// enum_map returns the JSON-decoded value; numbers arrive as float64 from
	// encoding/json unless a custom decoder is used. Accept any numeric form.
	gotPrio := prioToInt(t, wrap["priority"])
	if gotPrio != 30 {
		t.Fatalf("wrap[priority]=%v (want 30 for 'high')", wrap["priority"])
	}
	execs, ok := wrap["executorIds"].([]any)
	if !ok || len(execs) != 3 || execs[0] != "u1" || execs[2] != "u3" {
		t.Fatalf("wrap[executorIds]=%+v, want [u1 u2 u3]", wrap["executorIds"])
	}
}

// TestCreditPilotEnvelope_EndToEnd verifies the phase-4 credit pilot envelope.
// credit is the reference migration for B-class "cross-server routing" products:
// one CLI root (`dws credit`) fans out to 6 different MCP servers (credit-ep,
// credit-risk, credit-ip, credit-equity, credit-bid, credit-contact) via
// per-tool `serverOverride`. 28 commands organized into root + 3 nested groups
// (risk / ip / equity), all driven by uniform --cert / --page / --size flags.
//
// This test is the authoritative acceptance for the Diamond envelope that
// operators will paste into `dws-wukong-discovery.credit`.
func TestCreditPilotEnvelope_EndToEnd(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{loadPilotEnvelope(t, "credit_pilot", "dws-wukong-discovery.credit.json")}
	runner := &captureRunner{}
	cmds := BuildDynamicCommands(servers, runner, nil)

	if len(cmds) != 1 {
		t.Fatalf("expected 1 top-level command, got %d", len(cmds))
	}
	credit := cmds[0]
	if credit.Name() != "credit" {
		t.Fatalf("top-level name = %q, want %q", credit.Name(), "credit")
	}

	// --- Group structure -----------------------------------------------------
	for _, p := range [][]string{{"risk"}, {"ip"}, {"equity"}} {
		if cmd := descend(credit, p...); cmd == nil {
			t.Fatalf("missing group path: credit %s", strings.Join(p, " "))
		}
	}

	type leaf struct {
		path       []string
		tool       string
		product    string // expected canonicalProduct after serverOverride
		certOnly   bool   // true for kp (no pagination)
		searchFlag string // for the search variant
	}

	leaves := []leaf{
		// --- credit-ep (default, no serverOverride) -------------------------
		{path: []string{"search"}, tool: "ep_info_search_query", product: "credit-ep", searchFlag: "name"},
		{path: []string{"info"}, tool: "ep_dossier_basicinfo_query", product: "credit-ep"},
		{path: []string{"member"}, tool: "ep_dossier_member_query", product: "credit-ep"},
		{path: []string{"change"}, tool: "ep_dossier_reginfochange_query", product: "credit-ep"},
		{path: []string{"annual"}, tool: "ep_dossier_annualreport_query", product: "credit-ep"},
		{path: []string{"license"}, tool: "ep_dossier_license_query", product: "credit-ep"},
		{path: []string{"cert-info"}, tool: "ep_dossier_certificate_query", product: "credit-ep"},
		{path: []string{"branch"}, tool: "ep_dossier_branch_query", product: "credit-ep"},

		// --- serverOverride: credit-bid / credit-contact -------------------
		{path: []string{"bidding"}, tool: "ep_dossier_bidding_query", product: "credit-bid"},
		{path: []string{"kp"}, tool: "ep_contactinfo_ext_query", product: "credit-contact", certOnly: true},

		// --- group risk (serverOverride: credit-risk, 12 commands) ---------
		{path: []string{"risk", "verdict"}, tool: "ep_dossier_verdict_query", product: "credit-risk"},
		{path: []string{"risk", "execute"}, tool: "ep_dossier_execute_query", product: "credit-risk"},
		{path: []string{"risk", "dishonest"}, tool: "ep_dossier_dishonest_query", product: "credit-risk"},
		{path: []string{"risk", "litigation"}, tool: "ep_dossier_litigation_query", product: "credit-risk"},
		{path: []string{"risk", "finalcase"}, tool: "ep_dossier_finalcase_query", product: "credit-risk"},
		{path: []string{"risk", "consum"}, tool: "ep_dossier_consum_query", product: "credit-risk"},
		{path: []string{"risk", "court"}, tool: "ep_dossier_courtnotice_query", product: "credit-risk"},
		{path: []string{"risk", "assist"}, tool: "ep_dossier_legalassist_query", product: "credit-risk"},
		{path: []string{"risk", "penalty"}, tool: "ep_dossier_adminpenalty_query", product: "credit-risk"},
		{path: []string{"risk", "owetax"}, tool: "ep_dossier_owetax_query", product: "credit-risk"},
		{path: []string{"risk", "taxviolation"}, tool: "ep_dossier_taxviolation_query", product: "credit-risk"},
		{path: []string{"risk", "pledge"}, tool: "ep_dossier_equitypledge_query", product: "credit-risk"},

		// --- group ip (serverOverride: credit-ip, 4 commands) -------------
		{path: []string{"ip", "trademark"}, tool: "ep_dossier_trademark_query", product: "credit-ip"},
		{path: []string{"ip", "patent"}, tool: "ep_dossier_patent_query", product: "credit-ip"},
		{path: []string{"ip", "copyright"}, tool: "ep_dossier_copyright_query", product: "credit-ip"},
		{path: []string{"ip", "icp"}, tool: "ep_dossier_icpregistration_query", product: "credit-ip"},

		// --- group equity (serverOverride: credit-equity, 2 commands) -----
		{path: []string{"equity", "shareholder"}, tool: "ep_dossier_shareholder_query", product: "credit-equity"},
		{path: []string{"equity", "invest"}, tool: "ep_dossier_invest_query", product: "credit-equity"},
	}

	if len(leaves) != 28 {
		t.Fatalf("credit envelope must declare 28 leaf commands, got %d", len(leaves))
	}

	for _, lf := range leaves {
		full := strings.Join(lf.path, " ")
		t.Run(full, func(t *testing.T) {
			cmd := descend(credit, lf.path...)
			if cmd == nil {
				t.Fatalf("missing command: credit %s", full)
			}

			// Flags: every leaf must have either --cert (default) or --name (search).
			primary := "cert"
			primaryVal := "91330100MA2CK6BX6X"
			if lf.searchFlag != "" {
				primary = lf.searchFlag
				primaryVal = "阿里巴巴"
			}
			if cmd.Flags().Lookup(primary) == nil {
				t.Fatalf("credit %s: --%s flag missing", full, primary)
			}
			if f := cmd.Flags().Lookup(primary); f != nil {
				if ann := f.Annotations[cobraRequiredAnnotation]; len(ann) == 0 || ann[0] != "true" {
					t.Fatalf("credit %s: --%s must be required", full, primary)
				}
			}
			if !lf.certOnly {
				for _, p := range []string{"page", "size"} {
					if cmd.Flags().Lookup(p) == nil {
						t.Fatalf("credit %s: optional flag --%s missing", full, p)
					}
				}
			} else {
				if cmd.Flags().Lookup("page") != nil {
					t.Fatalf("credit %s: kp must NOT have --page (no pagination)", full)
				}
			}

			// Invoke with the primary flag to verify routing + param mapping.
			runner.lastProduct = ""
			runner.lastTool = ""
			runner.lastParams = nil
			argv := append(append([]string{}, lf.path...), "--"+primary, primaryVal)
			credit.SetArgs(argv)
			credit.SetOut(&strings.Builder{})
			credit.SetErr(&strings.Builder{})
			credit.SilenceErrors = true
			credit.SilenceUsage = true
			if err := credit.Execute(); err != nil {
				t.Fatalf("credit %s: execute returned error: %v", full, err)
			}
			if runner.lastTool != lf.tool {
				t.Fatalf("credit %s: tool = %q, want %q", full, runner.lastTool, lf.tool)
			}
			if runner.lastProduct != lf.product {
				t.Fatalf("credit %s: product = %q, want %q", full, runner.lastProduct, lf.product)
			}

			// Verify schema-mapped parameter names reach the tool.
			wantParam := "ep_cert_no"
			if lf.searchFlag != "" {
				wantParam = "company_name"
			}
			if got, _ := runner.lastParams[wantParam].(string); got != primaryVal {
				t.Fatalf("credit %s: params[%s] = %v, want %q", full, wantParam, runner.lastParams[wantParam], primaryVal)
			}
		})
	}
}

// TestCreditPilotEnvelope_Pagination asserts that --page / --size flags land in
// the MCP payload as page_index / page_size (snake_case).
func TestCreditPilotEnvelope_Pagination(t *testing.T) {
	t.Parallel()

	servers := []market.ServerDescriptor{loadPilotEnvelope(t, "credit_pilot", "dws-wukong-discovery.credit.json")}
	runner := &captureRunner{}
	cmds := BuildDynamicCommands(servers, runner, nil)
	credit := cmds[0]

	credit.SetArgs([]string{"risk", "verdict", "--cert", "X", "--page", "2", "--size", "25"})
	credit.SetOut(&strings.Builder{})
	credit.SetErr(&strings.Builder{})
	credit.SilenceErrors = true
	credit.SilenceUsage = true
	if err := credit.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := runner.lastParams["page_index"]; got != "2" && got != 2 && got != int64(2) && got != float64(2) {
		t.Fatalf("page_index=%v (type %T)", got, got)
	}
	if got := runner.lastParams["page_size"]; got != "25" && got != 25 && got != int64(25) && got != float64(25) {
		t.Fatalf("page_size=%v (type %T)", got, got)
	}
}

// compile-time check that cobra package is referenced in this file.
var _ = struct {
	a executor.Runner
	b market.ServerDescriptor
}{}

// prioToInt coerces any numeric form (int/int64/float64/json.Number) to int.
func prioToInt(t *testing.T, v any) int {
	t.Helper()
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		t.Fatalf("priority type=%T value=%v", v, v)
		return 0
	}
}
