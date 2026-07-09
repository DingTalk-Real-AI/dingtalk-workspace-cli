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
	RegisterRuntimeSchemaRoot("aitable", RuntimeSchemaRootHint{
		Source: "hardcoded:aitable",
		ToolNames: map[string]string{
			"aitable record query": "query_records",
			"aitable record list":  "query_records",
		},
		PrimaryCLIPaths: map[string]string{
			"query_records": "aitable record query",
		},
	})

	RegisterSchemaHints("aitable", map[string]ToolSchemaHint{
		"attachment_upload": {
			Parameters: map[string]ParameterSchemaHint{
				"size": {Required: boolPtr(true)},
			},
		},
		"chart_update": {
			Parameters: map[string]ParameterSchemaHint{
				"config": {Required: boolPtr(true)},
			},
		},
		"dashboard_create": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
		"dashboard_update": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
		"export_data": {
			Parameters: map[string]ParameterSchemaHint{
				"scope": {Required: boolPtr(true)},
			},
		},
		"field_create": {
			Parameters: map[string]ParameterSchemaHint{
				"fields": {Required: boolPtr(true)},
			},
		},
		"field_update": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
		"query_records": {
			Description:    "查询 AI 表格记录。默认返回单页；传 --all 时自动翻页累计全部记录。",
			PrimaryCLIPath: "aitable record query",
			Parameters: map[string]ParameterSchemaHint{
				"baseId": {
					FlagName:    "base-id",
					Description: "Base ID。",
				},
				"tableId": {
					FlagName:    "table-id",
					Description: "Table ID。",
				},
				"fieldIds": {
					FlagName:    "field-ids",
					Type:        "array",
					Description: "Field ID 列表，CLI 使用逗号分隔。",
				},
				"recordIds": {
					FlagName:    "record-ids",
					Type:        "array",
					Description: "Record ID 列表，CLI 使用逗号分隔。",
				},
				"filters": {
					Type:        "object",
					Description: "过滤条件 JSON。",
				},
				"sort": {
					Type:        "array",
					Description: "排序 JSON 数组。",
				},
			},
		},
		"record_upsert": {
			Parameters: map[string]ParameterSchemaHint{
				"records": {Required: boolPtr(true)},
			},
		},
		"view_update_aggregate": {
			Parameters: map[string]ParameterSchemaHint{
				"json": {Required: boolPtr(true)},
			},
		},
		"view_update_card": {
			Parameters: map[string]ParameterSchemaHint{
				"json": {Required: boolPtr(true)},
			},
		},
		"view_update_field_widths": {
			Parameters: map[string]ParameterSchemaHint{
				"json": {Required: boolPtr(true)},
			},
		},
		"view_update_timebar": {
			Parameters: map[string]ParameterSchemaHint{
				"json": {Required: boolPtr(true)},
			},
		},
		"view_update_visible_fields": {
			Parameters: map[string]ParameterSchemaHint{
				"field-ids": {Required: boolPtr(true)},
			},
		},
	})
}
