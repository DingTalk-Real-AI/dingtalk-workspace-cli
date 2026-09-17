// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import (
	"testing"

	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// Cross-package test helper. Production code must not call this; the ForTest
// suffix is the boundary.

// SwapMessageDecryptClientForTest replaces the injected decrypt client for the
// test duration and restores the previous value via testseam. Sequential tests
// only.
func SwapMessageDecryptClientForTest(t *testing.T, client *messagecrypto.Client) {
	t.Helper()
	testseam.Swap(t, &messageDecryptClient, client)
}
