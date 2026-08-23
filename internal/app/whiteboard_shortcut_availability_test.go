// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/spf13/cobra"
)

func assertWhiteboardPublicShortcutsStayAvailableInSchema(t *testing.T, root *cobra.Command, tools map[string]map[string]any) {
	t.Helper()
	for canonical, command := range map[string]string{
		"whiteboard.shortcut_query":  "+query",
		"whiteboard.shortcut_update": "+update",
	} {
		leaf, _, err := root.Find([]string{"whiteboard", command})
		if err != nil || leaf == nil || leaf.Name() != command {
			t.Errorf("find whiteboard %s: leaf=%v err=%v", command, leaf, err)
		} else if leaf.Hidden || !leaf.Runnable() {
			t.Errorf("whiteboard %s hidden/runnable=%v/%v, want false/true", command, leaf.Hidden, leaf.Runnable())
		}
		tool := tools[canonical]
		if tool == nil {
			t.Errorf("public %s missing from delivery Schema surface", canonical)
			continue
		}
		if got := schemaContractString(tool["availability"]); got != "available" {
			t.Errorf("%s availability=%q, want available", canonical, got)
		}
		if got := schemaContractString(tool["interface_mode"]); got != "composite" {
			t.Errorf("%s interface_mode=%q, want composite", canonical, got)
		}
		if got := schemaContractString(tool["interface_reason"]); got == "" {
			t.Errorf("%s missing composite adapter reason", canonical)
		}
		if tool["interface_ref"] != nil {
			t.Errorf("%s composite interface_ref=%#v, want nil", canonical, tool["interface_ref"])
		}
	}
}
