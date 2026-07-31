package app

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func configureRootHelp(root *cobra.Command) {
	if root == nil {
		return
	}

	// Replace the cobra-default English help command with a localized one so
	// that both its listing short (shown in `dws --help`) and its own
	// `dws help --help` long text follow the active locale.
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: i18n.T("查看任意命令的帮助信息"),
		Long: i18n.T("显示任意命令的帮助文案。\n" +
			"用法：dws help [命令路径] 查看完整说明。"),
		DisableAutoGenTag: true,
		Run: func(c *cobra.Command, args []string) {
			target, _, err := c.Root().Find(args)
			if target == nil || err != nil {
				c.Root().HelpFunc()(c.Root(), args)
				return
			}
			target.InitDefaultHelpFlag()
			_ = target.Help()
		},
	})

	defaultHelpFunc := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd != root {
			defaultHelpFunc(cmd, args)
			cli.RenderSafetyAnnotation(cmd)
			renderChatAgentSelectionHint(cmd)
			renderChatWorkbookHelpGuidance(cmd)
			return
		}
		renderRootHelp(root)
	})
}

type chatHelpGuidance struct {
	reason  string
	action  string
	example string
}

var chatWorkbookHelpGuidance = map[string]chatHelpGuidance{
	"chat group members": {
		"群成员列表固定使用 --id 传群 openConversationId，不使用消息命令的 --group。",
		"先查群 ID，再直接执行 members；不要追加多余的 list 子命令。",
		`dws chat group members --id <openConversationId> --format json`,
	},
	"chat group members add": {
		"添加群成员固定使用 --id 指定群、--users 指定成员。",
		"先查询群 ID 和成员 userId/openDingTalkId，再执行添加。",
		`dws chat group members add --id <openConversationId> --users <userId1>,<userId2> --format json`,
	},
	"chat group members remove": {
		"移除群成员使用 --id 和 --users，且不能移除群主。",
		"先确认成员和不可逆影响，检查群主身份后再执行。",
		`dws chat group members remove --id <openConversationId> --users <userId> --format json`,
	},
	"chat group members add-bot": {
		"添加机器人属于群成员管理，群参数沿用 --id，并需要 robot-code。",
		"确认机器人编码和目标群后执行。",
		`dws chat group members add-bot --id <openConversationId> --robot-code <robotCode> --format json`,
	},
	"chat group members remove-bot": {
		"移除机器人固定使用 --id 指定群、--bot-id 指定群内机器人。",
		"先列出群机器人取得 openBotId，再执行移除。",
		`dws chat group members remove-bot --id <openConversationId> --bot-id <openBotId> --format json`,
	},
	"chat group members list-by-ids": {
		"批量查询成员详情使用 --id + --users，users 为成员标识列表。",
		"确认目标群和成员 ID 后再查询。",
		`dws chat group members list-by-ids --id <openConversationId> --users <openDingTalkId1>,<openDingTalkId2> --format json`,
	},
	"chat group create": {
		"建群使用 --users；创建结果中的群 ID 可继续传给 members add 和 rename。",
		"先准备成员 userId，创建后保存返回的 openConversationId。",
		`dws chat group create --name "项目群" --users <userId1>,<userId2> --format json`,
	},
	"chat group rename": {
		"群改名只使用 --id + --name，不能使用 --group。",
		"先通过 chat search 获取 openConversationId。",
		`dws chat group rename --id <openConversationId> --name "新群名" --format json`,
	},
	"chat message list": {
		"message list 按会话和时间拉取消息，不执行服务端关键词搜索。",
		"按关键词查找时改用 message search；拉历史时提供会话和 time。",
		`dws chat message list --group <openConversationId> --time "2026-07-30 23:59:59" --direction older --format json`,
	},
	"chat message search": {
		"关键词审计应使用服务端搜索，并同时提供 query、start、end。",
		"不要用 message list 拉全量后人工筛选。",
		`dws chat message search --query "评审" --start "2026-07-01T00:00:00+08:00" --end "2026-07-31T23:59:59+08:00" --format json`,
	},
	"chat message search-advanced": {
		"简单关键词优先 message search；只有组合人员、@、会话等条件时才使用 search-advanced。",
		"至少提供一个真实搜索条件，分页参数不算搜索条件。",
		`dws chat message search-advanced --query "评审" --conversation-ids <openConversationId> --format json`,
	},
	"chat message list-all": {
		"list-all 按时间跨会话拉取消息，不执行关键词匹配。",
		"需要关键词时改用 message search，并始终限制时间范围。",
		`dws chat message list-all --start "2026-07-01T00:00:00+08:00" --end "2026-07-31T23:59:59+08:00" --format json`,
	},
	"chat message list-by-sender": {
		"list-by-sender 的核心条件是发送者；核心条件是关键词时应使用 message search。",
		"提供发送者 ID 和开始时间，按 nextCursor 翻页。",
		`dws chat message list-by-sender --sender-user-id <userId> --start "2026-07-01T00:00:00+08:00" --format json`,
	},
}

func renderChatWorkbookHelpGuidance(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	path := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "))
	guide, ok := chatWorkbookHelpGuidance[path]
	if !ok {
		meta, metaOK := cli.ResolveMeta(path)
		if !metaOK || meta.Identity.ProductID != "chat" {
			return
		}
		reason := meta.Selection.AgentSummary
		if reason == "" {
			reason = "执行前需要确认该 Chat 命令的适用场景、必填参数和安全边界。"
		}
		action := "根据帮助正文补齐必填参数，并在实际执行时增加 --format json。"
		if len(meta.Selection.UseWhen) > 0 {
			action = meta.Selection.UseWhen[0]
		}
		example := "dws " + path + " --format json"
		if len(meta.Selection.Examples) > 0 {
			example = meta.Selection.Examples[0]
			if !strings.Contains(example, "--format") {
				example += " --format json"
			}
		}
		guide = chatHelpGuidance{reason: reason, action: action, example: example}
	}
	w := cmd.ErrOrStderr()
	_, _ = fmt.Fprintln(w, "错误信息：当前为执行前 guidance，不是运行失败")
	_, _ = fmt.Fprintln(w, "原因："+guide.reason)
	_, _ = fmt.Fprintln(w, "建议操作：")
	_, _ = fmt.Fprintln(w, "1. "+guide.action)
	_, _ = fmt.Fprintln(w, "示例：")
	_, _ = fmt.Fprintln(w, "1. "+guide.example)
}

// renderChatAgentSelectionHint exposes the reviewed Chat selection contract in
// command help without reintroducing a second product-local guidance map.
// Selection prose remains authored in schema_hints/selection/chat.json and is
// consumed through the repository-wide ResolveMeta API.
func renderChatAgentSelectionHint(cmd *cobra.Command) {
	cliPath := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "))
	meta, ok := cli.ResolveMeta(cliPath)
	if !ok || meta.Identity.ProductID != "chat" {
		return
	}
	selection := meta.Selection

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(w, "Agent guidance:")
	if selection.AgentSummary != "" {
		_, _ = fmt.Fprintf(w, "  Outcome: %s\n", selection.AgentSummary)
	}
	for _, scenario := range selection.UseWhen {
		_, _ = fmt.Fprintf(w, "  Use when: %s\n", scenario)
	}
	for _, scenario := range selection.AvoidWhen {
		_, _ = fmt.Fprintf(w, "  Avoid when: %s\n", scenario)
	}
	for _, example := range selection.Examples {
		_, _ = fmt.Fprintf(w, "  Example: %s\n", example)
	}
	_, _ = fmt.Fprintln(w, "  Output: Agent execution should add --format json.")
}

func renderRootHelp(root *cobra.Command) {
	services := visibleMCPRootCommands(root)
	utilities := visibleUtilityRootCommands(root)
	w := root.OutOrStdout()

	_, _ = fmt.Fprintln(w, tui.Header("Workspace CLI", "DingTalk blue-white technical console"))
	_, _ = fmt.Fprintln(w, tui.Rule(76))
	_, _ = fmt.Fprintln(w)

	if len(services) == 0 {
		_, _ = fmt.Fprintf(w, "%s %s\n", tui.StateMark("warning"), tui.Warning("No MCP services discovered."))
		_, _ = fmt.Fprintln(w)
	} else {
		_, _ = fmt.Fprintln(w, tui.Section("Discovered MCP Services:"))
		_, _ = fmt.Fprintln(w)

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, service := range services {
			_, _ = fmt.Fprintf(tw, "  %s %s\t%s\n", tui.StateMark("ok"), tui.Bold(service.Name()), tui.Dim(strings.TrimSpace(service.Short)))
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintln(w, tui.Section("Usage:"))
	_, _ = fmt.Fprintf(w, "  %s %s\n", tui.Bullet(), tui.White("dws <service> [command] [flags]"))
	if len(utilities) > 0 {
		_, _ = fmt.Fprintf(w, "  %s %s\n", tui.Bullet(), tui.White("dws <command> [flags]"))
	}
	_, _ = fmt.Fprintln(w)
	if len(utilities) > 0 {
		_, _ = fmt.Fprintln(w, tui.Section("Utility Commands:"))
		_, _ = fmt.Fprintln(w)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, utility := range utilities {
			_, _ = fmt.Fprintf(tw, "  %s %s\t%s\n", tui.Bullet(), tui.Bold(utility.Name()), tui.Dim(commandShort(utility)))
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(w)
	}
	renderRootGlobalFlags(root)
	_, _ = fmt.Fprintf(w, "%s %s\n", tui.Key("Next"), `Use "dws <service> --help" for more information about a discovered MCP service or "dws <command> --help" for utility commands.`)

	// Render root.Long after the command list so agents see the upgrade
	// hint (or any other root-level guidance) after browsing all available
	// commands and concluding none of them fit. Cobra's default help template
	// would render Long automatically; the custom SetHelpFunc above replaces
	// it and dropped this, so we restore it explicitly here.
	if long := strings.TrimSpace(root.Long); long != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, tui.Dim(long))
	}
}

func renderRootGlobalFlags(root *cobra.Command) {
	if root == nil {
		return
	}
	flags := visiblePersistentFlags(root)
	if len(flags) == 0 {
		return
	}
	w := root.OutOrStdout()
	_, _ = fmt.Fprintln(w, tui.Section("Global Flags:"))
	_, _ = fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, flag := range flags {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", formatRootFlag(flag), tui.Dim(strings.TrimSpace(flag.Usage)))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintln(w)
}

func visiblePersistentFlags(root *cobra.Command) []*pflag.Flag {
	if root == nil {
		return nil
	}
	flags := make([]*pflag.Flag, 0)
	root.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if flag == nil || flag.Hidden {
			return
		}
		flags = append(flags, flag)
	})
	return flags
}

func formatRootFlag(flag *pflag.Flag) string {
	if flag == nil {
		return ""
	}
	name := "--" + flag.Name
	if flag.Value != nil && flag.Value.Type() != "bool" {
		name += " " + flag.Value.Type()
	}
	if flag.Shorthand == "" {
		return "    " + name
	}
	return "-" + flag.Shorthand + ", " + name
}

func commandShort(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	short := strings.TrimSpace(cmd.Short)
	if cmd.Name() == "help" && short == "Help about any command" {
		return i18n.T("查看任意命令的帮助信息")
	}
	return short
}

// resolveVisibleProducts returns the set of top-level product IDs that should
// be treated as visible. It unions the edition's VisibleProducts hook (when
// set) with DirectRuntimeProductIDs(), so dynamically-registered products —
// including plugins loaded via AppendDynamicServer — are never silently hidden
// by a static VisibleProducts list.
func resolveVisibleProducts() map[string]bool {
	allowed := map[string]bool{}
	if fn := edition.Get().VisibleProducts; fn != nil {
		for _, p := range fn() {
			allowed[p] = true
		}
	}
	for id := range DirectRuntimeProductIDs() {
		allowed[id] = true
	}
	return allowed
}

func visibleMCPRootCommands(root *cobra.Command) []*cobra.Command {
	if root == nil {
		return nil
	}

	allowed := resolveVisibleProducts()

	commands := make([]*cobra.Command, 0)
	for _, cmd := range root.Commands() {
		if cmd == nil || cmd.Hidden {
			continue
		}
		if !allowed[cmd.Name()] {
			continue
		}
		commands = append(commands, cmd)
	}
	return commands
}

func visibleUtilityRootCommands(root *cobra.Command) []*cobra.Command {
	if root == nil {
		return nil
	}

	productCommands := resolveVisibleProducts()

	commands := make([]*cobra.Command, 0)
	for _, cmd := range root.Commands() {
		if cmd == nil || cmd.Hidden {
			continue
		}
		if productCommands[cmd.Name()] {
			continue
		}
		commands = append(commands, cmd)
	}
	return commands
}
