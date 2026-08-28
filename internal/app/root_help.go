package app

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// feedbackFormURL points at the DingTalk Notable form collecting dws CLI
// user-experience feedback. The source parameter tags submissions that
// originated from the CLI help output so they can be told apart from
// responses arriving through other channels.
const feedbackFormURL = "https://alidocs.dingtalk.com/notable/share/form/v01eLbnj1bw1ELb0laN_dv19yqvsgs3oebp3pcjys_1qX0QQ0?source=dws-cli"

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
			cli.RenderHelpAffordances(cmd)
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
	renderAgentQuickstart(w)
	renderSafetyModel(w)
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

	// Keep the feedback entry last: everything above it is operational guidance
	// an agent acts on, while the survey is addressed to human readers who
	// scroll to the end.
	_, _ = fmt.Fprintln(w)
	renderRootFeedback(w)
}

func renderAgentQuickstart(w io.Writer) {
	_, _ = fmt.Fprintln(w, tui.Section("Agent Quickstart:"))
	_, _ = fmt.Fprintln(w, "  1. Browse a product: dws <service> --help")
	_, _ = fmt.Fprintln(w, "  2. Inspect leaf parameters and semantics: dws <path> --help")
	_, _ = fmt.Fprintln(w, `  3. Read the machine contract: dws schema --cli-path "<path>" --compact -f json`)
	_, _ = fmt.Fprintln(w, "  4. Prefer structured output. Use dry-run only when the leaf explicitly supports it.")
	_, _ = fmt.Fprintln(w, "  5. Never add --yes without explicit user confirmation.")
	_, _ = fmt.Fprintln(w)
}

func renderSafetyModel(w io.Writer) {
	_, _ = fmt.Fprintln(w, tui.Section("Safety model:"))
	_, _ = fmt.Fprintln(w, "  effect=read|write|destructive — whether the command reads, changes, or irreversibly removes state")
	_, _ = fmt.Fprintln(w, "  risk=low|medium|high — expected impact if the command is used incorrectly")
	_, _ = fmt.Fprintln(w, "  confirmation=not_required|user_required — whether explicit user approval is required")
	_, _ = fmt.Fprintln(w, "  idempotency=idempotent|retryable|non_idempotent|unknown — whether repeating the command is safe")
	_, _ = fmt.Fprintln(w)
}

// renderRootFeedback prints the user-experience survey entry. The URL occupies
// its own line and is never wrapped or padded through a tabwriter: it is longer
// than the help rule width, and breaking it would stop terminals from
// recognizing it as a clickable hyperlink. Soft wrapping performed by the
// terminal itself keeps the link intact.
//
// The label is intentionally not routed through i18n. Everything surrounding it
// in this listing — service descriptions, utility descriptions, global flag
// usage — is hardcoded Chinese, so translating this one line would render it in
// English on any host whose LANG is not zh_*, leaving a single English line
// inside an otherwise Chinese screen.
func renderRootFeedback(w io.Writer) {
	_, _ = fmt.Fprintln(w, tui.Section("Feedback:"))
	_, _ = fmt.Fprintf(w, "  %s %s\n", tui.Bullet(), tui.Dim("使用体验反馈问卷（1 分钟）"))
	_, _ = fmt.Fprintf(w, "    %s\n", tui.Cyan(feedbackFormURL))
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
