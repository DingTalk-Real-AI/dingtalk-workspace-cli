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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func writeCommandPayload(cmd *cobra.Command, payload any) error {
	return output.WriteCommandPayload(cmd, payload, output.FormatJSON)
}

func preferLegacyLeaf(cmd *cobra.Command) {
	cli.SetOverridePriority(cmd, 100)
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
