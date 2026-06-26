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

// report_hooks.go — CLI-side input resolution for the `report` product.
//
// The envelope publishes `report entry submit` (MCP tool create_report) with a
// `--contents` flag (json_parse, required) and a sibling `--contents-file`
// flag (omitWhen empty, no transform/mapsTo). On its own, `--contents-file`
// therefore goes nowhere: its value maps to the unused `contentsFile` param and
// the real `contents` param stays empty, so a `--contents-file`-only (or
// `--contents -` stdin) submit silently sends `contents: [null]` and the report
// fails. The literal-only `--contents` path works, which is why
// `report create` (the helper, inline-only) succeeds while
// `report entry submit --contents-file` does not.
//
// The wukong reference implementation reads the file/stdin natively inside its
// hand-written cobra RunE (dws-wukong/wukong/products/report.go
// resolveReportContentsFromFlags, priority: --contents-file > --contents -
// (stdin) > --contents '<json>'). The open-source CLI is envelope-driven, so we
// attach the equivalent native resolution as a build-time hook here, mirroring
// AttachReportListReadableEnrichment (which layers wukong-equivalent list
// enrichment onto the same envelope leaves). No discovery-config change is
// needed: the hook populates the real `--contents` flag before the envelope's
// json_parse transform runs, and the broken `contentsFile` override is left
// inert.
//
// Two build-time adjustments make `--contents-file`-only valid:
//
//  1. The envelope marks `--contents` individually required (cobra
//     MarkFlagRequired, enforced at parse time, before PreRunE). We clear that
//     annotation and instead declare a `contents` / `contents-file` one-of
//     group (MarkFlagsOneRequired, validated by ValidateFlagGroups — also
//     before PreRunE, but satisfied when either flag is set). Supplying
//     neither still errors, now naming both flags.
//  2. A chained PreRunE resolves the chosen source into `--contents` so the
//     downstream json_parse transform sees inline JSON regardless of origin.

package compat

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// reportContentsMaxBytes caps the contents payload at 10MB, matching the
// wukong upstream limit (dws-wukong/wukong/products/report.go
// reportContentsMaxBytes). Oversized input is rejected rather than truncated.
const reportContentsMaxBytes = 10 * 1024 * 1024

// reportToolsWithContentsFile lists every report toolName whose `--contents` /
// `--contents-file` pair needs native file/stdin resolution. Today only
// create_report (the `report entry submit` leaf) carries the pair.
var reportToolsWithContentsFile = map[string]bool{
	"create_report": true,
}

// installReportHook wires report-specific input resolution onto leaf commands
// emitted by BuildDynamicCommands. It is a no-op for non-report products and
// for report tools that do not expose the contents/contents-file pair.
//
// The hook chain preserves the cmd.PreRunE that NewDirectCommand already
// installed (currently validateRequireTogether) by invoking it first.
func installReportHook(cmd *cobra.Command, canonicalProduct, toolName string) {
	if cmd == nil {
		return
	}
	if strings.TrimSpace(canonicalProduct) != "report" {
		return
	}
	if !reportToolsWithContentsFile[toolName] {
		return
	}
	contents := cmd.Flags().Lookup("contents")
	file := cmd.Flags().Lookup("contents-file")
	if contents == nil || file == nil {
		// Envelope shape changed (renamed/removed flags) — do not block the
		// command; leave whatever the envelope declared untouched.
		return
	}

	// (1) Relax the individually-required `--contents` into a one-of group so
	// `--contents-file`-only (or `--contents -`) is accepted. Clearing the
	// required annotation must happen before parse-time ValidateRequiredFlags;
	// this hook runs at build time, so it does.
	if contents.Annotations != nil {
		delete(contents.Annotations, cobra.BashCompOneRequiredFlag)
	}
	cmd.MarkFlagsOneRequired("contents", "contents-file")

	// (2) Resolve the chosen source into --contents before the RunE transform.
	original := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if original != nil {
			if err := original(c, args); err != nil {
				return err
			}
		}
		return resolveReportContents(c)
	}
}

// resolveReportContents applies the wukong source priority — `--contents-file`
// (file) > `--contents -` (stdin) > `--contents '<json>'` (literal) — and
// writes the resolved JSON string back into the `--contents` flag so the
// downstream json_parse transform decodes it uniformly. When a file or stdin
// source is used, `--contents-file` is cleared so the envelope's omitWhen:empty
// drops the now-redundant param.
func resolveReportContents(cmd *cobra.Command) error {
	filePath, _ := cmd.Flags().GetString("contents-file")
	if strings.TrimSpace(filePath) != "" {
		data, err := readReportContentsFile(filePath)
		if err != nil {
			return err
		}
		if err := cmd.Flags().Set("contents", data); err != nil {
			return apperrors.NewInternal("failed to set --contents from --contents-file")
		}
		_ = cmd.Flags().Set("contents-file", "")
		return nil
	}

	raw, _ := cmd.Flags().GetString("contents")
	if strings.TrimSpace(raw) == "-" {
		data, err := readReportContentsLimited(cmd.InOrStdin(), "--contents -")
		if err != nil {
			return err
		}
		if err := cmd.Flags().Set("contents", data); err != nil {
			return apperrors.NewInternal("failed to set --contents from stdin")
		}
	}
	return nil
}

// readReportContentsFile opens a file path and reads its contents under the
// 10MB cap and UTF-8 check. Error wording mirrors wukong so agents and humans
// see a stable message across both editions.
func readReportContentsFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", apperrors.NewValidation(
				fmt.Sprintf("--contents-file: file not found: %s", path),
				apperrors.WithHint("确认路径存在，且指向一个 JSON 文件"),
			)
		}
		return "", apperrors.NewValidation(fmt.Sprintf("--contents-file: cannot read %s: %v", path, err))
	}
	defer file.Close()
	return readReportContentsLimited(file, fmt.Sprintf("--contents-file %s", path))
}

// readReportContentsLimited reads from r enforcing the 10MB cap and UTF-8
// validity. A LimitReader at cap+1 detects overflow without reading unbounded.
func readReportContentsLimited(r io.Reader, source string) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(reportContentsMaxBytes)+1))
	if err != nil {
		return "", apperrors.NewValidation(fmt.Sprintf("%s: read failed: %v", source, err))
	}
	if len(data) > reportContentsMaxBytes {
		return "", apperrors.NewValidation(
			fmt.Sprintf("%s: contents exceed maximum size of 10MB", source),
			apperrors.WithHint("精简内容或拆分为多份日志提交"),
		)
	}
	if !utf8.Valid(data) {
		return "", apperrors.NewValidation(fmt.Sprintf("%s: not valid UTF-8", source))
	}
	return string(data), nil
}
