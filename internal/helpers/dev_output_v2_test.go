package helpers

import (
	"bytes"
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

type countingDevV2Runner struct {
	calls int
}

func (r *countingDevV2Runner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls++
	invocation.Implemented = true
	return executor.Result{
		Invocation: invocation,
		Response: map[string]any{"content": map[string]any{
			"success": true,
			"result":  map[string]any{"id": "dev-1"},
		}},
	}, nil
}

func newDevV2Root(runner executor.Runner) *cobra.Command {
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

func TestDevAppV2ActiveExecutesOnceAndReturnsFrameworkResult(t *testing.T) {
	runner := &countingDevV2Runner{}
	root := newDevV2Root(runner)
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

func TestMigratedDevAppDefaultsToFrameworkV2(t *testing.T) {
	runner := &countingDevV2Runner{}
	root := newDevV2Root(runner)
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
		t.Fatalf("migrated dev command did not use v2 by default: %s", stdout.String())
	}
}

func TestDevDocSearchV2UsesInjectedRunnerOnce(t *testing.T) {
	runner := &countingDevV2Runner{}
	root := newDevV2Root(runner)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"dev", "doc", "search", "MCP"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d, want 1", runner.calls)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"outcome": "success"`)) || bytes.Contains(stdout.Bytes(), []byte(`"contract_version"`)) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestAllTerminalDevBusinessSurfacesDeclareV2Active(t *testing.T) {
	root := newDevV2Root(&countingDevV2Runner{})
	dev, _, err := root.Find([]string{"dev"})
	if err != nil {
		t.Fatal(err)
	}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		children := cmd.Commands()
		if cmd.Runnable() && len(children) == 0 {
			if got := output.CommandRollout(cmd); got != output.RolloutV2Active {
				t.Errorf("%s rollout=%s, want %s", cmd.CommandPath(), got, output.RolloutV2Active)
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
