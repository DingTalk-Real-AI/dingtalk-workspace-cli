// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

import (
	"context"
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/buildversion"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/clisignal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/clitelemetry"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemafastpath"
)

func tryTrackedVersion(options Options, deps dependencies) (bool, error) {
	if deps.trackRun == nil || deps.defaultIdentity == nil {
		return false, nil
	}
	configDir, plain := schemafastpath.PlainInvocation(options.Edition, options.SchemaIdentity, schemafastpath.Dependencies{
		Args: deps.args, Environment: deps.environ, Lstat: deps.lstat,
	})
	if !plain {
		return false, nil
	}
	return true, runTrackedPresentation(options, deps, configDir, func() error {
		_, err := fmt.Fprintf(deps.stdout, "dws version %s\n", buildversion.Format(options.Version, options.Commit, options.BuildTime))
		return err
	})
}

func runTrackedPresentation(options Options, deps dependencies, configDir string, execute func() error) error {
	identity := clitelemetry.Identity{}
	optedOut := telemetryOptedOut(deps.environ)
	if !optedOut {
		identity = deps.defaultIdentity(configDir)
	}
	code, commandPath, errorMessage := 0, "dws", ""
	cfg := clitelemetry.Configuration(options.Version, identity, &commandPath, &errorMessage)
	if optedOut {
		cfg.PID = ""
	}
	deps.trackRun(cfg, func() error {
		code, errorMessage = executeTrackedPresentation(deps, execute)
		if code == 0 {
			return nil
		}
		return clitelemetry.RenderedError{}
	}, func(error) int { return code })
	if code != 0 {
		return &ExitError{Code: code}
	}
	return nil
}

func executeTrackedPresentation(deps dependencies, execute func() error) (code int, summary string) {
	// Match core's outer recovery: cleanup runs before reporting a panic, and
	// the SDK receives only the fixed summary, never arbitrary panic data.
	defer func() {
		if value := recover(); value != nil {
			code, summary = 5, "internal panic"
			fmt.Fprintf(deps.stderr, "Error: internal panic: %v\n", value)
		}
	}()
	install := deps.presentationSignals
	if install == nil {
		install = func() (*clisignal.State, func()) {
			_, state, stop := clisignal.Install(context.Background(), nil)
			return state, stop
		}
	}
	state, stop := install()
	defer stop()
	err := execute()
	if interrupted, _ := state.Outcome(); interrupted != nil {
		err = interrupted.WithCancellationDetail(err)
	}
	if err == nil {
		return 0, ""
	}
	// Plain help/version has neither JSON-error nor verbosity flags. Preserve
	// core's classification, human error output and reviewed c5 summary.
	_ = apperrors.PrintHumanAt(deps.stderr, err, apperrors.VerbosityNormal)
	return apperrors.ExitCode(err), clitelemetry.ErrorSummary(err)
}
