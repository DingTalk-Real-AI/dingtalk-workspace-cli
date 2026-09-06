package launcher

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localename"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/roothelp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemafastpath"
)

func tryTrackedHelp(options Options, deps dependencies) (bool, error) {
	if len(deps.args) != 2 || deps.args[1] != "--help" || options.HelpSnapshot == "" || deps.trackRun == nil || deps.defaultIdentity == nil {
		return false, nil
	}
	configDir, plain := schemafastpath.PlainInvocation(options.Edition, options.SchemaIdentity, schemafastpath.Dependencies{Args: deps.args, Environment: deps.environ, Lstat: deps.lstat})
	if !plain {
		return false, nil
	}
	model, err := roothelp.DecodeSnapshot(options.HelpSnapshot, options.CoreSHA256, options.Commit, options.Edition, localename.Resolve(environmentValue(deps.environ, "LANG")))
	if err != nil {
		return false, nil
	}
	return true, runTrackedPresentation(options, deps, configDir, func() error {
		// Cobra's existing root HelpFunc ignores writer errors. Preserve that
		// contract, including no fallback after any partial output.
		roothelp.Render(deps.stdout, model)
		return nil
	})
}
