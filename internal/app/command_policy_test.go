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
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cmdpolicy"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// restrictForTest installs an edition whose Restrict hook returns rules, and
// restores the previous edition afterwards. The open-source defaults are kept
// so the command tree is the production one.
func restrictForTest(t *testing.T, restrict func() []edition.CommandRule) {
	t.Helper()
	previous := edition.Get()
	t.Cleanup(func() { edition.Override(previous) })
	hooks := *previous
	hooks.Name = "policy-test"
	hooks.Restrict = restrict
	edition.Override(&hooks)
}

func staticRules(rules ...edition.CommandRule) func() []edition.CommandRule {
	return func() []edition.CommandRule { return rules }
}

var authBasicRule = edition.CommandRule{
	Name: "auth-basic", Allow: []string{"auth/login", "auth/logout", "auth/status"}, AllowUnannotated: true,
}

func findPolicyCommand(t *testing.T, root *cobra.Command, path string) *cobra.Command {
	t.Helper()
	cmd, rest, err := root.Find(strings.Split(path, "/"))
	if err != nil || len(rest) != 0 || cmdpolicy.CommandPath(cmd) != path {
		t.Fatalf("command %q not found in the tree (got %q, rest %v, err %v)", path, cmdpolicy.CommandPath(cmd), rest, err)
	}
	return cmd
}

// executePolicyRoot runs args on a freshly built production root and returns
// combined output and the execution error.
func executePolicyRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func requireDenied(t *testing.T, err error, reasonCode string) *apperrors.Error {
	t.Helper()
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %v (%T), want a classified denial", err, err)
	}
	if typed.Category != apperrors.CategoryValidation || typed.Reason != "command_denied" {
		t.Fatalf("error category/reason = %s/%s, want validation/command_denied: %v", typed.Category, typed.Reason, err)
	}
	if got := apperrors.ExitCode(err); got != 3 {
		t.Fatalf("exit code = %d, want 3", got)
	}
	if got, _ := typed.Details["reason_code"].(string); reasonCode != "" && got != reasonCode {
		t.Fatalf("reason_code = %q, want %q: %v", got, reasonCode, err)
	}
	return typed
}

func TestCrossPlatformCoverageCommandPolicyDisabledLeavesTreeUntouched(t *testing.T) {
	root := NewRootCommand()
	if snapshot := cmdpolicy.For(root); snapshot != nil {
		t.Fatalf("policy snapshot = %+v, want none without a Restrict hook", snapshot)
	}
	if cmd := findPolicyCommand(t, root, "calendar/event/delete"); cmd.DisableFlagParsing {
		t.Fatal("a command was stubbed while the policy is disabled")
	}
	out, err := executePolicyRoot(t, "config", "policy", "show", "--format", "json")
	if err != nil {
		t.Fatalf("config policy show: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"state": "disabled"`) && !strings.Contains(out, `"state":"disabled"`) {
		t.Fatalf("config policy show output lacks the disabled state:\n%s", out)
	}
}

func TestCrossPlatformCoverageCommandPolicyReadOnlyCalendar(t *testing.T) {
	restrictForTest(t, staticRules(
		edition.CommandRule{Name: "calendar-readonly", Allow: []string{"calendar/**"}, MaxEffect: edition.EffectRead},
		authBasicRule,
	))
	root := NewRootCommand()
	snapshot := cmdpolicy.For(root)
	if snapshot.State() != cmdpolicy.StateActive {
		t.Fatalf("policy state = %s, want active", snapshot.State())
	}

	allowed := []string{"calendar/event/list", "auth/login", "auth/logout", "auth/status", "schema", "version", "config/policy/show"}
	for _, path := range allowed {
		if decision, ok := snapshot.Decision(path); !ok || !decision.Allowed {
			t.Errorf("%s: decision = %+v (evaluated %v), want allowed", path, decision, ok)
		}
	}
	denied := map[string]string{
		"calendar/event/delete": cmdpolicy.ReasonNoMatchingRule,
		"contact/user/get-self": cmdpolicy.ReasonNoMatchingRule,
		"auth/export":           cmdpolicy.ReasonNoMatchingRule,
		"api":                   cmdpolicy.ReasonChannelNotAllowed,
		"mcp/published/invoke":  cmdpolicy.ReasonChannelNotAllowed,
		"mcp/url/get":           cmdpolicy.ReasonChannelNotAllowed,
		"upgrade":               cmdpolicy.ReasonChannelNotAllowed,
		"plugin/install":        cmdpolicy.ReasonChannelNotAllowed,
	}
	for path, reason := range denied {
		decision, ok := snapshot.Decision(path)
		if !ok || decision.Allowed || decision.ReasonCode != reason {
			t.Errorf("%s: decision = %+v (evaluated %v), want denied with %s", path, decision, ok, reason)
		}
	}

	// Presentation: a fully denied product disappears from help, a partially
	// allowed one stays, and hiding never stands in for the denial itself.
	if !findPolicyCommand(t, root, "contact").Hidden {
		t.Error("contact has no allowed command and must be hidden")
	}
	if findPolicyCommand(t, root, "calendar").Hidden {
		t.Error("calendar still has allowed commands and must stay visible")
	}
	if !findPolicyCommand(t, root, "calendar/event/delete").Hidden {
		t.Error("a denied leaf must be hidden")
	}
}

func TestCrossPlatformCoverageCommandPolicyDeniedExecution(t *testing.T) {
	afterPreRun := 0
	restrictForTest(t, staticRules(
		edition.CommandRule{Name: "calendar-readonly", Allow: []string{"calendar/**"}, MaxEffect: edition.EffectRead},
	))
	hooks := *edition.Get()
	hooks.AfterPersistentPreRun = func(*cobra.Command, []string) error { afterPreRun++; return nil }
	edition.Override(&hooks)

	cases := map[string][]string{
		"leaf":              {"calendar", "event", "delete", "--id", "x"},
		"unknown flag":      {"calendar", "event", "delete", "--no-such-flag"},
		"help flag":         {"calendar", "event", "delete", "--help"},
		"other product":     {"contact", "user", "get-self"},
		"generic api":       {"api", "GET", "/v1.0/contact/users/me"},
		"published invoke":  {"mcp", "published", "invoke", "1", "tool"},
		"compatibility cmd": {"report", "create"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := executePolicyRoot(t, args...)
			requireDenied(t, err, "")
			if strings.Contains(out, "Usage:") {
				t.Fatalf("a denied command printed usage/help:\n%s", out)
			}
		})
	}
	if afterPreRun != 0 {
		t.Fatalf("AfterPersistentPreRun ran %d times for denied commands, want 0", afterPreRun)
	}
}

func TestCrossPlatformCoverageCommandPolicyDenyMessage(t *testing.T) {
	const guidance = "请联系 IT 开通日程查询"
	restrictForTest(t, staticRules(
		edition.CommandRule{Name: "auth-basic", Allow: []string{"auth/login", "auth/logout", "auth/status"}, AllowUnannotated: true},
		edition.CommandRule{Name: "deny-calendar-agenda", Deny: []string{"calendar/+agenda"}, AllowUnannotated: true, DenyMessage: guidance},
	))
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"calendar", "+agenda", "--format", "json"})
	err := root.Execute()
	typed := requireDenied(t, err, cmdpolicy.ReasonNoMatchingRule)
	if typed.Details["deny_message"] != guidance || !strings.HasSuffix(typed.Hint, guidance) {
		t.Fatalf("denial hint/details = %q / %+v, want the rule's deny message", typed.Hint, typed.Details)
	}
	if err := printExecutionError(root, &stdout, &stderr, err); err != nil {
		t.Fatalf("printExecutionError: %v", err)
	}
	if rendered := stdout.String() + stderr.String(); !strings.Contains(rendered, guidance) {
		t.Fatalf("rendered error does not carry the deny message:\n%s", rendered)
	}
}

func TestCrossPlatformCoverageCommandPolicyDeniedJSONError(t *testing.T) {
	restrictForTest(t, staticRules(edition.CommandRule{Name: "calendar-readonly", Allow: []string{"calendar/**"}, MaxEffect: edition.EffectRead}))
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"calendar", "event", "delete", "--format", "json"})
	err := root.Execute()
	typed := requireDenied(t, err, cmdpolicy.ReasonRiskTooHigh)
	if typed.Details["rule_name"] != "calendar-readonly" || typed.Details["path"] != "calendar/event/delete" {
		t.Fatalf("denial details = %+v", typed.Details)
	}
	if err := printExecutionError(root, &stdout, &stderr, err); err != nil {
		t.Fatalf("printExecutionError: %v", err)
	}
	rendered := stdout.String() + stderr.String()
	start := strings.Index(rendered, "{")
	if start < 0 {
		t.Fatalf("--format json denial was not rendered as JSON:\n%s", rendered)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rendered[start:]), &payload); err != nil {
		t.Fatalf("denial is not a JSON document: %v\n%s", err, rendered)
	}
	if !strings.Contains(rendered, "command_denied") {
		t.Fatalf("JSON denial lacks the command_denied reason:\n%s", rendered)
	}
}

// A compatibility node mounted separately from its primary must share the
// primary's verdict: denying the primary cannot leave the alias open.
func TestCrossPlatformCoverageCommandPolicyAliasSharesPrimaryVerdict(t *testing.T) {
	restrictForTest(t, staticRules(edition.CommandRule{
		Name: "report", Allow: []string{"report/**"}, Deny: []string{"report/entry/submit"}, AllowUnannotated: true,
	}))
	snapshot := cmdpolicy.For(NewRootCommand())
	for _, path := range []string{"report/entry/submit", "report/create"} {
		decision, ok := snapshot.Decision(path)
		if !ok || decision.Allowed || decision.ReasonCode != cmdpolicy.ReasonDenylisted {
			t.Errorf("%s: decision = %+v (evaluated %v), want command_denylisted", path, decision, ok)
		}
	}
	if decision, ok := snapshot.Decision("report/entry/get"); !ok || !decision.Allowed {
		t.Errorf("report/entry/get: decision = %+v, want allowed", decision)
	}
	// Lookups by canonical dot path and by authored CLI path resolve too.
	for _, identifier := range []string{"report.create_report", "report entry submit", "report create"} {
		if decision, ok := snapshot.Lookup(identifier); !ok || decision.Allowed {
			t.Errorf("Lookup(%q) = %+v (found %v), want denied", identifier, decision, ok)
		}
	}
}

// A hybrid group's own business operation and its children are judged
// independently, in both directions.
func TestCrossPlatformCoverageCommandPolicyHybridGroup(t *testing.T) {
	const self, child = "dingtalk-tag/connect", "dingtalk-tag/connect/status"
	tests := []struct {
		name               string
		rules              []edition.CommandRule
		selfOK, childOK    bool
		selfHidden, hidden bool
	}{
		{
			name:   "self denied, child allowed",
			rules:  []edition.CommandRule{{Name: "status", Allow: []string{child}, AllowUnannotated: true}},
			selfOK: false, childOK: true,
		},
		{
			name:   "self allowed, child denied",
			rules:  []edition.CommandRule{{Name: "connect", Allow: []string{self}, AllowUnannotated: true}},
			selfOK: true, childOK: false,
		},
		{
			name:   "both allowed",
			rules:  []edition.CommandRule{{Name: "all", Allow: []string{"dingtalk-tag/connect", "dingtalk-tag/connect/**"}, AllowUnannotated: true}},
			selfOK: true, childOK: true,
		},
		{
			name:   "both denied",
			rules:  []edition.CommandRule{{Name: "other", Allow: []string{"calendar/**"}}},
			selfOK: false, childOK: false, selfHidden: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restrictForTest(t, staticRules(tt.rules...))
			root := NewRootCommand()
			snapshot := cmdpolicy.For(root)
			if decision, ok := snapshot.Decision(self); !ok || decision.Allowed != tt.selfOK {
				t.Fatalf("%s: decision = %+v (evaluated %v), want allowed=%v", self, decision, ok, tt.selfOK)
			}
			if decision, ok := snapshot.Decision(child); !ok || decision.Allowed != tt.childOK {
				t.Fatalf("%s: decision = %+v (evaluated %v), want allowed=%v", child, decision, ok, tt.childOK)
			}
			group := findPolicyCommand(t, root, self)
			if group.Hidden != tt.selfHidden {
				t.Fatalf("%s Hidden = %v, want %v", self, group.Hidden, tt.selfHidden)
			}
			if group.DisableFlagParsing {
				t.Fatal("a hybrid group must keep flag parsing so its children stay reachable")
			}
			if tt.selfOK {
				return
			}
			// The denied business operation is refused...
			_, err := executePolicyRoot(t, "dingtalk-tag", "connect")
			requireDenied(t, err, "")
			// ...while a mistyped subcommand still gets the group's recovery,
			// not a policy denial.
			_, err = executePolicyRoot(t, "dingtalk-tag", "connect", "statu")
			var typed *apperrors.Error
			if errors.As(err, &typed) && typed.Reason == "command_denied" {
				t.Fatalf("a mistyped subcommand was reported as a policy denial: %v", err)
			}
			if err == nil {
				t.Fatal("a mistyped subcommand must fail with a command-resolution error")
			}
		})
	}
}

func TestCrossPlatformCoverageCommandPolicyChannelEntries(t *testing.T) {
	restrictForTest(t, staticRules(edition.CommandRule{Name: "wide", Allow: []string{"**"}, AllowUnannotated: true}))
	snapshot := cmdpolicy.For(NewRootCommand())
	for _, channel := range cmdpolicy.ChannelEntries() {
		decision, ok := snapshot.Decision(channel.Path)
		if !ok {
			t.Errorf("channel entry %q is not an executable command in the tree; the reviewed list is stale", channel.Path)
			continue
		}
		if decision.Allowed || decision.ReasonCode != cmdpolicy.ReasonChannelNotAllowed {
			t.Errorf("%s: decision = %+v, want channel_not_allowed under a wildcard rule", channel.Path, decision)
		}
	}
	if len(snapshot.Channels()) != len(cmdpolicy.ChannelEntries()) {
		t.Errorf("Channels() reports %d entries, want %d", len(snapshot.Channels()), len(cmdpolicy.ChannelEntries()))
	}

	// Naming a channel is one more AND condition of its rule. A rule scoped to
	// calendar and capped at read does not open the unannotated api command
	// just because it names it.
	restrictForTest(t, staticRules(edition.CommandRule{
		Name: "calendar", Allow: []string{"calendar/**"}, MaxEffect: edition.EffectRead, AllowChannels: []string{"api"},
	}))
	snapshot = cmdpolicy.For(NewRootCommand())
	if decision, _ := snapshot.Decision("api"); decision.Allowed {
		t.Errorf("api: decision = %+v, want denied: the naming rule's Allow and declaration checks still apply", decision)
	}

	restrictForTest(t, staticRules(edition.CommandRule{Name: "api", Allow: []string{"api"}, AllowChannels: []string{"api"}, AllowUnannotated: true}))
	snapshot = cmdpolicy.For(NewRootCommand())
	if decision, _ := snapshot.Decision("api"); !decision.Allowed || decision.RuleName != "api" {
		t.Errorf("api: decision = %+v, want allowed by rule api", decision)
	}
	if decision, _ := snapshot.Decision("upgrade"); decision.Allowed {
		t.Errorf("upgrade: decision = %+v, want still denied", decision)
	}
}

// Cobra accepts native aliases at every level ("log" for "report"). They are
// entry paths of the same identity: a Deny written against an alias spelling
// denies the command however it is typed.
func TestCrossPlatformCoverageCommandPolicyNativeAliasSharesVerdict(t *testing.T) {
	restrictForTest(t, staticRules(edition.CommandRule{
		Name: "report", Allow: []string{"report/**"}, Deny: []string{"log/**"}, AllowUnannotated: true,
	}))
	root := NewRootCommand()
	snapshot := cmdpolicy.For(root)
	for _, path := range []string{"report/entry/submit", "report/entry/get", "report/create"} {
		if decision, ok := snapshot.Decision(path); !ok || decision.Allowed || decision.ReasonCode != cmdpolicy.ReasonDenylisted {
			t.Errorf("%s: decision = %+v (evaluated %v), want command_denylisted through the log alias", path, decision, ok)
		}
	}
	if decision, ok := snapshot.Lookup("log entry submit"); !ok || decision.Allowed {
		t.Errorf("Lookup(log entry submit) = %+v (found %v), want denied", decision, ok)
	}
	for _, args := range [][]string{{"log", "entry", "get"}, {"report", "entry", "get"}} {
		_, err := executePolicyRoot(t, args...)
		requireDenied(t, err, cmdpolicy.ReasonDenylisted)
	}

	// Allow is matched on the primary path only, so the alias spelling of an
	// allowed command runs without being listed.
	restrictForTest(t, staticRules(edition.CommandRule{Name: "report", Allow: []string{"report/**"}, AllowUnannotated: true}))
	if decision, ok := cmdpolicy.For(NewRootCommand()).Lookup("log entry get"); !ok || !decision.Allowed {
		t.Errorf("Lookup(log entry get) = %+v (found %v), want allowed", decision, ok)
	}
}

// The policy diagnostic must work without valid sign-in: an edition hook that
// enforces authentication cannot block it, in either enabled state.
func TestCrossPlatformCoverageCommandPolicyShowBypassesEditionAuthHook(t *testing.T) {
	for name, restrict := range map[string]func() []edition.CommandRule{
		"active":  staticRules(edition.CommandRule{Name: "calendar", Allow: []string{"calendar/**"}}),
		"invalid": staticRules(edition.CommandRule{Name: "bad", MaxRisk: "nope"}),
	} {
		t.Run(name, func(t *testing.T) {
			restrictForTest(t, restrict)
			hookRuns := 0
			hooks := *edition.Get()
			hooks.AfterPersistentPreRun = func(*cobra.Command, []string) error {
				hookRuns++
				return errors.New("not signed in")
			}
			edition.Override(&hooks)

			out, err := executePolicyRoot(t, "config", "policy", "show", "--format", "json")
			if err != nil {
				t.Fatalf("config policy show was blocked by the edition auth hook: %v\n%s", err, out)
			}
			if !strings.Contains(out, name) {
				t.Fatalf("report does not state the %s policy:\n%s", name, out)
			}
			if hookRuns != 0 {
				t.Fatalf("AfterPersistentPreRun ran %d times for the policy diagnostic, want 0", hookRuns)
			}
		})
	}
}

// Every reviewed discovery path except the lazily mounted help command must be
// a live executable command.
func TestCrossPlatformCoverageCommandPolicyReviewedPathsAreLive(t *testing.T) {
	restrictForTest(t, staticRules())
	snapshot := cmdpolicy.For(NewRootCommand())
	for _, entry := range cmdpolicy.DiscoveryEntries() {
		if entry.Path == "help" {
			continue // mounted by Cobra at execution time
		}
		if decision, ok := snapshot.Decision(entry.Path); !ok || !decision.Allowed {
			t.Errorf("discovery entry %q: decision = %+v (evaluated %v), want allowed", entry.Path, decision, ok)
		}
	}
}

func TestCrossPlatformCoverageCommandPolicyEmptyRuleSetDeniesBusiness(t *testing.T) {
	restrictForTest(t, staticRules())
	snapshot := cmdpolicy.For(NewRootCommand())
	if snapshot.State() != cmdpolicy.StateActive {
		t.Fatalf("state = %s, want active for an empty rule set", snapshot.State())
	}
	if decision, _ := snapshot.Decision("calendar/event/list"); decision.Allowed {
		t.Fatal("an empty rule set must deny business commands, not disable the policy")
	}
	if out, err := executePolicyRoot(t, "version"); err != nil {
		t.Fatalf("version must stay available: %v\n%s", err, out)
	}
	if out, err := executePolicyRoot(t, "--help"); err != nil || strings.Contains(out, "calendar") {
		t.Fatalf("root help must work and list no denied product: err=%v\n%s", err, out)
	}
}

func TestCrossPlatformCoverageCommandPolicyInvalidFailsClosed(t *testing.T) {
	for name, restrict := range map[string]func() []edition.CommandRule{
		"invalid rule": staticRules(edition.CommandRule{Name: "bad", MaxRisk: "read"}),
		"panic":        func() []edition.CommandRule { panic("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			restrictForTest(t, restrict)
			snapshot := cmdpolicy.For(NewRootCommand())
			if snapshot.State() != cmdpolicy.StateInvalid {
				t.Fatalf("state = %s, want invalid", snapshot.State())
			}
			for _, path := range []string{"calendar/event/list", "schema", "auth/login"} {
				if decision, ok := snapshot.Decision(path); !ok || decision.Allowed || decision.ReasonCode != cmdpolicy.ReasonPolicyInvalid {
					t.Errorf("%s: decision = %+v, want policy_invalid", path, decision)
				}
			}
			out, err := executePolicyRoot(t, "config", "policy", "show", "--format", "json")
			if err != nil || !strings.Contains(out, "invalid") {
				t.Fatalf("config policy show must report the invalid policy: err=%v\n%s", err, out)
			}
			if out, err := executePolicyRoot(t, "version"); err != nil {
				t.Fatalf("version must stay available: %v\n%s", err, out)
			}
		})
	}
}

func TestCrossPlatformCoverageCommandPolicyShowReport(t *testing.T) {
	restrictForTest(t, staticRules(
		edition.CommandRule{Name: "no-delete", Allow: []string{"calendar/**"}, Deny: []string{"calendar/event/delete"}},
		edition.CommandRule{Name: "everything"},
	))
	out, err := executePolicyRoot(t, "config", "policy", "show", "--format", "json")
	if err != nil {
		t.Fatalf("config policy show: %v\n%s", err, out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &envelope); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if envelope["ok"] != true {
		t.Fatalf("envelope ok = %v, want the unified success envelope:\n%s", envelope["ok"], out)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["state"] != "active" || data["source"] != "policy-test" || data["extensions_disabled"] != true {
		t.Fatalf("report header = %v/%v/%v", data["state"], data["source"], data["extensions_disabled"])
	}
	if rules, _ := data["rules"].([]any); len(rules) != 2 {
		t.Fatalf("rules = %v, want 2", data["rules"])
	}
	kinds := map[string]bool{}
	for _, raw := range data["hints"].([]any) {
		hint := raw.(map[string]any)
		kinds[hint["kind"].(string)] = true
		if hint["kind"] == cmdpolicy.HintDenyOverridden && hint["path"] == "calendar/event/delete" {
			if hint["rule"] != "no-delete" || hint["other_rule"] != "everything" {
				t.Fatalf("deny_overridden hint = %v", hint)
			}
		}
	}
	if !kinds[cmdpolicy.HintDenyOverridden] || !kinds[cmdpolicy.HintRuleAllowsAll] {
		t.Fatalf("hints = %v, want both deny_overridden and rule_allows_all", data["hints"])
	}
	if channels, _ := data["channels"].([]any); len(channels) != len(cmdpolicy.ChannelEntries()) {
		t.Fatalf("channels = %d entries, want %d", len(channels), len(cmdpolicy.ChannelEntries()))
	}
}

// A restricted build loads nothing the local user controls: no runtime plugin
// (and therefore no plugin endpoint, sh -c hook, or skill sync).
func TestCrossPlatformCoverageCommandPolicyDisablesRuntimeExtensions(t *testing.T) {
	loads := 0
	testseam.Swap(t, &rootLoadPlugins, func(*cobra.Command, *pipeline.Engine, executor.Runner, string) []*cobra.Command {
		loads++
		return nil
	})
	NewRootCommand()
	if loads != 1 {
		t.Fatalf("plugin loader ran %d times without a policy, want 1", loads)
	}
	restrictForTest(t, staticRules(edition.CommandRule{Name: "all"}))
	NewRootCommand()
	if loads != 1 {
		t.Fatalf("plugin loader ran while command restriction is enabled (%d loads)", loads)
	}
	restrictForTest(t, staticRules(edition.CommandRule{Name: "bad", MaxRisk: "nope"}))
	NewRootCommand()
	if loads != 1 {
		t.Fatalf("plugin loader ran while the policy is invalid (%d loads)", loads)
	}
}

// A user-defined shortcut declares its own risk in a local file. That value is
// never a trusted declaration: a restricted build does not load user shortcuts
// at all, so a self-declared "read" cannot pass a read-only rule.
//
// There is deliberately no positive control here: loading a user shortcut
// registers it in the process-global shortcut registry for every later test.
func TestCrossPlatformCoverageCommandPolicyIgnoresUserShortcuts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	shortcuts := filepath.Join(dir, "shortcuts")
	if err := os.MkdirAll(shortcuts, 0o700); err != nil {
		t.Fatal(err)
	}
	spec := `version: 1
service: policyprobe
command: "+notify"
product: chat
risk: read
execute:
  tool: send_message
  bind:
    open_conversation_id: "cid_x"
    text: "${text}"
flags:
  - {name: text, type: string, required: true, desc: text}
`
	if err := os.WriteFile(filepath.Join(shortcuts, "policyprobe.notify.yaml"), []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	restrictForTest(t, staticRules(edition.CommandRule{Name: "any-read", MaxEffect: edition.EffectRead, AllowUnannotated: true}))
	root := NewRootCommand()
	for _, child := range root.Commands() {
		if child.Name() == "policyprobe" {
			t.Fatal("a user-defined shortcut was mounted while command restriction is enabled")
		}
	}
	if decision, evaluated := cmdpolicy.For(root).Decision("policyprobe/+notify"); evaluated {
		t.Fatalf("a user-defined shortcut was evaluated by the policy: %+v", decision)
	}
}

// Each executable tree owns its snapshot; the declaration-only Schema source
// tree carries none, and concurrent construction cannot cross-contaminate.
func TestCrossPlatformCoverageCommandPolicySnapshotIsPerTree(t *testing.T) {
	restrictForTest(t, staticRules(edition.CommandRule{Name: "calendar", Allow: []string{"calendar/**"}}))
	restricted := NewRootCommand()
	if source := NewSchemaSourceRootCommand(); cmdpolicy.For(source) != nil {
		t.Fatal("the Schema source root must never carry a command policy")
	} else if findPolicyCommand(t, source, "contact/user/get-self").DisableFlagParsing {
		t.Fatal("the Schema source root was stubbed by the command policy")
	}

	restrictForTest(t, staticRules(edition.CommandRule{Name: "contact", Allow: []string{"contact/**"}}))
	other := NewRootCommand()
	if decision, _ := cmdpolicy.For(restricted).Decision("calendar/event/list"); !decision.Allowed {
		t.Fatal("a later tree changed the policy of an existing tree")
	}
	if decision, _ := cmdpolicy.For(other).Decision("calendar/event/list"); decision.Allowed {
		t.Fatal("the second tree did not get its own policy")
	}

	var wg sync.WaitGroup
	results := make([]bool, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			root := newRootCommandWithMode(context.Background(), nil, false, false, false)
			decision, ok := cmdpolicy.For(root).Decision("contact/user/get-self")
			results[i] = ok && decision.Allowed
		}()
	}
	wg.Wait()
	for i, ok := range results {
		if !ok {
			t.Fatalf("concurrent tree %d did not resolve its own policy", i)
		}
	}
}

// The policy's identity attribution must agree with the Schema collector: the
// same primaries, and every declared alias attributed to the same primary.
func TestCrossPlatformCoverageCommandPolicyIdentityMatchesSchemaCollector(t *testing.T) {
	specs, _, err := cli.CollectIdentitySpecs(NewSchemaSourceRootCommand())
	if err != nil {
		t.Fatalf("CollectIdentitySpecs: %v", err)
	}
	// Deny everything by primary path only; an alias attributed to the wrong
	// identity (or to none) would then be judged on its own path instead.
	restrictForTest(t, staticRules(edition.CommandRule{Name: "probe", Allow: []string{"__none__"}}))
	snapshot := cmdpolicy.For(newRootCommandWithMode(context.Background(), nil, false, false, false))

	var missing []string
	for _, spec := range specs {
		primary := cmdpolicy.DeclaredPath(spec.PrimaryCLIPath)
		want, ok := snapshot.Lookup(spec.CanonicalPath)
		if !ok {
			missing = append(missing, spec.CanonicalPath)
			continue
		}
		if got, ok := snapshot.Decision(primary); !ok || got != want {
			t.Errorf("%s: primary %q decision %+v differs from canonical lookup %+v", spec.CanonicalPath, primary, got, want)
		}
		for _, alias := range spec.Aliases {
			got, ok := snapshot.Lookup(alias)
			if !ok || got != want {
				t.Errorf("%s: alias %q decision %+v (found %v) differs from its primary %+v", spec.CanonicalPath, alias, got, ok, want)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%d Schema identities are unknown to the command policy, e.g. %v", len(missing), missing[:min(5, len(missing))])
	}
	if len(specs) == 0 {
		t.Fatal("the Schema collector returned no identities")
	}
}

// corecmd.ExecuteForTest is the supported way to run self-constructed trees;
// this guards the hybrid-denial primitive on one.
func TestCrossPlatformCoverageDenyGroupBusinessKeepsGroupContract(t *testing.T) {
	ran := false
	group := &cobra.Command{Use: "connect", RunE: func(*cobra.Command, []string) error { ran = true; return nil }}
	group.Flags().String("name", "", "")
	_ = group.MarkFlagRequired("name")
	group.AddCommand(&cobra.Command{Use: "status", RunE: func(*cobra.Command, []string) error { return nil }})
	corecmd.ApplyGroupPolicy(group, corecmd.GroupPolicy{Mode: corecmd.GroupHybrid, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoverySibling})
	denial := errors.New("denied")
	corecmd.DenyGroupBusiness(group, func(*cobra.Command, []string) error { return denial })
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(group)

	run := func(args ...string) error {
		root.SetArgs(args)
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		return corecmd.ExecuteForTest(root)
	}
	// The denial precedes required-flag validation of the business operation.
	if err := run("connect"); !errors.Is(err, denial) {
		t.Fatalf("connect = %v, want the denial before required-flag validation", err)
	}
	if err := run("connect", "status"); err != nil {
		t.Fatalf("connect status = %v, want the child to stay runnable", err)
	}
	if err := run("connect", "statu"); err == nil || errors.Is(err, denial) {
		t.Fatalf("connect statu = %v, want command-resolution recovery, not the denial", err)
	}
	if ran {
		t.Fatal("the denied business operation ran")
	}
}
