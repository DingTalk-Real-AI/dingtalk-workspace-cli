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
	"encoding/json"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// attendanceScheduleInnerRequired are the fields every scheduleVOS item must
// carry; the backend rejects partial items with an opaque error, so the CLI
// validates them up front (mirrors wukong's attendance.go).
var attendanceScheduleInnerRequired = []string{"userId", "workDate", "classId", "isRest"}

var attendanceGroupTypes = map[string]bool{"FIXED": true, "TURN": true, "NONE": true}

// installAttendanceHook wires attendance-specific PreRunE validators that
// mirror wukong's client-side checks (inner-JSON required fields, group type,
// FIXED conditional requirements, group-update no-op). No-op for other
// products / tools. Preserves any PreRunE NewDirectCommand already installed.
func installAttendanceHook(cmd *cobra.Command, canonicalProduct, toolName string) {
	if cmd == nil || strings.TrimSpace(canonicalProduct) != "attendance" {
		return
	}
	var validate func(*cobra.Command) error
	switch toolName {
	case "generateTurnSchedule":
		validate = validateAttendanceScheduleImport
	case "create_class_setting":
		validate = validateAttendanceClassCreate
	case "create_group_setting":
		validate = validateAttendanceGroupCreate
	case "update_group_setting":
		validate = validateAttendanceGroupUpdate
	default:
		return
	}
	original := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if original != nil {
			if err := original(c, args); err != nil {
				return err
			}
		}
		return validate(c)
	}
}

func attFlagString(cmd *cobra.Command, names ...string) string {
	for _, n := range names {
		if cmd.Flags().Lookup(n) == nil {
			continue
		}
		if v, err := cmd.Flags().GetString(n); err == nil && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func validateAttendanceScheduleImport(cmd *cobra.Command) error {
	raw := attFlagString(cmd, "scheduleVOS", "schedules")
	if raw == "" {
		return nil // empty is owned by the required-flag check
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil // malformed JSON is owned by a separate check
	}
	for _, item := range items {
		for _, f := range attendanceScheduleInnerRequired {
			if _, ok := item[f]; !ok {
				return apperrors.NewValidation("missing required field: " + f + "（--scheduleVOS 每个排班项必填）")
			}
		}
	}
	return nil
}

func validateAttendanceClassCreate(cmd *cobra.Command) error {
	raw := attFlagString(cmd, "class-vo", "TopAtClassVO")
	if raw == "" {
		return nil
	}
	var vo map[string]any
	if err := json.Unmarshal([]byte(raw), &vo); err != nil {
		return nil
	}
	if _, ok := vo["sections"]; !ok {
		return apperrors.NewValidation("missing required field: sections（班次时段，--class-vo 内必填）")
	}
	return nil
}

func validateAttendanceGroupCreate(cmd *cobra.Command) error {
	typ := strings.TrimSpace(attFlagString(cmd, "type"))
	if typ != "" && !attendanceGroupTypes[typ] {
		return apperrors.NewValidation("考勤组类型不合法：--type 应为 FIXED / TURN / NONE 之一")
	}
	if typ == "FIXED" {
		var vo map[string]any
		if raw := attFlagString(cmd, "group-vo", "groupVO"); raw != "" {
			_ = json.Unmarshal([]byte(raw), &vo)
		}
		if vo == nil {
			vo = map[string]any{}
		}
		if _, ok := vo["workDayClassList"]; !ok {
			return apperrors.NewValidation("type=FIXED 时 --group-vo 内必填 workDayClassList（工作日班次列表）")
		}
		if _, ok := vo["defaultClassId"]; !ok {
			return apperrors.NewValidation("type=FIXED 时 --group-vo 内必填 defaultClassId（默认班次 ID）")
		}
	}
	return nil
}

func validateAttendanceGroupUpdate(cmd *cobra.Command) error {
	for _, f := range []string{"name", "type", "owner", "enable-outside-check", "classIds", "group-vo"} {
		if fl := cmd.Flags().Lookup(f); fl != nil && cmd.Flags().Changed(f) {
			return nil
		}
	}
	return apperrors.NewValidation("至少需要指定一个修改项（--name / --type / --owner / --enable-outside-check / --classIds / --group-vo）")
}
