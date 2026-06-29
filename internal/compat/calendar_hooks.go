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

package compat

import (
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// calendarRecurrenceTools are the calendar leaves whose recurrence fields must
// be supplied as a complete set (the MCP backend does not merge partial
// recurrence, so a partial update would silently overwrite the rule). Mirrors
// wukong's calendar.go event create/update validation.
var calendarRecurrenceTools = map[string]bool{
	"create_calendar_event": true,
	"update_calendar_event": true,
}

// calendarRecurrenceFlags is the full set of --recurrence-* flags; touching any
// of them requires the core structural fields to be present.
var calendarRecurrenceFlags = []string{
	"recurrence-type", "recurrence-interval", "recurrence-range-type",
	"recurrence-count", "recurrence-end-date", "recurrence-days-of-week",
	"recurrence-day-of-month", "recurrence-month", "recurrence-week-index",
	"recurrence-first-day-of-week",
}

// installCalendarHook wires calendar-specific PreRunE validators onto leaf
// commands emitted by BuildDynamicCommands. No-op for non-calendar products and
// calendar tools without extra client-side checks. The hook chain preserves the
// PreRunE that NewDirectCommand already installed by invoking it first.
func installCalendarHook(cmd *cobra.Command, canonicalProduct, toolName string) {
	if cmd == nil || strings.TrimSpace(canonicalProduct) != "calendar" {
		return
	}
	if !calendarRecurrenceTools[toolName] {
		return
	}
	original := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if original != nil {
			if err := original(c, args); err != nil {
				return err
			}
		}
		return validateCalendarRecurrence(c)
	}
}

// validateCalendarRecurrence refuses a partial recurrence structure. If any
// --recurrence-* flag is set, recurrence-type / interval / range-type must be
// present, and weekly / relativeMonthly patterns require days-of-week. Error
// wording carries the kebab flag names so the messages match wukong and the
// auto-test substring assertions (days-of-week / recurrence-type).
func validateCalendarRecurrence(cmd *cobra.Command) error {
	if cmd == nil {
		return nil
	}
	used := false
	for _, f := range calendarRecurrenceFlags {
		if fl := cmd.Flags().Lookup(f); fl != nil && cmd.Flags().Changed(f) {
			used = true
			break
		}
	}
	if !used {
		return nil
	}

	recType := strings.TrimSpace(calendarFlagString(cmd, "recurrence-type"))
	if recType == "" {
		return apperrors.NewValidation(
			"recurrence 结构不完整：使用任一 --recurrence-* 时必须整体重传完整循环字段" +
				"（至少 --recurrence-type / --recurrence-interval / --recurrence-range-type，" +
				"MCP 不合并部分字段）")
	}
	if !calendarFlagSet(cmd, "recurrence-interval") {
		return apperrors.NewValidation(
			"recurrence 结构不完整：缺少 --recurrence-interval（循环间隔，recurrence 整体必填）")
	}
	if !calendarFlagSet(cmd, "recurrence-range-type") {
		return apperrors.NewValidation(
			"recurrence 结构不完整：缺少 --recurrence-range-type（循环范围类型，recurrence 整体必填）")
	}
	if recType == "weekly" || recType == "relativeMonthly" {
		if strings.TrimSpace(calendarFlagString(cmd, "recurrence-days-of-week")) == "" {
			return apperrors.NewValidation(
				"weekly / relativeMonthly 循环必须提供 --recurrence-days-of-week (daysOfWeek)")
		}
	}
	return nil
}

func calendarFlagString(cmd *cobra.Command, name string) string {
	if cmd.Flags().Lookup(name) == nil {
		return ""
	}
	v, _ := cmd.Flags().GetString(name)
	return v
}

// calendarFlagSet reports whether a flag was explicitly provided by the user,
// tolerating both string and int (--recurrence-interval) flag kinds.
func calendarFlagSet(cmd *cobra.Command, name string) bool {
	fl := cmd.Flags().Lookup(name)
	if fl == nil {
		return false
	}
	return cmd.Flags().Changed(name)
}
