package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

type countingDevUnifiedRunner struct {
	calls int
}

func (r *countingDevUnifiedRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls++
	invocation.Implemented = true
	return executor.Result{
		Invocation: invocation,
		Response: map[string]any{"content": map[string]any{
			"success": true,
			"result":  map[string]any{"id": "dev-1", "hasMore": false},
		}},
	}, nil
}

func newDevUnifiedRoot(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().String("fields", "", "")
	root.PersistentFlags().String("jq", "", "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentPostRunE = func(cmd *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(cmd)
		return err
	}
	root.AddCommand(devHandler{}.Command(runner))
	return root
}

func TestDevAppUnifiedActiveExecutesOnceAndReturnsFrameworkResult(t *testing.T) {
	runner := &countingDevUnifiedRunner{}
	root := newDevUnifiedRoot(runner)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"dev", "app", "list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d, want exactly 1", runner.calls)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"outcome": "success"`)) || !bytes.Contains(stdout.Bytes(), []byte(`"id": "dev-1"`)) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestMigratedDevAppDefaultsToUnifiedFramework(t *testing.T) {
	runner := &countingDevUnifiedRunner{}
	root := newDevUnifiedRoot(runner)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"dev", "app", "list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d, want 1", runner.calls)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"outcome": "success"`)) || bytes.Contains(stdout.Bytes(), []byte(`"contract_version"`)) {
		t.Fatalf("migrated dev command did not use unified output by default: %s", stdout.String())
	}
}

func TestDevDocSearchStaysLegacyUntilPagePaginationContractExists(t *testing.T) {
	cmd := newDevDocSearchCommand(&countingDevUnifiedRunner{})
	if got := output.CommandRollout(cmd); got != output.RolloutLegacyOnly {
		t.Fatalf("dev doc search rollout=%s, want legacy_only until page pagination is modeled", got)
	}
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok {
		t.Fatal("dev doc search is missing ContractFinal")
	}
	if final.Result != nil || final.Pagination != nil {
		t.Fatalf("legacy dev doc search must not publish unified result/pagination schema: %#v", final)
	}
}

func TestDevTerminalRolloutKeepsPublishedConnectStatusLegacy(t *testing.T) {
	root := newDevUnifiedRoot(&countingDevUnifiedRunner{})
	dev, _, err := root.Find([]string{"dev"})
	if err != nil {
		t.Fatal(err)
	}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		children := cmd.Commands()
		if cmd.Runnable() && len(children) == 0 {
			want := output.RolloutUnifiedActive
			switch cmd.CommandPath() {
			case "dws dev connect status", "dws dev connect list", "dws dev doc search":
				want = output.RolloutLegacyOnly
			}
			if got := output.CommandRollout(cmd); got != want {
				t.Errorf("%s rollout=%s, want %s", cmd.CommandPath(), got, want)
			}
		}
		if cmd.CommandPath() == "dws dev connect" && output.CommandRollout(cmd) != output.RolloutLegacyOnly {
			t.Errorf("%s must remain legacy until a streaming contract exists", cmd.CommandPath())
		}
		for _, child := range children {
			walk(child)
		}
	}
	walk(dev)
}

func TestDevConnectStatusPreservesPublishedHumanAndJSONShapes(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	humanRoot := newDevUnifiedRoot(&countingDevUnifiedRunner{})
	var humanOut bytes.Buffer
	humanRoot.SetOut(&humanOut)
	humanRoot.SetArgs([]string{"dev", "connect", "status", "--robot-client-id", "missing"})
	if err := humanRoot.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(humanOut.Bytes(), []byte("not_running")) || bytes.Contains(humanOut.Bytes(), []byte(`"outcome"`)) {
		t.Fatalf("default status output no longer uses the legacy human view: %s", humanOut.String())
	}

	jsonRoot := newDevUnifiedRoot(&countingDevUnifiedRunner{})
	var jsonOut bytes.Buffer
	jsonRoot.SetOut(&jsonOut)
	jsonRoot.SetArgs([]string{"dev", "connect", "status", "--robot-client-id", "missing", "--json"})
	if err := jsonRoot.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatalf("legacy --json output is invalid: %v\n%s", err, jsonOut.String())
	}
	if payload["state"] != "not_running" || payload["supervised"] != false {
		t.Fatalf("legacy health fields moved or changed: %#v", payload)
	}
	for _, forbidden := range []string{"ok", "outcome", "data"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("legacy --json unexpectedly gained unified key %q: %#v", forbidden, payload)
		}
	}
}
