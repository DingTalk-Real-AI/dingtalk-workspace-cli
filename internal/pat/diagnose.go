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
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// diagnoseResult is the shape emitted by `dws pat status --diagnose`.
// Fields are intentionally restricted to derived booleans and non-secret
// metadata — never the env value, OAuth client id, PAT flow id or access
// token — so the output is safe to paste into issues / logs.
type diagnoseResult struct {
	HostOwnedPAT        bool     `json:"hostOwnedPAT"`
	AgentCodeEnvPresent bool     `json:"agentCodeEnvPresent"`
	AgentCodeEnvName    string   `json:"agentCodeEnvName"`
	Edition             string   `json:"edition"`
	Version             string   `json:"version"`
	Hints               []string `json:"hints"`
}

// runDiagnose renders the host-owned PAT decision for the current process
// without performing any network or credential I/O. It is a pure local
// view: keychain, disk tokens, MCP and OAuth endpoints are NOT touched.
func runDiagnose(cmd *cobra.Command, format string) error {
	res := collectDiagnose(cmd)

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return writeDiagnoseJSON(cmd.OutOrStdout(), res)
	case "", "text", "human":
		return writeDiagnoseHuman(cmd.OutOrStdout(), res)
	default:
		return fmt.Errorf("unsupported --format %q (allowed: json, text)", format)
	}
}

// collectDiagnose builds the diagnoseResult. It reads ONLY derived booleans
// from HostOwnsPATFlow() and checks env presence — never the raw value.
func collectDiagnose(cmd *cobra.Command) diagnoseResult {
	envPresent := os.Getenv(authpkg.AgentCodeEnv) != ""
	hostOwned := authpkg.HostOwnsPATFlow()

	editionName := edition.Get().Name
	if editionName == "" {
		editionName = "open"
	}

	ver := "dev"
	if cmd != nil {
		if root := cmd.Root(); root != nil && strings.TrimSpace(root.Version) != "" {
			ver = root.Version
		}
	}

	hints := []string{}
	if !hostOwned && !envPresent {
		hints = append(hints,
			fmt.Sprintf("%s 未设置；请 export 该环境变量或由宿主传入", authpkg.AgentCodeEnv))
	}

	return diagnoseResult{
		HostOwnedPAT:        hostOwned,
		AgentCodeEnvPresent: envPresent,
		AgentCodeEnvName:    authpkg.AgentCodeEnv,
		Edition:             editionName,
		Version:             ver,
		Hints:               hints,
	}
}

func writeDiagnoseJSON(w io.Writer, res diagnoseResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

func writeDiagnoseHuman(w io.Writer, res diagnoseResult) error {
	envState := "unset"
	if res.AgentCodeEnvPresent {
		envState = "set"
	}
	conclusion := "自行渲染 PAT 授权 UI"
	if res.HostOwnedPAT {
		conclusion = "走 host-owned 单行 JSON 路径"
	}

	fmt.Fprintln(w, "PAT Host-Control Diagnose")
	fmt.Fprintf(w, "  Edition         : %s (Version: %s)\n", res.Edition, res.Version)
	fmt.Fprintf(w, "  Env var         : %s = <%s>\n", res.AgentCodeEnvName, envState)
	fmt.Fprintf(w, "  hostOwnedPAT()  : %t\n", res.HostOwnedPAT)
	fmt.Fprintf(w, "  → 结论: CLI 将%s\n", conclusion)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "如未生效，请检查：")
	fmt.Fprintln(w, "  1. 该环境变量必须通过 `export` 传给子进程（仅 `VAR=value` 不够）")
	fmt.Fprintln(w, "  2. 检查 shell 的 `hash -r` 或 $PATH 是否指向不同版本的 dws")
	fmt.Fprintln(w, "  3. 运行 `which -a dws` 确认实际执行的 binary")
	for _, h := range res.Hints {
		fmt.Fprintf(w, "  - 提示: %s\n", h)
	}
	return nil
}
