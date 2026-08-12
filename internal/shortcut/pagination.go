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
	"fmt"
	"time"
)

// AutoPageControlFlags returns the shared item and pacing controls used by
// cursor-based shortcuts. Callers retain ownership of --page-all and their
// product-specific --page-limit defaults.
func AutoPageControlFlags() []Flag {
	const evidence = "--max-items/--page-delay 仅与 --page-all 一起使用；值必须大于等于 0"
	return []Flag{
		{Name: "max-items", Type: FlagInt, Desc: "自动翻页最多返回条数（默认 0 表示不限制）。" + evidence},
		{Name: "page-delay", Type: FlagInt, Desc: "自动翻页每页之间等待毫秒数（默认 0 表示不等待）。" + evidence},
	}
}

// AutoPageControlConstraints publishes the runtime-only relationship between
// the pagination switch and its shared controls into Help and Schema.
func AutoPageControlConstraints() []Constraint {
	return []Constraint{{
		Kind:        ConstraintCustom,
		Flags:       []string{"page-all", "max-items", "page-delay"},
		Description: "--max-items/--page-delay 仅与 --page-all 一起使用；值必须大于等于 0",
	}}
}

// ValidateAutoPageControls enforces the shared contract while preserving the
// historical behavior that defaulted controls do not activate pagination.
func ValidateAutoPageControls(rt *RuntimeContext) error {
	if !rt.Bool("page-all") {
		if rt.Changed("max-items") || rt.Changed("page-delay") {
			return fmt.Errorf("--max-items/--page-delay 仅与 --page-all 一起使用")
		}
		return nil
	}
	if rt.Int("max-items") < 0 {
		return fmt.Errorf("--max-items 必须大于等于 0")
	}
	if rt.Int("page-delay") < 0 {
		return fmt.Errorf("--page-delay 必须大于等于 0")
	}
	return nil
}

// AutoPageRequestSize caps the next lower-page request to the remaining item
// budget. A cursor returned for that request is therefore safe to resume from:
// the CLI did not intentionally discard a suffix of the lower page.
func AutoPageRequestSize(rt *RuntimeContext, pageSize, itemCount int) int {
	maxItems := rt.Int("max-items")
	if maxItems <= 0 {
		return pageSize
	}
	remaining := maxItems - itemCount
	if remaining > 0 && remaining < pageSize {
		return remaining
	}
	return pageSize
}

// WaitAutoPageDelay waits between successful pages and remains cancellable so
// a throttled pagination run cannot ignore command cancellation.
func WaitAutoPageDelay(rt *RuntimeContext) error {
	delayMS := rt.Int("page-delay")
	if delayMS <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(delayMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-rt.Command().Context().Done():
		return rt.Command().Context().Err()
	case <-timer.C:
		return nil
	}
}
