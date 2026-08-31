package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// 跨产品透明路由（Proxy Route）
//
// 目的：
//   解决 LLM Agent 按直觉路径调用不存在命令的问题。例如 Agent 会尝试
//   `dws wiki node list`，但该操作实际由 doc 产品实现（`dws doc list`）。
//   这是因为 dws 的命令归属由底层 MCP Server 的 tool 划分决定，而非用户
//   认知模型决定。wiki/doc/drive 三个产品属于同一"文档域"，概念交织。
//
// 解决方案：
//   在源产品（wiki/drive）中注册 hidden 子命令，当被调用时透明转发到
//   目标产品（doc）的对应命令。对调用者完全透明，首次即成功。
//
// 适用范围：
//   仅 wiki → doc、drive → doc 两条链路。其他 15+ 产品边界清晰，无此问题。
//
// 全局搜索标识：
//   grep "// [PROXY]" 可定位所有路由声明点
//   grep "proxySubCmd(" 可定位所有转发命令注册处
// ──────────────────────────────────────────────────────────

// proxySubCmd 创建一个 hidden 子命令，被调用时透明转发到目标命令。
//
// 与 hintSubCmd 的区别：hintSubCmd 打印提示后退出（Agent 需重试），
// proxySubCmd 直接执行目标命令（首次即成功）。
//
// 目标命令通过延迟解析获取（运行时从命令树查找），因为 wiki.go 初始化时
// doc 命令尚未挂载到 root 上。
//
// 参数：
//   - use:           子命令名（如 "list"、"read"）
//   - targetProduct: 目标产品名（如 "doc"）
//   - targetPath:    目标子路径，空格分隔（如 "list"、"block insert"）
//   - flagRenames:   flag 重命名映射（如 {"workspace": "workspace-ids"}），nil 表示全部透传
func proxySubCmd(use, targetProduct, targetPath string, flagRenames map[string]string) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Hidden:             true,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 延迟解析：运行时从命令树查找目标
			root := cmd.Root()
			var targetCmd *cobra.Command
			for _, c := range root.Commands() {
				if c.Name() == targetProduct {
					targetCmd = c
					break
				}
			}
			if targetCmd != nil && targetPath != "" {
				for _, part := range strings.Fields(targetPath) {
					found := false
					for _, child := range targetCmd.Commands() {
						if child.Name() == part {
							targetCmd = child
							found = true
							break
						}
					}
					if !found {
						targetCmd = nil
						break
					}
				}
			}
			if targetCmd == nil {
				return fmt.Errorf("proxy target not found: dws %s %s", targetProduct, targetPath)
			}

			fmt.Fprintf(os.Stderr, "→ redirecting to: %s\n", targetCmd.CommandPath())

			// flag 重命名（仅在有映射时分配新 slice）
			finalArgs := args
			if len(flagRenames) > 0 {
				finalArgs = make([]string, len(args))
				copy(finalArgs, args)
				for i, arg := range finalArgs {
					if !strings.HasPrefix(arg, "--") {
						continue
					}
					flagPart := strings.TrimPrefix(arg, "--")
					eqIdx := strings.Index(flagPart, "=")
					var flagName string
					if eqIdx >= 0 {
						flagName = flagPart[:eqIdx]
					} else {
						flagName = flagPart
					}
					if newName, ok := flagRenames[flagName]; ok {
						if eqIdx >= 0 {
							finalArgs[i] = "--" + newName + "=" + flagPart[eqIdx+1:]
						} else {
							finalArgs[i] = "--" + newName
						}
					}
				}
			}

			// 直接调用目标命令的 RunE，绕过 root.Execute() 避免无限递归
			if targetCmd.DisableFlagParsing {
				if targetCmd.RunE != nil {
					return targetCmd.RunE(targetCmd, finalArgs)
				}
				if targetCmd.Run != nil {
					targetCmd.Run(targetCmd, finalArgs)
					return nil
				}
			} else {
				if err := targetCmd.ParseFlags(finalArgs); err != nil {
					return fmt.Errorf("proxy flag parse error for %q: %w", targetCmd.CommandPath(), err)
				}
				targetArgs := targetCmd.Flags().Args()
				if targetCmd.RunE != nil {
					return targetCmd.RunE(targetCmd, targetArgs)
				}
				if targetCmd.Run != nil {
					targetCmd.Run(targetCmd, targetArgs)
					return nil
				}
			}
			return fmt.Errorf("proxy target %q has no RunE/Run", targetCmd.CommandPath())
		},
	}
}

// ──────────────────────────────────────────────────────────
// dws wiki — 知识库
// ──────────────────────────────────────────────────────────

func newWikiCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/wiki.json
	// products.wiki). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "wiki",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-wiki"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("知识库深度指南", "dingtalk-wiki", "references/wiki.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "管理钉钉知识库空间、节点与成员权限",
			UseWhen: []string{
				"查找或管理知识库、知识库内节点及知识库成员时",
			},
			AvoidWhen: []string{
				"需要编辑在线文档正文时使用 doc；只管理钉盘普通文件时使用 drive",
			},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "wiki",
		Short: "知识库 / 空间管理 / 节点管理 / 成员管理 / 动态查询",
		Long:  `管理钉钉文档知识库：空间管理（创建/查看/列出/搜索/删除）、节点管理（列出/创建/复制/移动/删除）、成员管理（添加/更新/列出/移除）、动态查询（知识库活动动态）。`,
		RunE:  groupRunE,
	})
	installDocDelegationAuth(root)

	spaceCmd := newGroupCommand(&cobra.Command{Use: "space", Short: "知识库管理", RunE: groupRunE})

	spaceCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建知识库",
		Long: `创建一个新的钉钉文档知识库（WikiSpace）。

创建成功后返回新知识库的 workspaceId，可用于后续在该知识库下创建文档或遍历文件。
操作受权限控制，仅当调用者具备在当前组织内创建知识库的权限时可成功创建。`,
		Example: `  dws wiki space create --name "产品文档库"
  dws wiki space create --name "技术方案" --desc "团队技术方案归档"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"name": mustGetFlag(cmd, "name"),
			}
			if v := flagOrFallback(cmd, "desc", "description"); v != "" {
				toolArgs["description"] = v
			}
			if v := mustGetFlag(cmd, "icon"); v != "" {
				toolArgs["icon"] = v
			}
			return callMCPTool("create_wikiSpace", toolArgs)
		},
	}
	DeclareLeafMetadata(spaceCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "create_wikiSpace",
				CanonicalPath:  "wiki.create_wikiSpace",
				CLIPath:        "wiki space create",
				PrimaryCLIPath: "wiki space create",
			},
			Description: "创建一个新的钉钉文档知识库（WikiSpace）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "create_wikiSpace"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建一个新的钉钉文档知识库（WikiSpace）",
				UseWhen:      []string{"用户要新建一个知识库容器时"},
				AvoidWhen:    []string{"在已有知识库内建文档/文件夹用 node create，不要反复 create space"},
				Examples: []string{
					"dws wiki space create --name \"产品文档库\" --format json",
					"dws wiki space create --name \"技术方案\" --desc \"团队技术方案归档\" --format json",
				},
			},
			// name is validated in RunE; publish required via ParamDecl
			Parameters: []contract.ParamDecl{
				{Name: "desc", Property: "description"},
				{Name: "name", Required: boolPtr(true)},
			},
		},
	})

	spaceGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查看知识库详情",
		Long: `获取指定知识库的详细信息，包括名称、描述、创建者、创建时间、成员数量等。

支持传入知识库 ID 或知识库 URL，系统自动识别。
知识库 URL 格式：https://alidocs.dingtalk.com/i/spaces/{workspaceId}/overview`,
		Example: `  dws wiki space get --workspace <workspaceId>
  dws wiki space get --workspace "https://alidocs.dingtalk.com/i/spaces/xxx/overview"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID := flagOrFallback(cmd, "workspace", "workspace-id")
			if workspaceID == "" {
				return fmt.Errorf("flag --workspace is required")
			}
			return callMCPTool("get_wikiSpace", map[string]any{
				"workspaceId": workspaceID,
			})
		},
	}
	DeclareLeafMetadata(spaceGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "get_wikiSpace",
				CanonicalPath:  "wiki.get_wikiSpace",
				CLIPath:        "wiki space get",
				PrimaryCLIPath: "wiki space get",
			},
			Description: "获取指定知识库的详细信息，包括名称、描述、创建者、创建时间、成员数量等",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "get_wikiSpace"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定知识库的详细信息，包括名称、描述、创建者、创建时间、成员数量等",
				UseWhen:      []string{"查看指定知识库详情（名称、描述、创建者等）时"},
				AvoidWhen:    []string{"列知识库用 list；删库用 space delete（需确认）"},
				Examples:     []string{"dws wiki space get --workspace <workspaceId> --format json"},
			},
			// workspace is validated in RunE; publish required via ParamDecl
			Parameters: []contract.ParamDecl{
				{Name: "workspace", Property: "workspaceId", Required: boolPtr(true)},
			},
		},
	})

	spaceListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出空间（知识库 / 钉盘空间）",
		Long: `获取当前用户有权访问的空间列表。统一管理两种空间类型。

通过 --type 参数控制返回范围：
  orgWikiSpace  — 组织知识库列表（默认，支持分页）
  myWikiSpace   — 当前用户的「我的文档」个人空间（固定 1 条）
  orgSpace      — 钉盘企业空间（团队文件）列表
  mySpace       — 钉盘「我的文件」个人空间`,
		Example: `  dws wiki space list
  dws wiki space list --type myWikiSpace
  dws wiki space list --type orgWikiSpace --limit 50
  dws wiki space list --type orgSpace
  dws wiki space list --type mySpace`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceType := mustGetFlag(cmd, "type")

			// 钉盘空间类型：路由到 drive MCP server
			if spaceType == "orgSpace" || spaceType == "mySpace" {
				driveArgs := map[string]any{"spaceType": spaceType}
				if v := mustGetFlag(cmd, "limit"); v != "" {
					if n, err := strconv.Atoi(v); err == nil {
						driveArgs["maxResults"] = n
					} else {
						driveArgs["maxResults"] = v
					}
				}
				if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
					driveArgs["nextToken"] = v
				}
				return callMCPToolOnServer("drive", "list_spaces", driveArgs)
			}

			// 文档知识库类型：原有逻辑
			toolArgs := map[string]any{}
			if spaceType != "" {
				toolArgs["wikiSpaceType"] = spaceType
			}
			if v := mustGetFlag(cmd, "limit"); v != "" {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
				toolArgs["pageToken"] = v
			}
			return callMCPTool("list_wikiSpaces", toolArgs)
		},
	}
	DeclareLeafMetadata(spaceListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "list_wikiSpaces",
				CanonicalPath:  "wiki.list_wikiSpaces",
				CLIPath:        "wiki space list",
				PrimaryCLIPath: "wiki space list",
			},
			Description: "列出当前用户可访问的知识库或钉盘空间",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The CLI command routes by --type between wiki/list_wikiSpaces and drive/list_spaces, so the reviewed executable wrapper has no single direct MCP interface.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出当前用户可访问的知识库或钉盘空间",
				UseWhen: []string{
					"列出组织知识库（默认）、我的文档(--type myWikiSpace)、钉盘企业空间(--type orgSpace)或我的文件(--type mySpace)时",
					"需要 workspaceId / rootFolderId 作为后续 node/drive 操作前置时",
				},
				AvoidWhen: []string{
					"按名称搜知识库用 space search",
					"看单个知识库详情用 space get",
				},
				Examples: []string{
					"dws wiki space list --format json",
					"dws wiki space list --type myWikiSpace --format json",
				},
			},
		},
	})

	spaceSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索知识库",
		Long:  `根据关键词搜索当前用户有权限访问的知识库列表，匹配知识库名称和描述。`,
		Example: `  dws wiki space search --query "产品文档"
  dws wiki space search --query "技术方案" --limit 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceType := mustGetFlag(cmd, "type")

			// 「我的文档」场景：走 list_wikiSpaces 获取个人空间
			if spaceType == "myWikiSpace" {
				return callMCPTool("list_wikiSpaces", map[string]any{
					"wikiSpaceType": "myWikiSpace",
				})
			}

			// 常规搜索场景：query 必填
			query := flagOrFallback(cmd, "query", "keyword")
			if query == "" {
				return fmt.Errorf("flag --query is required")
			}
			toolArgs := map[string]any{
				"keyword": query,
			}
			if v := mustGetFlag(cmd, "limit"); v != "" {
				toolArgs["pageSize"] = v
			}
			return callMCPTool("search_wikiSpaces", toolArgs)
		},
	}
	DeclareLeafMetadata(spaceSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "search_wikiSpaces",
				CanonicalPath:  "wiki.search_wikiSpaces",
				CLIPath:        "wiki space search",
				PrimaryCLIPath: "wiki space search",
			},
			Description: "根据关键词搜索当前用户有权限访问的知识库列表，匹配知识库名称和描述",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "search_wikiSpaces"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "根据关键词搜索当前用户有权限访问的知识库列表，匹配知识库名称和描述",
				UseWhen:      []string{"按关键词搜索有权访问的组织知识库时"},
				AvoidWhen:    []string{"直接列全部知识库用 space list；我的文档用 list --type myWikiSpace"},
				Examples:     []string{"dws wiki space search --query \"产品文档\" --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "pageSize"},
				{Name: "query", Property: "keyword"},
			},
		},
	})

	// space create flags
	spaceCreateCmd.Flags().String("name", "", "知识库名称 (必填，不超过 32 字符)")
	spaceCreateCmd.Flags().String("desc", "", "知识库描述 (选填，不超过 500 字符)")
	spaceCreateCmd.Flags().String("icon", "", "知识库图标标识 (选填)")

	// space get flags — primary is --workspace, consistent with doc commands.
	// --workspace-id is hidden alias: LLMs derive it from API response field "workspaceId" (camelCase → kebab-case).
	spaceGetCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	spaceGetCmd.Flags().String("workspace-id", "", "")
	_ = spaceGetCmd.Flags().MarkHidden("workspace-id")

	// space list flags
	spaceListCmd.Flags().String("type", "orgWikiSpace", "空间类型: orgWikiSpace(默认) / myWikiSpace / orgSpace(钉盘企业空间) / mySpace(钉盘我的文件)")
	spaceListCmd.Flags().String("limit", "", "每页数量 1-50 (默认 20)")
	spaceListCmd.Flags().String("cursor", "", "分页游标 (首页留空)")

	// space search flags
	spaceSearchCmd.Flags().String("query", "", "搜索关键词 (搜索组织知识库时必填)")
	spaceSearchCmd.Flags().String("type", "", "知识库类型: myWikiSpace 时直接返回「我的文档」，省略则搜索组织知识库")
	spaceSearchCmd.Flags().String("limit", "", "返回数量 1-20 (默认 10)")

	// ── cross-product hidden aliases ──
	for _, cmd := range []*cobra.Command{spaceCreateCmd, spaceGetCmd, spaceListCmd, spaceSearchCmd} {
		RegisterCrossProductAliases(cmd)
	}

	spaceDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除知识库",
		Long: `将指定知识库移入回收站。

删除后知识库会进入回收站，可在回收站中恢复（有保留期限）。
支持传入知识库 ID 或知识库 URL，系统自动识别。
知识库 URL 格式：https://alidocs.dingtalk.com/i/spaces/{workspaceId}/overview

注意：
- 操作者必须具备知识库的 OWNER 角色。
- 这是一个危险操作，执行前请确认。`,
		Example: `  dws wiki space delete --workspace <workspaceId>
  dws wiki space delete --workspace "https://alidocs.dingtalk.com/i/spaces/xxx/overview"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			return callMCPTool("delete_wikiSpace", map[string]any{
				"workspaceId": workspaceID,
			})
		},
	}
	DeclareLeafMetadata(spaceDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "delete_wikiSpace",
				CanonicalPath:  "wiki.delete_wikiSpace",
				CLIPath:        "wiki space delete",
				PrimaryCLIPath: "wiki space delete",
			},
			Description: "将指定知识库移入回收站",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "delete_wikiSpace"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将指定知识库移入回收站",
				UseWhen:      []string{"用户明确要求删除整个知识库（移入回收站），且已确认 workspace 与影响范围时"},
				AvoidWhen: []string{
					"只删库内某个节点用 wiki node delete",
					"未确认或只要看详情用 space get",
				},
				Examples: []string{"dws wiki space delete --workspace <workspaceId> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	spaceDeleteCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	spaceDeleteCmd.Flags().String("workspace-id", "", "")
	_ = spaceDeleteCmd.Flags().MarkHidden("workspace-id")
	RegisterCrossProductAliases(spaceDeleteCmd)

	spaceCmd.AddCommand(spaceCreateCmd, spaceGetCmd, spaceListCmd, spaceSearchCmd, spaceDeleteCmd)

	// ── member (知识库成员管理) ───────────────────────────────
	memberCmd := newGroupCommand(&cobra.Command{
		Use:   "member",
		Short: "知识库成员管理",
		Long:  `管理钉钉知识库的成员：添加成员、更新成员权限、查询成员列表、移除成员。`,
		RunE:  groupRunE,
	})

	memberAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加知识库成员",
		Args:  cobra.NoArgs,
		Long: `为指定知识库添加一个或多个成员，并授予指定角色。

两种传参方式（互斥）：
  旧格式：--users 传入逗号分隔的 userId 列表 + --role 指定统一角色（仅 USER 类型）
  新格式：--members 传入 JSON 数组，支持四种成员类型，每个 member 携带独立 roleId

成员类型说明：
  USER          用户，id 为用户 userId，需携带 corpId（标识用户所属组织）
  DEPT          部门，id 为部门 ID，需携带 corpId（标识部门所属组织）
  CONVERSATION  群聊，id 为群聊 conversationId（cid 开头），无需 corpId
  TAG           角色标签（也称角色组），id 为角色标签 ID，需携带 corpId。当用户要求"添加角色组"或"添加角色标签"时使用此类型

支持的角色（大小写不敏感）：
  MANAGER     管理员，可读写、管理成员
  EDITOR      编辑者，可查看、编辑、上传内容
  DOWNLOADER  查看下载者，可查看并下载内容
  READER      仅可查看者，仅可查看，不可下载

注意：
- OWNER 角色不可通过此接口添加，知识库创建者默认为所有者。
- 操作者须满足该知识库配置的权限管理最低角色要求（默认 MANAGER，可配置为 EDITOR 等），权限不足返回 forbidden.accessDenied。
- 单次请求最多 30 个成员，超出请分批调用。
- --notify 仅在 --members 新格式时生效，仅对 USER 和 CONVERSATION 类型成员发送通知（DEPT 和 TAG 不通知），默认 false；省略时 CLI 不向服务端发送该字段，服务端按不通知处理，需要通知请显式传 --notify。

支持通过 --workspace 传入知识库 ID 或知识库 URL，系统自动识别。
用户 uid 可通过「钉钉通讯录」相关命令检索，如:
  dws contact user search --keyword "姓名"`,
		Example: `  dws wiki member add --workspace <workspaceId> --users uid1 --role READER
  dws wiki member add --workspace <workspaceId> --users uid1,uid2,uid3 --role EDITOR
  dws wiki member add --workspace "https://alidocs.dingtalk.com/i/spaces/xxx/overview" --users uid1 --role MANAGER
  dws wiki member add --workspace <workspaceId> --members '[{"type":"USER","id":"uid1","roleId":"READER","corpId":"xxx"},{"type":"DEPT","id":"deptId1","roleId":"EDITOR","corpId":"xxx"}]' --notify
  dws wiki member add --workspace <workspaceId> --members '[{"type":"CONVERSATION","id":"cidXXX","roleId":"READER"},{"type":"TAG","id":"tagId1","roleId":"EDITOR","corpId":"xxx"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			if err := validateMembersExclusivity(cmd); err != nil {
				return err
			}
			toolArgs := map[string]any{"workspaceId": workspaceID}
			members, mErr := collectMembers(cmd, false)
			if mErr != nil {
				return mErr
			}
			if len(members) > 0 {
				for _, m := range members {
					if r, _ := m["roleId"].(string); r == "OWNER" {
						return apperrors.NewValidation("OWNER 角色不可通过 wiki member add 添加")
					}
				}
				toolArgs["members"] = members
				if cmd.Flags().Changed("notify") {
					notify, _ := cmd.Flags().GetBool("notify")
					toolArgs["notify"] = notify
				}
			} else {
				if err := validateRequiredFlags(cmd, "role"); err != nil {
					return err
				}
				role := normalizePermissionRole(mustGetFlag(cmd, "role"))
				if role == "OWNER" {
					return apperrors.NewValidation("OWNER 角色不可通过 wiki member add 添加")
				}
				userIds, err := collectUserIDs(cmd)
				if err != nil {
					return err
				}
				toolArgs["roleId"] = role
				toolArgs["userIds"] = userIds
			}
			return callMCPTool("add_member", toolArgs)
		},
	}
	DeclareLeafMetadata(memberAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "add_member",
				CanonicalPath:  "wiki.add_member",
				CLIPath:        "wiki member add",
				PrimaryCLIPath: "wiki member add",
			},
			Description: "为指定知识库添加一个或多个成员，并授予指定角色",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "add_member"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为指定知识库添加一个或多个成员，并授予指定角色",
				UseWhen:      []string{"给知识库容器添加成员（USER + 角色）；新员工入职开通整库访问时"},
				AvoidWhen: []string{
					"「我的文档」不支持容器成员——改用 drive/doc permission add 做节点授权",
					"单篇文档分享用节点 permission，不要用 member add",
				},
				Examples: []string{
					"dws wiki member add --workspace <WS_ID> --users uid1 --role READER --format json",
					"dws wiki member add --workspace <WS_ID> --users uid1,uid2 --role EDITOR --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "members", Property: "members"},
				{Name: "notify", Property: "notify"},
				{Name: "role", Property: "roleId"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	memberAddCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	memberAddCmd.Flags().String("users", "", "被添加的用户 userId 列表，逗号分隔 (旧格式，单次最多 30 个)")
	memberAddCmd.Flags().String("user", "", "")
	_ = memberAddCmd.Flags().MarkHidden("user")
	memberAddCmd.Flags().String("role", "", "权限角色: MANAGER / EDITOR / DOWNLOADER / READER (旧格式必填，大小写不敏感)")
	memberAddCmd.Flags().String("members", "", "成员列表 JSON 数组（新格式），支持 USER/DEPT/CONVERSATION/TAG 类型（TAG=角色组），与 --users 互斥")
	memberAddCmd.Flags().Bool("notify", false, "是否通知被添加的成员（仅 --members 新格式时生效，需显式传入才通知）")

	memberUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新知识库成员权限",
		Args:  cobra.NoArgs,
		Long: `更新指定知识库已有成员的角色。

两种传参方式（互斥）：
  旧格式：--users 传入逗号分隔的 userId 列表 + --role 指定统一角色（仅 USER 类型）
  新格式：--members 传入 JSON 数组，支持四种成员类型，每个 member 携带独立 roleId

成员类型说明：
  USER          用户，id 为用户 userId，需携带 corpId
  DEPT          部门，id 为部门 ID，需携带 corpId
  CONVERSATION  群聊，id 为群聊 conversationId（cid 开头），无需 corpId
  TAG           角色标签（也称角色组），id 为角色标签 ID，需携带 corpId

支持的角色（大小写不敏感）：
  MANAGER     管理员
  EDITOR      编辑者
  DOWNLOADER  查看下载者
  READER      仅可查看者

注意：
- OWNER 角色不可通过此接口变更。
- 同一成员在同一知识库只能拥有一个角色，变更后旧角色自动替换。
- 操作者须满足该知识库配置的权限管理最低角色要求（默认 MANAGER，可配置为 EDITOR 等），权限不足返回 forbidden.accessDenied。
- --notify 仅在 --members 新格式时生效，仅对 USER 和 CONVERSATION 类型成员发送通知，默认 false。

仅可更新已存在成员关系的成员，新增成员请使用 dws wiki member add。`,
		Example: `  dws wiki member update --workspace <workspaceId> --users uid1 --role EDITOR
  dws wiki member update --workspace <workspaceId> --users uid1,uid2 --role READER
  dws wiki member update --workspace <workspaceId> --members '[{"type":"USER","id":"uid1","roleId":"EDITOR","corpId":"xxx"}]' --notify=false
  dws wiki member update --workspace <workspaceId> --members '[{"type":"TAG","id":"tagId1","roleId":"READER","corpId":"xxx"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			if err := validateMembersExclusivity(cmd); err != nil {
				return err
			}
			toolArgs := map[string]any{"workspaceId": workspaceID}
			members, mErr := collectMembers(cmd, false)
			if mErr != nil {
				return mErr
			}
			if len(members) > 0 {
				toolArgs["members"] = members
				if cmd.Flags().Changed("notify") {
					notify, _ := cmd.Flags().GetBool("notify")
					toolArgs["notify"] = notify
				}
			} else {
				if err := validateRequiredFlags(cmd, "role"); err != nil {
					return err
				}
				userIds, err := collectUserIDs(cmd)
				if err != nil {
					return err
				}
				toolArgs["roleId"] = normalizePermissionRole(mustGetFlag(cmd, "role"))
				toolArgs["userIds"] = userIds
			}
			return callMCPTool("update_member", toolArgs)
		},
	}
	DeclareLeafMetadata(memberUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "update_member",
				CanonicalPath:  "wiki.update_member",
				CLIPath:        "wiki member update",
				PrimaryCLIPath: "wiki member update",
			},
			Description: "更新指定知识库已有成员的角色",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "update_member"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新指定知识库已有成员的角色",
				UseWhen:      []string{"调整知识库成员角色（如升为 EDITOR/MANAGER）时"},
				AvoidWhen:    []string{"新加成员用 add；移除用 remove；OWNER 不可经此变更"},
				Examples:     []string{"dws wiki member update --workspace <WS_ID> --users uid1 --role EDITOR --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "members", Property: "members"},
				{Name: "notify", Property: "notify"},
				{Name: "role", Property: "roleId"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	memberUpdateCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	memberUpdateCmd.Flags().String("users", "", "被更新的用户 userId 列表，逗号分隔 (旧格式，单次最多 30 个)")
	memberUpdateCmd.Flags().String("user", "", "")
	_ = memberUpdateCmd.Flags().MarkHidden("user")
	memberUpdateCmd.Flags().String("role", "", "新权限角色: MANAGER / EDITOR / DOWNLOADER / READER (旧格式必填，大小写不敏感)")
	memberUpdateCmd.Flags().String("members", "", "成员列表 JSON 数组（新格式），支持 USER/DEPT/CONVERSATION/TAG 类型（TAG=角色组），与 --users 互斥")
	memberUpdateCmd.Flags().Bool("notify", false, "是否通知被变更的成员（仅 --members 新格式时生效）")

	memberListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查询知识库成员列表",
		Long: `查询指定知识库的成员列表，返回每位成员的 userId、姓名、角色等信息。

底层一次性返回全量成员后在内存中按 pageSize 分页，支持通过 nextToken 翻页。
出参包含 totalCount（全量成员总数）、hasMore（是否还有下一页）和 nextToken（下一页游标）。
当 hasMore 为 true 时，传入下一次请求的 --next-token 即可获取下一页。
操作者需满足该知识库配置的权限管理最低角色要求，权限不足返回 forbidden.accessDenied。
ORG 类型授权不会出现在查询结果中。`,
		Example: `  dws wiki member list --workspace <workspaceId>
  dws wiki member list --workspace <workspaceId> --limit 50
  dws wiki member list --workspace <workspaceId> --filter-role MANAGER,EDITOR
  dws wiki member list --workspace <workspaceId> --next-token <上次返回的 nextToken>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"workspaceId": workspaceID,
			}
			if size, ok, err := permissionPageSizeFromFlags(cmd); err != nil {
				return err
			} else if ok {
				toolArgs["pageSize"] = size
			}
			if v := flagOrFallback(cmd, "next-token", "cursor", "page-token"); v != "" {
				toolArgs["nextToken"] = v
			}
			if v := mustGetFlag(cmd, "filter-role"); v != "" {
				toolArgs["filterRoleIds"] = parseRoleList(v)
			}
			return callMCPTool("list_member", toolArgs)
		},
	}
	DeclareLeafMetadata(memberListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "list_member",
				CanonicalPath:  "wiki.list_member",
				CLIPath:        "wiki member list",
				PrimaryCLIPath: "wiki member list",
			},
			Description: "查询指定知识库的成员列表，返回每位成员的 userId、姓名、角色等信息",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "list_member"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询指定知识库的成员列表，返回每位成员的 userId、姓名、角色等信息",
				UseWhen:      []string{"查看知识库成员名单与角色时"},
				AvoidWhen: []string{
					"返回无 userId：要 update/remove 需另用 contact user search 反查",
					"增删改成员用 add/update/remove",
				},
				Examples: []string{"dws wiki member list --workspace <WS_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "filter-role", Property: "filterRoleIds"},
				// limit 不声明 Property：运行时经 cap 校验（1-50）转换为 pageSize，
				// 属 CLI 分页输入而非 1:1 RPC property（reviewed mapping exclusion）。
				{Name: "limit"},
				{Name: "next-token", Property: "nextToken"},
				{Name: "workspace", Property: "workspaceId"},
			},
			Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "next-token"},
		},
	})

	memberListCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	memberListCmd.Flags().Int("limit", 30, "返回成员数上限，默认 30，最大 50")
	memberListCmd.Flags().Int("max-results", 0, "")
	_ = memberListCmd.Flags().MarkHidden("max-results")
	memberListCmd.Flags().String("filter-role", "", "按角色过滤（逗号分隔）：OWNER / MANAGER / EDITOR / DOWNLOADER / READER")
	memberListCmd.Flags().String("next-token", "", "分页游标，首次不传，后续传入上一次返回的 nextToken")

	memberRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "移除知识库成员",
		Long: `从指定知识库中移除一个或多个成员。

两种传参方式（互斥）：
  旧格式：--users 传入逗号分隔的 userId 列表（仅 USER 类型）
  新格式：--members 传入 JSON 数组，支持四种成员类型，只需 type 和 id（USER/DEPT/TAG 还需 corpId）

成员类型说明：
  USER          用户，id 为用户 userId，需携带 corpId
  DEPT          部门，id 为部门 ID，需携带 corpId
  CONVERSATION  群聊，id 为群聊 conversationId（cid 开头），无需 corpId
  TAG           角色标签（也称角色组），id 为角色标签 ID，需携带 corpId

移除后相关用户将无法访问该知识库下的内容（除非通过节点级权限另行授权）。

注意：
- OWNER 角色不可通过此接口移除。
- 操作者须满足该知识库配置的权限管理最低角色要求（默认 MANAGER，可配置为 EDITOR 等），权限不足返回 forbidden.accessDenied。
- 单次请求最多 30 个成员，超出请分批调用。`,
		Example: `  dws wiki member remove --workspace <workspaceId> --users uid1
  dws wiki member remove --workspace <workspaceId> --users uid1,uid2,uid3
  dws wiki member remove --workspace <workspaceId> --members '[{"type":"USER","id":"uid1","corpId":"xxx"},{"type":"DEPT","id":"deptId1","corpId":"xxx"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			if err := validateMembersExclusivity(cmd); err != nil {
				return err
			}
			toolArgs := map[string]any{"workspaceId": workspaceID}
			members, mErr := collectMembers(cmd, true)
			if mErr != nil {
				return mErr
			}
			if len(members) > 0 {
				toolArgs["members"] = members
			} else {
				userIds, err := collectUserIDs(cmd)
				if err != nil {
					return err
				}
				toolArgs["userIds"] = userIds
			}
			return callMCPTool("remove_member", toolArgs)
		},
	}
	DeclareLeafMetadata(memberRemoveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			// 批量移除（最多 30 个 USER/DEPT/CONVERSATION/TAG）会一次性撤销整个
			// 知识库容器级别的成员访问，部门/群聊/角色组还可能间接影响大量用户，
			// 与删除同级的 destructive 入口，必须经过用户确认（--yes 或交互 yes）。
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "remove_member",
				CanonicalPath:  "wiki.remove_member",
				CLIPath:        "wiki member remove",
				PrimaryCLIPath: "wiki member remove",
			},
			Description: "从指定知识库中移除一个或多个成员",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "remove_member"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "从指定知识库中移除一个或多个成员",
				UseWhen:      []string{"从知识库移除成员访问（离职/清理）时"},
				AvoidWhen: []string{
					"改角色用 update；节点级权限用 drive permission remove",
					"OWNER 不可移除此接口",
				},
				Examples: []string{"dws wiki member remove --workspace <WS_ID> --users uid1 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "members", Property: "members"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	memberRemoveCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	memberRemoveCmd.Flags().String("users", "", "被移除的用户 userId 列表，逗号分隔 (旧格式，单次最多 30 个)")
	memberRemoveCmd.Flags().String("user", "", "")
	_ = memberRemoveCmd.Flags().MarkHidden("user")
	memberRemoveCmd.Flags().String("members", "", "成员列表 JSON 数组（新格式），只需 type 和 id（USER/DEPT/TAG 还需 corpId），与 --users 互斥")

	// member 子命令的 --workspace-id 隐藏别名（LLMs derive from API field "workspaceId"）
	memberAliasCmds := []*cobra.Command{memberAddCmd, memberUpdateCmd, memberListCmd, memberRemoveCmd}
	for _, c := range memberAliasCmds {
		c.Flags().String("workspace-id", "", "")
		_ = c.Flags().MarkHidden("workspace-id")
		RegisterCrossProductAliases(c)
	}

	memberCmd.AddCommand(memberAddCmd, memberUpdateCmd, memberListCmd, memberRemoveCmd)

	root.AddCommand(spaceCmd, memberCmd)

	// ── node (知识库节点管理) ─────────────────────────────────
	// 对齐飞书 cli-lark wiki node 设计：内建 list/create/copy/move/delete，
	// 一跳直达 doc MCP server，不再经过 proxy chain。
	nodeCmd := newGroupCommand(&cobra.Command{
		Use:   "node",
		Short: "知识库节点管理",
		Long:  `管理钉钉知识库中的节点（文档/文件夹/表格等）：列出、创建、复制、移动、删除。`,
		RunE:  groupRunE,
	})

	nodeListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "列出知识库节点",
		Long: `列出指定知识库下的直接子节点（文档、文件夹、表格等）。

通过 --folder 指定父节点可列出子目录内容；不传 --folder 则列出知识库根目录。
支持分页，通过 --cursor 传入上次返回的 pageToken 获取下一页。`,
		Example: `  dws wiki node list --workspace <workspaceId>
  dws wiki node list --workspace <workspaceId> --folder <parentNodeId>
  dws wiki node list --workspace <workspaceId> --limit 20 --cursor <pageToken>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"workspaceId": workspaceID,
			}
			if folder := docFolderFlag(cmd, "folder", "node", "parent-id"); folder != "" {
				if err := validateDocFolderID(folder); err != nil {
					return err
				}
				toolArgs["folderId"] = folder
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
				toolArgs["pageToken"] = v
			}
			return callMCPToolOnServer("doc", "list_nodes", toolArgs)
		},
	}
	DeclareLeafMetadata(nodeListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "list_nodes",
				CanonicalPath:  "wiki.list_nodes",
				CLIPath:        "wiki node list",
				PrimaryCLIPath: "wiki node list",
			},
			Description: "列出指定知识库下的直接子节点（文档、文件夹、表格等）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "list_nodes"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出指定知识库下的直接子节点（文档、文件夹、表格等）",
				UseWhen:      []string{"在已知 workspace 下浏览知识库根目录或某文件夹子节点时"},
				AvoidWhen: []string{
					"库内关键词搜索用 node search；全局搜用 drive search",
					"读文档内容拿到 nodeId 后用 doc read",
				},
				Examples: []string{
					"dws wiki node list --workspace <workspaceId> --format json",
					"dws wiki node list --workspace <workspaceId> --folder <parentNodeId> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "pageToken"},
				{Name: "folder", Property: "folderId"},
				{Name: "limit", Property: "pageSize"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	nodeListCmd.Flags().String("workspace", "", "知识库 ID (必填)")
	nodeListCmd.Flags().String("folder", "", "父节点 nodeId (选填，不传则列出根目录)")
	nodeListCmd.Flags().Int("limit", 0, "每页数量 (默认 50，最大 50)")
	nodeListCmd.Flags().String("cursor", "", "分页游标")

	nodeCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "在知识库中创建节点",
		Long: `在指定知识库中创建文档、文件夹或其他类型的节点。

通过 --type 指定节点类型（服务端支持以下值，asheet 不被支持）：
  adoc      在线文档 (默认)
  axls      在线电子表格
  able      多维表
  appt      在线演示
  adraw     白板/画板
  amind     脑图
  folder    文件夹

通过 --folder 指定父节点，不传则创建在知识库根目录。`,
		Example: `  dws wiki node create --workspace <workspaceId> --name "新文档"
  dws wiki node create --workspace <workspaceId> --name "方案目录" --type folder
  dws wiki node create --workspace <workspaceId> --name "数据表" --type axls --folder <parentNodeId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"workspaceId": workspaceID,
				"name":        mustGetFlag(cmd, "name"),
			}
			if v := mustGetFlag(cmd, "type"); v != "" {
				toolArgs["type"] = v
			}
			if folder := docFolderFlag(cmd, "folder", "parent-id"); folder != "" {
				if err := validateDocFolderID(folder); err != nil {
					return err
				}
				toolArgs["folderId"] = folder
			}
			return callMCPToolOnServer("doc", "create_file", toolArgs)
		},
	}
	DeclareLeafMetadata(nodeCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "create_file",
				CanonicalPath:  "wiki.create_file",
				CLIPath:        "wiki node create",
				PrimaryCLIPath: "wiki node create",
			},
			Description: "在指定知识库中创建文档、文件夹或其他类型的节点",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "create_file"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "在指定知识库中创建文档、文件夹或其他类型的节点",
				UseWhen:      []string{"在知识库内创建空节点：adoc/axls/appt/adraw/amind/able/folder（必须 --workspace）时"},
				AvoidWhen: []string{
					"要带初始 Markdown 的文字文档可用 doc create",
					"钉盘普通文件夹用 drive mkdir（无需 workspace）",
					"type 不要用 asheet，在线表格用 axls",
				},
				Examples: []string{
					"dws wiki node create --workspace <workspaceId> --name \"新文档\" --format json",
					"dws wiki node create --workspace <workspaceId> --name \"方案目录\" --type folder --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "folderId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	nodeCreateCmd.Flags().String("workspace", "", "知识库 ID (必填)")
	nodeCreateCmd.Flags().String("name", "", "节点名称 (必填)")
	nodeCreateCmd.Flags().String("type", "adoc", "节点类型: adoc / axls / able / appt / adraw / amind / folder（asheet 不支持）")
	nodeCreateCmd.Flags().String("folder", "", "父节点 nodeId (选填，不传则在根目录创建)")

	nodeCopyCmd := &cobra.Command{
		Use:   "copy",
		Short: "复制知识库节点",
		Long: `将知识库中的节点复制到指定位置。

通过 --node 指定源节点，通过 --folder 指定目标文件夹。
不传 --folder 时复制到 --workspace 指定知识库的根目录。`,
		Example: `  dws wiki node copy --workspace <workspaceId> --node <nodeId>
  dws wiki node copy --workspace <workspaceId> --node <nodeId> --folder <targetFolderId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			nodeID, err := mustFlagOrFallback(cmd, "node", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":      nodeID,
				"workspaceId": workspaceID,
			}
			if folder := docFolderFlag(cmd, "folder", "parent-id", "parent-node-id", "parent-folder-id"); folder != "" {
				if err := validateDocFolderID(folder); err != nil {
					return err
				}
				toolArgs["targetFolderId"] = folder
			}
			return callMCPToolOnServer("doc", "copy_document", toolArgs)
		},
	}
	DeclareLeafMetadata(nodeCopyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "node_copy",
				CanonicalPath:  "wiki.node_copy",
				CLIPath:        "wiki node copy",
				PrimaryCLIPath: "wiki node copy",
			},
			Description: "将知识库中的节点复制到指定位置",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "copy_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将知识库中的节点复制到指定位置",
				UseWhen:      []string{"在知识库上下文内复制节点到同库某文件夹（需 --workspace + --node）时"},
				AvoidWhen: []string{
					"跨产品/默认文档空间复制优先 dws drive copy",
					"要移动不留副本用 node move",
				},
				Examples: []string{"dws wiki node copy --workspace <workspaceId> --node <nodeId> --folder <targetFolderId> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "targetFolderId"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	nodeCopyCmd.Flags().String("workspace", "", "知识库 ID (必填)")
	nodeCopyCmd.Flags().String("node", "", "源节点 ID (必填)")
	nodeCopyCmd.Flags().String("folder", "", "目标文件夹 nodeId (选填)")

	nodeMoveCmd := &cobra.Command{
		Use:   "move",
		Short: "移动知识库节点",
		Long: `将知识库中的节点移动到指定位置。

通过 --node 指定源节点，通过 --folder 指定目标文件夹。
不传 --folder 时移动到 --workspace 指定知识库的根目录。

注意：跨知识库移动需要同时具备源和目标的相应权限。`,
		Example: `  dws wiki node move --workspace <workspaceId> --node <nodeId> --folder <targetFolderId>
  dws wiki node move --workspace <workspaceId> --node <nodeId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			nodeID, err := mustFlagOrFallback(cmd, "node", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":      nodeID,
				"workspaceId": workspaceID,
			}
			if folder := docFolderFlag(cmd, "folder", "parent-id", "parent-node-id", "parent-folder-id"); folder != "" {
				if err := validateDocFolderID(folder); err != nil {
					return err
				}
				toolArgs["targetFolderId"] = folder
			}
			return callMCPToolOnServer("doc", "move_document", toolArgs)
		},
	}
	DeclareLeafMetadata(nodeMoveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "node_move",
				CanonicalPath:  "wiki.node_move",
				CLIPath:        "wiki node move",
				PrimaryCLIPath: "wiki node move",
			},
			Description: "将知识库中的节点移动到指定位置",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "move_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将知识库中的节点移动到指定位置",
				UseWhen:      []string{"在知识库内移动节点到目标文件夹时"},
				AvoidWhen:    []string{"要保留副本用 node copy / drive copy"},
				Examples:     []string{"dws wiki node move --workspace <workspaceId> --node <nodeId> --folder <targetFolderId> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "targetFolderId"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	nodeMoveCmd.Flags().String("workspace", "", "知识库 ID (必填)")
	nodeMoveCmd.Flags().String("node", "", "源节点 ID (必填)")
	nodeMoveCmd.Flags().String("folder", "", "目标文件夹 nodeId (选填)")

	nodeDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除知识库节点",
		Long: `将知识库中的节点移入回收站。

注意: 这是一个危险操作。执行前需要确认，或传入 --yes 跳过确认。
删除后节点会进入回收站，有保留期限可恢复。

权限要求: 对节点有"管理"权限。`,
		Example: `  dws wiki node delete --workspace <workspaceId> --node <nodeId>
  dws wiki node delete --workspace <workspaceId> --node <nodeId> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := mustFlagOrFallback(cmd, "workspace", "workspace-id"); err != nil {
				return err
			}
			nodeID, err := mustFlagOrFallback(cmd, "node", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("doc", "delete_document", map[string]any{
				"nodeId": nodeID,
			})
		},
	}
	DeclareLeafMetadata(nodeDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "delete_document",
				CanonicalPath:  "wiki.delete_document",
				CLIPath:        "wiki node delete",
				PrimaryCLIPath: "wiki node delete",
			},
			Description: "将知识库中的节点移入回收站",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "delete_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将知识库中的节点移入回收站",
				UseWhen:      []string{"用户确认后删除知识库内节点（移入回收站；需 --workspace 校验权限）时"},
				AvoidWhen: []string{
					"删整个知识库用 space delete",
					"未确认或 node 不明时不要删",
				},
				Examples: []string{"dws wiki node delete --workspace <workspaceId> --node <nodeId> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	nodeDeleteCmd.Flags().String("workspace", "", "知识库 ID (必填，用于权限校验)")
	nodeDeleteCmd.Flags().String("node", "", "节点 ID (必填)")

	nodeSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "在知识库中搜索节点",
		Long: `在指定知识库内搜索文档/文件夹/表格等节点。

通过 --workspace 限定搜索范围到某个知识库，通过 --query 指定搜索关键词。
支持按文件扩展名过滤（--extensions），如 adoc、asheet、pdf 等。`,
		Example: `  dws wiki node search --workspace <workspaceId> --query "产品方案"
  dws wiki node search --workspace <workspaceId> --query "设计" --extensions adoc,asheet
  dws wiki node search --workspace <workspaceId> --query "合同" --limit 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			query := flagOrFallback(cmd, "query", "keyword")
			if query == "" {
				return fmt.Errorf("flag --query is required")
			}
			toolArgs := map[string]any{
				"keyword":      query,
				"workspaceIds": []string{workspaceID},
			}
			if v, _ := cmd.Flags().GetStringSlice("extensions"); len(v) > 0 {
				toolArgs["extensions"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
				toolArgs["pageToken"] = v
			}
			return callMCPToolOnServer("doc", "search_documents", toolArgs)
		},
	}
	DeclareLeafMetadata(nodeSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "node_search",
				CanonicalPath:  "wiki.node_search",
				CLIPath:        "wiki node search",
				PrimaryCLIPath: "wiki node search",
			},
			Description: "在指定知识库内搜索文档/文件夹/表格等节点",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "search_documents"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "在指定知识库内搜索文档/文件夹/表格等节点",
				UseWhen:      []string{"在指定知识库内按关键词搜节点（可 --extensions）时"},
				AvoidWhen:    []string{"全局/钉盘搜索用 drive search；浏览目录用 node list"},
				Examples: []string{
					"dws wiki node search --workspace <workspaceId> --query \"方案\" --format json",
					"dws wiki node search --workspace <workspaceId> --query \"周报\" --extensions adoc --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "pageToken"},
				{Name: "limit", Property: "pageSize"},
				{Name: "query", Property: "keyword"},
				{Name: "workspace", Property: "workspaceIds"},
			},
		},
	})
	nodeSearchCmd.Flags().String("workspace", "", "知识库 ID (必填)")
	nodeSearchCmd.Flags().String("query", "", "搜索关键词 (必填)")
	nodeSearchCmd.Flags().String("keyword", "", "--query 的别名")
	_ = nodeSearchCmd.Flags().MarkHidden("keyword")
	nodeSearchCmd.Flags().StringSlice("extensions", nil, "按文件扩展名过滤 (如 adoc,asheet,pdf)")
	nodeSearchCmd.Flags().Int("limit", 0, "每页数量 (默认 10，最大 30)")
	nodeSearchCmd.Flags().String("cursor", "", "分页游标")

	// node 子命令的 hidden aliases
	nodeAliasCmds := []*cobra.Command{nodeListCmd, nodeCreateCmd, nodeCopyCmd, nodeMoveCmd, nodeDeleteCmd, nodeSearchCmd}
	for _, c := range nodeAliasCmds {
		c.Flags().String("workspace-id", "", "")
		_ = c.Flags().MarkHidden("workspace-id")
		RegisterCrossProductAliases(c)
	}

	nodeCmd.AddCommand(nodeListCmd, nodeCreateCmd, nodeCopyCmd, nodeMoveCmd, nodeDeleteCmd, nodeSearchCmd)

	root.AddCommand(nodeCmd)

	// ── feed (知识库动态查询) ─────────────────────────────────
	feedCmd := newGroupCommand(&cobra.Command{
		Use:   "feed",
		Short: "知识库动态查询",
		Long:  `查询知识库的动态：谁在什么时间更新/上传/评论了哪些文档。`,
		RunE:  groupRunE,
	})

	feedListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查询知识库动态列表",
		Long: `查询指定知识库的动态列表，返回动态类型、时间、内容摘要等信息。

通过 --workspace 指定知识库，支持传入知识库 ID 或知识库 URL。
支持分页，通过 --cursor 传入上次返回的 nextToken 获取下一页。
使用 --exclude-file 可排除普通文件、媒体文件、文件夹及 Office 文件动态，仅保留在线文档操作（创建/更新/评论/点赞等）。
当用户要求"只看文档操作""排除文件上传"等意图时，必须使用此 flag，
禁止在客户端自行过滤。`,
		Example: `  dws wiki feed list --workspace <workspaceId>
  dws wiki feed list --workspace <workspaceId> --limit 10
  dws wiki feed list --workspace <workspaceId> --exclude-file
  dws wiki feed list --workspace <workspaceId> --limit 10 --exclude-file
  dws wiki feed list --workspace <workspaceId> --cursor <nextToken>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"workspaceId": workspaceID,
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
				toolArgs["nextToken"] = v
			}
			if cmd.Flags().Changed("exclude-file") {
				v, _ := cmd.Flags().GetBool("exclude-file")
				toolArgs["excludeFile"] = v
			}
			// 调用后对 feeds[].time 毫秒时间戳做本地化格式化，
			// 使 Agent 直接看到可读时间，无需自行转换
			text, err := callMCPToolReturnText(context.Background(), "list_workspace_feeds", toolArgs)
			if err != nil {
				return err
			}
			// 保持与共享 dispatcher (callMCPToolInternalOpts) 一致的输出契约：
			// 仅 --format json 做时间/标签增强，raw、table 及其他格式输出原始 MCP 文本
			if deps.Caller.Format() != "json" {
				deps.Out.PrintRaw(text)
				return nil
			}
			return deps.Out.PrintJSON(formatFeedTime(text))
		},
	}
	DeclareLeafMetadata(feedListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "wiki",
				Name:           "list_workspace_feeds",
				CanonicalPath:  "wiki.list_workspace_feeds",
				CLIPath:        "wiki feed list",
				PrimaryCLIPath: "wiki feed list",
			},
			Description: "查询指定知识库的动态列表，返回动态类型、时间、内容摘要等信息",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "wiki", RPCName: "list_workspace_feeds"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询知识库的活动动态：谁在什么时间更新/上传/评论了哪些文档",
				UseWhen: []string{
					"用户问某个知识库最近有什么更新、谁改了什么、有哪些评论等协作动态时",
					"需要巡检知识库变更并按时间线汇总成动态摘要时",
				},
				AvoidWhen: []string{
					"要看知识库当前有哪些节点用 node list；库内按关键词找文档用 node search",
					"要读某篇文档正文用 doc read；跨库全局找文件用 drive search",
				},
				Examples: []string{
					"dws wiki feed list --workspace <workspaceId> --format json",
					"dws wiki feed list --workspace <workspaceId> --limit 10 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "exclude-file", Property: "excludeFile"},
				{Name: "limit", Property: "maxResults"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	feedListCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	feedListCmd.Flags().Int("limit", 0, "每页数量 (默认 10，最大 20)。用户未明确要求条数时禁止加此 flag，让服务端走默认 10")
	feedListCmd.Flags().String("cursor", "", "分页游标 (首页留空)")
	feedListCmd.Flags().Bool("exclude-file", false, "排除普通文件、媒体文件、文件夹及 Office 文件动态，仅保留在线文档操作（创建/更新/评论/点赞）。用户要求排除文件/只看文档操作时必须使用此 flag，禁止客户端过滤")
	feedListCmd.Flags().String("workspace-id", "", "")
	_ = feedListCmd.Flags().MarkHidden("workspace-id")
	RegisterCrossProductAliases(feedListCmd)

	feedCmd.AddCommand(feedListCmd)

	root.AddCommand(feedCmd)

	// ── [PROXY] wiki create/get/list/search → wiki space create/get/list/search ──
	// Agent 常省略 "space" 直接输入 dws wiki list，透明转发到 wiki space 对应命令
	root.AddCommand(
		proxySubCmd("create", "wiki", "space create", nil), // [PROXY] wiki create → wiki space create
		proxySubCmd("get", "wiki", "space get", nil),       // [PROXY] wiki get → wiki space get
		proxySubCmd("list", "wiki", "space list", nil),     // [PROXY] wiki list → wiki space list
		proxySubCmd("search", "wiki", "space search", nil), // [PROXY] wiki search → wiki space search
		proxySubCmd("delete", "wiki", "space delete", nil), // [PROXY] wiki delete → wiki space delete
	)

	// ── [PROXY] wiki file/doc * → doc * (兼容旧路径) ──
	fileGroup := newGroupCommand(&cobra.Command{Use: "file", Hidden: true, RunE: groupRunE})
	fileGroup.AddCommand(
		proxySubCmd("list", "doc", "list", nil),                                                 // [PROXY] wiki file list → doc list
		proxySubCmd("search", "doc", "search", map[string]string{"workspace": "workspace-ids"}), // [PROXY] wiki file search → doc search
	)
	root.AddCommand(fileGroup)

	docGroup := newGroupCommand(&cobra.Command{Use: "doc", Hidden: true, RunE: groupRunE})
	docGroup.AddCommand(
		proxySubCmd("list", "doc", "list", nil),                                                 // [PROXY] wiki doc list → doc list
		proxySubCmd("read", "doc", "read", nil),                                                 // [PROXY] wiki doc read → doc read
		proxySubCmd("search", "doc", "search", map[string]string{"workspace": "workspace-ids"}), // [PROXY] wiki doc search → doc search
	)
	root.AddCommand(docGroup)

	return root
}

// beijingLoc 用于将时间戳格式化为北京时间（UTC+8）
var beijingLoc = time.FixedZone("CST", 8*3600)

// formatFeedTime 将知识库动态 feeds[].time 毫秒时间戳转为可读时间字符串，
// 写入 timeFormatted 字段，同时保留原始 time 毫秒时间戳不变（避免破坏已有脚本）。
// 解析失败时原样返回，不阻断输出。
func formatFeedTime(raw string) any {
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return raw
	}
	feeds, ok := result["feeds"].([]any)
	if !ok {
		return result
	}
	for _, f := range feeds {
		item, ok := f.(map[string]any)
		if !ok {
			continue
		}
		ms, ok := item["time"].(float64)
		if !ok || ms <= 0 {
			continue
		}
		// 保留原始 time 毫秒时间戳，新增 timeFormatted 可读字段
		item["timeFormatted"] = time.UnixMilli(int64(ms)).In(beijingLoc).Format("2006-01-02 15:04")
		enrichFeedFields(item)
	}
	return result
}

// enrichFeedFields 为单条 feed 补充可读字段（typeLabel），
func enrichFeedFields(item map[string]any) {
	// 将 type 数字映射为可读标签，使 Agent 无需查表即可直接展示
	if typeNum, ok := item["type"].(float64); ok {
		if label, known := feedTypeLabels[int(typeNum)]; known {
			item["typeLabel"] = label
		}
	}
}

// feedTypeLabels 将知识库动态 type 数字映射为可读中文标签，
// 与 lippi-combo OpenFeedItemDTO.type 枚举对齐。
var feedTypeLabels = map[int]string{
	0:  "创建文档",
	1:  "更新文档",
	2:  "评论文档",
	3:  "点赞文档",
	4:  "加入团队空间",
	5:  "表格选区数据变更",
	6:  "更新 office 文件",
	7:  "上传普通文件",
	8:  "上传媒体文件",
	9:  "上传文件夹",
	10: "上传文件夹 V2",
	11: "加入团队",
	12: "创建知识库",
}
