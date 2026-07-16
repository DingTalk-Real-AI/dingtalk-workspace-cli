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

package cli

func init() {
	RegisterSchemaHints("attendance", map[string]ToolSchemaHint{
		"get_user_attendance_record": {
			Parameters: map[string]ParameterSchemaHint{
				"user":  {Required: boolPtr(true)},
				"date":  {Required: boolPtr(true)},
				"users": {Type: "array", Required: boolPtr(true)},
				"start": {Required: boolPtr(true)},
				"end":   {Required: boolPtr(true)},
			},
		},
		"batch_get_employee_shifts": {
			Parameters: map[string]ParameterSchemaHint{
				"users": {Type: "array", Required: boolPtr(true)},
				"start": {Required: boolPtr(true)},
				"end":   {Required: boolPtr(true)},
			},
		},
		"get_attendance_summary": {
			Parameters: map[string]ParameterSchemaHint{
				"user":       {Required: boolPtr(true)},
				"date":       {Required: boolPtr(true)},
				"stats-type": {Required: boolPtr(true)},
			},
		},
		"query_attendance_group_or_rules": {
			Parameters: map[string]ParameterSchemaHint{
				"date": {Required: boolPtr(true)},
			},
		},
		"boss_check": {
			Parameters: map[string]ParameterSchemaHint{
				"plan-id": {Required: boolPtr(true)},
			},
		},
		"save_self_setting": {
			Parameters: map[string]ParameterSchemaHint{
				"check-result-msg": {Required: boolPtr(true)},
			},
		},
		"update_group_member": {
			Parameters: map[string]ParameterSchemaHint{
				"add-users": {Required: boolPtr(true)},
			},
		},
		"update_group_setting": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
	})
}
