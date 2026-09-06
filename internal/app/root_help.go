package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/roothelp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// feedbackFormURL points at the DingTalk Notable form collecting dws CLI
// user-experience feedback. The source parameter tags submissions that
// originated from the CLI help output so they can be told apart from
// responses arriving through other channels.
const feedbackFormURL = roothelp.FeedbackFormURL

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
			if isChatProductRoot(cmd, root) {
				renderChatShortcutFirstHelp(cmd)
				cli.RenderHelpAffordances(cmd)
				return
			}
			defaultHelpFunc(cmd, args)
			renderPreferredShortcutAffordance(cmd)
			cli.RenderHelpAffordances(cmd)
			return
		}
		renderRootHelp(root)
	})
}

func isChatProductRoot(cmd, root *cobra.Command) bool {
	return cmd != nil && root != nil && cmd.Parent() == root && cmd.Name() == "chat"
}

func renderChatShortcutFirstHelp(cmd *cobra.Command) {
	w := cmd.OutOrStdout()
	if long := strings.TrimSpace(cmd.Long); long != "" {
		_, _ = fmt.Fprintln(w, long)
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintln(w, "选择顺序：")
	_, _ = fmt.Fprintln(w, "  1. 优先使用 +shortcut 完成用户任务。")
	_, _ = fmt.Fprintln(w, "  2. 本页只展示高频 Featured Shortcuts；其他正式 Shortcut 仍可通过 Catalog 和精确 Help 发现。")
	_, _ = fmt.Fprintln(w, "  3. 只有 Shortcut 不支持所需底层参数或原始响应时，才使用 Atomic API Resources。")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintf(w, "  %s [flags]\n", cmd.CommandPath())
	_, _ = fmt.Fprintf(w, "  %s [command]\n", cmd.CommandPath())
	if len(cmd.Aliases) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "Aliases:")
		_, _ = fmt.Fprintf(w, "  %s, %s\n", cmd.Name(), strings.Join(cmd.Aliases, ", "))
	}

	featured, catalog, atomic := chatHelpCommands(cmd)
	renderChatHelpCommandSection(w, "Featured Shortcuts:", featured)
	renderChatHelpCommandSection(w, "Atomic API Resources:", atomic)

	_, _ = fmt.Fprintln(w, "More Chat Shortcuts:")
	_, _ = fmt.Fprintf(w, "  当前有 %d 个 canonical Shortcut；本页展示 %d 个高频入口，另有 %d 个低频正式入口。\n",
		len(featured)+len(catalog), len(featured), len(catalog))
	_, _ = fmt.Fprintln(w, "  完整列表：dws shortcut list --service chat --format json")
	_, _ = fmt.Fprintln(w, "  精确帮助：dws chat +<command> --help")
	_, _ = fmt.Fprintln(w, "  机器契约：dws schema --cli-path \"chat +<command>\" --compact -f json")

	renderCommandFlagSections(w, cmd)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Use \"%s [command] --help\" for exact command help.\n", cmd.CommandPath())
}

func chatHelpCommands(cmd *cobra.Command) (featured, catalog, atomic []*cobra.Command) {
	for _, child := range cmd.Commands() {
		if child == nil || child.Hidden || child.Deprecated != "" || child.Name() == "help" {
			continue
		}
		if !strings.HasPrefix(child.Name(), "+") {
			if child.Annotations != nil && child.Annotations[preferredShortcutCLIPathAnnotation] != "" {
				continue
			}
			atomic = append(atomic, child)
			continue
		}
		tier := ""
		if child.Annotations != nil {
			tier = child.Annotations[shortcut.HelpTierAnnotation]
		}
		switch shortcut.HelpTier(tier) {
		case shortcut.HelpTierCatalog:
			catalog = append(catalog, child)
		case shortcut.HelpTierCompatibility, shortcut.HelpTierUnavailable:
			continue
		default:
			// User-defined Shortcuts and pre-tier declarations remain visible so
			// the product Help never silently drops an explicitly installed entry.
			featured = append(featured, child)
		}
	}
	for _, commands := range [][]*cobra.Command{featured, catalog, atomic} {
		sort.Slice(commands, func(i, j int) bool { return commands[i].Name() < commands[j].Name() })
	}
	return featured, catalog, atomic
}

func renderPreferredShortcutAffordance(cmd *cobra.Command) {
	if cmd == nil || cmd.Annotations == nil {
		return
	}
	owner := strings.TrimSpace(cmd.Annotations[preferredShortcutCLIPathAnnotation])
	if owner == "" {
		return
	}
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Preferred Shortcut:")
	_, _ = fmt.Fprintf(w, "  dws %s\n", owner)
	_, _ = fmt.Fprintln(w, "  默认使用 Shortcut；只有需要 Shortcut 未公开的底层参数或原始响应时才直接调用本 atomic 命令。")
}

func renderChatHelpCommandSection(w io.Writer, title string, commands []*cobra.Command) {
	if len(commands) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, title)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, command := range commands {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", command.Name(), strings.TrimSpace(command.Short))
	}
	_ = tw.Flush()
}

func renderCommandFlagSections(w io.Writer, cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	if flags := strings.TrimRight(cmd.LocalNonPersistentFlags().FlagUsages(), "\n"); strings.TrimSpace(flags) != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "Flags:")
		_, _ = fmt.Fprintln(w, flags)
	}
	if flags := strings.TrimRight(cmd.InheritedFlags().FlagUsages(), "\n"); strings.TrimSpace(flags) != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "Global Flags:")
		_, _ = fmt.Fprintln(w, flags)
	}
}

func renderRootHelp(root *cobra.Command) {
	roothelp.Render(root.OutOrStdout(), RootHelpModel(root))
}

// RootHelpModel projects help from the final tree using the same visibility,
// locale and pflag type rules as the public entrypoint. It neither resolves
// Schema nor reads a cache; callers must not treat it as declaration authority.
func RootHelpModel(root *cobra.Command) roothelp.Model {
	model := roothelp.Model{Long: root.Long}
	for _, cmd := range visibleMCPRootCommands(root) {
		model.Services = append(model.Services, roothelp.Command{Name: cmd.Name(), Short: cmd.Short})
	}
	for _, cmd := range visibleUtilityRootCommands(root) {
		model.Utilities = append(model.Utilities, roothelp.Command{Name: cmd.Name(), Short: commandShort(cmd)})
	}
	model.Flags = rootHelpFlags(root)
	return model
}

func rootHelpFlags(root *cobra.Command) []roothelp.Flag {
	var flags []roothelp.Flag
	for _, flag := range visiblePersistentFlags(root) {
		flags = append(flags, roothelp.Flag{Label: formatRootFlag(flag), Usage: flag.Usage})
	}
	return flags
}

func renderRootGlobalFlags(root *cobra.Command) {
	if root == nil {
		return
	}
	roothelp.RenderGlobalFlags(root.OutOrStdout(), rootHelpFlags(root))
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
// set), StaticServers product IDs, and DirectRuntimeProductIDs(), so
// dynamically-registered products — including plugins loaded via
// AppendDynamicServer — are never silently hidden by a static VisibleProducts
// list.
//
// StaticServers are consulted directly (without injectStaticServers) so the
// declaration-only Schema source root can keep reviewed products visible
// without mutating the process-global dynamic endpoint registry.
// SupplementServers stay out of this set: they are helper-only endpoints and
// must not synthesize top-level product visibility.
func resolveVisibleProducts() map[string]bool {
	allowed := map[string]bool{}
	if fn := edition.Get().VisibleProducts; fn != nil {
		for _, p := range fn() {
			allowed[p] = true
		}
	}
	if fn := edition.Get().StaticServers; fn != nil {
		for _, server := range fn() {
			if id := strings.TrimSpace(server.ID); id != "" {
				allowed[id] = true
			}
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
