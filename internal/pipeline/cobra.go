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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
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

	// Cobra's Traverse does not merge root persistent flags before deciding
	// whether a leading flag consumes the next token. For example, it can treat
	// the command name in `--dry-run calendar event list` as a value and resolve
	// the unrelated root command `event list`. Remove only known root-persistent
	// flags from the traversal copy; the original argv remains intact for the
	// handlers and Cobra's real parse.
	target, _, err := root.Traverse(argsForCommandTraversal(root, rawArgs))
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
		// Cobra has not parsed persistent flags yet, but this error is rendered
		// immediately by app.Execute. Prime only the presentation controls so
		// --format/--debug/--verbose affect this early error exactly as they do
		// errors returned after Cobra parsing. No credentials, profiles, output
		// paths, or execution controls are applied here.
		if presentationErr := primeEarlyErrorPresentation(root, target, ctx.Args); presentationErr != nil {
			slog.Debug("pipeline pre-parse presentation flags", "error", presentationErr)
		}
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

func argsForCommandTraversal(root *cobra.Command, rawArgs []string) []string {
	if root == nil || len(rawArgs) == 0 {
		return rawArgs
	}
	flags := root.PersistentFlags()
	if flags == nil || !flags.HasFlags() {
		return rawArgs
	}

	matcher := newFlagTokenMatcher(flags)
	out := make([]string, 0, len(rawArgs))
	for index := 0; index < len(rawArgs); index++ {
		argument := rawArgs[index]
		if argument == "--" {
			out = append(out, rawArgs[index:]...)
			break
		}
		flag, inlineValue, matched := matcher.matchTraversalToken(argument)
		if !matched {
			out = append(out, argument)
			continue
		}
		if index+1 < len(rawArgs) {
			if _, ok := separatedBoolValue(argument, rawArgs[index+1], flag, inlineValue); ok {
				index++
				continue
			}
		}
		if !inlineValue && flag.NoOptDefVal == "" && index+1 < len(rawArgs) {
			index++
		}
	}
	return out
}

// separatedBoolValue recognises model-friendly `--flag false` and exact
// shorthand `-f false` spellings without mistaking an already attached value
// or a shorthand cluster for a detached value. pflag otherwise treats a bare
// bool flag as true and leaves the following token positional.
func separatedBoolValue(argument, following string, flag *pflag.Flag, inlineValue bool) (string, bool) {
	if flag == nil || (flag.Value.Type() != "bool" && flag.Value.Type() != "boolean") {
		return "", false
	}
	if strings.HasPrefix(argument, "--") {
		if inlineValue || strings.Contains(argument, "=") {
			return "", false
		}
	} else if flag.Shorthand == "" || argument != "-"+flag.Shorthand {
		return "", false
	}
	return cmdutil.NormalizeBoolLiteral(following)
}

type longFlagMatch struct {
	flag       *pflag.Flag
	value      string
	hasValue   bool
	recognized bool
}

type flagTokenMatcher struct {
	byName      map[string]*pflag.Flag
	byShorthand map[string]*pflag.Flag
	known       map[string]bool
	candidates  []string
	specByName  map[string]FlagInfo
}

func newFlagTokenMatcher(flagSets ...*pflag.FlagSet) *flagTokenMatcher {
	matcher := &flagTokenMatcher{
		byName:      make(map[string]*pflag.Flag),
		byShorthand: make(map[string]*pflag.Flag),
		known:       make(map[string]bool),
		specByName:  make(map[string]FlagInfo),
	}
	for _, flags := range flagSets {
		if flags == nil {
			continue
		}
		flags.VisitAll(func(flag *pflag.Flag) {
			if _, exists := matcher.byName[flag.Name]; exists {
				return
			}
			matcher.byName[flag.Name] = flag
			matcher.known[flag.Name] = true
			matcher.candidates = append(matcher.candidates, flag.Name)
			matcher.specByName[flag.Name] = flagInfoFromPflag(flag)
			if flag.Shorthand != "" {
				matcher.byShorthand[flag.Shorthand] = flag
			}
		})
	}
	return matcher
}

func (m *flagTokenMatcher) matchLongToken(argument string) longFlagMatch {
	if m == nil || !strings.HasPrefix(argument, "--") || argument == "--" {
		return longFlagMatch{}
	}

	canonical := argument
	body := strings.TrimPrefix(canonical, "--")
	name, value, hasValue := strings.Cut(body, "=")
	if flag := m.byName[name]; flag != nil {
		return longFlagMatch{flag: flag, value: value, hasValue: hasValue, recognized: true}
	}

	if normalized, ok := NormalizeFlagToken(argument, m.known); ok {
		canonical = normalized
	} else if split, ok := SplitStickyFlag(argument, m.specByName); ok {
		name = strings.TrimPrefix(split.Flag, "--")
		return longFlagMatch{flag: m.byName[name], value: split.Value, hasValue: true, recognized: true}
	} else if fuzzy, ok := FuzzyMatchFlag(argument, m.known, m.candidates); ok {
		canonical = fuzzy
	} else {
		return longFlagMatch{}
	}

	body = strings.TrimPrefix(canonical, "--")
	name, value, hasValue = strings.Cut(body, "=")
	flag := m.byName[name]
	return longFlagMatch{flag: flag, value: value, hasValue: hasValue, recognized: flag != nil}
}

func (m *flagTokenMatcher) matchTraversalToken(argument string) (*pflag.Flag, bool, bool) {
	if m == nil || argument == "" || argument == "-" || argument == "--" {
		return nil, false, false
	}
	if strings.HasPrefix(argument, "--") {
		match := m.matchLongToken(argument)
		return match.flag, match.hasValue, match.recognized
	}
	if !strings.HasPrefix(argument, "-") {
		return nil, false, false
	}

	body := strings.TrimPrefix(argument, "-")
	shorthands := []rune(body)
	for index, shorthand := range shorthands {
		flag := m.byShorthand[string(shorthand)]
		if flag == nil {
			return nil, false, false
		}
		if flag.NoOptDefVal == "" {
			// A value-taking shorthand consumes the next token only when it is
			// last; otherwise the remainder is its attached value (`-vfjson`).
			return flag, index < len(shorthands)-1, true
		}
	}
	return m.byShorthand[string(shorthands[0])], true, true
}

func primeEarlyErrorPresentation(root, target *cobra.Command, rawArgs []string) error {
	if root == nil || target == nil || len(rawArgs) == 0 {
		return nil
	}
	rootFlags := root.PersistentFlags()
	presentationFlags := pflag.NewFlagSet("early-error-presentation", pflag.ContinueOnError)
	presentationFlags.SetOutput(io.Discard)
	presentationFlags.ParseErrorsWhitelist.UnknownFlags = true
	presentationFlags.SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		return pflag.NormalizedName(cmdutil.Morph(name))
	})

	names := make([]string, 0, 3)
	if source := rootFlags.Lookup("format"); source != nil {
		value, err := rootFlags.GetString("format")
		if err != nil {
			return fmt.Errorf("read presentation flag --format: %w", err)
		}
		presentationFlags.StringP("format", source.Shorthand, value, source.Usage)
		names = append(names, "format")
	}
	for _, name := range []string{"debug", "verbose"} {
		source := rootFlags.Lookup(name)
		if source == nil {
			continue
		}
		value, err := rootFlags.GetBool(name)
		if err != nil {
			return fmt.Errorf("read presentation flag --%s: %w", name, err)
		}
		presentationFlags.BoolP(name, source.Shorthand, value, source.Usage)
		names = append(names, name)
	}
	// Keep the existing contract in which a PreParse conflict wins over help;
	// registering help prevents pflag's special unknown-help early return.
	presentationFlags.BoolP("help", "h", false, "")

	parseErr := presentationFlags.Parse(rawArgs)
	errs := []error{parseErr}
	for _, name := range names {
		if !presentationFlags.Changed(name) {
			continue
		}
		value := presentationFlags.Lookup(name).Value.String()
		if err := rootFlags.Set(name, value); err != nil {
			errs = append(errs, fmt.Errorf("apply presentation flag --%s: %w", name, err))
		}
	}
	return errors.Join(errs...)
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
		Shorthand:    f.Shorthand,
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
