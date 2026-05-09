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

// Package compat — pipeline executor for CLIToolOverride.Pipeline.
//
// A pipeline turns a single CLI command into an ordered sequence of MCP
// tool calls plus optional HTTP-download sinks, declared entirely in the
// envelope JSON. Use cases:
//
//  1. submit-job + poll-status + download-result patterns (the canonical
//     example: `dws sheet export --node X --output PATH` calls
//     submit_export_job → query_export_job (poll until status=done) →
//     HTTP GET downloadUrl → write to PATH).
//  2. compose-then-update flows where step 2's args reference step 1's
//     response.
//
// Templates supported in PipelineStep.Args / DownloadURLField:
//
//	$flag.<aliasName>      — value of the user's CLI flag whose alias
//	                         equals <aliasName>
//	$step.<idx>.<dotPath>  — field from a prior step's response
//	literal string         — passed through unchanged
//
// Limitations (intentional, to keep the executor small):
//   - No conditional branching: steps run unconditionally in order.
//   - No retry-on-error: the pipeline aborts on the first runner error.
//   - PollUntil compares as strings; numeric/boolean comparisons stringify.
//   - Download step uses the standard library net/http with no custom
//     timeout (relies on the user's Ctrl-C).

package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

// pipelineCtx carries flag values + accumulated step responses through
// the executor. Unexported because callers always interact via runPipeline.
type pipelineCtx struct {
	flags       map[string]string
	stepOutputs []map[string]any
}

// runPipeline executes route.Pipeline against runner, returning the last
// "call"-type step's response (or a synthesized success payload if the
// pipeline ends with a "download" step). The map is the shape returned to
// the user via the standard output formatter.
func runPipeline(
	ctx context.Context,
	cmd *cobra.Command,
	runner executor.Runner,
	route Route,
	flagValues map[string]string,
) (map[string]any, error) {
	pctx := &pipelineCtx{
		flags:       flagValues,
		stepOutputs: make([]map[string]any, 0, len(route.Pipeline)),
	}

	var lastCallResponse map[string]any
	for i, step := range route.Pipeline {
		stepType := strings.TrimSpace(step.Type)
		if stepType == "" {
			stepType = "call"
		}
		switch stepType {
		case "call":
			resp, err := executePipelineCall(ctx, runner, route, step, pctx)
			if err != nil {
				return nil, fmt.Errorf("pipeline step %d (%s): %w", i, step.Tool, err)
			}
			pctx.stepOutputs = append(pctx.stepOutputs, resp)
			lastCallResponse = resp
		case "download":
			resp, err := executePipelineDownload(cmd, step, pctx)
			if err != nil {
				return nil, fmt.Errorf("pipeline step %d (download): %w", i, err)
			}
			pctx.stepOutputs = append(pctx.stepOutputs, resp)
		default:
			return nil, apperrors.NewValidation(
				fmt.Sprintf("pipeline step %d: unsupported type %q (allowed: call, download)", i, stepType),
			)
		}
	}

	if lastCallResponse != nil {
		return lastCallResponse, nil
	}
	return map[string]any{"success": true}, nil
}

// executePipelineCall resolves args templates, then either polls or fires
// a single MCP tool invocation via runner. PollUntilField + PollUntilValue
// non-empty enable polling.
func executePipelineCall(
	ctx context.Context,
	runner executor.Runner,
	route Route,
	step market.PipelineStep,
	pctx *pipelineCtx,
) (map[string]any, error) {
	if strings.TrimSpace(step.Tool) == "" {
		return nil, apperrors.NewValidation("pipeline call step requires non-empty `tool`")
	}
	args, err := resolveArgs(step.Args, pctx)
	if err != nil {
		return nil, err
	}

	invoke := func() (map[string]any, error) {
		invocation := executor.NewCompatibilityInvocation(
			route.Use,
			route.Target.CanonicalProduct,
			step.Tool,
			args,
		)
		result, err := runner.Run(ctx, invocation)
		if err != nil {
			return nil, err
		}
		if result.Response == nil {
			return map[string]any{}, nil
		}
		// Fail-fast on MCP business errors. Pre-execution validation (cobra
		// MarkFlagRequired) only checks that the flag was set, not that
		// the value is non-empty — so a `--required-flag ""` reaches here
		// and the upstream tool rejects with errorCode. Without this check
		// the pipeline proceeds to poll/download and either spins until
		// PollTimeout or burns through retries.
		if errCode := getDotPath(result.Response, "content.errorCode"); errCode != nil && fmt.Sprint(errCode) != "" {
			msg := getDotPath(result.Response, "content.errorMessage")
			return nil, apperrors.NewValidation(fmt.Sprintf(
				"%s rejected: %s — %v", step.Tool, errCode, msg,
			))
		}
		return result.Response, nil
	}

	if strings.TrimSpace(step.PollUntilField) == "" {
		return invoke()
	}

	// Polling loop.
	interval := time.Duration(step.PollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	timeoutSec := step.PollTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	for {
		resp, err := invoke()
		if err != nil {
			return nil, err
		}
		actual := getDotPath(resp, step.PollUntilField)
		if actual != nil && fmt.Sprint(actual) == step.PollUntilValue {
			return resp, nil
		}
		if time.Now().After(deadline) {
			return nil, apperrors.NewValidation(fmt.Sprintf(
				"pipeline poll timeout after %ds: field %q never reached value %q (last seen: %v)",
				timeoutSec, step.PollUntilField, step.PollUntilValue, actual,
			))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// executePipelineDownload resolves the URL template, fetches the body via
// HTTP GET, and writes it to the path supplied by OutputFlag's user value.
// Empty output path → print URL to stdout (terminal-friendly mode).
func executePipelineDownload(
	cmd *cobra.Command,
	step market.PipelineStep,
	pctx *pipelineCtx,
) (map[string]any, error) {
	urlAny, err := resolveTemplate(step.DownloadURLField, pctx)
	if err != nil {
		return nil, err
	}
	urlStr := strings.TrimSpace(fmt.Sprint(urlAny))
	if urlStr == "" {
		return nil, apperrors.NewValidation(fmt.Sprintf(
			"pipeline download: URL template %q resolved to empty value",
			step.DownloadURLField,
		))
	}

	outputPath := strings.TrimSpace(pctx.flags[step.OutputFlag])
	jobID := fmt.Sprint(inferJobIDFromContext(pctx))

	// Always print machine-parseable "key: value" lines. Tests and shell
	// pipelines that consume the pipeline output (regex / awk) rely on
	// this exact format. The structured JSON output follows via
	// output.WriteCommandPayload, so AI / SDK callers still get a typed
	// response.
	if jobID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "jobId: %s\n", jobID)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "downloadUrl: %s\n", urlStr)

	if outputPath == "" {
		return map[string]any{
			"success":     true,
			"downloadUrl": urlStr,
			"jobId":       jobID,
		}, nil
	}

	// If outputPath is a directory, infer filename from URL basename.
	if info, statErr := os.Stat(outputPath); statErr == nil && info.IsDir() {
		filename := inferFilenameFromURL(urlStr)
		if filename == "" {
			filename = fmt.Sprintf("export_%d", time.Now().Unix())
		}
		outputPath = filepath.Join(outputPath, filename)
	}

	resp, err := http.Get(urlStr) //nolint:gosec // user-supplied URL via MCP discovery is expected
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", urlStr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP GET %s: status %d", urlStr, resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", outputPath, err)
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("write %s: %w", outputPath, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "导出完成: %s (%d bytes)\n", outputPath, written)
	return map[string]any{
		"success":     true,
		"downloadUrl": urlStr,
		"jobId":       jobID,
		"output":      outputPath,
		"size":        written,
	}, nil
}

// resolveArgs applies resolveTemplate to every value in the map.
func resolveArgs(args map[string]string, pctx *pipelineCtx) (map[string]any, error) {
	out := make(map[string]any, len(args))
	for k, tmpl := range args {
		v, err := resolveTemplate(tmpl, pctx)
		if err != nil {
			return nil, fmt.Errorf("arg %q: %w", k, err)
		}
		out[k] = v
	}
	return out, nil
}

// resolveTemplate evaluates a single template string. Returns the literal
// when input does not start with '$'.
func resolveTemplate(tmpl string, pctx *pipelineCtx) (any, error) {
	s := strings.TrimSpace(tmpl)
	if !strings.HasPrefix(s, "$") {
		return s, nil
	}

	// Split on the first dot: head ("$flag" / "$step") + tail (rest).
	dot := strings.Index(s, ".")
	if dot <= 0 || dot == len(s)-1 {
		return nil, apperrors.NewValidation(fmt.Sprintf("malformed template %q (expected $flag.<name> or $step.<idx>.<path>)", tmpl))
	}
	head := s[:dot]
	tail := s[dot+1:]

	switch head {
	case "$flag":
		// tail is a flag alias name (no nested path supported)
		return pctx.flags[tail], nil
	case "$step":
		// tail format: <idx>.<dotPath>
		secondDot := strings.Index(tail, ".")
		if secondDot <= 0 || secondDot == len(tail)-1 {
			return nil, apperrors.NewValidation(fmt.Sprintf("malformed $step template %q (expected $step.<idx>.<dotPath>)", tmpl))
		}
		idxStr := tail[:secondDot]
		dotPath := tail[secondDot+1:]
		idx, err := strconv.Atoi(idxStr)
		if err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("$step template %q has non-numeric index", tmpl))
		}
		if idx < 0 || idx >= len(pctx.stepOutputs) {
			return nil, apperrors.NewValidation(fmt.Sprintf("$step template %q references step %d, but only %d step(s) executed so far", tmpl, idx, len(pctx.stepOutputs)))
		}
		return getDotPath(pctx.stepOutputs[idx], dotPath), nil
	default:
		return nil, apperrors.NewValidation(fmt.Sprintf("unknown template prefix %q in %q (allowed: $flag, $step)", head, tmpl))
	}
}

// getDotPath walks dotPath through nested map[string]any. Returns nil if
// any segment is missing or the value isn't a map at an intermediate step.
func getDotPath(m map[string]any, dotPath string) any {
	parts := strings.Split(dotPath, ".")
	var current any = m
	for _, p := range parts {
		nested, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = nested[p]
	}
	return current
}

// inferFilenameFromURL extracts the basename from a URL's path component,
// stripping query string + fragment. Returns "" if the URL doesn't parse
// or has no useful basename.
func inferFilenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if base == "" || base == "/" || base == "." {
		return ""
	}
	return base
}

// inferJobIDFromContext walks prior step outputs looking for a `jobId`
// field at top level or one level under common MCP wrappers ("content" /
// "result"), so the synthetic download response can echo it back to the
// user. Returns "" when no jobId is present anywhere in prior responses.
func inferJobIDFromContext(pctx *pipelineCtx) any {
	candidates := []string{"jobId", "content.jobId", "result.jobId"}
	for i := len(pctx.stepOutputs) - 1; i >= 0; i-- {
		for _, p := range candidates {
			if v := getDotPath(pctx.stepOutputs[i], p); v != nil && fmt.Sprint(v) != "" {
				return v
			}
		}
	}
	return ""
}

// extractFlagValuesByAlias reads the cobra command's flag values keyed by
// the FlagBinding's primary CLI flag name, so the pipeline executor can
// resolve "$flag.<name>" templates in O(1). Pipeline-local flags are
// always included (they are the whole point of the lookup).
//
// Note on key choice: buildOverrideBindings populates FlagName from the
// envelope's `alias` field (or kebab-case of the MCP property name when
// alias is empty), and leaves the FlagBinding.Alias struct field empty —
// so $flag templates reference the user-visible CLI flag name, e.g.
// "$flag.node" matches `--node`.
func extractFlagValuesByAlias(cmd *cobra.Command, bindings []FlagBinding) map[string]string {
	flags := cmd.Flags()
	out := make(map[string]string, len(bindings))
	for _, b := range bindings {
		primary := strings.TrimSpace(b.FlagName)
		if primary == "" {
			primary = strings.TrimSpace(b.Alias)
		}
		if primary == "" {
			continue
		}
		// Try the primary flag name first, then any of the extra aliases.
		// Whichever the user actually set wins; if none was set, the
		// cobra-level default value is returned.
		candidates := make([]string, 0, 2+len(b.Aliases))
		candidates = append(candidates, primary)
		if a := strings.TrimSpace(b.Alias); a != "" && a != primary {
			candidates = append(candidates, a)
		}
		for _, a := range b.Aliases {
			if a = strings.TrimSpace(a); a != "" {
				candidates = append(candidates, a)
			}
		}
		var value string
		for _, c := range candidates {
			f := flags.Lookup(c)
			if f == nil {
				continue
			}
			value = f.Value.String()
			if f.Changed {
				break
			}
		}
		out[primary] = value
	}
	return out
}

// jsonRoundTrip marshals + unmarshals so user-provided strings come out
// the other side as Go primitives where appropriate. Unused for now —
// the resolveTemplate path returns strings as-is to keep the contract
// simple; tools that need JSON-shaped values can use the existing
// `transform: "json_parse_strict"` on the relevant flag (post-pipeline
// composition is not in scope for the MVP).
var _ = json.Unmarshal
