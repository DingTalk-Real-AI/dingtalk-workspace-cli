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
	RegisterSchemaHints("calendar", map[string]ToolSchemaHint{
		"create_calendar_event": {
			Parameters: calendarRecurrenceSchemaParameters(true),
		},
		"query_busy_status": {
			Parameters: map[string]ParameterSchemaHint{
				"users": {Required: boolPtr(true)},
			},
		},
		"update_calendar_event": {
			Parameters: calendarRecurrenceSchemaParameters(false),
		},
	})

	RegisterSchemaHints("minutes", map[string]ToolSchemaHint{
		"get_minutes_ai_summary": {
			Parameters: map[string]ParameterSchemaHint{
				"taskUuid": {Required: boolPtr(true)},
			},
		},
	})

	RegisterSchemaHints("group-chat", map[string]ToolSchemaHint{
		"search_messages_by_sender": {
			Parameters: map[string]ParameterSchemaHint{
				"sender-user-id": {Required: boolPtr(true)},
			},
		},
	})

	RegisterSchemaHints("dev", map[string]ToolSchemaHint{
		"get_extension_webapp_config": {
			Parameters: map[string]ParameterSchemaHint{
				"unifiedAppId": {Required: boolPtr(true)},
			},
		},
		"set_extension_robot_config": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
		"set_extension_webapp_config": {
			Parameters: map[string]ParameterSchemaHint{
				"homepageUrl": {Required: boolPtr(true)},
			},
		},
		"update_dev_app": {
			Parameters: map[string]ParameterSchemaHint{
				"name": {Required: boolPtr(true)},
			},
		},
		"update_dev_app_security_config": {
			Parameters: map[string]ParameterSchemaHint{
				"redirectUrls": {Required: boolPtr(true)},
			},
		},
	})
}

func calendarRecurrenceSchemaParameters(includeCreateRequired bool) map[string]ParameterSchemaHint {
	parameters := map[string]ParameterSchemaHint{
		"recurrence.pattern.dayOfMonth": {
			Required:     boolPtr(false),
			RequiredWhen: "recurrence-type is absoluteMonthly or absoluteYearly",
		},
		"recurrence.pattern.daysOfWeek": {
			Required:     boolPtr(false),
			RequiredWhen: "recurrence-type is weekly or relativeMonthly",
		},
		"recurrence.pattern.index": {
			Required:     boolPtr(false),
			RequiredWhen: "recurrence-type is relativeMonthly",
		},
		"recurrence.pattern.interval": {
			Required:     boolPtr(false),
			RequiredWhen: "any recurrence-* flag is provided",
		},
		"recurrence.pattern.type": {
			Required:     boolPtr(false),
			RequiredWhen: "any recurrence-* flag is provided",
		},
		"recurrence.range.endDate": {
			Required:     boolPtr(false),
			RequiredWhen: "recurrence-range-type is endDate",
		},
		"recurrence.range.numberOfOccurrences": {
			Required:     boolPtr(false),
			RequiredWhen: "recurrence-range-type is numbered",
		},
		"recurrence.range.type": {
			Required:     boolPtr(false),
			RequiredWhen: "any recurrence-* flag is provided",
		},
	}
	if includeCreateRequired {
		parameters["summary"] = ParameterSchemaHint{Required: boolPtr(true)}
		parameters["startDateTime"] = ParameterSchemaHint{Required: boolPtr(true)}
		parameters["endDateTime"] = ParameterSchemaHint{Required: boolPtr(true)}
	}
	return parameters
}
