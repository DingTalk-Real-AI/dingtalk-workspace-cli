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

package helpers

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func writeCommandPayload(cmd *cobra.Command, payload any) error {
	return output.WriteCommandPayload(cmd, payload, output.FormatJSON)
}

func preferLegacyLeaf(cmd *cobra.Command) {
	cli.SetOverridePriority(cmd, 100)
}

// resolveStringFlag reads a string flag, resolves @file/@- input sources,
// and falls back to stdin pipe when the flag is the designated primary
// content flag and the user did not provide an explicit value.
//
// primaryContent indicates this flag is the default stdin receiver for the
// command (e.g. --text for chat send). When true and the flag value is empty,
// stdin pipe data is used automatically.
func resolveStringFlag(cmd *cobra.Command, flagName string, guard *cli.StdinGuard, primaryContent bool) (string, error) {
	raw, err := cmd.Flags().GetString(flagName)
	if err != nil {
		return "", apperrors.NewInternal("failed to read --" + flagName)
	}

	// Resolve @file / @- syntax.
	resolved, err := cli.ResolveInputSource(raw, flagName, guard)
	if err != nil {
		return "", err
	}

	// Implicit stdin fallback: only for the primary content flag, only when
	// the user did not provide an explicit value and stdin is unclaimed.
	if resolved == "" && primaryContent && !guard.Claimed() && cli.StdinIsPipe() {
		if claimErr := guard.Claim("implicit stdin → --" + flagName); claimErr != nil {
			return "", claimErr
		}
		return cli.ReadStdin()
	}

	return resolved, nil
}

func commandDryRun(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	root := cmd.Root()
	var rootFlags *pflag.FlagSet
	if root != nil {
		rootFlags = root.PersistentFlags()
	}
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags(), rootFlags} {
		if flags == nil {
			continue
		}
		flag := flags.Lookup("dry-run")
		if flag == nil {
			continue
		}
		value, err := flags.GetBool("dry-run")
		if err == nil {
			return value
		}
	}
	return false
}
