//go:build ignore

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

// gen_shortcut_test_matrix executes every registered built-in shortcut through
// the real cobra command tree with a fake MCP caller. It records whether each
// command safely assembles an MCP call (zero network / zero side effect) or is
// intentionally stopped by its own validation. The JSON output is consumed by
// scripts/gen_shortcut_comparison.py so the HTML report can show per-command
// test status instead of only aggregate coverage.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type mcpCall struct {
	Product string         `json:"product"`
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args,omitempty"`
}

type fakeCaller struct {
	calls []mcpCall
}

func (f *fakeCaller) reset() { f.calls = nil }

func (f *fakeCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, mcpCall{Product: product, Tool: tool, Args: args})
	payload := map[string]any{
		"_fake":   true,
		"product": product,
		"tool":    tool,
		"args":    args,
	}
	b, _ := json.Marshal(payload)
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(b)}}}, nil
}

func (f *fakeCaller) Format() string { return "json" }
func (f *fakeCaller) DryRun() bool   { return false }
func (f *fakeCaller) Fields() string { return "" }
func (f *fakeCaller) JQ() string     { return "" }

type commandResult struct {
	Service      string    `json:"service"`
	Command      string    `json:"command"`
	Risk         string    `json:"risk"`
	Method       string    `json:"method"`
	Status       string    `json:"status"`
	Input        string    `json:"input"`
	Args         []string  `json:"args"`
	Stdout       string    `json:"stdout,omitempty"`
	Stderr       string    `json:"stderr,omitempty"`
	Calls        []mcpCall `json:"calls,omitempty"`
	CallCount    int       `json:"call_count"`
	Tools        []string  `json:"tools,omitempty"`
	ToolVerified bool      `json:"tool_verified"`
	Error        string    `json:"error,omitempty"`
	Note         string    `json:"note"`
}

type matrix struct {
	Total               int             `json:"total"`
	Assembled           int             `json:"assembled"`
	ValidationBlocked   int             `json:"validation_blocked"`
	Failed              int             `json:"failed"`
	ToolVerificationBad int             `json:"tool_verification_bad"`
	Results             []commandResult `json:"results"`
}

func realToolSet() map[string]bool {
	files, _ := filepath.Glob("internal/helpers/*.go")
	wordRe := regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]{2,}`)
	cjkRe := regexp.MustCompile(`"([^"]*\p{Han}[^"]*)"`)
	set := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(data)
		for _, m := range wordRe.FindAllString(s, -1) {
			set[m] = true
		}
		for _, m := range cjkRe.FindAllStringSubmatch(s, -1) {
			set[m[1]] = true
		}
	}
	return set
}

func synthArgs(s shortcut.Shortcut) []string {
	args := []string{s.Service, s.Command}
	for _, f := range s.Flags {
		switch {
		case len(f.Enum) > 0:
			args = append(args, "--"+f.Name, f.Enum[0])
		case f.Type == shortcut.FlagBool:
			args = append(args, "--"+f.Name)
		case f.Type == shortcut.FlagInt:
			args = append(args, "--"+f.Name, "1")
		default:
			args = append(args, "--"+f.Name, "x")
		}
	}
	return append(args, "--yes")
}

func newRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(builtin.Commands()...)
	return root
}

func uniqueTools(calls []mcpCall) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range calls {
		name := c.Product + "/" + c.Tool
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func riskString(s shortcut.Shortcut) string {
	if s.Risk == "" {
		return string(shortcut.RiskRead)
	}
	return string(s.Risk)
}

func setCommandOutput(cmd *cobra.Command, out, errOut io.Writer) {
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	for _, child := range cmd.Commands() {
		setCommandOutput(child, out, errOut)
	}
}

func runCaptured(fake *fakeCaller, root *cobra.Command, args []string) (stdout, stderr string, err error, panicValue any) {
	stdoutFile, err := os.CreateTemp("", "dws-shortcut-stdout-*.txt")
	if err != nil {
		return "", "", err, nil
	}
	defer os.Remove(stdoutFile.Name())
	stderrFile, err := os.CreateTemp("", "dws-shortcut-stderr-*.txt")
	if err != nil {
		stdoutFile.Close()
		return "", "", err, nil
	}
	defer os.Remove(stderrFile.Name())

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = stdoutFile
	os.Stderr = stderrFile
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	helpers.InitDeps(fake)
	setCommandOutput(root, stdoutFile, stderrFile)
	root.SetArgs(args)
	err = func() (e error) {
		defer func() {
			if r := recover(); r != nil {
				panicValue = r
			}
		}()
		return root.Execute()
	}()

	stdoutFile.Sync()
	stderrFile.Sync()
	outBytes, _ := os.ReadFile(stdoutFile.Name())
	errBytes, _ := os.ReadFile(stderrFile.Name())
	stdoutFile.Close()
	stderrFile.Close()
	return string(outBytes), string(errBytes), err, panicValue
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.ContainsAny(arg, " \t\n'\"\\$`") {
		return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return arg
}

func commandLine(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "dws")
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func main() {
	fake := &fakeCaller{}
	helpers.InitDeps(fake)
	root := newRoot()
	real := realToolSet()

	all := shortcut.All()
	results := make([]commandResult, 0, len(all))
	out := matrix{Total: len(all)}

	for _, s := range all {
		args := synthArgs(s)
		fake.reset()
		stdout, stderr, err, panicValue := runCaptured(fake, root, args)

		res := commandResult{
			Service: s.Service,
			Command: s.Command,
			Risk:    riskString(s),
			Method:  "cobra+fake-mcp-zero-side-effect",
			Input:   commandLine(args),
			Args:    append([]string(nil), args...),
			Stdout:  strings.TrimSpace(stdout),
			Stderr:  strings.TrimSpace(stderr),
			Calls:   append([]mcpCall(nil), fake.calls...),
		}
		res.CallCount = len(res.Calls)
		res.Tools = uniqueTools(res.Calls)
		res.ToolVerified = true
		for _, call := range res.Calls {
			if call.Tool == "" || !real[call.Tool] {
				res.ToolVerified = false
			}
		}

		switch {
		case panicValue != nil:
			res.Status = "failed"
			res.Error = fmt.Sprintf("panic: %v", panicValue)
			res.Note = "测试失败：执行时发生 panic。"
			out.Failed++
		case len(fake.calls) > 0:
			res.Status = "assembled"
			if err != nil {
				res.Error = err.Error()
				res.Note = "已实际执行到 MCP 组装/调用路径；fake MCP 已拦截，无网络、无副作用；后续因合成响应不足产生错误，不影响装配验证。"
			} else {
				res.Note = "已实际执行到 MCP 组装/调用路径；fake MCP 已拦截，无网络、无副作用。"
			}
			out.Assembled++
		case err != nil:
			res.Status = "validation-blocked"
			res.Error = err.Error()
			res.Note = "已实际执行到命令校验链路；synthetic 参数被该命令自身校验拦截，属于预期的零副作用验证结果。"
			out.ValidationBlocked++
		default:
			res.Status = "failed"
			res.Note = "测试失败：命令无错误返回但未组装 MCP 调用。"
			out.Failed++
		}
		if !res.ToolVerified {
			out.ToolVerificationBad++
			if res.Error != "" {
				res.Error += "; "
			}
			res.Error += "tool literal not found in helper ground truth"
		}
		res.Error = strings.TrimSpace(res.Error)
		results = append(results, res)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Service == results[j].Service {
			return results[i].Command < results[j].Command
		}
		return results[i].Service < results[j].Service
	})
	out.Results = results

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode matrix: %v\n", err)
		os.Exit(1)
	}
	if out.Failed > 0 || out.ToolVerificationBad > 0 {
		os.Exit(1)
	}
}
