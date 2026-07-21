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

package pipeline

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RunPreParse resolves the target command from the raw args, extracts
// flag names from the Cobra command tree, and runs all PreParse
// handlers. The corrected args are set back on the root command via
// SetArgs so that Cobra's subsequent ExecuteC uses the corrected
// values.
//
// If the target command cannot be resolved (e.g. the user typed a
// non-existent command), PreParse is skipped silently and Cobra will
// handle the error.
func RunPreParse(root *cobra.Command, engine *Engine) error {
	_, err := RunPreParseArgs(root, engine, os.Args[1:])
	return err
}

// RunPreParseArgs is the testable form of RunPreParse. Production passes
// os.Args[1:]; end-to-end tests can pass an isolated argv while exercising the
// exact same command traversal, FlagInfo extraction, handler chain, and
// root.SetArgs delivery path.
func RunPreParseArgs(root *cobra.Command, engine *Engine, rawArgs []string) (*Context, error) {
	if engine == nil || !engine.HasHandlers(PreParse) {
		return nil, nil
	}

	if len(rawArgs) == 0 {
		return nil, nil
	}

	// Traverse the command tree to find the target command.
	target, _, err := root.Traverse(rawArgs)
	if err != nil {
		return nil, nil
	}

	// Build FlagInfo from the target command's registered flags.
	flagInfos := FlagInfoFromCommand(target)
	if len(flagInfos) == 0 {
		return nil, nil
	}

	ctx := &Context{
		Args: append([]string{}, rawArgs...),
		// target.CommandPath() still carries the "dws" prefix; PreParse
		// handlers that key off a command (e.g. the semantic-alias table)
		// normalize it themselves, so the runtime key matches the build key.
		Command:   target.CommandPath(),
		FlagSpecs: flagInfos,
	}

	if err := engine.RunPhase(PreParse, ctx); err != nil {
		slog.Debug("pipeline pre-parse", "error", err)
		return ctx, err
	}

	// Only set corrected args if PreParse actually changed something.
	if len(ctx.Corrections) > 0 {
		root.SetArgs(ctx.Args)
		for _, c := range ctx.Corrections {
			slog.Debug("pipeline correction",
				"handler", c.Handler,
				"kind", c.Kind,
				"field", c.Field,
				"original", c.Original,
				"corrected", c.Corrected,
			)
		}
	}
	return ctx, nil
}

// FlagInfoFromCommand extracts FlagInfo entries from a Cobra
// command's registered flags (both local and inherited).
//
// JSON Schema "format" and "enum" hints injected via pflag
// annotations (x-cli-format / x-cli-enum, see
// internal/compat/dynamic_commands.go) are surfaced on FlagInfo
// so PreParse handlers can validate sticky-split candidates against
// the actual schema, not just the pflag type.
func FlagInfoFromCommand(cmd *cobra.Command) []FlagInfo {
	if cmd == nil {
		return nil
	}

	seen := make(map[string]bool)
	var infos []FlagInfo

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		appendFlagInfo(&infos, seen, f)
	})

	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		appendFlagInfo(&infos, seen, f)
	})

	return infos
}

func appendFlagInfo(infos *[]FlagInfo, seen map[string]bool, flag *pflag.Flag) {
	if seen[flag.Name] {
		return
	}
	seen[flag.Name] = true
	*infos = append(*infos, flagInfoFromPflag(flag))
}

// flagInfoFromPflag builds a FlagInfo from a pflag.Flag, copying
// schema metadata stashed in the flag's annotations map.
func flagInfoFromPflag(f *pflag.Flag) FlagInfo {
	fi := FlagInfo{
		Name:         f.Name,
		PropertyName: f.Name,
		Type:         f.Value.Type(),
	}
	if v := f.Annotations["x-cli-format"]; len(v) > 0 {
		fi.Format = v[0]
	}
	if v := f.Annotations["x-cli-enum"]; len(v) > 0 {
		fi.Enum = append([]string{}, v...)
	}
	return fi
}
