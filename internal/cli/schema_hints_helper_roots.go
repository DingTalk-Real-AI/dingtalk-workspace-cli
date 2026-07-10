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
	RegisterSchemaProductVisibility("doc-comment", SchemaVisibilityInternal)
	RegisterSchemaProductVisibility("hrmregister", SchemaVisibilityInternal)

	RegisterRuntimeSchemaRoot("doc", RuntimeSchemaRootHint{
		Source: "hardcoded:doc",
		ToolNames: map[string]string{
			"doc search":                "search_documents",
			"doc list":                  "list_nodes",
			"doc read":                  "get_document_content",
			"doc info":                  "get_document_info",
			"doc create":                "create_document",
			"doc update":                "update_document",
			"doc download":              "download_file",
			"doc file create":           "create_file",
			"doc folder create":         "create_folder",
			"doc copy":                  "copy_document",
			"doc move":                  "move_document",
			"doc rename":                "rename_document",
			"doc block list":            "list_document_blocks",
			"doc block insert":          "insert_document_block",
			"doc block update":          "update_document_block",
			"doc block delete":          "delete_document_block",
			"doc comment list":          "list_comments",
			"doc comment create":        "create_comment",
			"doc comment reply":         "reply_comment",
			"doc comment create-inline": "create_inline_comment",
			"doc export get":            "query_export_job",
			"doc permission add":        "add_permission",
			"doc permission update":     "update_permission",
			"doc permission list":       "list_permission",
			"doc permission remove":     "remove_permission",
			"doc delete":                "delete_document",
			"doc media download":        "download_doc_attachment",
		},
	})

	RegisterRuntimeSchemaRoot("drive", RuntimeSchemaRootHint{
		Source: "hardcoded:drive",
		ToolNames: map[string]string{
			"drive list":            "list_files",
			"drive list-spaces":     "list_spaces",
			"drive info":            "get_file_info",
			"drive download":        "download_file",
			"drive mkdir":           "create_folder",
			"drive upload-info":     "get_upload_info",
			"drive commit":          "commit_upload",
			"drive delete":          "delete_document",
			"drive search":          "search_files",
			"drive copy":            "copy_document",
			"drive move":            "move_document",
			"drive rename":          "rename_document",
			"drive permission add":  "add_permission",
			"drive permission list": "list_permission",
			"drive folder create":   "create_folder",
		},
	})

	RegisterRuntimeSchemaRoot("sheet", RuntimeSchemaRootHint{
		Source: "hardcoded:sheet",
		ToolNames: map[string]string{
			"sheet create":               "create_workspace_sheet",
			"sheet list":                 "get_all_sheets",
			"sheet new":                  "create_sheet",
			"sheet update":               "update_sheet",
			"sheet copy":                 "copy_sheet",
			"sheet delete-sheet":         "delete_sheet",
			"sheet range write":          "set_cell_range",
			"sheet range clear":          "clear_range",
			"sheet range sort":           "sort_range",
			"sheet range fill":           "fill_range",
			"sheet range copy":           "copy_range",
			"sheet range move":           "move_range",
			"sheet find":                 "find_cells",
			"sheet append":               "append_rows",
			"sheet csv-put":              "set_range_from_csv",
			"sheet csv-get":              "get_range_as_csv",
			"sheet insert-dimension":     "insert_dimension",
			"sheet delete-dimension":     "delete_dimension",
			"sheet update-dimension":     "update_dimension",
			"sheet move-dimension":       "move_dimension",
			"sheet add-dimension":        "add_dimension",
			"sheet merge-cells":          "merge_cells",
			"sheet unmerge-cells":        "unmerge_range",
			"sheet set-dropdown":         "set_dropdown_lists",
			"sheet get-dropdown":         "get_dropdown_lists",
			"sheet delete-dropdown":      "delete_dropdown_lists",
			"sheet replace":              "replace_all",
			"sheet filter get":           "get_filter",
			"sheet filter delete":        "delete_filter",
			"sheet filter create":        "create_filter",
			"sheet filter update":        "update_filter",
			"sheet filter sort":          "sort_filter",
			"sheet filter-view list":     "get_filter_views",
			"sheet filter-view create":   "create_filter_view",
			"sheet filter-view update":   "update_filter_view",
			"sheet filter-view delete":   "delete_filter_view",
			"sheet cond-format list":     "get_cond_format",
			"sheet cond-format create":   "create_cond_format",
			"sheet cond-format update":   "update_cond_format",
			"sheet cond-format delete":   "delete_cond_format",
			"sheet create-float-image":   "create_float_image",
			"sheet list-float-images":    "list_float_images",
			"sheet update-float-image":   "update_float_image",
			"sheet export":               "submit_export_job",
			"sheet write-image":          "write_image",
			"sheet range update-complex": "update_range",
		},
	})

	RegisterRuntimeSchemaRoot("wiki", RuntimeSchemaRootHint{
		Source: "hardcoded:wiki",
		ToolNames: map[string]string{
			"wiki create":        "create_wikiSpace",
			"wiki space create":  "create_wikiSpace",
			"wiki get":           "get_wikiSpace",
			"wiki space get":     "get_wikiSpace",
			"wiki list":          "list_wikiSpaces",
			"wiki space list":    "list_wikiSpaces",
			"wiki search":        "search_wikiSpaces",
			"wiki space search":  "search_wikiSpaces",
			"wiki delete":        "delete_wikiSpace",
			"wiki space delete":  "delete_wikiSpace",
			"wiki member add":    "add_member",
			"wiki member update": "update_member",
			"wiki member list":   "list_member",
			"wiki member remove": "remove_member",
			"wiki node list":     "list_nodes",
			"wiki doc list":      "list_nodes",
			"wiki file create":   "create_file",
			"wiki node create":   "create_file",
			"wiki file delete":   "delete_document",
			"wiki node delete":   "delete_document",
			"wiki file search":   "search_documents",
			"wiki doc search":    "search_documents",
		},
		PrimaryCLIPaths: map[string]string{
			"create_wikiSpace":  "wiki space create",
			"get_wikiSpace":     "wiki space get",
			"list_wikiSpaces":   "wiki space list",
			"search_wikiSpaces": "wiki space search",
			"delete_wikiSpace":  "wiki space delete",
		},
	})

	RegisterRuntimeSchemaRoot("pat", RuntimeSchemaRootHint{
		Source: "hardcoded:pat",
		ToolNames: map[string]string{
			"pat chmod": "batch_grant",
		},
	})
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
