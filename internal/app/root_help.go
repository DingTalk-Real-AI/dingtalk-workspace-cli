package app

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func configureRootHelp(root *cobra.Command) {
	if root == nil {
		return
	}

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
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, `Use "dws <service> --help" for more information about a discovered MCP service.`)
}

func visibleMCPRootCommands(root *cobra.Command) []*cobra.Command {
	if root == nil {
		return nil
	}

	allowed := DirectRuntimeProductIDs()
	if len(allowed) == 0 {
		return nil
	}

	commands := make([]*cobra.Command, 0)
	seen := make(map[string]struct{})
	for _, cmd := range root.Commands() {
		if cmd == nil || cmd.Hidden {
			continue
		}
		name := strings.TrimSpace(cmd.Name())
		if !allowed[name] {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		commands = append(commands, cmd)
	}
	return commands
}
