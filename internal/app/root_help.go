package app

import (
	"fmt"
	"strings"
	"text/tabwriter"

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
			return
		}
		renderRootHelp(root)
	})
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
