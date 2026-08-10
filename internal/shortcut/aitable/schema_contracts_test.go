// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageAITableLegacyContractLedgerIsExact(t *testing.T) {
	if got, want := len(reviewedAITableShortcutContractCommands), 53; got != want {
		t.Fatalf("legacy contract migration ledger = %d, want %d", got, want)
	}
	for command := range reviewedAITableShortcutContractCommands {
		found := false
		for _, item := range shortcut.All() {
			if item.Service == "aitable" && item.Command == command {
				found = true
				if item.Contract.Empty() {
					t.Errorf("%s: migration did not deliver Contract", command)
				}
				if item.Safety.Confirmation == "" {
					t.Errorf("%s: migration did not deliver explicit Safety", command)
				}
				break
			}
		}
		if !found {
			t.Errorf("stale migration ledger entry %s", command)
		}
	}
}
