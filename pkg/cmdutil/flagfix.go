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

package cmdutil

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagFixResult holds the result of SuggestFlagFix analysis.
type FlagFixResult struct {
	Suggestion   string
	AutoFixFlag  string
	AutoFixValue string
}

// CommonFlagAliases maps commonly misused flag names to their correct equivalents.
var CommonFlagAliases = map[string]string{
	"json":            "format json",
	"output":          "format",
	"out":             "format",
	"o":               "format",
	"silent":          "quiet",
	"dry":             "dry-run",
	"force":           "yes",
	"f":               "yes",
	"timeout-seconds": "timeout",
	"device-flow":     "device",
	"deviceflow":      "device",
}

// DetectNumericTypeError checks if err is a Cobra/pflag numeric type
// validation error. Returns the flag name and the bad value if detected.
func DetectNumericTypeError(err error) (flagName, badValue string, ok bool) {
	msg := err.Error()
	if !strings.Contains(msg, "strconv.Parse") {
		return "", "", false
	}
	const argPrefix = "invalid argument \""
	argIdx := strings.Index(msg, argPrefix)
	if argIdx < 0 {
		return "", "", false
	}
	afterArg := msg[argIdx+len(argPrefix):]
	argEnd := strings.Index(afterArg, "\"")
	if argEnd < 0 {
		return "", "", false
	}
	badVal := afterArg[:argEnd]

	const marker = "\" for \"--"
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return "", "", false
	}
	rest := msg[idx+len(marker):]
	endIdx := strings.Index(rest, "\" flag")
	if endIdx < 0 {
		return "", "", false
	}
	return rest[:endIdx], badVal, true
}

// flagFixCandidate reports whether f should participate in unknown-flag
// suggestion candidates and Flags: listings. Hidden flags (e.g. wukong's
// MarkHidden compatibility aliases) and internal json/params merge flags
// are skipped so the hint candidate set stays a subset of what --help shows.
func flagFixCandidate(f *pflag.Flag) bool {
	if f == nil || f.Hidden {
		return false
	}
	switch f.Name {
	case "json", "params":
		return false
	}
	return true
}

// VisibleFlagNames returns sorted candidate flag names for cmd.Flags()
// using flagFixCandidate. Intended for agent-facing error recovery
// (available_flags).
func VisibleFlagNames(cmd *cobra.Command) []string {
	if cmd == nil {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !flagFixCandidate(f) || seen[f.Name] {
			return
		}
		seen[f.Name] = true
		names = append(names, f.Name)
	})
	sort.Strings(names)
	return names
}

// SuggestFlagFix detects flag-value concatenation errors, common flag aliases,
// and Levenshtein-close typos.
func SuggestFlagFix(cmd *cobra.Command, flagErr error) FlagFixResult {
	msg := flagErr.Error()
	const prefix = "unknown flag: --"
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return FlagFixResult{}
	}
	body := strings.TrimSpace(msg[idx+len(prefix):])

	if alias, ok := CommonFlagAliases[body]; ok {
		return FlagFixResult{Suggestion: fmt.Sprintf("Did you mean --%s? Run '%s --help' for options", alias, cmd.CommandPath())}
	}

	var bestFlag, bestValue string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !flagFixCandidate(f) {
			return
		}
		name := f.Name
		if strings.HasPrefix(body, name) && len(body) > len(name) {
			if len(name) > len(bestFlag) {
				bestFlag = name
				bestValue = body[len(name):]
			}
		}
	})
	if bestFlag != "" {
		lf := cmd.Flags().Lookup(bestFlag)
		if lf != nil {
			fmtStr := ""
			if v := lf.Annotations["x-cli-format"]; len(v) > 0 {
				fmtStr = v[0]
			}
			var enumCopy []string
			if v := lf.Annotations["x-cli-enum"]; len(v) > 0 {
				enumCopy = append([]string{}, v...)
			}
			if SuffixLooksLikeValue(bestValue, lf.Value.Type(), fmtStr, enumCopy) {
				suggestion := fmt.Sprintf("Space required between flag and value: --%s %s", bestFlag, bestValue)
				return FlagFixResult{Suggestion: suggestion, AutoFixFlag: bestFlag, AutoFixValue: bestValue}
			}
		}
	}

	bestName, bestDist := "", 999
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !flagFixCandidate(f) {
			return
		}
		d := LevenshteinDist(body, f.Name)
		if d < bestDist {
			bestDist = d
			bestName = f.Name
		}
	})
	threshold := LevenshteinThreshold(len(body))
	if bestDist > 0 && bestDist <= threshold && bestName != "" {
		suf := formatFlagHintSuffix(cmd.Flags().Lookup(bestName))
		return FlagFixResult{Suggestion: fmt.Sprintf("Did you mean --%s?%s", bestName, suf)}
	}

	return FlagFixResult{Suggestion: fmt.Sprintf("Run '%s --help' to see available options", cmd.CommandPath())}
}

func formatFlagHintSuffix(f *pflag.Flag) string {
	if f == nil {
		return ""
	}
	var parts []string
	if u := strings.TrimSpace(f.Usage); u != "" {
		if len(u) > 100 {
			u = u[:97] + "..."
		}
		parts = append(parts, u)
	}
	if v := f.Annotations["x-cli-format"]; len(v) > 0 && v[0] != "" {
		parts = append(parts, "format="+v[0])
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// LevenshteinThreshold returns the max edit distance allowed based on string length.
func LevenshteinThreshold(nameLen int) int {
	if nameLen <= 3 {
		return 1
	}
	if nameLen <= 8 {
		return 2
	}
	return 3
}

// LevenshteinDist returns the edit distance between two strings.
func LevenshteinDist(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	dp := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		dp[j] = j
	}
	for i := 1; i <= la; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= lb; j++ {
			temp := dp[j]
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			dp[j] = min(dp[j]+1, min(dp[j-1]+1, prev+cost))
			prev = temp
		}
	}
	return dp[lb]
}
