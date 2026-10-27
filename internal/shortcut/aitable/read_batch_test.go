// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type independentReadCaller struct {
	mu                       sync.Mutex
	entered, active, maximum int
	release                  chan struct{}
}

func (c *independentReadCaller) CallTool(ctx context.Context, _, _ string, args map[string]any) (*edition.ToolResult, error) {
	c.mu.Lock()
	c.entered++
	c.active++
	c.maximum = max(c.maximum, c.active)
	if c.entered == 3 {
		close(c.release)
	}
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.active--; c.mu.Unlock() }()
	select {
	case <-c.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	raw, _ := json.Marshal(map[string]any{"status": "success", "data": args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(raw)}}}, nil
}
func (*independentReadCaller) Format() string { return "json" }
func (*independentReadCaller) DryRun() bool   { return false }
func (*independentReadCaller) Fields() string { return "" }
func (*independentReadCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageReadBatchConcurrentReadsRetainInputOrder(t *testing.T) {
	caller := &independentReadCaller{release: make(chan struct{})}
	helpers.InitDepsForTest(t, caller)
	stdout := &bytes.Buffer{}
	helpers.GetFormatter().SetWriters(stdout, &bytes.Buffer{})
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx, _ = output.WithResultStore(ctx)
	root.SetContext(ctx)
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"aitable", "+read-batch", "--base-id", "B", "--concurrency", "3", "--requests", `[{"tool":"get_chart","arguments":{"baseId":"B","dashboardId":"D","chartId":"first"}},{"tool":"get_chart","arguments":{"baseId":"B","dashboardId":"D","chartId":"second"}},{"tool":"get_chart","arguments":{"baseId":"B","dashboardId":"D","chartId":"third"}}]`})
	cmd, err := corecmd.ExecuteCForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = output.EmitStoredResult(cmd); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Data struct {
			Items []struct {
				Result struct {
					Data struct {
						ChartID string `json:"chartId"`
					} `json:"data"`
				} `json:"result"`
			} `json:"items"`
		} `json:"data"`
	}
	if err = json.Unmarshal(stdout.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Data.Items) != 3 || caller.maximum != 3 || caller.entered != 3 {
		t.Fatalf("concurrency/result: %d/%d/%d", caller.maximum, caller.entered, len(wire.Data.Items))
	}
	for i, want := range []string{"first", "second", "third"} {
		if wire.Data.Items[i].Result.Data.ChartID != want {
			t.Fatalf("result %d changed input order: %s", i, stdout.String())
		}
	}
}
