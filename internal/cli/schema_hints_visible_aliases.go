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
	RegisterRuntimeSchemaRoot("calendar", newRuntimeAliasRoot("hardcoded:calendar",
		map[string]string{
			"calendar attendee add":    "add_calendar_participant",
			"calendar attendee delete": "remove_calendar_participant",
			"calendar attendee list":   "get_calendar_participants",
		},
		nil,
	))

	RegisterRuntimeSchemaRoot("chat", newRuntimeAliasRoot("hardcoded:chat",
		map[string]string{
			"chat bot find":                 "search_bots",
			"chat bot search":               "search_my_robots",
			"chat category create-smart":    "create_smart_conv_category",
			"chat group bots":               "list_group_bots",
			"chat group create":             "create_group_conversation",
			"chat group members add-bot":    "add_robot_to_group",
			"chat group members remove-bot": "remove_robot_in_group",
			"chat message recall-by-bot":    "recall_robot_message",
			"chat message reply":            "reply_personal_message",
			"chat message send":             "send_personal_message",
			"chat message send-by-bot":      "send_robot_message",
			"chat message send-by-webhook":  "send_message_by_custom_robot",
		},
		nil,
	))

	RegisterRuntimeSchemaRoot("contact", newRuntimeAliasRoot("hardcoded:contact",
		map[string]string{
			"contact get":       "get_user_info_by_user_ids",
			"contact search":    "search_contact_by_key_word",
			"contact user list": "search_contact_by_key_word",
		},
		map[string]string{
			"get_user_info_by_user_ids":  "contact user get",
			"search_contact_by_key_word": "contact user search",
		},
	))

	RegisterRuntimeSchemaRoot("devdoc", newRuntimeAliasRoot("hardcoded:devdoc",
		map[string]string{
			"devdoc article search":                      "search_open_platform_docs_rag",
			"devdoc article-search":                      "search_open_platform_docs_rag",
			"devdoc article rag-pretest":                 "devdoc_rag_pretest",
			"devdoc article-rag-pretest":                 "devdoc_rag_pretest",
			"devdoc article legacy-search":               "search_dingtalk_open_docs",
			"devdoc article-legacy-search":               "search_dingtalk_open_docs",
			"devdoc article legacy-search-open-platform": "search_open_platform_docs",
			"devdoc article-legacy-search-open-platform": "search_open_platform_docs",
			"devdoc error diagnose":                      "search_open_error_code_rag",
			"devdoc error-diagnose":                      "search_open_error_code_rag",
		},
		map[string]string{
			"search_open_platform_docs_rag": "devdoc article search",
			"search_open_error_code_rag":    "devdoc error diagnose",
		},
	))

	RegisterRuntimeSchemaRoot("live", newRuntimeAliasRoot("hardcoded:live",
		map[string]string{
			"live list": "get_my_lives",
		},
		nil,
	))

	RegisterRuntimeSchemaRoot("mail", newRuntimeAliasRoot("hardcoded:mail",
		map[string]string{
			"mail message list": "search_emails",
			"mail search":       "search_emails",
			"mail send":         "send_email",
		},
		map[string]string{
			"search_emails": "mail message search",
			"send_email":    "mail message send",
		},
	))

	RegisterRuntimeSchemaRoot("minutes", newRuntimeAliasRoot("hardcoded:minutes",
		map[string]string{
			"minutes list all":    "list_by_keyword_and_time_range",
			"minutes list mine":   "list_by_keyword_and_time_range",
			"minutes list shared": "list_by_keyword_and_time_range",
		},
		map[string]string{
			"list_by_keyword_and_time_range": "minutes list mine",
		},
	))

	RegisterRuntimeSchemaRoot("report", newRuntimeAliasRoot("hardcoded:report",
		map[string]string{
			"report create":          "create_report",
			"report submit":          "create_report",
			"report detail":          "get_report_entry_details",
			"report get":             "get_report_entry_details",
			"report inbox-list":      "get_received_report_list",
			"report list":            "get_received_report_list",
			"report outbox-list":     "get_send_report_list",
			"report sent":            "get_send_report_list",
			"report created":         "get_send_report_list",
			"report stats":           "get_report_statistics_by_id",
			"report template detail": "get_template_details_by_name",
			"report template get":    "get_template_details_by_name",
			"report template-get":    "get_template_details_by_name",
			"report template list":   "get_available_report_templates",
			"report template-list":   "get_available_report_templates",
		},
		map[string]string{
			"create_report":                "report entry submit",
			"get_report_entry_details":     "report entry get",
			"get_report_statistics_by_id":  "report entry stats",
			"get_received_report_list":     "report inbox list",
			"get_send_report_list":         "report outbox list",
			"get_template_details_by_name": "report template get",
		},
	))

	RegisterRuntimeSchemaRoot("todo", newRuntimeAliasRoot("hardcoded:todo",
		map[string]string{
			"todo create":              "create_personal_todo",
			"todo task create":         "create_personal_todo",
			"todo list":                "get_user_todos_in_current_org",
			"todo task list":           "get_user_todos_in_current_org",
			"todo task update":         "update_todo_task",
			"todo done":                "update_todo_done_status",
			"todo task done":           "update_todo_done_status",
			"todo task get":            "get_todo_detail",
			"todo task delete":         "delete_todo",
			"todo task add-attachment": "add_todo_attachment",
		},
		map[string]string{
			"create_personal_todo":          "todo task create",
			"get_user_todos_in_current_org": "todo task list",
			"update_todo_done_status":       "todo task done",
		},
	))
}

func newRuntimeAliasRoot(source string, toolNames map[string]string, primary map[string]string) RuntimeSchemaRootHint {
	include := make(map[string]bool, len(toolNames))
	for cliPath := range toolNames {
		include[cliPath] = true
	}
	return RuntimeSchemaRootHint{
		Source:          source,
		ToolNames:       toolNames,
		PrimaryCLIPaths: primary,
		IncludeCLIPaths: include,
	}
}
