package app

import (
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/recovery"
)

func captureRuntimeFailure(invocation executor.Invocation, rawErr, wrappedErr error) {
	if rawErr == nil && wrappedErr == nil {
		return
	}
	store := recovery.NewStore(defaultConfigDir())
	if store == nil || !store.Enabled() {
		return
	}
	input := recovery.CaptureInput{
		CommandPath: runtimeCommandPath(invocation),
		ServerID:    strings.TrimSpace(invocation.CanonicalProduct),
		ToolName:    strings.TrimSpace(invocation.Tool),
		Args:        cloneRecoveryArgs(invocation.Params),
		Argv:        append([]string(nil), os.Args[1:]...),
		RawErr:      rawErr,
		WrappedErr:  wrappedErr,
	}
	_, _ = store.Capture(recovery.BuildContext(input), recovery.BuildReplay(input))
}

func runtimeCommandPath(invocation executor.Invocation) []string {
	if path := currentCommandPath(); len(path) > 0 {
		return path
	}
	if legacy := strings.Fields(strings.TrimSpace(invocation.LegacyPath)); len(legacy) > 0 {
		return legacy
	}
	if product := strings.TrimSpace(invocation.CanonicalProduct); product != "" {
		if tool := strings.TrimSpace(invocation.Tool); tool != "" {
			return []string{product, tool}
		}
		return []string{product}
	}
	return nil
}

func currentCommandPath() []string {
	boolFlags := map[string]struct{}{
		"--verbose": {},
		"-v":        {},
		"--debug":   {},
		"--mock":    {},
		"--dry-run": {},
		"--yes":     {},
		"-y":        {},
		"--help":    {},
		"-h":        {},
		"--json":    {},
	}
	path := make([]string, 0, len(os.Args))
	skipNext := false
	for _, arg := range os.Args[1:] {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			if strings.Contains(arg, "=") {
				continue
			}
			if _, ok := boolFlags[arg]; ok {
				continue
			}
			skipNext = true
			continue
		}
		path = append(path, arg)
	}
	return path
}

func cloneRecoveryArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}
