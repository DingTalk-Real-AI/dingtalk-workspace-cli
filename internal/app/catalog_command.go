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

package app

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/spf13/cobra"
)

// toolMappingParam 描述一个 MCP 参数到 CLI flag + 中文友好名的映射。
type toolMappingParam struct {
	Flag  string `json:"flag"`
	Label string `json:"label"`
	Type  string `json:"type,omitempty"`
}

// toolMappingEntry 是单个 MCP 工具的映射条目。key 用 RPCName，对齐 SLS 日志的 tool 字段。
type toolMappingEntry struct {
	Product     string                      `json:"product"`
	CLICommand  string                      `json:"cliCommand"`
	DisplayName string                      `json:"displayName"`
	Params      map[string]toolMappingParam `json:"params,omitempty"`
}

// toolMapping 是给开放平台日志页渲染用的全量映射契约。
type toolMapping struct {
	Version string                      `json:"version"`
	Count   int                         `json:"count"`
	Tools   map[string]toolMappingEntry `json:"tools"`
}

// newCatalogCommand 提供 `dws catalog export`：把已发现的工具目录投影成
// tool→指令 映射 JSON，供开放平台 MCP/DWS 日志页把 tool/args 渲染成中文友好名。
// 复用 root 注入的带 auth 的 loader（缓存优先；建议先 `dws cache refresh`）。
func newCatalogCommand(loader cli.CatalogLoader) *cobra.Command {
	catalogCmd := &cobra.Command{
		Use:    "catalog",
		Short:  "导出已发现的工具目录（内部用）",
		Hidden: true,
	}

	var out string
	var version string
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出 tool→指令 映射 JSON（供开放平台日志页渲染）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, err := loader.Load(cmd.Context())
			if err != nil {
				return err
			}
			mapping := projectToolMapping(catalog, version)
			data, err := json.MarshalIndent(mapping, "", "  ")
			if err != nil {
				return err
			}
			data = append(data, '\n')
			if strings.TrimSpace(out) == "" {
				_, werr := os.Stdout.Write(data)
				return werr
			}
			return os.WriteFile(out, data, 0o644)
		},
	}
	exportCmd.Flags().StringVar(&out, "out", "", "输出文件路径（默认 stdout）")
	exportCmd.Flags().StringVar(&version, "version", "dev", "版本号标记")

	catalogCmd.AddCommand(exportCmd)
	return catalogCmd
}

// projectToolMapping 把 ir.Catalog 投影成 toolMapping 契约。
func projectToolMapping(catalog ir.Catalog, version string) toolMapping {
	mapping := toolMapping{Version: version, Tools: make(map[string]toolMappingEntry)}
	for _, product := range catalog.Products {
		command := ""
		if product.CLI != nil {
			command = strings.TrimSpace(product.CLI.Command)
		}
		if command == "" {
			command = product.ID
		}
		for _, tool := range product.Tools {
			if tool.Hidden {
				continue
			}
			entry := toolMappingEntry{
				Product:     command,
				CLICommand:  tmBuildCLICommand(command, tool),
				DisplayName: tmFirstNonEmpty(tool.Title, tmFirstNonEmpty(tmFirstLine(tool.Description), tool.RPCName)),
				Params:      make(map[string]toolMappingParam),
			}
			for name, raw := range tmSchemaProperties(tool.InputSchema) {
				prop, _ := raw.(map[string]any)
				overlay, hasOverlay := tool.FlagOverlay[name]
				if hasOverlay && overlay.Hidden {
					continue
				}
				flag := tmKebab(name)
				if hasOverlay && strings.TrimSpace(overlay.Alias) != "" {
					flag = strings.TrimSpace(overlay.Alias)
				}
				label := tmMapStr(prop, "title")
				if label == "" {
					label = tmFirstLine(tmMapStr(prop, "description"))
				}
				entry.Params[name] = toolMappingParam{
					Flag:  flag,
					Label: label,
					Type:  tmMapStr(prop, "type"),
				}
			}
			if len(entry.Params) == 0 {
				entry.Params = nil
			}
			mapping.Tools[tool.RPCName] = entry
		}
	}
	mapping.Count = len(mapping.Tools)
	return mapping
}

// tmBuildCLICommand 拼出 CLI 命令路径，如 chat + message + list -> "chat message list"。
func tmBuildCLICommand(command string, tool ir.ToolDescriptor) string {
	parts := make([]string, 0, 3)
	if command != "" {
		parts = append(parts, command)
	}
	if g := strings.TrimSpace(tool.Group); g != "" {
		parts = append(parts, g)
	}
	name := strings.TrimSpace(tool.CLIName)
	if name == "" {
		name = tool.RPCName
	}
	parts = append(parts, name)
	return strings.Join(parts, " ")
}

func tmSchemaProperties(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	props, _ := schema["properties"].(map[string]any)
	return props
}

func tmMapStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

// tmFirstLine 取第一句中文/换行前的片段，作为长描述的短标签兜底。
func tmFirstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\n。"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func tmFirstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

// tmKebab 把 camelCase 参数名转 kebab-case 作为默认 flag。
func tmKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
