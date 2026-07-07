package helpers

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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
	root := &cobra.Command{
		Use:   "wiki",
		Short: "知识库 / 空间管理 / 节点管理 / 成员管理",
		Long:  `管理钉钉文档知识库：空间管理（创建/查看/列出/搜索/删除）、节点管理（列出/创建/复制/移动/删除）、成员管理（添加/更新/列出/移除）。`,
		RunE:  groupRunE,
	}

	spaceCmd := &cobra.Command{Use: "space", Short: "知识库管理", RunE: groupRunE}

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

	// space create flags
	spaceCreateCmd.Flags().String("name", "", "知识库名称 (必填，不超过 100 字符)")
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
			if !confirmDelete("知识库", workspaceID) {
				return nil
			}
			return callMCPTool("delete_wikiSpace", map[string]any{
				"workspaceId": workspaceID,
			})
		},
	}
	spaceDeleteCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	spaceDeleteCmd.Flags().String("workspace-id", "", "")
	_ = spaceDeleteCmd.Flags().MarkHidden("workspace-id")
	RegisterCrossProductAliases(spaceDeleteCmd)

	spaceCmd.AddCommand(spaceCreateCmd, spaceGetCmd, spaceListCmd, spaceSearchCmd, spaceDeleteCmd)

	// ── member (知识库成员管理) ───────────────────────────────
	memberCmd := &cobra.Command{
		Use:   "member",
		Short: "知识库成员管理",
		Long:  `管理钉钉知识库的成员：添加成员、更新成员权限、查询成员列表、移除成员。`,
		RunE:  groupRunE,
	}

	memberAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加知识库成员",
		Long: `为指定知识库添加一个或多个成员，并授予指定角色。

通过 --users 传入逗号分隔的 userId 列表，多个用户将被授予同一角色。

支持的角色 (--role)（必须大写）：
  MANAGER     管理员，可读写、管理成员
  EDITOR      编辑者，可查看、编辑、上传内容
  DOWNLOADER  查看下载者，可查看并下载内容
  READER      仅可查看者，仅可查看，不可下载

注意：
- OWNER 角色不可通过此接口添加，知识库创建者默认为所有者。
- 操作者需具备知识库的 OWNER 或 MANAGER 权限。
- 单次请求最多 30 个成员，超出请分批调用。

支持通过 --workspace 传入知识库 ID 或知识库 URL，系统自动识别。
用户 uid 可通过「钉钉通讯录」相关命令检索，如:
  dws contact user search --keyword "姓名"`,
		Example: `  dws wiki member add --workspace <workspaceId> --users uid1 --role READER
  dws wiki member add --workspace <workspaceId> --users uid1,uid2,uid3 --role EDITOR
  dws wiki member add --workspace "https://alidocs.dingtalk.com/i/spaces/xxx/overview" --users uid1 --role MANAGER`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "role"); err != nil {
				return err
			}
			userIds, err := collectUserIDs(cmd)
			if err != nil {
				return err
			}
			return callMCPTool("add_member", map[string]any{
				"workspaceId": workspaceID,
				"roleId":      normalizePermissionRole(mustGetFlag(cmd, "role")),
				"userIds":     userIds,
			})
		},
	}

	memberAddCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	memberAddCmd.Flags().String("users", "", "被添加的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)")
	memberAddCmd.Flags().String("user", "", "")
	_ = memberAddCmd.Flags().MarkHidden("user")
	memberAddCmd.Flags().String("role", "", "权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填，大小写不敏感)")

	memberUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新知识库成员权限",
		Long: `更新指定知识库已有成员的角色。

支持的角色 (--role)（必须大写）：
  MANAGER     管理员
  EDITOR      编辑者
  DOWNLOADER  查看下载者
  READER      仅可查看者

注意：
- OWNER 角色不可通过此接口变更。
- 同一成员在同一知识库只能拥有一个角色，变更后旧角色自动替换。
- 操作者需具备知识库的 OWNER 或 MANAGER 权限。

仅可更新已存在成员关系的成员，新增成员请使用 dws wiki member add。`,
		Example: `  dws wiki member update --workspace <workspaceId> --users uid1 --role EDITOR
  dws wiki member update --workspace <workspaceId> --users uid1,uid2 --role READER`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "role"); err != nil {
				return err
			}
			userIds, err := collectUserIDs(cmd)
			if err != nil {
				return err
			}
			return callMCPTool("update_member", map[string]any{
				"workspaceId": workspaceID,
				"roleId":      normalizePermissionRole(mustGetFlag(cmd, "role")),
				"userIds":     userIds,
			})
		},
	}

	memberUpdateCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	memberUpdateCmd.Flags().String("users", "", "被更新的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)")
	memberUpdateCmd.Flags().String("user", "", "")
	_ = memberUpdateCmd.Flags().MarkHidden("user")
	memberUpdateCmd.Flags().String("role", "", "新权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填，大小写不敏感)")

	memberListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查询知识库成员列表",
		Long: `查询指定知识库的成员列表，返回每位成员的 userId、姓名、角色等信息。

注意：底层不支持游标分页，--limit 仅控制单次返回的最大条数（最大 200）。
若结果被截断（出参 truncated=true），可通过 --filter-role 收窄查询范围；
ORG 类型授权不会出现在查询结果中。`,
		Example: `  dws wiki member list --workspace <workspaceId>
  dws wiki member list --workspace <workspaceId> --limit 100
  dws wiki member list --workspace <workspaceId> --filter-role MANAGER,EDITOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"workspaceId": workspaceID,
			}
			limit := 0
			if cmd.Flags().Changed("limit") {
				limit, _ = cmd.Flags().GetInt("limit")
			} else if cmd.Flags().Changed("max-results") {
				limit, _ = cmd.Flags().GetInt("max-results")
			}
			if limit > 0 {
				toolArgs["maxResults"] = limit
			}
			if v := mustGetFlag(cmd, "filter-role"); v != "" {
				toolArgs["filterRoleIds"] = parseRoleList(v)
			}
			return callMCPTool("list_member", toolArgs)
		},
	}

	memberListCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	memberListCmd.Flags().Int("limit", 30, "返回成员数上限，默认 30，最大 200")
	memberListCmd.Flags().Int("max-results", 0, "")
	_ = memberListCmd.Flags().MarkHidden("max-results")
	memberListCmd.Flags().String("filter-role", "", "按角色过滤（逗号分隔）：OWNER / MANAGER / EDITOR / DOWNLOADER / READER")

	memberRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "移除知识库成员",
		Long: `从指定知识库中移除一个或多个成员（仅支持 USER 类型）。

移除后相关用户将无法访问该知识库下的内容（除非通过节点级权限另行授权）。

注意：
- OWNER 角色不可通过此接口移除。
- 操作者需具备知识库的 OWNER 或 MANAGER 权限。
- 单次请求最多 30 个成员，超出请分批调用。`,
		Example: `  dws wiki member remove --workspace <workspaceId> --users uid1
  dws wiki member remove --workspace <workspaceId> --users uid1,uid2,uid3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceID, err := mustFlagOrFallback(cmd, "workspace", "workspace-id")
			if err != nil {
				return err
			}
			userIds, err := collectUserIDs(cmd)
			if err != nil {
				return err
			}
			return callMCPTool("remove_member", map[string]any{
				"workspaceId": workspaceID,
				"userIds":     userIds,
			})
		},
	}
	memberRemoveCmd.Flags().String("workspace", "", "知识库 ID 或 URL (必填)")
	memberRemoveCmd.Flags().String("users", "", "被移除的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)")
	memberRemoveCmd.Flags().String("user", "", "")
	_ = memberRemoveCmd.Flags().MarkHidden("user")

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
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "知识库节点管理",
		Long:  `管理钉钉知识库中的节点（文档/文件夹/表格等）：列出、创建、复制、移动、删除。`,
		RunE:  groupRunE,
	}

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
			if !confirmDelete("知识库节点", nodeID) {
				return nil
			}
			return callMCPToolOnServer("doc", "delete_document", map[string]any{
				"nodeId": nodeID,
			})
		},
	}
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
	fileGroup := &cobra.Command{Use: "file", Hidden: true, RunE: groupRunE}
	fileGroup.AddCommand(
		proxySubCmd("list", "doc", "list", nil),                                                 // [PROXY] wiki file list → doc list
		proxySubCmd("search", "doc", "search", map[string]string{"workspace": "workspace-ids"}), // [PROXY] wiki file search → doc search
	)
	root.AddCommand(fileGroup)

	docGroup := &cobra.Command{Use: "doc", Hidden: true, RunE: groupRunE}
	docGroup.AddCommand(
		proxySubCmd("list", "doc", "list", nil),                                                 // [PROXY] wiki doc list → doc list
		proxySubCmd("read", "doc", "read", nil),                                                 // [PROXY] wiki doc read → doc read
		proxySubCmd("search", "doc", "search", map[string]string{"workspace": "workspace-ids"}), // [PROXY] wiki doc search → doc search
	)
	root.AddCommand(docGroup)

	return root
}
