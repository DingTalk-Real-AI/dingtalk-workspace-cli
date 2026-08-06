// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import "github.com/spf13/cobra"

// RunDocImportShortcut exposes the existing, fully-tested Doc import pipeline
// to the Shortcut application layer. The Cobra leaf still owns its own flags
// and Contract; this bridge only shares the raw/API execution primitive.
func RunDocImportShortcut(cmd *cobra.Command) error {
	return runImportCommand(cmd, nil, docImportFlowConfig())
}

// RunDocMediaInsertShortcut shares the existing prepare + OSS PUT + block
// insertion implementation with the canonical Doc Shortcut.
func RunDocMediaInsertShortcut(cmd *cobra.Command) error {
	return runMediaInsert(cmd, nil)
}

// RunDocResourceUpdateShortcut shares the cover upload/transfer pipeline.
func RunDocResourceUpdateShortcut(cmd *cobra.Command) error {
	return runDocStyleCoverSet(cmd, nil)
}
