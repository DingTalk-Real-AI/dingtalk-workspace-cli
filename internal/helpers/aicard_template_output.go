// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const aicardTemplateOutputConflictMessage = "--out and --output must refer to different files"

var errAicardTemplateOutputConflict = errors.New(aicardTemplateOutputConflictMessage)

func aicardTemplateCommandOutput(cmd *cobra.Command) (string, error) {
	if cmd.Flags().Lookup("output") == nil {
		return "", nil
	}
	value, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", apperrors.NewValidation("Cannot read --output", apperrors.WithCause(err))
	}
	if value = strings.TrimSpace(value); value == "" {
		return "", nil
	}
	path, err := aicardTemplateAbs(value)
	if err != nil {
		return "", apperrors.NewValidation("Cannot resolve --output", apperrors.WithCause(err))
	}
	return path, nil
}

func aicardTemplateOutputConflict(path, commandOutput string) bool {
	if commandOutput == "" {
		return false
	}
	if path == commandOutput {
		return true
	}
	if filepath.Base(path) != filepath.Base(commandOutput) {
		return false
	}
	// Parent identity also catches symlinked directory aliases before retrieval.
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return false
	}
	other, err := os.Stat(filepath.Dir(commandOutput))
	return err == nil && os.SameFile(parent, other)
}
