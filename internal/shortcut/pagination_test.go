// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shortcut

import (
	"context"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
)

func autoPageRuntimeForTest(t *testing.T, pageAll bool, maxItems, pageDelay string) *RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "page"}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Int("max-items", 0, "")
	cmd.Flags().Int("page-delay", 0, "")
	if pageAll {
		if err := cmd.Flags().Set("page-all", "true"); err != nil {
			t.Fatal(err)
		}
	}
	for name, value := range map[string]string{"max-items": maxItems, "page-delay": pageDelay} {
		if value != "" {
			if err := cmd.Flags().Set(name, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	return RuntimeContextForTest(cmd, Shortcut{})
}

func TestCrossPlatformCoverageAutoPageControlsBoundDelayAndRequestSize(t *testing.T) {
	if err := ValidateAutoPageControls(autoPageRuntimeForTest(t, false, "1", "")); err == nil {
		t.Fatal("max-items without page-all unexpectedly succeeded")
	}
	if err := ValidateAutoPageControls(autoPageRuntimeForTest(t, true, "-1", "")); err == nil {
		t.Fatal("negative max-items unexpectedly succeeded")
	}
	if err := ValidateAutoPageControls(autoPageRuntimeForTest(t, true, "", "-1")); err == nil {
		t.Fatal("negative page-delay unexpectedly succeeded")
	}
	if err := ValidateAutoPageControls(autoPageRuntimeForTest(t, true, "", strconv.FormatInt(maxAutoPageDelayMS, 10))); err != nil {
		t.Fatalf("maximum safe page-delay failed: %v", err)
	}
	if strconv.IntSize == 64 {
		tooLarge := strconv.FormatInt(maxAutoPageDelayMS+1, 10)
		if err := ValidateAutoPageControls(autoPageRuntimeForTest(t, true, "", tooLarge)); err == nil {
			t.Fatal("overflowing page-delay unexpectedly succeeded")
		}
	}

	if got := AutoPageRequestSize(autoPageRuntimeForTest(t, true, "0", ""), 100, 40); got != 100 {
		t.Fatalf("unlimited request size = %d", got)
	}
	if got := AutoPageRequestSize(autoPageRuntimeForTest(t, true, "50", ""), 100, 40); got != 10 {
		t.Fatalf("remaining request size = %d", got)
	}
	if got := AutoPageRequestSize(autoPageRuntimeForTest(t, true, "50", ""), 100, 50); got != 100 {
		t.Fatalf("exhausted request size = %d", got)
	}
}

func TestCrossPlatformCoverageWaitAutoPageDelayNoopAndCancellation(t *testing.T) {
	if err := WaitAutoPageDelay(autoPageRuntimeForTest(t, true, "", "0")); err != nil {
		t.Fatalf("zero delay = %v", err)
	}
	if err := WaitAutoPageDelay(autoPageRuntimeForTest(t, true, "", "1")); err != nil {
		t.Fatalf("elapsed delay = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rt := autoPageRuntimeForTest(t, true, "", "1")
	rt.Command().SetContext(ctx)
	if err := WaitAutoPageDelay(rt); err != context.Canceled {
		t.Fatalf("canceled delay = %v, want context.Canceled", err)
	}
}
