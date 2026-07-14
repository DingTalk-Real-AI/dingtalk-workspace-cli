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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/fatih/color"
)

var (
	manualAgentExamplePlaceholderPattern = regexp.MustCompile(`<([^>]+)>`)
	manualAgentExampleDryRunJSONPattern  = regexp.MustCompile(`(?i)"dry_run"\s*:\s*true`)
)

// TestManualAgentExamplesContract is the always-on gate. It validates every
// example, including contract_only entries, against the live bound Cobra path,
// flags, required arguments, constraints, and final typed safety.
func TestManualAgentExamplesContract(t *testing.T) {
	plan := manualAgentExampleExecutionPlan(t)
	if plan.Total == 0 {
		t.Fatal("no reviewed Agent examples were contract validated")
	}
	t.Logf("Agent example contract: total=%d contract=%d dry_run=%d contract_only=%d", plan.Total, plan.Contract, plan.DryRun, plan.ContractOnly)
}

// TestManualAgentExamplesDryRun first validates every reviewed example against
// its real BoundCommand, Cobra required arguments, and final typed constraints.
// It then executes only the deterministic, explicitly declared dry_run subset
// without injecting --yes. Global flag inheritance is not treated as capability
// evidence. Runtime failures never create implicit skips. No shell is involved
// and HOME is isolated.
func TestManualAgentExamplesDryRun(t *testing.T) {
	if os.Getenv("DWS_AGENT_EXAMPLES_DRY_RUN") != "1" {
		t.Skip("set DWS_AGENT_EXAMPLES_DRY_RUN=1 to execute the explicitly reviewed Agent dry-run subset")
	}

	sandboxRoot := t.TempDir()
	homeDir := filepath.Join(sandboxRoot, "home")
	configDir := filepath.Join(sandboxRoot, "config")
	for _, dir := range []string{homeDir, configDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create isolated test directory %s: %v", dir, err)
		}
	}
	t.Setenv("HOME", homeDir)
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")

	plan := manualAgentExampleExecutionPlan(t)
	if plan.Total == 0 {
		t.Fatal("no reviewed Agent examples were contract validated")
	}
	t.Chdir(sandboxRoot)
	files := newManualAgentExampleFiles(t, sandboxRoot)

	selected := 0
	executed := 0
	for _, execution := range plan.Examples {
		if !manualAgentExampleShouldExerciseDryRun(execution) {
			continue
		}
		selected++
		execution := execution
		t.Run(fmt.Sprintf("%s/%d", strings.ReplaceAll(execution.CanonicalPath, ".", "/"), execution.Index), func(t *testing.T) {
			argv, err := cli.ParseManualAgentExampleArgv(execution.Example)
			if err != nil {
				t.Fatalf("parse example %q: %v", execution.Example, err)
			}
			args := materializeManualAgentExampleArgv(argv[1:], files)
			if manualAgentExampleHasFlag(args, "yes") {
				t.Fatalf("dry-run gate must not inject or accept --yes\nsource: %s\nargv: %q", execution.Example, args)
			}
			if !manualAgentExampleHasFlag(args, "dry-run") {
				args = append([]string{"--dry-run"}, args...)
			}

			capture, err := executeManualAgentExampleCapture(t, args)
			if capture.ToolCallAttempts != 0 {
				t.Fatalf("eligible dry-run attempted %d ToolCaller invocation(s)\nsource: %s\nargv: %q\noutput:\n%s", capture.ToolCallAttempts, execution.Example, args, capture.Output)
			}
			if capture.StdinBytesRead != 0 || manualAgentExamplePromptObserved(capture.Output) {
				t.Fatalf("eligible dry-run entered an interactive confirmation path (stdin bytes read: %d)\nsource: %s\nargv: %q\noutput:\n%s", capture.StdinBytesRead, execution.Example, args, capture.Output)
			}
			if err != nil {
				t.Fatalf("dry-run example failed: %v\nsource: %s\nargv: %q\noutput:\n%s", err, execution.Example, args, capture.Output)
			}
			previewKind, observed := manualAgentExampleDryRunEvidence(capture)
			if !observed {
				t.Fatalf("example returned without audited dry-run evidence (caller dry-run checks: %d)\nsource: %s\nargv: %q\noutput:\n%s", capture.DryRunChecks, execution.Example, args, capture.Output)
			}
			if want := execution.DryRun.PreviewKind; previewKind != want {
				t.Fatalf("dry-run preview kind = %q, Schema declares %q\nsource: %s\nargv: %q\noutput:\n%s", previewKind, want, execution.Example, args, capture.Output)
			}
			t.Logf("dry_run_capability_candidate=%s", previewKind)
			executed++
		})
	}
	if executed != selected {
		t.Fatalf("executed dry_run examples = %d, selected capability set requires %d", executed, selected)
	}
	t.Logf("Agent examples: total=%d contract=%d dry_run_selected=%d planned_dry_run=%d contract_only=%d reviewed_manual=%d", plan.Total, plan.Contract, selected, plan.DryRun, plan.ContractOnly, plan.ReviewedContractOnly)
	reasonCodes := make([]string, 0, len(plan.ContractOnlyByReason))
	for reasonCode := range plan.ContractOnlyByReason {
		reasonCodes = append(reasonCodes, string(reasonCode))
	}
	sort.Strings(reasonCodes)
	for _, reasonCode := range reasonCodes {
		t.Logf("Agent examples contract_only[%s]=%d", reasonCode, plan.ContractOnlyByReason[cli.ManualAgentExampleReasonCode(reasonCode)])
	}
}

// manualAgentExampleShouldExerciseDryRun is the single selection boundary for
// the runtime gate. Capability comes only from the final typed ToolSpec; the
// example disposition may narrow that set but can never invent support.
func manualAgentExampleShouldExerciseDryRun(execution cli.ManualAgentExampleExecution) bool {
	return execution.DryRun != nil && execution.Mode == cli.ManualAgentExampleModeDryRun
}

func manualAgentExampleExecutionPlan(t testing.TB) cli.ManualAgentExampleExecutionPlan {
	t.Helper()
	hints, err := cli.LoadAgentHintsFromSelectionForValidation(os.DirFS("../cli/schema_hints/selection"))
	if err != nil {
		t.Fatalf("LoadAgentHintsFromSelectionForValidation() error = %v", err)
	}
	contractRoot := NewRootCommand()
	if _, err := cli.ApplyEmbeddedManualSchemaHints(contractRoot); err != nil {
		t.Fatalf("ApplyEmbeddedManualSchemaHints() error = %v", err)
	}
	effective, err := cli.BuildEffectiveCommandRegistry(contractRoot)
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry() error = %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(contractRoot, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	registry, err := cli.AssembleSchemaRegistryFromBound(bound)
	if err != nil {
		t.Fatalf("AssembleSchemaRegistryFromBound() error = %v", err)
	}
	if err := cli.ValidateReviewedDryRunCapabilityDelivery(registry); err != nil {
		t.Fatalf("ValidateReviewedDryRunCapabilityDelivery() error = %v", err)
	}
	plan, err := cli.BuildManualAgentExampleExecutionPlan(bound, registry, hints)
	if err != nil {
		t.Fatalf("BuildManualAgentExampleExecutionPlan() error = %v", err)
	}
	return plan
}

type manualAgentExampleCapture struct {
	Output           string
	DryRunChecks     int64
	ToolCallAttempts int64
	StdinBytesRead   int64
}

type manualAgentExampleFailClosedCaller struct {
	dryRunChecks     atomic.Int64
	toolCallAttempts atomic.Int64
}

func (c *manualAgentExampleFailClosedCaller) CallTool(_ context.Context, productID, toolName string, _ map[string]any) (*edition.ToolResult, error) {
	c.toolCallAttempts.Add(1)
	return nil, fmt.Errorf("real ToolCaller invocation blocked during Agent example dry-run: %s/%s", productID, toolName)
}

func (c *manualAgentExampleFailClosedCaller) Format() string { return "json" }

func (c *manualAgentExampleFailClosedCaller) DryRun() bool {
	c.dryRunChecks.Add(1)
	return true
}

func (c *manualAgentExampleFailClosedCaller) Fields() string { return "" }
func (c *manualAgentExampleFailClosedCaller) JQ() string     { return "" }

func executeManualAgentExampleCapture(t testing.TB, args []string) (manualAgentExampleCapture, error) {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{"dws"}, args...)
	defer func() { os.Args = oldArgs }()
	oldStdin := os.Stdin
	promptInput, err := os.CreateTemp(t.TempDir(), "agent-example-stdin-*.txt")
	if err != nil {
		t.Fatalf("open guarded stdin: %v", err)
	}
	defer promptInput.Close()
	if _, err := promptInput.WriteString("no\n"); err != nil {
		t.Fatalf("seed guarded stdin: %v", err)
	}
	if _, err := promptInput.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind guarded stdin: %v", err)
	}
	os.Stdin = promptInput
	defer func() { os.Stdin = oldStdin }()

	oldStdout, oldStderr := os.Stdout, os.Stderr
	oldColorOutput, oldColorError := color.Output, color.Error
	captureFile, err := os.CreateTemp(t.TempDir(), "agent-example-output-*.log")
	if err != nil {
		t.Fatalf("open output capture file: %v", err)
	}
	defer captureFile.Close()
	os.Stdout, os.Stderr = captureFile, captureFile
	color.Output, color.Error = captureFile, captureFile
	defer func() {
		os.Stdout, os.Stderr = oldStdout, oldStderr
		color.Output, color.Error = oldColorOutput, oldColorError
	}()

	root := NewRootCommand()
	originalCaller := helpers.GetCaller()
	auditCaller := &manualAgentExampleFailClosedCaller{}
	helpers.InitDeps(auditCaller)
	defer helpers.InitDeps(originalCaller)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(args)
	execErr := root.Execute()

	os.Stdout, os.Stderr = oldStdout, oldStderr
	color.Output, color.Error = oldColorOutput, oldColorError
	if _, err := captureFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind output capture file: %v", err)
	}
	captured, readErr := io.ReadAll(captureFile)
	if readErr != nil {
		t.Fatalf("read output capture file: %v", readErr)
	}
	stdinBytesRead, err := promptInput.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatalf("inspect guarded stdin: %v", err)
	}
	return manualAgentExampleCapture{
		Output:           output.String() + string(captured),
		DryRunChecks:     auditCaller.dryRunChecks.Load(),
		ToolCallAttempts: auditCaller.toolCallAttempts.Load(),
		StdinBytesRead:   stdinBytesRead,
	}, execErr
}

type manualAgentExampleFiles struct {
	root     string
	markdown string
	json     string
	batch    string
	binary   string
	image    string
}

func newManualAgentExampleFiles(t testing.TB, root string) manualAgentExampleFiles {
	t.Helper()
	markdown := filepath.Join(root, "content.md")
	jsonFile := filepath.Join(root, "report.json")
	batch := filepath.Join(root, "styles.json")
	binary := filepath.Join(root, "report.pdf")
	image := filepath.Join(root, "chart.png")
	for path, content := range map[string][]byte{
		markdown: []byte("# Agent dry-run fixture\n\nNo business call is allowed.\n"),
		jsonFile: []byte(`[{"content":"Agent dry-run fixture","sort":"0","key":"fixture","contentType":"markdown","type":"1"}]`),
		batch:    []byte(`[{"sheetId":"Sheet1","range":"A1:B2","fontWeight":"bold"}]`),
		binary:   []byte("%PDF-1.4\n%%EOF\n"),
		image:    {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("write dry-run fixture %s: %v", path, err)
		}
	}
	return manualAgentExampleFiles{root: root, markdown: markdown, json: jsonFile, batch: batch, binary: binary, image: image}
}

func materializeManualAgentExampleArgv(argv []string, files manualAgentExampleFiles) []string {
	result := append([]string(nil), argv...)
	for index := range result {
		result[index] = manualAgentExamplePlaceholderPattern.ReplaceAllStringFunc(result[index], func(match string) string {
			name := strings.TrimSuffix(strings.TrimPrefix(match, "<"), ">")
			switch strings.ToLower(name) {
			case "basetime", "remindertimestamp", "reminder-time-stamp":
				return "1780000000000"
			case "duedateoffset", "due-date-offset":
				return "0"
			case "reminderrules", "reminder-rules":
				return `[{"remindType":"minute","remindTime":10}]`
			case "filepath", "file-path":
				return files.binary
			case "uuid1,uuid2":
				return "uuid1,uuid2"
			default:
				clean := strings.NewReplacer(",", "_", "-", "_", ".", "_").Replace(name)
				return "test_" + clean
			}
		})
	}

	for index := 0; index < len(result); index++ {
		name, inline, ok := manualAgentExampleLongFlag(result[index])
		if !ok {
			continue
		}
		valueIndex := index + 1
		value := inline
		if inline == "" && valueIndex < len(result) {
			value = result[valueIndex]
		}
		replacement := ""
		switch name {
		case "file", "file-path":
			if strings.Contains(strings.ToLower(value), "png") {
				replacement = files.image
			} else {
				replacement = files.binary
			}
		case "content-file":
			replacement = files.markdown
		case "contents-file":
			replacement = files.json
		case "batch":
			if strings.HasSuffix(strings.ToLower(value), "styles.json") {
				replacement = files.batch
			}
		case "output":
			if value == "." || value == "" {
				replacement = files.root
			} else {
				replacement = filepath.Join(files.root, filepath.Base(value))
			}
		}
		if replacement == "" {
			continue
		}
		if inline != "" {
			result[index] = "--" + name + "=" + replacement
		} else if valueIndex < len(result) {
			result[valueIndex] = replacement
			index++
		}
	}
	return result
}

func manualAgentExampleLongFlag(argument string) (name, inline string, ok bool) {
	if !strings.HasPrefix(argument, "--") {
		return "", "", false
	}
	name, inline, _ = strings.Cut(strings.TrimPrefix(argument, "--"), "=")
	return name, inline, name != ""
}

func manualAgentExampleHasFlag(argv []string, target string) bool {
	for _, argument := range argv {
		if argument == "--"+target || strings.HasPrefix(argument, "--"+target+"=") {
			return true
		}
	}
	return false
}

func manualAgentExampleDryRunObserved(capture manualAgentExampleCapture) bool {
	_, ok := manualAgentExampleDryRunEvidence(capture)
	return ok
}

func manualAgentExampleDryRunEvidence(capture manualAgentExampleCapture) (string, bool) {
	normalized := strings.ToLower(capture.Output)
	if manualAgentExampleDryRunJSONPattern.MatchString(capture.Output) {
		return cli.DryRunPreviewRequest, true
	}
	if strings.Contains(normalized, "[dry-run]") {
		return cli.DryRunPreviewInvocation, true
	}
	if capture.DryRunChecks > 0 && strings.Contains(capture.Output, "操作:") {
		return cli.DryRunPreviewPlan, true
	}
	return "", false
}

func TestManualAgentExampleDryRunEvidenceAcceptsSharedAndCommandPlans(t *testing.T) {
	if !manualAgentExampleDryRunObserved(manualAgentExampleCapture{Output: "[DRY-RUN] Preview only, not executed:\nTool: calendar_list"}) {
		t.Fatal("dry-run output with a Tool and nil Arguments was not recognized")
	}
	if manualAgentExampleDryRunObserved(manualAgentExampleCapture{Output: "Tool: calendar_list"}) {
		t.Fatal("a Tool line without dry-run evidence must not be accepted")
	}
	for _, falseEvidence := range []string{
		"unknown flag: --dry-run",
		"Run again with --dry-run to preview the operation",
		`{"dry_run":false,"executed":true}`,
	} {
		if manualAgentExampleDryRunObserved(manualAgentExampleCapture{Output: falseEvidence}) {
			t.Errorf("non-evidence text was mistaken for a successful dry-run: %q", falseEvidence)
		}
	}
	operationSummary := "操作:             下载钉盘文件\n文件ID: test"
	if manualAgentExampleDryRunObserved(manualAgentExampleCapture{Output: operationSummary}) {
		t.Fatal("a human-only operation summary without an audited dry-run check must not be accepted")
	}
	if !manualAgentExampleDryRunObserved(manualAgentExampleCapture{Output: operationSummary, DryRunChecks: 1}) {
		t.Fatal("a command plan guarded by the injected caller's dry-run check was not recognized")
	}
}

func manualAgentExamplePromptObserved(output string) bool {
	normalized := strings.ToLower(output)
	for _, marker := range []string{
		"confirm ",
		"confirm deletion?",
		"confirm action?",
		"confirm create?",
		"confirm update?",
		"confirm save?",
		"confirm import?",
		"are you sure",
		"operation cancelled",
		"操作已取消",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func TestManualAgentExamplePromptObservedRejectsInteractiveConfirmation(t *testing.T) {
	for _, prompt := range []string{
		"Confirm deletion? (yes/no):",
		"Confirm action? (yes/no):",
		"Confirm create? (yes/no):",
		"Confirm update? (yes/no):",
		"Confirm save? (yes/no):",
		"Confirm import? (yes/no):",
		"Are you sure you want to continue?",
		"Operation cancelled",
	} {
		if !manualAgentExamplePromptObserved(prompt) {
			t.Errorf("interactive confirmation output was not detected: %q", prompt)
		}
	}
	if manualAgentExamplePromptObserved(`{"dry_run":true,"confirmation":"user_required"}`) {
		t.Fatal("typed safety metadata was mistaken for an interactive prompt")
	}
}

func TestAitableAdvpermDisableDryRunSkipsConfirmationAndToolCall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	args := []string{
		"--dry-run", "--format", "json",
		"aitable", "advperm", "disable",
		"--base-id", "BASE_ID",
	}
	if manualAgentExampleHasFlag(args, "yes") {
		t.Fatal("regression test must not bypass confirmation with --yes")
	}
	capture, err := executeManualAgentExampleCapture(t, args)
	if err != nil {
		t.Fatalf("advperm disable fail-closed dry-run failed: %v\noutput:\n%s", err, capture.Output)
	}
	if capture.StdinBytesRead != 0 || manualAgentExamplePromptObserved(capture.Output) {
		t.Fatalf("advperm disable dry-run entered confirmation (stdin bytes read: %d)\noutput:\n%s", capture.StdinBytesRead, capture.Output)
	}
	if capture.ToolCallAttempts != 0 {
		t.Fatalf("advperm disable dry-run attempted %d real ToolCaller invocation(s)\noutput:\n%s", capture.ToolCallAttempts, capture.Output)
	}
	if !manualAgentExampleDryRunObserved(capture) {
		t.Fatalf("advperm disable returned no audited dry-run evidence (caller dry-run checks: %d)\noutput:\n%s", capture.DryRunChecks, capture.Output)
	}
}

func TestManualAgentExampleFailClosedCallerRecordsToolCalls(t *testing.T) {
	caller := &manualAgentExampleFailClosedCaller{}
	if !caller.DryRun() {
		t.Fatal("fail-closed caller must advertise dry-run mode")
	}
	if _, err := caller.CallTool(context.Background(), "calendar", "list_events", nil); err == nil {
		t.Fatal("fail-closed caller accepted a ToolCaller invocation")
	}
	if got := caller.dryRunChecks.Load(); got != 1 {
		t.Fatalf("DryRun() checks = %d, want 1", got)
	}
	if got := caller.toolCallAttempts.Load(); got != 1 {
		t.Fatalf("CallTool() attempts = %d, want 1", got)
	}
}

func TestManualAgentExampleChatGroupMuteMemberUsesCommandDryRunPreview(t *testing.T) {
	sandboxRoot := t.TempDir()
	configDir := filepath.Join(sandboxRoot, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create isolated config directory: %v", err)
	}
	t.Setenv("HOME", sandboxRoot)
	t.Setenv("DWS_CONFIG_DIR", configDir)

	capture, err := executeManualAgentExampleCapture(t, []string{
		"--dry-run",
		"chat", "group-mute-member",
		"--group", "test_openConversationId",
		"--users", "userId1,userId2",
		"--mute-time", "3600000",
	})
	if err != nil {
		t.Fatalf("group-mute-member dry-run failed: %v\noutput:\n%s", err, capture.Output)
	}
	if capture.ToolCallAttempts != 0 {
		t.Fatalf("group-mute-member dry-run attempted %d ToolCaller invocation(s)\noutput:\n%s", capture.ToolCallAttempts, capture.Output)
	}
	if capture.DryRunChecks == 0 {
		t.Fatalf("group-mute-member did not enter its audited command dry-run path\noutput:\n%s", capture.Output)
	}
	if capture.StdinBytesRead != 0 || manualAgentExamplePromptObserved(capture.Output) {
		t.Fatalf("group-mute-member dry-run entered an interactive prompt (stdin bytes read: %d)\noutput:\n%s", capture.StdinBytesRead, capture.Output)
	}
	if !manualAgentExampleDryRunObserved(capture) {
		t.Fatalf("group-mute-member returned no audited dry-run evidence\noutput:\n%s", capture.Output)
	}
	for _, expected := range []string{`"uids"`, `"userId1"`, `"userId2"`} {
		if !strings.Contains(capture.Output, expected) {
			t.Fatalf("group-mute-member command preview missing %s\noutput:\n%s", expected, capture.Output)
		}
	}
	if strings.Contains(capture.Output, `"openDingTalkIds"`) {
		t.Fatalf("group-mute-member dry-run unexpectedly resolved user IDs remotely\noutput:\n%s", capture.Output)
	}
}
