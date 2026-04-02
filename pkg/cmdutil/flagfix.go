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
		name := f.Name
		if strings.HasPrefix(body, name) && len(body) > len(name) {
			if len(name) > len(bestFlag) {
				bestFlag = name
				bestValue = body[len(name):]
			}
		}
	})
	if bestFlag != "" {
		canAutoFix := len(bestValue) > 0
		suggestion := fmt.Sprintf("Space required between flag and value: --%s %s", bestFlag, bestValue)
		if canAutoFix {
			return FlagFixResult{Suggestion: suggestion, AutoFixFlag: bestFlag, AutoFixValue: bestValue}
		}
		return FlagFixResult{Suggestion: suggestion}
	}

	bestName, bestDist := "", 999
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		d := LevenshteinDist(body, f.Name)
		if d < bestDist {
			bestDist = d
			bestName = f.Name
		}
	})
	threshold := LevenshteinThreshold(len(body))
	if bestDist > 0 && bestDist <= threshold && bestName != "" {
		return FlagFixResult{Suggestion: fmt.Sprintf("Did you mean --%s?", bestName)}
	}

	return FlagFixResult{Suggestion: fmt.Sprintf("Run '%s --help' to see available options", cmd.CommandPath())}
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
