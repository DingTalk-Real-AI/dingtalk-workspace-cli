// Package roothelp renders the root's declaration-derived help projection.
// It does not parse commands or own their visibility, flags, or descriptions.
package roothelp

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
)

const FeedbackFormURL = "https://alidocs.dingtalk.com/notable/share/form/v01eLbnj1bw1ELb0laN_dv19yqvsgs3oebp3pcjys_1qX0QQ0?source=dws-cli"

// Model is a display projection of the final command tree, not a command registry.
// The producer supplies visibility, locale, and display order; rendering does
// not infer or modify the executable command surface.
type Model struct {
	Services  []Command
	Utilities []Command
	Flags     []Flag
	Long      string
}
type Command struct{ Name, Short string }
type Flag struct{ Label, Usage string }

func Render(w io.Writer, model Model) {
	services, utilities := model.Services, model.Utilities

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
			_, _ = fmt.Fprintf(tw, "  %s %s\t%s\n", tui.StateMark("ok"), tui.Bold(service.Name), tui.Dim(strings.TrimSpace(service.Short)))
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
			_, _ = fmt.Fprintf(tw, "  %s %s\t%s\n", tui.Bullet(), tui.Bold(utility.Name), tui.Dim(utility.Short))
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(w)
	}
	RenderGlobalFlags(w, model.Flags)
	renderAgentQuickstart(w)
	renderSafetyModel(w)
	_, _ = fmt.Fprintf(w, "%s %s\n", tui.Key("Next"), `Use "dws <service> --help" for more information about a discovered MCP service or "dws <command> --help" for utility commands.`)

	// Render model.Long after the command list so agents see the upgrade
	// hint (or any other root-level guidance) after browsing all available
	// commands and concluding none of them fit. Cobra's default help template
	// would render Long automatically; the custom SetHelpFunc above replaces
	// it and dropped this, so we restore it explicitly here.
	if long := strings.TrimSpace(model.Long); long != "" {
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
	_, _ = fmt.Fprintf(w, "    %s\n", tui.Cyan(FeedbackFormURL))
}

func RenderGlobalFlags(w io.Writer, flags []Flag) {
	if len(flags) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, tui.Section("Global Flags:"))
	_, _ = fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, flag := range flags {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", flag.Label, tui.Dim(strings.TrimSpace(flag.Usage)))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintln(w)
}
