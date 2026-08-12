// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRunRepositoryAndSetupFailures(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(root, app.NewRootCommand(), &stdout, &stderr); code != 0 {
		t.Fatalf("repository policy run = %d, stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "multi IM Skill chain check: ok") {
		t.Fatalf("success output = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(t.TempDir(), &cobra.Command{Use: "dws"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "read IM intent route manifest") {
		t.Fatalf("missing-manifest run = %d, stderr=%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(root, &cobra.Command{Use: "dws"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "absent from BoundCommandRegistry") {
		t.Fatalf("unbound-root run = %d, stderr=%s", code, stderr.String())
	}

	oldBuild, oldBind := buildEffective, bindEffective
	t.Cleanup(func() { buildEffective, bindEffective = oldBuild, oldBind })
	buildEffective = func(*cobra.Command) (cli.EffectiveCommandRegistry, error) {
		return cli.EffectiveCommandRegistry{}, os.ErrInvalid
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(root, app.NewRootCommand(), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "build effective CommandRegistry") {
		t.Fatalf("registry-build-failure run = %d, stderr=%s", code, stderr.String())
	}
	buildEffective = oldBuild

	bindEffective = func(*cobra.Command, cli.EffectiveCommandRegistry) (cli.BoundCommandRegistry, error) {
		return cli.BoundCommandRegistry{}, os.ErrInvalid
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(root, app.NewRootCommand(), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "bind effective CommandRegistry") {
		t.Fatalf("binding-failure run = %d, stderr=%s", code, stderr.String())
	}
	bindEffective = oldBind

	failureRoot := t.TempDir()
	writeTestFile(t, failureRoot, manifestRelativePath, `{"version":1}`)
	stdout.Reset()
	stderr.Reset()
	if code := run(failureRoot, app.NewRootCommand(), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "multi IM Skill chain check failed") {
		t.Fatalf("validation-failure run = %d, stderr=%s", code, stderr.String())
	}
}

func TestCrossPlatformCoverageMainUsesInjectableProcessBoundaries(t *testing.T) {
	oldGetwd, oldExit := mainGetwd, mainExit
	t.Cleanup(func() { mainGetwd, mainExit = oldGetwd, oldExit })
	var exitCode int
	mainExit = func(code int) { exitCode = code }
	mainGetwd = func() (string, error) { return "", os.ErrPermission }
	main()
	if exitCode != 2 {
		t.Fatalf("getwd failure exit = %d", exitCode)
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	mainGetwd = func() (string, error) { return root, nil }
	main()
	if exitCode != 0 {
		t.Fatalf("successful main exit = %d", exitCode)
	}
}

func TestCrossPlatformCoverageValidateManifestAcceptsExactReviewedRoute(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "skills/multi/dingtalk-chat/SKILL.md", "| 发消息 | `dws chat +send` | <!-- dws-intent: chat.send -->\n")
	writeTypedContractReference(t, root, "skills/multi/dingtalk-chat/references/contracts.md")
	writeEventHandoffReference(t, root, "skills/multi/dingtalk-event/references/event-im-output.md")
	manifest := routeManifest{
		Version:           3,
		MarkerRoots:       []string{"skills/multi/dingtalk-chat"},
		RetiredScripts:    []string{"skills/multi/dingtalk-chat/scripts/retired.py"},
		ContractReference: "skills/multi/dingtalk-chat/references/contracts.md",
		HandoffReference:  "skills/multi/dingtalk-event/references/event-im-output.md",
		Intents: []intentRoute{{
			ID:                    "chat.send",
			PreferredTool:         "chat.shortcut_send",
			ForbiddenDefaultTools: []string{"chat.atomic_send"},
			References:            []string{"skills/multi/dingtalk-chat/SKILL.md"},
		}},
	}
	tools := map[string]toolFact{
		"chat.shortcut_send": {Canonical: "chat.shortcut_send", PrimaryPath: "chat +send", Confirmation: "user_required", UseWhen: []string{"普通发送"}, AvoidWhen: []string{"底层字段另选"}, HasMeta: true},
		"chat.atomic_send":   {Canonical: "chat.atomic_send", PrimaryPath: "chat message send", Confirmation: "not_required", UseWhen: []string{"底层字段"}, AvoidWhen: []string{"普通发送使用 +send"}, HasMeta: true},
	}
	if failures := validateManifest(root, manifest, tools); len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
}

func TestCrossPlatformCoverageValidateManifestRejectsWrongMarkerAndSafetyDowngrade(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "skills/multi/dingtalk-chat/SKILL.md", "| 发消息 | `dws chat message send` | <!-- dws-intent: chat.send -->\n")
	writeTypedContractReference(t, root, "skills/multi/dingtalk-chat/references/contracts.md")
	writeEventHandoffReference(t, root, "skills/multi/dingtalk-event/references/event-im-output.md")
	manifest := routeManifest{
		Version:           3,
		MarkerRoots:       []string{"skills/multi/dingtalk-chat"},
		RetiredScripts:    []string{"skills/multi/dingtalk-chat/scripts/retired.py"},
		ContractReference: "skills/multi/dingtalk-chat/references/contracts.md",
		HandoffReference:  "skills/multi/dingtalk-event/references/event-im-output.md",
		Intents: []intentRoute{{
			ID:                    "chat.send",
			PreferredTool:         "chat.shortcut_send",
			AllowedFallbacks:      []routeFallback{{Tool: "chat.atomic_send", ReasonCode: "raw_field"}},
			ForbiddenDefaultTools: []string{"chat.atomic_send"},
			References:            []string{"skills/multi/dingtalk-chat/SKILL.md"},
		}},
	}
	tools := map[string]toolFact{
		"chat.shortcut_send": {Canonical: "chat.shortcut_send", PrimaryPath: "chat +send", Confirmation: "user_required", UseWhen: []string{"普通发送"}, AvoidWhen: []string{"底层字段另选"}, HasMeta: true},
		"chat.atomic_send":   {Canonical: "chat.atomic_send", PrimaryPath: "chat message send", Confirmation: "not_required", UseWhen: []string{"普通发送"}, HasMeta: true},
	}
	failures := strings.Join(validateManifest(root, manifest, tools), "\n")
	for _, want := range []string{"confirmation", "needs non-empty use_when and avoid_when", "must contain preferred path", "uses forbidden default"} {
		if !strings.Contains(failures, want) {
			t.Fatalf("failures = %s, want %q", failures, want)
		}
	}
}

func TestCrossPlatformCoverageLoadManifestRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateRetiredScriptsRejectsRepublishedAndDuplicatePaths(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "skills/multi/dingtalk-chat/scripts/export.py", "republished\n")
	failures := strings.Join(validateRetiredScripts(root, []string{
		"skills/multi/dingtalk-chat/scripts/export.py",
		"skills/multi/dingtalk-chat/scripts/export.py",
		"../unsafe.py",
	}), "\n")
	for _, want := range []string{"was republished", "duplicate retired", "invalid retired"} {
		if !strings.Contains(failures, want) {
			t.Fatalf("failures = %s, want %q", failures, want)
		}
	}
}

func TestCrossPlatformCoverageValidateTypedContractReferenceRejectsDrift(t *testing.T) {
	root := t.TempDir()
	writeTypedContractReference(t, root, "contracts.md")
	path := filepath.Join(root, "contracts.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "`im.message-list.v1`", "`drifted`", 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	failures := strings.Join(validateTypedContractReference(root, "contracts.md"), "\n")
	if !strings.Contains(failures, "MESSAGE_RESULT contract differs") {
		t.Fatalf("failures = %s", failures)
	}
}

func TestCrossPlatformCoverageValidateEventHandoffReferenceRejectsNaturalTargetDrift(t *testing.T) {
	root := t.TempDir()
	writeEventHandoffReference(t, root, "handoff.md")
	path := filepath.Join(root, "handoff.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "--group <conversation_id>", "--chat-query <display_name>", 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	failures := strings.Join(validateEventHandoffReference(root, "handoff.md"), "\n")
	if !strings.Contains(failures, "exact stable-ID mapping") {
		t.Fatalf("failures = %s", failures)
	}
}

func TestCrossPlatformCoverageValidateReferencesAcceptCRLF(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		path     string
		write    func(*testing.T, string, string)
		validate func(string, string) []string
	}{
		{path: "contracts.md", write: writeTypedContractReference, validate: validateTypedContractReference},
		{path: "handoff.md", write: writeEventHandoffReference, validate: validateEventHandoffReference},
	} {
		tc.write(t, root, tc.path)
		path := filepath.Join(root, tc.path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if failures := tc.validate(root, tc.path); len(failures) != 0 {
			t.Errorf("%s CRLF failures = %v", tc.path, failures)
		}
	}
}

func TestCrossPlatformCoverageManifestFailureMatrix(t *testing.T) {
	root := t.TempDir()
	writeTypedContractReference(t, root, "contracts.md")
	writeEventHandoffReference(t, root, "handoff.md")
	writeTestFile(t, root, "refs/declared.md", "<!-- dws-intent: declared --> missing preferred; dws chat forbidden\n<!-- dws-intent: declared --> duplicate\n")
	writeTestFile(t, root, "refs/markers.md", "<!-- dws-intent: unknown -->\n<!-- dws-intent: declared -->\n<!-- dws-intent: preferred-absent -->\n")
	tools := map[string]toolFact{
		"preferred.empty":    {PrimaryPath: "chat preferred", HasMeta: false},
		"fallback.empty":     {PrimaryPath: "chat fallback", HasMeta: false},
		"fallback.different": {PrimaryPath: "chat fallback-different", HasMeta: true, Confirmation: "not_required"},
		"forbidden.no-route": {PrimaryPath: "chat forbidden", HasMeta: true, Confirmation: "not_required", AvoidWhen: []string{"keep using lower tool"}},
		"forbidden.no-entry": {PrimaryPath: "chat forbidden-missing"},
	}
	manifest := routeManifest{
		Version:           1,
		MarkerRoots:       []string{"refs", "../unsafe", "missing-root"},
		RetiredScripts:    nil,
		ContractReference: "contracts.md",
		HandoffReference:  "handoff.md",
		Intents: []intentRoute{
			{ID: "INVALID ID"},
			{ID: "preferred-absent", PreferredTool: "absent"},
			{ID: "preferred-no-selection", PreferredTool: "forbidden.no-entry"},
			{
				ID: "declared", PreferredTool: "preferred.empty",
				AllowedFallbacks: []routeFallback{
					{},
					{Tool: "fallback.empty", ReasonCode: "BAD-CODE"},
					{Tool: "fallback.empty", ReasonCode: "duplicate"},
					{Tool: "fallback.absent", ReasonCode: "missing"},
					{Tool: "fallback.different", ReasonCode: "raw_field"},
				},
				ForbiddenDefaultTools: []string{"", "forbidden.absent", "forbidden.no-entry", "forbidden.no-route", "forbidden.no-route"},
				References:            []string{"../unsafe.md", "refs/wrong.txt", "refs/missing.md", "refs/declared.md", "refs/declared.md"},
			},
			{ID: "declared"},
		},
	}
	failures := strings.Join(validateManifest(root, manifest, tools), "\n")
	for _, want := range []string{
		"manifest version", "retired_scripts", "intent id",
		"preferred tool", "ResolveMeta", "needs non-empty", "allowed fallback", "reason_code", "fallback tool",
		"confirmation", "forbidden default", "invalid reference", "does not exist", "repeats reference",
		"unknown dws-intent", "undeclared dws-intent", "missing its dws-intent marker", "has 2 dws-intent markers",
		"invalid marker root", "scan marker root", "duplicate intent id",
	} {
		if !strings.Contains(failures, want) {
			t.Errorf("failure matrix missing %q:\n%s", want, failures)
		}
	}
	if failures := validateManifest(root, routeManifest{}, nil); len(failures) == 0 {
		t.Fatal("empty manifest unexpectedly passed")
	}
}

func TestCrossPlatformCoverageReferenceFailureBranches(t *testing.T) {
	root := t.TempDir()
	if got := validateTypedContractReference(root, "../unsafe.md"); len(got) == 0 || !strings.Contains(got[0], "invalid contract_reference") {
		t.Fatalf("unsafe contract result = %#v", got)
	}
	if got := validateTypedContractReference(root, "missing.md"); len(got) == 0 || !strings.Contains(got[0], "read typed contract") {
		t.Fatalf("missing contract result = %#v", got)
	}
	writeTestFile(t, root, "malformed.md", "<!-- DWS_MESSAGE_RESULT_CONTRACT_END -->\n<!-- DWS_MESSAGE_RESULT_CONTRACT_START -->")
	if got := strings.Join(validateTypedContractReference(root, "malformed.md"), "\n"); !strings.Contains(got, "malformed MESSAGE_RESULT") || !strings.Contains(got, "marker pair") {
		t.Fatalf("malformed contract result = %s", got)
	}
	if got := validateEventHandoffReference(root, "../unsafe.md"); len(got) == 0 || !strings.Contains(got[0], "invalid handoff_reference") {
		t.Fatalf("unsafe handoff result = %#v", got)
	}
	if got := validateEventHandoffReference(root, "missing-handoff.md"); len(got) == 0 || !strings.Contains(got[0], "read event handoff") {
		t.Fatalf("missing handoff result = %#v", got)
	}
	writeTestFile(t, root, "handoff-markers.md", "no markers")
	if got := strings.Join(validateEventHandoffReference(root, "handoff-markers.md"), "\n"); !strings.Contains(got, "marker pair") {
		t.Fatalf("handoff marker result = %s", got)
	}

	if markdownCodeList(nil) != "—" || markdownBreakList(nil) != "—" {
		t.Fatal("empty markdown list rendering drifted")
	}
	tooLong := strings.Repeat("x", 5000) + ".py"
	if got := strings.Join(validateRetiredScripts(root, []string{tooLong}), "\n"); !strings.Contains(got, "inspect retired script") {
		t.Fatalf("long retired script result = %s", got)
	}
	for _, path := range []string{"", ".", "..", "../x", "a/../b"} {
		if safeRepositoryPath(path) {
			t.Errorf("unsafe repository path accepted: %q", path)
		}
	}
	if containsCLIPath("run dws chat send-more", "chat send") || !containsCLIPath("run dws chat send`", "chat send") {
		t.Fatal("CLI path boundary check drifted")
	}
	if stringSliceContains([]string{"a/b"}, "missing") {
		t.Fatal("string slice containment false positive")
	}
}

func TestCrossPlatformCoverageMarkerOpenFailure(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "refs/marker.md", "<!-- dws-intent: declared --> dws chat preferred\n")
	oldOpen := markerOpen
	markerOpen = func(string) (*os.File, error) { return nil, os.ErrPermission }
	t.Cleanup(func() { markerOpen = oldOpen })
	failures, _ := scanMarkers(root, []string{"refs"}, map[string]intentRoute{
		"declared": {ID: "declared", PreferredTool: "preferred", References: []string{"refs/marker.md"}},
	}, map[string]toolFact{"preferred": {PrimaryPath: "chat preferred"}})
	if len(failures) == 0 || !strings.Contains(failures[0], "scan marker root") {
		t.Fatalf("marker open failures = %#v", failures)
	}
}

func writeTypedContractReference(t *testing.T, root, relative string) {
	t.Helper()
	var content strings.Builder
	for _, block := range []struct {
		name string
		body string
	}{
		{name: "MESSAGE_RESULT", body: renderMessageResultContract()},
		{name: "IDENTITY_CAPABILITY", body: renderIdentityCapabilityContract()},
		{name: "CARD_WORKFLOW", body: renderCardWorkflowContract()},
		{name: "CAPABILITY_BOUNDARY", body: renderCapabilityBoundaryContract()},
	} {
		content.WriteString("<!-- DWS_" + block.name + "_CONTRACT_START -->\n")
		content.WriteString(block.body)
		content.WriteString("\n<!-- DWS_" + block.name + "_CONTRACT_END -->\n")
	}
	writeTestFile(t, root, relative, content.String())
}

func writeEventHandoffReference(t *testing.T, root, relative string) {
	t.Helper()
	writeTestFile(t, root, relative, `<!-- DWS_EVENT_CHAT_HANDOFF_START -->
| event field | exact chat target |
|---|---|
| `+"`conversation_id`"+` | `+"`dws chat +messages-send --as user --group <conversation_id>`"+` |
| `+"`sender_open_dingtalk_id`"+` | `+"`dws chat +messages-send --as user --open-dingtalk-id <sender_open_dingtalk_id>`"+` |
<!-- DWS_EVENT_CHAT_HANDOFF_END -->
`)
}

func writeTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
