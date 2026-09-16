// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	outputpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

func TestCrossPlatformCoverageWhiteboardInvalidRunStopsRenderAndCreate(t *testing.T) {
	source := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"day0-slot0","type":"shape","text":{"blocks":[{"type":"paragraph","runs":[{"text":"上午\n\n待安排"}]}]}}]}`
	for _, operation := range []string{"render", "create-with-content"} {
		t.Run(operation, func(t *testing.T) {
			caller := &whiteboardTestCaller{format: "json"}
			buf := installWhiteboardTestCaller(t, caller)
			cmd := newWhiteboardCommand()
			cmd.PersistentFlags().Bool("yes", false, "")
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			artifact := filepath.Join(t.TempDir(), "preview.svg")
			args := []string{operation, "--source", source}
			if operation == "render" {
				args = append(args, "--output", artifact)
			} else {
				args = append(args, "--name", "日历", "--request-id", "invalid-run", "--yes")
			}
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "/source/nodes/0/text/blocks/0/runs/0/text") || !strings.Contains(err.Error(), "day0-slot0") {
				t.Fatalf("missing actionable error: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid source reached MCP: %#v", caller.calls)
			}
			if _, err := os.Stat(artifact); !os.IsNotExist(err) {
				t.Fatalf("invalid source created artifact: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardRenderCommandWritesLocalArtifact(t *testing.T) {
	source := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[
		{"id":"card","type":"shape","x":40,"y":30,"width":200,"height":100,"geometry":"dml:roundRect","text":"<确认预览>"},
		{"id":"media","type":"image","x":280,"y":30,"width":120,"height":100,"url":"https://secret.example/image.png"}
	]}`
	sourcePath := writeWhiteboardFixture(t, source)
	artifactPath := filepath.Join(t.TempDir(), "preview.svg")
	caller := &whiteboardTestCaller{format: "json"}
	buffer := installWhiteboardTestCaller(t, caller)
	cmd := newWhiteboardCommand()
	ctx, _ := outputpkg.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(buffer)
	cmd.SetArgs([]string{"render", "--source", "@" + sourcePath, "--output", artifactPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	leaf, _, err := cmd.Find([]string{"render"})
	if err != nil {
		t.Fatal(err)
	}
	final, ok := contractfinal.RuntimeContractFinal(leaf)
	if !ok || final.Safety == nil || final.Safety.Effect != "read" || final.Safety.Confirmation != "not_required" {
		t.Fatalf("render ContractFinal=%#v", final)
	}
	if _, emitted, err := outputpkg.EmitStoredResult(leaf); err != nil || !emitted {
		t.Fatalf("emit render result: emitted=%v err=%v", emitted, err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("render unexpectedly called MCP: %#v", caller.calls)
	}
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(artifact, []byte("<svg")) || strings.Contains(string(artifact), "secret.example") || strings.Contains(string(artifact), "<确认预览>") {
		t.Fatalf("unsafe or malformed artifact: %s", artifact)
	}
	var envelope map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	if data["artifactPath"] != artifactPath || data["mimeType"] != "image/svg+xml" ||
		data["remoteReads"] != false || data["placeholderCount"] != float64(1) {
		t.Fatalf("render result=%s", buffer.String())
	}
	if data["nextAction"] != "await_user_confirmation" || !strings.Contains(data["confirmationInstruction"].(string), "停止执行") {
		t.Fatalf("render must stop for user review: %#v", data)
	}
	if !opennodes.ValidDigest(data["sourceDigest"].(string)) {
		t.Fatalf("sourceDigest=%#v", data["sourceDigest"])
	}
}

func TestCrossPlatformCoverageWhiteboardRenderInputsAndNoClobber(t *testing.T) {
	direct := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}`
	previousStdin := whiteboardRenderStdin
	whiteboardRenderStdin = strings.NewReader(direct)
	t.Cleanup(func() { whiteboardRenderStdin = previousStdin })
	if source, err := loadWhiteboardRenderSource("-"); err != nil || source == nil {
		t.Fatalf("stdin source=%#v err=%v", source, err)
	}
	if source, err := loadWhiteboardRenderSource(direct); err != nil || source == nil {
		t.Fatalf("inline source=%#v err=%v", source, err)
	}
	for _, invalid := range []string{"", `{`, `{"schemaVersion":"2.0","catalogVersion":"dml-v1","nodes":[]}`, filepath.Join(t.TempDir(), "missing.json")} {
		if _, err := loadWhiteboardRenderSource(invalid); err == nil {
			t.Fatalf("invalid source %q succeeded", invalid)
		}
	}

	source, err := opennodes.Parse([]byte(direct))
	if err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(t.TempDir(), "existing.svg")
	if err := os.WriteFile(existing, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := callWhiteboardRenderResult(&cobra.Command{}, "", map[string]any{"source": source, "output": existing}); err == nil {
		t.Fatal("existing output was overwritten without --force")
	}
	unchanged, _ := os.ReadFile(existing)
	if string(unchanged) != "original" {
		t.Fatalf("existing file changed: %q", unchanged)
	}
	if _, err := callWhiteboardRenderResult(&cobra.Command{}, "", map[string]any{"source": source, "output": existing, "force": true}); err != nil {
		t.Fatal(err)
	}
	if artifact, _ := os.ReadFile(existing); !bytes.HasPrefix(artifact, []byte("<svg")) {
		t.Fatalf("forced artifact=%q", artifact)
	}
	if _, err := callWhiteboardRenderResult(&cobra.Command{}, "", map[string]any{"source": source, "output": filepath.Join(t.TempDir(), "preview.png")}); err == nil {
		t.Fatal("non-SVG output extension accepted")
	}
}

func TestCrossPlatformCoverageWhiteboardCreateDigestGuardStopsBeforeRPC(t *testing.T) {
	direct := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"shape"}]}`
	source, err := opennodes.Parse([]byte(direct))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := opennodes.DigestSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateStandaloneWhiteboardExpectedDigest("bad"); err == nil {
		t.Fatal("malformed expected digest accepted")
	}

	caller := &whiteboardTestCaller{format: "json", response: func(whiteboardTestCall, int) string {
		return `{"success":true,"nodeId":"wb-new","requestId":"create-1","revision":"0"}`
	}}
	installWhiteboardTestCaller(t, caller)
	badArgs := map[string]any{"source": direct, "requestId": "create-1", "expectedSourceDigest": "sha256:" + strings.Repeat("0", 64)}
	if _, err := callStandaloneWhiteboardCreateResult(&cobra.Command{}, "", badArgs); err == nil {
		t.Fatal("mismatched digest accepted")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("digest mismatch called MCP: %#v", caller.calls)
	}

	goodArgs := map[string]any{"source": direct, "requestId": "create-1", "expectedSourceDigest": digest}
	if _, err := callStandaloneWhiteboardCreateResult(&cobra.Command{}, "", goodArgs); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls=%#v", caller.calls)
	}
	if _, exists := caller.calls[0].args["expectedSourceDigest"]; exists {
		t.Fatalf("CLI-only digest leaked to MCP args: %#v", caller.calls[0].args)
	}
}

// A matching render digest proves content identity, never human approval.
func TestCrossPlatformCoverageWhiteboardCreateRequiresUserConfirmation(t *testing.T) {
	source := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"title","type":"text","text":"课表"}]}`
	parsed, err := opennodes.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := opennodes.DigestSource(parsed)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, answer                            string
		yes, dry, mismatch, wantCall, wantError bool
		omitDigest                              bool
	}{
		{name: "no reply", wantError: true},
		{name: "declined", answer: "no\n", wantError: true},
		{name: "confirmed current SVG", answer: "yes\n", wantCall: true},
		{name: "explicit confirmation flag", yes: true, wantCall: true},
		{name: "legacy script without digest", yes: true, omitDigest: true, wantCall: true},
		{name: "without digest still needs confirmation", omitDigest: true, wantError: true},
		{name: "dry run needs no approval", dry: true},
		{name: "changed after confirmation", yes: true, mismatch: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &whiteboardTestCaller{format: "json", dry: tc.dry, response: func(whiteboardTestCall, int) string {
				return `{"success":true,"nodeId":"wb-new","requestId":"create-1","revision":"0"}`
			}}
			buf := installWhiteboardTestCaller(t, caller)
			cmd := newWhiteboardCommand()
			cmd.PersistentFlags().Bool("yes", false, "")
			cmd.PersistentFlags().Bool("dry-run", false, "")
			cmd.SetIn(strings.NewReader(tc.answer))
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			ctx, _ := outputpkg.WithResultStore(context.Background())
			cmd.SetContext(ctx)
			expected := digest
			if tc.mismatch {
				expected = "sha256:" + strings.Repeat("0", 64)
			}
			args := []string{"create-with-content", "--name", "课表", "--source", source, "--request-id", "create-1", "--expected-source-digest", expected}
			if tc.omitDigest {
				args = args[:len(args)-2]
			}
			if tc.yes {
				args = append(args, "--yes")
			}
			if tc.dry {
				args = append(args, "--dry-run")
			}
			cmd.SetArgs(args)
			err := cmd.Execute()
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%v", err, tc.wantError)
			}
			if (len(caller.calls) > 0) != tc.wantCall {
				t.Fatalf("calls=%#v", caller.calls)
			}
			if tc.name == "no reply" && !strings.Contains(err.Error(), "需要用户确认") {
				t.Fatalf("wrong failure: %v", err)
			}
		})
	}
}
