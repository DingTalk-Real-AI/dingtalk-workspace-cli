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
	RegisterSchemaHints("pat", map[string]ToolSchemaHint{
		"batch_grant": {
			Parameters: map[string]ParameterSchemaHint{
				"sessionId": {
					Required:     boolPtr(false),
					RequiredWhen: "grant-type is session",
				},
			},
		},
	})

	RegisterSchemaHints("doc", map[string]ToolSchemaHint{
		"create_document": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
		"get_document_content": {
			Parameters: map[string]ParameterSchemaHint{
				"node": {Required: boolPtr(true)},
			},
		},
		"update_document": {
			Parameters: map[string]ParameterSchemaHint{
				"node": {Required: boolPtr(true)},
			},
		},
		"delete_document": {
			Parameters: map[string]ParameterSchemaHint{
				"node": {Required: boolPtr(true)},
			},
		},
	})

	RegisterSchemaHints("drive", map[string]ToolSchemaHint{
		"list_files": {
			Parameters: map[string]ParameterSchemaHint{
				"limit": {Type: "integer"},
			},
		},
		"list_spaces": {
			Parameters: map[string]ParameterSchemaHint{
				"limit": {Type: "integer"},
			},
		},
	})

	RegisterSchemaHints("sheet", map[string]ToolSchemaHint{
		"create_workspace_sheet": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
		"get_all_sheets": {
			Parameters: map[string]ParameterSchemaHint{
				"node": {Required: boolPtr(true)},
			},
		},
		"create_sheet": {
			Parameters: map[string]ParameterSchemaHint{
				"node": {Required: boolPtr(true)},
				"name": {Required: boolPtr(true)},
			},
		},
	})

	RegisterSchemaHints("wiki", map[string]ToolSchemaHint{
		"create_wikiSpace": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
		"get_wikiSpace": {
			Parameters: map[string]ParameterSchemaHint{
				"space": {Required: boolPtr(true)},
			},
		},
	})
}
