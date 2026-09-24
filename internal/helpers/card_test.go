// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/authoring"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/delivery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilectx"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type cardTestCall struct {
	product, tool string
	args          map[string]any
}
type cardTestCaller struct {
	calls  []cardTestCall
	dryRun bool
}

func (c *cardTestCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, cardTestCall{product: product, tool: tool, args: args})
	response := `{"success":true}`
	if tool == "create_and_send_a2ui_card" {
		response = `{"result":{"bizId":"biz-test-42","cardInstanceId":12345}}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}
func (*cardTestCaller) Format() string { return "json" }
func (c *cardTestCaller) DryRun() bool { return c.dryRun }
func (*cardTestCaller) Fields() string { return "" }
func (*cardTestCaller) JQ() string     { return "" }

func runCardCommand(t *testing.T, caller *cardTestCaller, args ...string) (string, error) {
	t.Helper()
	testseam.Swap(t, &cardCurrentProfileIdentity, func() profilectx.Identity {
		selector := ""
		for i, arg := range args {
			if arg == "--profile" && i+1 < len(args) {
				selector = strings.TrimSpace(args[i+1])
				break
			}
		}
		if selector == "profile-b" {
			return profilectx.Identity{CorpID: "corp-b", UserID: "user-b"}
		}
		return profilectx.Identity{CorpID: "corp-a", UserID: "user-a"}
	})
	return runCardCommandWithoutProfileSeam(t, caller, args...)
}

func runCardCommandWithoutProfileSeam(t *testing.T, caller *cardTestCaller, args ...string) (string, error) {
	t.Helper()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	InitDepsForTest(t, caller)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().String("fields", "", "")
	root.PersistentFlags().String("jq", "", "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().String("profile", "", "")
	root.PersistentPostRunE = func(cmd *cobra.Command, _ []string) error { _, _, err := output.EmitStoredResult(cmd); return err }
	root.AddCommand(newCardCommand())
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"card"}, args...))
	err := root.Execute()
	return stdout.String(), err
}

func writeCardMessages(t *testing.T) string {
	t.Helper()
	messages, err := authoring.Compile(authoring.Spec{Recipe: "form", SurfaceID: "test-surface", Title: "发布信息", Status: "待填写", Body: "请补充本次发布信息。", PrimaryCTA: "提交", Secondary: "取消"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "messages.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCrossPlatformCoverageCardOfflineComposeLintPreview(t *testing.T) {
	caller := &cardTestCaller{}
	messagePath := writeCardMessages(t)
	stdout, err := runCardCommand(t, caller, "lint", "--file", messagePath, "--mode", "create")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"valid": true`) {
		t.Fatalf("lint output=%s", stdout)
	}
	previewPath := filepath.Join(t.TempDir(), "preview.html")
	stdout, err = runCardCommand(t, caller, "preview", "--file", messagePath, "--output", previewPath)
	if err != nil {
		t.Fatal(err)
	}
	var previewResult struct {
		Data struct {
			Output      string `json:"output"`
			PreviewKind string `json:"previewKind"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &previewResult); err != nil {
		t.Fatalf("decode preview output: %v: %s", err, stdout)
	}
	if previewResult.Data.PreviewKind != "reference_preview" || previewResult.Data.Output != previewPath {
		t.Fatalf("preview output=%s", stdout)
	}
	raw, err := os.ReadFile(previewPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("reference_preview")) || !bytes.Contains(raw, []byte("发布信息")) {
		t.Fatalf("preview=%s", raw)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("offline commands made calls: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardComposeReportsInferredRecipe(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(specPath, []byte(`{"surfaceId":"inferred","title":"普通通知","body":"处理完成"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, err := runCardCommand(t, &cardTestCaller{}, "compose", "--file", specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"recipe": "notification"`) {
		t.Fatalf("compose output=%s", stdout)
	}
}

func TestCrossPlatformCoverageCardDoctorReportsInvokedBinaryCapabilities(t *testing.T) {
	stdout, err := runCardCommand(t, &cardTestCaller{}, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data struct {
			Binary       map[string]any `json:"binary"`
			Capabilities map[string]any `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Binary["pathAvailable"] != true || response.Data.Binary["path"] == "" {
		t.Fatalf("doctor binary identity is missing: %s", stdout)
	}
	if response.Data.Binary["cliVersion"] == "" {
		t.Fatalf("doctor CLI version is missing: %s", stdout)
	}
	for _, capability := range []string{"a2uiSend", "a2uiUpdate", "a2uiFinish", "a2uiSnapshot", "visualLint", "visualReview", "semanticBlocks"} {
		if response.Data.Capabilities[capability] != true {
			t.Errorf("doctor lacks %s capability: %s", capability, stdout)
		}
	}
}

func TestCrossPlatformCoverageCardGuideExposesInformationPlan(t *testing.T) {
	caller := &cardTestCaller{}
	for _, args := range [][]string{
		{"guide", "recommend", "--intent", "GitHub issue #1438 CLI timestamp mismatch"},
		{"guide", "rules"},
	} {
		stdout, err := runCardCommand(t, caller, args...)
		if err != nil {
			t.Fatalf("card %v: %v", args, err)
		}
		var response struct {
			Data struct {
				InformationPlan  authoring.InformationPlanGuidance `json:"informationPlan"`
				RecipeIsOptional bool                              `json:"recipeIsOptional"`
				RecipeMatch      bool                              `json:"recipeMatch"`
				SuggestedPath    string                            `json:"suggestedPath"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &response); err != nil {
			t.Fatalf("card %v: %v\n%s", args, err, stdout)
		}
		plan := response.Data.InformationPlan
		if len(plan.Roles) < 5 || len(plan.GroupingRules) == 0 || len(plan.VisualRules) == 0 || len(plan.Review) == 0 {
			t.Fatalf("card %v returned incomplete information plan: %+v", args, plan)
		}
		if args[1] == "recommend" && !response.Data.RecipeIsOptional {
			t.Fatalf("card %v made the Recipe mandatory", args)
		}
		if args[1] == "recommend" && response.Data.RecipeMatch {
			t.Fatalf("card %v treated an unknown intent as a Recipe match", args)
		}
		if args[1] == "recommend" && response.Data.SuggestedPath != "custom" {
			t.Fatalf("card %v suggested path=%q, want custom", args, response.Data.SuggestedPath)
		}
		if len(plan.Artifact.RequiredFields) != 7 || !slices.Contains(plan.Artifact.RequiredDecisions, "messageArchetype") || !slices.Contains(plan.Artifact.RequiredDecisions, "headerDecision") || len(plan.Artifact.PriorityValues) != 3 {
			t.Fatalf("card %v returned incomplete plan artifact: %+v", args, plan.Artifact)
		}
	}
	if len(caller.calls) != 0 {
		t.Fatalf("guide commands made remote calls: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardReviewIsBundledAndSendUsesSameVisualPreflight(t *testing.T) {
	path := writeCardMessages(t)
	caller := &cardTestCaller{}
	stdout, err := runCardCommand(t, caller, "review", "--file", path, "--design-archetype", "approval", "--minimum-validation-width", "240")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"status": "needs_client_review"`, `"staticStatus": "fail"`, `"code": "NOTIFICATION_WIDTH_TOO_NARROW"`, `"clientRenderingVerified": false`, `"authorChecks"`, `"clientChecks"`} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("review missing %s: %s", expected, stdout)
		}
	}
	if len(caller.calls) != 0 {
		t.Fatalf("offline review made remote calls: %+v", caller.calls)
	}
	stdout, err = runCardCommand(t, caller, "send", "--profile", "profile-a", "--open-dingtalk-id", helperCurrentDOpenID, "--file", path, "--design-archetype", "approval", "--minimum-validation-width", "240", "--yes")
	if err == nil && !strings.Contains(stdout, "NOTIFICATION_WIDTH_TOO_NARROW") {
		t.Fatalf("send accepted a deterministic visual error: %s", stdout)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("rejected send made remote calls: %+v", caller.calls)
	}
	caller.dryRun = true
	stdout, err = runCardCommand(t, caller, "send", "--profile", "profile-a", "--open-dingtalk-id", helperCurrentDOpenID, "--file", path, "--design-archetype", "approval", "--minimum-validation-width", "360", "--yes")
	if err != nil || !strings.Contains(stdout, `"visualReview"`) || !strings.Contains(stdout, `"clientRenderingVerified": false`) {
		t.Fatalf("dry-run lacks visual review: err=%v output=%s", err, stdout)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run made remote calls: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardGuideRecognizesNotification(t *testing.T) {
	stdout, err := runCardCommand(t, &cardTestCaller{}, "guide", "recommend", "--intent", "风险通知")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"recipeMatch": true`) || !strings.Contains(stdout, `"name": "notification"`) || !strings.Contains(stdout, `"suggestedPath": "review_recipe"`) {
		t.Fatalf("matched notification guidance=%s", stdout)
	}
}

func TestCrossPlatformCoverageLegacyA2UIChatCommandsPointToCardCommands(t *testing.T) {
	root := newChatCommand()
	for _, test := range []struct {
		path []string
		want string
	}{
		{path: []string{"message", "send-a2ui-card"}, want: "dws card send"},
		{path: []string{"message", "update-a2ui-card"}, want: "dws card update"},
	} {
		command, _, err := root.Find(test.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(command.Deprecated, test.want) {
			t.Fatalf("%v deprecated=%q, want %q", test.path, command.Deprecated, test.want)
		}
	}
}

func TestCrossPlatformCoverageCardLintFailureIsStructuredAndOffline(t *testing.T) {
	caller := &cardTestCaller{}
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`[{"version":"v1.0","createSurface":{"surfaceId":"s"}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, err := runCardCommand(t, caller, "lint", "--file", path, "--mode", "create")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"outcome": "failure"`, `"subtype": "a2ui_validation_failed"`, `"A2UI_CATALOG_REQUIRED"`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("lint output missing %s: %s", want, stdout)
		}
	}
	if len(caller.calls) != 0 {
		t.Fatalf("invalid lint made calls: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardLintEnforcesAcceptanceWidth(t *testing.T) {
	caller := &cardTestCaller{}
	stdout, err := runCardCommand(t, caller, "lint", "--file", writeCardMessages(t), "--design-archetype", "approval", "--minimum-validation-width", "240")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"outcome": "failure"`) || !strings.Contains(stdout, `"NOTIFICATION_WIDTH_TOO_NARROW"`) {
		t.Fatalf("lint did not reject the unsupported approval width: %s", stdout)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("visual lint made remote calls: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardSendUpdateUsesFakeIMAndLedger(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("DWS_CARD_STATE_DIR", stateDir)
	caller := &cardTestCaller{}
	messagePath := writeCardMessages(t)
	stdout, err := runCardCommand(t, caller, "send", "--profile", "test", "--conversation-id", "cid-test", "--file", messagePath, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct{ Handle, BizID string } `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode %s: %v", stdout, err)
	}
	if envelope.Data.Handle == "" || envelope.Data.BizID != "biz-test-42" {
		t.Fatalf("send output=%s", stdout)
	}
	if len(caller.calls) != 1 || caller.calls[0].product != "im" || caller.calls[0].tool != "create_and_send_a2ui_card" {
		t.Fatalf("calls=%+v", caller.calls)
	}
	wire, ok := caller.calls[0].args["a2uiMessages"].([]string)
	if !ok || len(wire) != 3 {
		t.Fatalf("wire=%#v", caller.calls[0].args["a2uiMessages"])
	}
	deltaPath := filepath.Join(t.TempDir(), "delta.json")
	delta := []map[string]any{{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "test-surface", "path": "/content/status", "value": "已完成"}}}
	raw, _ := json.Marshal(delta)
	if err := os.WriteFile(deltaPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, err = runCardCommand(t, caller, "finish", "--profile", "test", "--handle", envelope.Data.Handle, "--file", deltaPath, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"flowStatus": "FINISH"`) || !strings.Contains(stdout, `"revision": 2`) {
		t.Fatalf("finish output=%s", stdout)
	}
	if len(caller.calls) != 2 || caller.calls[1].tool != "update_a2ui_card" || caller.calls[1].args["bizId"] != "biz-test-42" {
		t.Fatalf("calls=%+v", caller.calls)
	}
	record, err := (delivery.Store{Dir: stateDir}).Load(envelope.Data.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != 2 || record.FlowStatus != "FINISH" || record.Profile.Selector != "corp-a:user-a" || record.Profile.CorpID != "corp-a" || record.Profile.UserID != "user-a" || record.Profile.Environment == "" {
		t.Fatalf("record=%+v", record)
	}
	keyHash := sha256.Sum256([]byte(caller.calls[0].args["bizCardId"].(string)))
	if record.CardInstanceID != "12345" || record.CreateRequestID != caller.calls[0].args["requestId"] || record.IdempotencyKeySHA256 != fmt.Sprintf("%x", keyHash) {
		t.Fatalf("create identity not retained in ledger: %+v", record)
	}
	stdout, err = runCardCommand(t, caller, "finish", "--profile", "test", "--handle", envelope.Data.Handle, "--flow-status", "ABORTED", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"flowStatus": "ABORTED"`) || len(caller.calls) != 3 {
		t.Fatalf("status-only finish output=%s calls=%+v", stdout, caller.calls)
	}
	if wire, ok := caller.calls[2].args["a2uiMessages"].([]string); !ok || len(wire) != 0 {
		t.Fatalf("status-only wire=%#v", caller.calls[2].args["a2uiMessages"])
	}
}

func TestCrossPlatformCoverageCardWritesRequireConfirmation(t *testing.T) {
	t.Setenv("DWS_CARD_STATE_DIR", t.TempDir())
	caller := &cardTestCaller{}
	_, err := runCardCommand(t, caller, "send", "--profile", "test", "--conversation-id", "cid-test", "--file", writeCardMessages(t))
	if err == nil || !strings.Contains(err.Error(), "confirmation") && !strings.Contains(err.Error(), "确认") {
		t.Fatalf("error=%v, want confirmation gate", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote calls before confirmation=%+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardSendRejectsPreviewOnlyImageURL(t *testing.T) {
	t.Setenv("DWS_CARD_STATE_DIR", t.TempDir())
	messages, err := authoring.Compile(authoring.Spec{
		Recipe: "information", SurfaceID: "preview-only-image", Title: "Image delivery gate",
		Body: "body", ImageURL: "data:image/svg+xml;base64,PHN2Zy8+",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "preview-only-image.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &cardTestCaller{}
	_, err = runCardCommand(t, caller, "send", "--profile", "test", "--conversation-id", "cid-test", "--file", path, "--yes")
	if err == nil || !strings.Contains(err.Error(), "absolute HTTPS URL") {
		t.Fatalf("error=%v, want delivery resource rejection", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("preview-only image reached remote: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardUpdateRejectsDifferentProfile(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("DWS_CARD_STATE_DIR", stateDir)
	caller := &cardTestCaller{}
	stdout, err := runCardCommand(t, caller, "send", "--profile", "profile-a", "--conversation-id", "cid-test", "--file", writeCardMessages(t), "--yes")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct{ Handle string } `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	deltaPath := filepath.Join(t.TempDir(), "delta.json")
	if err := os.WriteFile(deltaPath, []byte(`[{"version":"v1.0","updateDataModel":{"surfaceId":"test-surface","path":"/content/status","value":"done"}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runCardCommand(t, caller, "update", "--profile", "profile-b", "--handle", envelope.Data.Handle, "--file", deltaPath, "--yes")
	if err == nil || !strings.Contains(err.Error(), "different profile") {
		t.Fatalf("error=%v, want scope rejection", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("cross-profile update reached remote: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardSendRequiresResolvedExactIdentity(t *testing.T) {
	t.Setenv("DWS_CARD_STATE_DIR", t.TempDir())
	testseam.Swap(t, &cardCurrentProfileIdentity, func() profilectx.Identity { return profilectx.Identity{} })
	caller := &cardTestCaller{}
	_, err := runCardCommandWithoutProfileSeam(t, caller, "send", "--profile", "ambiguous", "--conversation-id", "cid-test", "--file", writeCardMessages(t), "--yes")
	if err == nil || !strings.Contains(err.Error(), "exact corpId:userId") {
		t.Fatalf("error=%v, want exact identity rejection", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unresolved profile reached remote: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardSendResolvesIdentityBeforeRunner(t *testing.T) {
	t.Setenv("DWS_CARD_STATE_DIR", t.TempDir())
	testseam.Swap(t, &cardCurrentProfileIdentity, func() profilectx.Identity { return profilectx.Identity{} })
	testseam.Swap(t, &cardResolveProfileIdentity, func(selector string) (profilectx.Identity, error) {
		if selector != "corp-a:user-a" {
			t.Fatalf("selector = %q, want corp-a:user-a", selector)
		}
		return profilectx.Identity{CorpID: "corp-a", UserID: "user-a"}, nil
	})
	caller := &cardTestCaller{dryRun: true}
	stdout, err := runCardCommandWithoutProfileSeam(t, caller, "send", "--profile", "corp-a:user-a", "--conversation-id", "cid-test", "--file", writeCardMessages(t), "--dry-run")
	if err != nil {
		t.Fatalf("send dry-run error = %v", err)
	}
	if !strings.Contains(stdout, `"dryRun": true`) {
		t.Fatalf("stdout = %s, want dry-run receipt", stdout)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run reached remote: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardUpdateRejectsInvalidFinalSurface(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("DWS_CARD_STATE_DIR", stateDir)
	caller := &cardTestCaller{}
	stdout, err := runCardCommand(t, caller, "send", "--profile", "test", "--conversation-id", "cid-test", "--file", writeCardMessages(t), "--yes")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct{ Handle string } `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	deltaPath := filepath.Join(t.TempDir(), "invalid-delta.json")
	delta := `[{
  "version":"v1.0",
  "updateComponents":{"surfaceId":"test-surface","components":[{"id":"root","component":"Card","child":"missing"}]}
}]`
	if err := os.WriteFile(deltaPath, []byte(delta), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runCardCommand(t, caller, "update", "--profile", "test", "--handle", envelope.Data.Handle, "--file", deltaPath, "--yes")
	if err == nil || !strings.Contains(err.Error(), "invalid surface") {
		t.Fatalf("error=%v, want merged surface rejection", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("invalid final surface reached remote: %+v", caller.calls)
	}
}

func TestCrossPlatformCoverageCardSnapshotPreservesUserInput(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("DWS_CARD_STATE_DIR", stateDir)
	caller := &cardTestCaller{}
	stdout, err := runCardCommand(t, caller, "send", "--profile", "test", "--conversation-id", "cid-test", "--file", writeCardMessages(t), "--yes")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct{ Handle string } `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	store := delivery.Store{Dir: stateDir}
	record, err := store.Load(envelope.Data.Handle)
	if err != nil {
		t.Fatal(err)
	}
	rawData, _ := json.Marshal(record.Surface.Data)
	var desiredData map[string]any
	if err := json.Unmarshal(rawData, &desiredData); err != nil {
		t.Fatal(err)
	}
	desiredData["content"].(map[string]any)["status"] = "已完成"
	desiredData["form"].(map[string]any)["comment"] = "must not overwrite user input"
	snapshot := map[string]any{"version": "v1.0", "surfaceId": record.Surface.SurfaceID, "catalogId": record.Surface.CatalogID, "components": record.Surface.Components, "dataModel": desiredData}
	snapshotRaw, _ := json.Marshal(snapshot)
	snapshotPath := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(snapshotPath, snapshotRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runCardCommand(t, caller, "update", "--profile", "test", "--handle", envelope.Data.Handle, "--file", snapshotPath, "--input-mode", "snapshot", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls=%+v", caller.calls)
	}
	wire, ok := caller.calls[1].args["a2uiMessages"].([]string)
	if !ok || len(wire) != 1 || !strings.Contains(wire[0], "/content/status") || strings.Contains(strings.Join(wire, "\n"), "/form") {
		t.Fatalf("snapshot wire=%#v", caller.calls[1].args["a2uiMessages"])
	}
	updated, err := store.Load(envelope.Data.Handle)
	if err != nil {
		t.Fatal(err)
	}
	data := updated.Surface.Data.(map[string]any)
	if got := data["form"].(map[string]any)["comment"]; got == "must not overwrite user input" {
		t.Fatalf("snapshot overwrote protected input: %#v", got)
	}
}

func TestCrossPlatformCoverageCardSendDryRunDoesNotCallOrPersist(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("DWS_CARD_STATE_DIR", stateDir)
	caller := &cardTestCaller{dryRun: true}
	stdout, err := runCardCommand(t, caller, "send", "--profile", "test", "--conversation-id", "cid-test", "--file", writeCardMessages(t), "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 || !strings.Contains(stdout, `"dry_run": true`) {
		t.Fatalf("calls=%+v output=%s", caller.calls, stdout)
	}
	records, err := (delivery.Store{Dir: stateDir}).List()
	if err != nil || len(records) != 0 {
		t.Fatalf("records=%+v err=%v", records, err)
	}
}

func TestCrossPlatformCoverageCardSendResolvesQueryTargets(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("DWS_CARD_STATE_DIR", stateDir)
	messagePath := writeCardMessages(t)

	t.Run("chat query", func(t *testing.T) {
		caller := &cardTestCaller{dryRun: true}
		stdout, err := runCardCommand(t, caller, "send", "--profile", "test", "--chat-query", "cid123456789", "--file", messagePath, "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, `"openConversationId": "cid123456789"`) || len(caller.calls) != 0 {
			t.Fatalf("output=%s calls=%+v", stdout, caller.calls)
		}
	})

	t.Run("user query", func(t *testing.T) {
		caller := &cardTestCaller{dryRun: true}
		stdout, err := runCardCommand(t, caller, "send", "--profile", "test", "--user-query", helperCurrentDOpenID, "--file", messagePath, "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, `"receiverOpenDingTalkId": "`+helperCurrentDOpenID+`"`) || len(caller.calls) != 0 {
			t.Fatalf("output=%s calls=%+v", stdout, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageCardUserQueryHelpMatchesExactResolver(t *testing.T) {
	flag := newCardSendCommand().Flags().Lookup("user-query")
	if flag == nil {
		t.Fatal("missing --user-query flag")
	}
	for _, want := range []string{"精确", "userId", "openDingTalkId", "不执行姓名模糊搜索"} {
		if !strings.Contains(flag.Usage, want) {
			t.Fatalf("--user-query help %q missing %q", flag.Usage, want)
		}
	}
}
