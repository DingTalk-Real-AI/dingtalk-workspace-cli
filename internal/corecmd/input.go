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

package corecmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
)

// FlagSpec.Input source constants. They declare extra input sources for a
// KindString flag beyond the literal command-line value, mirroring the
// lark-cli Flag.Input capability so large payloads (markdown, JSON, CSV)
// never need shell quoting.
const (
	// InputFile allows the flag value @path to be replaced by the file content.
	InputFile = "file"
	// InputStdin allows the flag value - to be replaced by the stdin content.
	InputStdin = "stdin"
)

// utf8BOM is stripped from file/stdin content so a Windows-edited payload
// cannot corrupt the first CSV cell or break JSON parsing downstream.
const utf8BOM = "\ufeff"

// inputResolvedAnnotation marks a flag whose value was loaded from @file or
// stdin during this invocation. The key deliberately avoids the dws.schema.*
// prefix: it is per-invocation runtime state, not a published Schema fact, and
// schema assembly only reads dws.schema.* keys.
const inputResolvedAnnotation = "dws.runtime.input_resolved"

// markInputResolved records that name's value came from an external source.
func markInputResolved(cmd *cobra.Command, name string) {
	_ = cmd.Flags().SetAnnotation(name, inputResolvedAnnotation, []string{"true"})
}

// resetInputResolved clears the markers a previous run left behind. The marker
// is mutable state on the shared cobra command object, which is correct because
// a command object has at most one invocation in flight; reset covers
// sequential reuse (a test or wrapper executing the same object twice), where an
// earlier @file call would otherwise keep making a later inline value look
// source-loaded and authoritative. A future in-process concurrent dispatcher
// must carry this provenance in per-invocation state instead of the annotation.
func resetInputResolved(cmd *cobra.Command, flags []FlagSpec) {
	for _, flag := range flags {
		if len(flag.Input) == 0 {
			continue
		}
		for _, name := range append([]string{flag.Name}, flag.Aliases...) {
			// SetAnnotation only errors for an unregistered flag; RegisterFlags
			// registers the main name and every declared alias.
			_ = cmd.Flags().SetAnnotation(name, inputResolvedAnnotation, nil)
		}
	}
}

// InputResolvedFromSource reports whether the named flag's value was loaded
// from @file or stdin rather than typed inline. A domain guard that applies
// shape heuristics to inline values ("this looks like a path — did you forget
// the @?") must skip resolved values: their content was already read from the
// right place and may legitimately look like anything, including a path.
// Without this bit such a guard re-rejects correct @file / stdin invocations,
// because by the time Validate runs both arrive as plain text.
func InputResolvedFromSource(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup(name)
	return flag != nil && len(flag.Annotations[inputResolvedAnnotation]) > 0
}

// inputResolvedAny reports whether any declared name of the flag carried a
// source-loaded payload this invocation. Later pipeline stages (constraint
// provision, args assembly) consult it so the "a source-loaded payload is
// authoritative" rule holds end to end and not only inside rawValue.
func inputResolvedAny(cmd *cobra.Command, flag FlagSpec) bool {
	if len(flag.Input) == 0 {
		return false
	}
	for _, name := range append([]string{flag.Name}, flag.Aliases...) {
		if cmd.Flags().Changed(name) && InputResolvedFromSource(cmd, name) {
			return true
		}
	}
	return false
}

// validateInputSpecs rejects malformed Input declarations at build time. Like
// the other declaration checks this panics: a bad Input spec is a programming
// error every test and startup path should trip immediately.
func validateInputSpecs(spec Spec) {
	for _, flag := range spec.Flags {
		if len(flag.Input) == 0 {
			continue
		}
		if flag.Kind != KindString {
			panic(fmt.Sprintf(
				"command %q flag %q: Input is only supported on KindString flags",
				spec.Use, flag.Name))
		}
		seen := map[string]bool{}
		for _, source := range flag.Input {
			if source != InputFile && source != InputStdin {
				panic(fmt.Sprintf(
					"command %q flag %q: unknown Input source %q (allowed: %s, %s)",
					spec.Use, flag.Name, source, InputFile, InputStdin))
			}
			if seen[source] {
				panic(fmt.Sprintf(
					"command %q flag %q: duplicate Input source %q",
					spec.Use, flag.Name, source))
			}
			seen[source] = true
		}
		// ConfirmFirst confirms before resolution, so the prompt would read the
		// piped payload as its answer: a payload whose first line is "yes"
		// authorizes the operation while the flag itself ends up empty — data
		// acting as a control channel. The default ordering is safe because
		// resolution consumes stdin first and the prompt then fails closed with
		// confirmation_required.
		if spec.ConfirmFirst && inputSupports(flag, InputStdin) {
			panic(fmt.Sprintf(
				"command %q flag %q: ConfirmFirst cannot be combined with Input %q: the confirmation prompt would consume the piped payload and a payload starting with \"yes\" would authorize the operation; drop ConfirmFirst and require --yes instead",
				spec.Use, flag.Name, InputStdin))
		}
	}
}

// inputSupports reports whether the flag declared the given input source.
func inputSupports(flag FlagSpec, source string) bool {
	for _, s := range flag.Input {
		if s == source {
			return true
		}
	}
	return false
}

// explicitInputFlagName picks the declared name that carries the user's
// explicit token, mirroring rawValue's order and usability judgement exactly:
// the main flag wins only when changed and usable (trim-judged only when
// Trim is set), then declared aliases in order. Diverging here could rewrite
// an alias that rawValue would shadow (or vice versa). EnvVar fallback and
// registration defaults are never input-resolved — @file only applies to what
// the user literally typed.
func explicitInputFlagName(cmd *cobra.Command, flag FlagSpec) string {
	usable := func(v string) bool {
		if flag.Trim {
			v = strings.TrimSpace(v)
		}
		return v != ""
	}
	for _, name := range append([]string{flag.Name}, flag.Aliases...) {
		if cmd.Flags().Changed(name) && usable(cmdutil.MustGetFlag(cmd, name)) {
			return name
		}
	}
	return ""
}

// resolveInputFlags rewrites the explicit values of Input-declaring flags
// before required/enum/constraint/Validate checks, so every downstream stage
// sees the real payload content. Semantics:
//
//   - "-" reads stdin; a process has a single stdin, so a second Input flag
//     using "-" in the same invocation is rejected.
//   - "@path" reads the named file (leading/trailing whitespace trimmed).
//   - "@@value" escapes to the literal "@value" and is not source-resolved.
//
// Resolution runs once per invocation inside runDeclaredPreflight. It
// rewrites the cobra flag value in place (Set keeps Changed=true) and marks the
// flag as source-resolved, so fallback-chain readers (EffectiveValue/BuildArgs)
// observe the content verbatim — including an empty payload, which stays
// authoritative instead of falling back to an alias/env/Default value.
//
// Interaction with confirmation: stdin is consumed here, before the Safety
// prompt of a user_required write would read it; such an interactive prompt
// then sees EOF and fails closed with confirmation_required. Write commands
// declaring InputStdin must be invoked with --yes (or --dry-run).
func resolveInputFlags(cmd *cobra.Command, flags []FlagSpec) error {
	resetInputResolved(cmd, flags)
	stdinConsumed := false
	for _, flag := range flags {
		if len(flag.Input) == 0 {
			continue
		}
		name := explicitInputFlagName(cmd, flag)
		if name == "" {
			continue
		}
		raw := cmdutil.MustGetFlag(cmd, name)
		// Trim flags judge usability on the trimmed value (rawValue), so the
		// prefix check must agree or " @path" would slip through unresolved.
		if flag.Trim {
			raw = strings.TrimSpace(raw)
		}

		switch {
		case raw == "-":
			if !inputSupports(flag, InputStdin) {
				return apperrors.NewValidation(
					fmt.Sprintf("参数 --%s 不支持 stdin 输入（-）", flag.Name))
			}
			if stdinConsumed {
				return apperrors.NewValidation(
					fmt.Sprintf("参数 --%s：stdin（-）只能被一个参数使用", flag.Name),
					apperrors.WithHint(fmt.Sprintf(
						"一个进程只有一份 stdin，其余参数请内联传值或用 @文件路径（如 --%s @./payload.json）",
						flag.Name)))
			}
			stdinConsumed = true
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return apperrors.NewValidation(
					fmt.Sprintf("参数 --%s 读取 stdin 失败：%v", flag.Name, err))
			}
			// Setting a registered string flag cannot fail.
			_ = cmd.Flags().Set(name, strings.TrimPrefix(string(data), utf8BOM))
			markInputResolved(cmd, name)

		case strings.HasPrefix(raw, "@@"):
			// Escape: strip the first @, keep the rest as a literal inline value.
			_ = cmd.Flags().Set(name, raw[1:])

		case strings.HasPrefix(raw, "@"):
			if !inputSupports(flag, InputFile) {
				return apperrors.NewValidation(
					fmt.Sprintf("参数 --%s 不支持文件输入（@路径）", flag.Name))
			}
			path := strings.TrimSpace(raw[1:])
			if path == "" {
				return apperrors.NewValidation(
					fmt.Sprintf("参数 --%s：@ 后的文件路径不能为空", flag.Name))
			}
			data, err := os.ReadFile(path)
			if err != nil {
				var opts []apperrors.Option
				if inputSupports(flag, InputStdin) {
					// Rejected @file paths are usually absolute (temp files).
					// Steer toward stdin rather than copying the file around.
					opts = append(opts, apperrors.WithHint(fmt.Sprintf(
						"该参数也支持 stdin：把文件内容管道进命令并传 --%s -", flag.Name)))
				}
				return apperrors.NewValidation(
					fmt.Sprintf("参数 --%s 读取文件 %q 失败：%v", flag.Name, path, err), opts...)
			}
			_ = cmd.Flags().Set(name, strings.TrimPrefix(string(data), utf8BOM))
			markInputResolved(cmd, name)
		}
	}
	return nil
}
