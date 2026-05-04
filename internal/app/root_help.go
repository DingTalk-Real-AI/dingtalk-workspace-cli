package app

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
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

	if len(services) == 0 {
		_, _ = fmt.Fprintln(w, "No MCP services discovered.")
		_, _ = fmt.Fprintln(w)
	} else {
		_, _ = fmt.Fprintln(w, "Discovered MCP Services:")
		_, _ = fmt.Fprintln(w)

		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, service := range services {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\n", service.Name(), strings.TrimSpace(service.Short))
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  dws <service> [command] [flags]")
	if len(utilities) > 0 {
		_, _ = fmt.Fprintln(w, "  dws <command> [flags]")
	}
	_, _ = fmt.Fprintln(w)
	if len(utilities) > 0 {
		_, _ = fmt.Fprintln(w, "Utility Commands:")
		_, _ = fmt.Fprintln(w)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, utility := range utilities {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\n", utility.Name(), strings.TrimSpace(utility.Short))
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintln(w, `Use "dws <service> --help" for more information about a discovered MCP service or "dws <command> --help" for utility commands.`)
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
	if len(allowed) == 0 {
		return nil
	}

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
