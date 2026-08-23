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

package chat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

type LeafContract = corecmd.ContractDecl

type LeafSpec struct {
	Safety   contract.SafetySpec
	Contract LeafContract
}

type Deps struct {
	GroupRunE            func(*cobra.Command, []string) error
	CallMCPToolOnServer  func(string, string, map[string]any) error
	DeclareLeafMetadata  func(*cobra.Command, LeafSpec) *cobra.Command
	ValidateRequiredFlag func(*cobra.Command, ...string) error
}

var deps = Deps{
	GroupRunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	CallMCPToolOnServer: func(serverID, toolName string, args map[string]any) error {
		return fmt.Errorf("im helpers dependency CallMCPToolOnServer is not initialized")
	},
	DeclareLeafMetadata: func(cmd *cobra.Command, spec LeafSpec) *cobra.Command {
		return cmd
	},
	ValidateRequiredFlag: validateRequiredFlagsLocal,
}

func SetDeps(next Deps) {
	if next.GroupRunE != nil {
		deps.GroupRunE = next.GroupRunE
	}
	if next.CallMCPToolOnServer != nil {
		deps.CallMCPToolOnServer = next.CallMCPToolOnServer
	}
	if next.DeclareLeafMetadata != nil {
		deps.DeclareLeafMetadata = next.DeclareLeafMetadata
	}
	if next.ValidateRequiredFlag != nil {
		deps.ValidateRequiredFlag = next.ValidateRequiredFlag
	}
}

func NewChatToolbarCommand() *cobra.Command {
	return newChatToolbarCommand()
}

func groupRunE(cmd *cobra.Command, args []string) error {
	return deps.GroupRunE(cmd, args)
}

func callMCPToolOnServer(serverID, toolName string, args map[string]any) error {
	return deps.CallMCPToolOnServer(serverID, toolName, args)
}

func DeclareLeafMetadata(cmd *cobra.Command, spec LeafSpec) *cobra.Command {
	return deps.DeclareLeafMetadata(cmd, spec)
}

func validateRequiredFlags(cmd *cobra.Command, names ...string) error {
	return deps.ValidateRequiredFlag(cmd, names...)
}

func mustGetFlag(cmd *cobra.Command, name string) string {
	val, _ := cmd.Flags().GetString(name)
	if val == "" {
		val, _ = cmd.InheritedFlags().GetString(name)
	}
	return strings.TrimSpace(val)
}

func parseCSVInt64(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	result := make([]int64, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", v)
		}
		result = append(result, n)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one ID is required")
	}
	return result, nil
}

func validateRequiredFlagsLocal(cmd *cobra.Command, names ...string) error {
	for _, name := range names {
		if strings.TrimSpace(mustGetFlag(cmd, name)) == "" {
			return apperrors.NewValidation(
				fmt.Sprintf("flag --%s is required", name),
				apperrors.WithReason("missing_required_flag"),
				apperrors.WithHint(fmt.Sprintf("使用 --%s 指定必填参数", name)),
			)
		}
	}
	return nil
}

func commandBoolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	value, _ := cmd.Flags().GetBool(name)
	if !value {
		value, _ = cmd.InheritedFlags().GetBool(name)
	}
	return value
}

func boolPtr(v bool) *bool {
	return &v
}
