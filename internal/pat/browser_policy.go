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

package pat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const browserPolicyFile = "pat_policy.json"

type browserPolicyValue struct {
	OpenBrowser bool `json:"openBrowser"`
}

type BrowserPolicy struct {
	Default *browserPolicyValue           `json:"default,omitempty"`
	Agents  map[string]browserPolicyValue `json:"agents,omitempty"`
}

type BrowserPolicySelection struct {
	Scope       string `json:"scope"`
	AgentCode   string `json:"agentCode,omitempty"`
	OpenBrowser bool   `json:"openBrowser"`
	Source      string `json:"source"`
}

func patPolicyPath(configDir string) string {
	return filepath.Join(configDir, browserPolicyFile)
}

func patConfigDir() string {
	if envDir := os.Getenv("DWS_CONFIG_DIR"); envDir != "" {
		return envDir
	}
	if fn := edition.Get().ConfigDir; fn != nil {
		return fn()
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".dws"
	}
	return filepath.Join(homeDir, ".dws")
}

func LoadBrowserPolicy(configDir string) (*BrowserPolicy, error) {
	path := patPolicyPath(configDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &BrowserPolicy{}, nil
		}
		return nil, fmt.Errorf("reading PAT browser policy: %w", err)
	}

	var policy BrowserPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("parsing PAT browser policy: %w", err)
	}
	if policy.Agents == nil {
		policy.Agents = map[string]browserPolicyValue{}
	}
	return &policy, nil
}

func saveBrowserPolicy(configDir string, policy *BrowserPolicy) error {
	if policy == nil {
		policy = &BrowserPolicy{}
	}
	if policy.Agents == nil {
		policy.Agents = map[string]browserPolicyValue{}
	}
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling PAT browser policy: %w", err)
	}
	data = append(data, '\n')
	if err := helpers.AtomicWriteJSON(patPolicyPath(configDir), data); err != nil {
		return fmt.Errorf("writing PAT browser policy: %w", err)
	}
	return nil
}

func ResolveBrowserPolicy(configDir, explicitAgentCode string) (BrowserPolicySelection, error) {
	agentCode, err := resolveAgentCode(explicitAgentCode, false)
	if err != nil {
		return BrowserPolicySelection{}, err
	}
	policy, err := LoadBrowserPolicy(configDir)
	if err != nil {
		return BrowserPolicySelection{}, err
	}

	if agentCode != "" {
		if entry, ok := policy.Agents[agentCode]; ok {
			return BrowserPolicySelection{
				Scope:       "agent",
				AgentCode:   agentCode,
				OpenBrowser: entry.OpenBrowser,
				Source:      "agent",
			}, nil
		}
	}

	if policy.Default != nil {
		return BrowserPolicySelection{
			Scope:       "default",
			OpenBrowser: policy.Default.OpenBrowser,
			Source:      "default",
		}, nil
	}

	return BrowserPolicySelection{
		Scope:       "builtin_default",
		AgentCode:   agentCode,
		OpenBrowser: true,
		Source:      "builtin_default",
	}, nil
}

func EffectiveOpenBrowser(configDir string) bool {
	selection, err := ResolveBrowserPolicy(configDir, "")
	if err != nil {
		return true
	}
	return selection.OpenBrowser
}

func resolveBrowserPolicyWriteAgentCode(explicitAgentCode string) (string, error) {
	agentCode := strings.TrimSpace(explicitAgentCode)
	if agentCode == "" {
		return "", nil
	}
	if err := validateAgentCode(agentCode); err != nil {
		return "", err
	}
	return agentCode, nil
}

func SetBrowserPolicy(configDir, explicitAgentCode string, enabled bool) (BrowserPolicySelection, error) {
	agentCode, err := resolveBrowserPolicyWriteAgentCode(explicitAgentCode)
	if err != nil {
		return BrowserPolicySelection{}, err
	}

	policy, err := LoadBrowserPolicy(configDir)
	if err != nil {
		return BrowserPolicySelection{}, err
	}
	if policy.Agents == nil {
		policy.Agents = map[string]browserPolicyValue{}
	}

	if agentCode != "" {
		policy.Agents[agentCode] = browserPolicyValue{OpenBrowser: enabled}
		if err := saveBrowserPolicy(configDir, policy); err != nil {
			return BrowserPolicySelection{}, err
		}
		return BrowserPolicySelection{
			Scope:       "agent",
			AgentCode:   agentCode,
			OpenBrowser: enabled,
			Source:      "agent",
		}, nil
	}

	policy.Default = &browserPolicyValue{OpenBrowser: enabled}
	if err := saveBrowserPolicy(configDir, policy); err != nil {
		return BrowserPolicySelection{}, err
	}
	return BrowserPolicySelection{
		Scope:       "default",
		OpenBrowser: enabled,
		Source:      "default",
	}, nil
}

func newBrowserPolicyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "browser-policy",
		Short:             "配置 PAT 授权时是否打开浏览器",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("enabled") {
				return fmt.Errorf("--enabled is required")
			}

			enabled, err := cmd.Flags().GetBool("enabled")
			if err != nil {
				return err
			}
			agentCode, err := cmd.Flags().GetString("agentCode")
			if err != nil {
				return err
			}

			selection, err := SetBrowserPolicy(patConfigDir(), agentCode, enabled)
			if err != nil {
				return err
			}
			return output.WriteJSON(cmd.OutOrStdout(), selection)
		},
	}

	cmd.Flags().Bool("enabled", false, "PAT 撞墙时是否允许本地打开浏览器")
	cmd.Flags().String("agentCode", "", "Agent 唯一标识（可选；不填则写入全局默认策略，不从 env DINGTALK_DWS_AGENTCODE 回退）")
	return cmd
}
