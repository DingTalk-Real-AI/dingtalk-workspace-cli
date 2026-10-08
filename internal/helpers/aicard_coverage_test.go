// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAicardCommandFailureBranches(t *testing.T) {
	file := aicardTestFile(t, aicardTestSnapshot)
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{"invalid preflight mode", []string{"lint", "--file", file, "--preflight", "other"}, "Preflight requires"},
		{"preflight with fragment", []string{"lint", "--file", file, "--preflight", "resources", "--fragment"}, "Preflight requires"},
		{"unreadable lint file", []string{"lint", "--file", file + ".missing"}, "Cannot read file"},
		{"unknown batch name", []string{"explain", "Text", "unknown-name"}, "At least one name"},
		{"unknown compact batch name", []string{"explain", "Text", "unknown-name", "--compact"}, "At least one name"},
		{"missing preview file", []string{"preview", "--file", " "}, "--file is required"},
		{"empty preview summary", []string{"preview", "--file", file, "--summary", " "}, "--summary must not be empty"},
		{"unreadable preview file", []string{"preview", "--file", file + ".missing"}, "card.json.missing"},
		{"invalid preview structure", []string{"preview", "--file", aicardTestFile(t, `[{"version":"v1.0","createSurface":{"surfaceId":1}}]`)}, "structure is invalid"},
		{"missing card data", []string{"preview", "--file", aicardTestFile(t, strings.Replace(aicardTestSnapshot, `,"dataModel":{}`, "", 1))}, "delivery.data_model_missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, code, err := executeAicard(t, &aicardCaller{}, tc.args...)
			if err != nil || code == 0 || !strings.Contains(result["error"].(map[string]any)["message"].(string), tc.message) {
				t.Fatalf("%v: code=%d err=%v result=%v", tc.args, code, err, result)
			}
		})
	}
}

func TestCrossPlatformCoverageAicardCommandInjectedFailures(t *testing.T) {
	file := aicardTestFile(t, aicardTestSnapshot)
	t.Run("protocol load", func(t *testing.T) {
		testseam.Swap(t, &aicardLoadProtocol, func() (*a2ui.Protocol, error) { return nil, errors.New("damaged embedded protocol") })
		for _, args := range [][]string{{"lint", "--self-check"}, {"preview", "--file", file}} {
			result, code, err := executeAicard(t, &aicardCaller{}, args...)
			if err != nil || code == 0 || !strings.Contains(result["error"].(map[string]any)["message"].(string), "damaged embedded protocol") {
				t.Fatalf("%v: code=%d err=%v result=%v", args, code, err, result)
			}
		}
		result, code, err := executeAicard(t, &aicardCaller{}, "explain", "Text")
		if err != nil || code != 0 || result["ok"] != true {
			t.Fatalf("explain must not initialize the validator: code=%d err=%v result=%v", code, err, result)
		}
	})
	t.Run("explain load", func(t *testing.T) {
		testseam.Swap(t, &aicardLoadExplain, func() (*a2ui.ExplainStore, error) { return nil, errors.New("damaged explain bundle") })
		result, code, err := executeAicard(t, &aicardCaller{}, "explain", "Text")
		if err != nil || code == 0 || !strings.Contains(result["error"].(map[string]any)["message"].(string), "damaged explain bundle") {
			t.Fatalf("code=%d err=%v result=%v", code, err, result)
		}
	})
	t.Run("lint failure", func(t *testing.T) {
		testseam.Swap(t, &aicardLint, func(*a2ui.Protocol, []byte, bool, bool) (a2ui.Report, error) {
			return a2ui.Report{}, errors.New("validator failed")
		})
		for _, args := range [][]string{{"lint", "--file", file}, {"preview", "--file", file}} {
			result, code, err := executeAicard(t, &aicardCaller{}, args...)
			if err != nil || code == 0 || !strings.Contains(result["error"].(map[string]any)["message"].(string), "validator failed") {
				t.Fatalf("%v: code=%d err=%v result=%v", args, code, err, result)
			}
		}
	})
	t.Run("preflight failure", func(t *testing.T) {
		testseam.Swap(t, &aicardPreflight, func(*a2ui.Protocol, []string, string) (map[string]any, error) {
			return nil, errors.New("preflight input failed")
		})
		for _, args := range [][]string{{"lint", "--file", file, "--preflight", "new-card"}, {"preview", "--file", file, "--dry-run"}} {
			caller := &aicardCaller{}
			result, code, err := executeAicard(t, caller, args...)
			if err != nil || code == 0 || !strings.Contains(result["error"].(map[string]any)["message"].(string), "preflight input failed") || len(caller.calls) != 0 {
				t.Fatalf("code=%d err=%v result=%v calls=%v", code, err, result, caller.calls)
			}
		}
	})
	if err := aicardReport(&cobra.Command{}, a2ui.Report{Metrics: map[string]any{"bad": make(chan int)}}); err == nil {
		t.Fatal("serialization failure must reach caller")
	}
}

func TestCrossPlatformCoverageAicardPreviewIdentityAndReceiptFailures(t *testing.T) {
	file := aicardTestFile(t, aicardTestSnapshot)
	for _, tc := range []struct {
		name      string
		caller    *aicardCaller
		wantCalls int
		wantError string
	}{
		{"profile lookup", &aicardCaller{profileErr: errors.New("profile unavailable")}, 1, "profile unavailable"},
		{"recipient resolution", &aicardCaller{userID: "employee-1", resolveErr: errors.New("lookup unavailable")}, 3, "cannot resolve userId"},
		{"receipt is not an object", &aicardCaller{response: `[]`}, 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, code, err := executeAicard(t, tc.caller, "preview", "--file", file)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || code != -1 || result != nil {
					t.Fatalf("want error %q: code=%d err=%v result=%+v", tc.wantError, code, err, result)
				}
			} else {
				if err != nil || code == 0 || result["ok"] != false {
					t.Fatalf("want structured failure: code=%d err=%v result=%+v", code, err, result)
				}
				failure, ok := result["error"].(map[string]any)
				message, hasMessage := failure["message"].(string)
				if !ok || !hasMessage || !strings.Contains(message, "not an object") {
					t.Fatalf("want non-object receipt failure: %+v", result)
				}
			}
			if len(tc.caller.calls) != tc.wantCalls {
				t.Fatalf("calls=%v, want %d", tc.caller.calls, tc.wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageAicardPublicFactory(t *testing.T) {
	registryMu.Lock()
	factories := append([]registeredFactory(nil), publicFactories...)
	registryMu.Unlock()
	found := 0
	for _, registered := range factories {
		if handler := registered.factory(); handler.Name() == "aicard" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("aicard public factory count = %d, want 1", found)
	}
}

func TestCrossPlatformCoverageAicardExplainCorruptContracts(t *testing.T) {
	raw := `{"formatVersion":1,"manifestSha256":"` + strings.Repeat("0", 64) + `","entries":{"Text":{"kind":"component","definition":{"name":"wrong","kind":"component"}}}}`
	store, err := a2ui.NewExplainStore(fstest.MapFS{"explain.json": &fstest.MapFile{Data: []byte(raw)}})
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &aicardLoadExplain, func() (*a2ui.ExplainStore, error) { return store, nil })
	for _, args := range [][]string{
		{"explain", "Text"},
		{"explain", "Text", "--compact"},
		{"explain", "Text", "Row"},
		{"explain", "Text", "Row", "--compact"},
	} {
		result, code, err := executeAicard(t, &aicardCaller{}, args...)
		if err != nil || code == 0 || !strings.Contains(result["error"].(map[string]any)["message"].(string), "Cannot read an embedded explain contract") {
			t.Fatalf("%v: code=%d err=%v result=%v", args, code, err, result)
		}
	}
}

func TestCrossPlatformCoverageAicardSelfCheckRejectsMissingAssets(t *testing.T) {
	testseam.Swap(t, &aicardLoadProtocol, func() (*a2ui.Protocol, error) { return &a2ui.Protocol{}, nil })
	result, code, err := executeAicard(t, &aicardCaller{}, "lint", "--self-check")
	if err != nil || code == 0 || !strings.Contains(result["error"].(map[string]any)["message"].(string), "Cannot verify the embedded explain contracts") {
		t.Fatalf("code=%d err=%v result=%v", code, err, result)
	}
}

func TestCrossPlatformCoverageAicardWholeContractReferences(t *testing.T) {
	result, code, err := executeAicard(t, &aicardCaller{}, "explain", "Text", "text", "--compact")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v result=%v", code, err, result)
	}
	bundle := result["data"].(map[string]any)
	for _, item := range bundle["contracts"].([]any) {
		ref, ok := item.(map[string]any)["$contractRef"].(string)
		if !ok || bundle["definitions"].(map[string]any)[ref].(map[string]any)["name"] != "Text" {
			t.Fatalf("expected whole Text contract reference: %v", item)
		}
	}
	// Distinct raw names normalize to one long unknown contract and are compacted.
	unknown := strings.Repeat("unknown", 50)
	result, code, err = executeAicard(t, &aicardCaller{}, "explain", unknown, " "+unknown, "--compact")
	if err != nil || code == 0 || !strings.Contains(result["error"].(map[string]any)["message"].(string), "At least one name") {
		t.Fatalf("unknown reference accepted: code=%d err=%v result=%v", code, err, result)
	}
}
