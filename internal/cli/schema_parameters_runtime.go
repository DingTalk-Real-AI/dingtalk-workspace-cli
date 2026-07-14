// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

const (
	runtimeSchemaFlagMetadataRequiredAnnotation     = "dws.schema.metadata.required"
	runtimeSchemaFlagMetadataRequiredWhenAnnotation = "dws.schema.metadata.required_when"
	runtimeSchemaFlagMetadataFormatAnnotation       = "dws.schema.metadata.format"
	runtimeSchemaFlagMetadataEnumAnnotation         = "dws.schema.metadata.enum"
	runtimeSchemaFlagMetadataExampleAnnotation      = "dws.schema.metadata.example"
)

// RuntimeSchemaParameterMetadata contains reviewed CLI parameter semantics
// that Cobra cannot represent by itself. It is an independent generation
// input: generated Catalog data is never read back into this registry.
type RuntimeSchemaParameterMetadata struct {
	Inherited    []string
	Required     []string
	RequiredWhen map[string]string
	Formats      map[string]string
	Enums        map[string][]string
	Examples     map[string]string
}

var runtimeSchemaParameterMetadataByCanonical = map[string]RuntimeSchemaParameterMetadata{}

// RegisterRuntimeSchemaParameterMetadata records strong, code-owned parameter
// semantics for one canonical command path.
func RegisterRuntimeSchemaParameterMetadata(canonicalPath string, metadata RuntimeSchemaParameterMetadata) {
	canonicalPath = strings.TrimSpace(canonicalPath)
	if canonicalPath == "" {
		return
	}
	if _, exists := runtimeSchemaParameterMetadataByCanonical[canonicalPath]; exists {
		panic("duplicate runtime schema parameter metadata: " + canonicalPath)
	}
	runtimeSchemaParameterMetadataByCanonical[canonicalPath] = metadata
}

func applyRuntimeSchemaParameterMetadata(cmd *cobra.Command, canonicalPath string) {
	metadata, ok := runtimeSchemaParameterMetadataByCanonical[strings.TrimSpace(canonicalPath)]
	if !ok {
		return
	}
	for _, flagName := range metadata.Required {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagMetadataRequiredAnnotation, "true")
		}
	}
	for flagName, expression := range metadata.RequiredWhen {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagMetadataRequiredWhenAnnotation, strings.TrimSpace(expression))
		}
	}
	for flagName, format := range metadata.Formats {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagMetadataFormatAnnotation, strings.TrimSpace(format))
		}
	}
	for flagName, values := range metadata.Enums {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotationValues(flag, runtimeSchemaFlagMetadataEnumAnnotation, values...)
		}
	}
	for flagName, example := range metadata.Examples {
		if flag := runtimeCommandFlag(cmd, flagName); flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagMetadataExampleAnnotation, strings.TrimSpace(example))
		}
	}
}

// RuntimeSchemaParameterMetadataDefinitions returns a defensive copy for
// build-time contract validation.
func RuntimeSchemaParameterMetadataDefinitions() map[string]RuntimeSchemaParameterMetadata {
	out := make(map[string]RuntimeSchemaParameterMetadata, len(runtimeSchemaParameterMetadataByCanonical))
	for canonicalPath, metadata := range runtimeSchemaParameterMetadataByCanonical {
		copyMetadata := RuntimeSchemaParameterMetadata{
			Inherited:    append([]string(nil), metadata.Inherited...),
			Required:     append([]string(nil), metadata.Required...),
			RequiredWhen: cloneRuntimeSchemaStringMap(metadata.RequiredWhen),
			Formats:      cloneRuntimeSchemaStringMap(metadata.Formats),
			Examples:     cloneRuntimeSchemaStringMap(metadata.Examples),
			Enums:        make(map[string][]string, len(metadata.Enums)),
		}
		for flagName, values := range metadata.Enums {
			copyMetadata.Enums[flagName] = append([]string(nil), values...)
		}
		out[canonicalPath] = copyMetadata
	}
	return out
}

func cloneRuntimeSchemaStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func init() {
	RegisterRuntimeSchemaParameterMetadata("aisearch.enterprise_person_search", RuntimeSchemaParameterMetadata{
		Required: []string{"keyword"},
	})
	RegisterRuntimeSchemaParameterMetadata("aitable.export_data", RuntimeSchemaParameterMetadata{
		RequiredWhen: map[string]string{
			"table-id": "scope is table or view",
			"view-id":  "scope is view",
		},
		Enums: map[string][]string{
			"scope":         {"all", "table", "view"},
			"export-format": {"excel", "attachment", "excel_and_attachment", "excel_with_inline_images"},
		},
	})
	RegisterRuntimeSchemaParameterMetadata("aitable.record_get", RuntimeSchemaParameterMetadata{
		Required: []string{"record-ids"},
	})
	RegisterRuntimeSchemaParameterMetadata("aitable.view_update_filter", RuntimeSchemaParameterMetadata{
		Required: []string{"json"},
		Formats:  map[string]string{"json": "json"},
		Examples: map[string]string{"json": `[{"operator":"and","operands":[{"operator":"eq","operands":["fldX","value"]}]}]`},
	})
	RegisterRuntimeSchemaParameterMetadata("aitable.view_update_group", RuntimeSchemaParameterMetadata{
		Required: []string{"json"},
		Formats:  map[string]string{"json": "json"},
		Examples: map[string]string{"json": `[{"fieldId":"fldX","direction":"asc"}]`},
	})
	RegisterRuntimeSchemaParameterMetadata("aitable.view_update_sort", RuntimeSchemaParameterMetadata{
		Required: []string{"json"},
		Formats:  map[string]string{"json": "json"},
		Examples: map[string]string{"json": `[{"fieldId":"fldX","direction":"asc"}]`},
	})
	RegisterRuntimeSchemaParameterMetadata("attendance.batch_get_employee_shifts", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{"start": "date", "end": "date"},
	})
	RegisterRuntimeSchemaParameterMetadata("attendance.approve_list", RuntimeSchemaParameterMetadata{
		Formats:  map[string]string{"start": "date", "end": "date"},
		Examples: map[string]string{"types": "overtime,leave"},
	})
	RegisterRuntimeSchemaParameterMetadata("attendance.selfsetting_get", RuntimeSchemaParameterMetadata{
		Enums: map[string][]string{
			"setting-scene": {
				"checkRemind", "fastCheck", "checkResultNotify", "lackRemind",
				"personalAttendStatNotify", "bossAttendStatNotify",
			},
		},
	})
	RegisterRuntimeSchemaParameterMetadata("calendar.add_attachments", RuntimeSchemaParameterMetadata{
		Formats:  map[string]string{"files": "file-id-name-list"},
		Examples: map[string]string{"files": "file-smoke:report.pdf"},
	})
	RegisterRuntimeSchemaParameterMetadata("calendar.add_calendar_participant", RuntimeSchemaParameterMetadata{
		Inherited: []string{"event", "calendar-id"},
	})
	RegisterRuntimeSchemaParameterMetadata("calendar.get_calendar_participants", RuntimeSchemaParameterMetadata{
		Inherited: []string{"event", "calendar-id"},
	})
	RegisterRuntimeSchemaParameterMetadata("calendar.remove_calendar_participant", RuntimeSchemaParameterMetadata{
		Inherited: []string{"event", "calendar-id"},
	})
	RegisterRuntimeSchemaParameterMetadata("calendar.respond", RuntimeSchemaParameterMetadata{
		Enums: map[string][]string{"status": {"needsAction", "accepted", "declined", "tentative"}},
	})
	RegisterRuntimeSchemaParameterMetadata("chat.download_media", RuntimeSchemaParameterMetadata{
		Enums: map[string][]string{"type": {"mediaId"}},
	})
	RegisterRuntimeSchemaParameterMetadata("chat.list_owned_or_admin_groups", RuntimeSchemaParameterMetadata{
		Enums: map[string][]string{"role": {"OWNER", "ADMIN"}},
	})
	RegisterRuntimeSchemaParameterMetadata("chat.send_personal_message", RuntimeSchemaParameterMetadata{
		RequiredWhen: map[string]string{
			"media-id":  "msg-type is image",
			"file-path": "msg-type is file or audio or video",
		},
		Examples: map[string]string{"media-id": "@lADP_schema_smoke"},
	})
	RegisterRuntimeSchemaParameterMetadata("chat.set_group_member_mute_list", RuntimeSchemaParameterMetadata{
		RequiredWhen: map[string]string{"mute-time": "off is false"},
		Enums: map[string][]string{
			"mute-time": {"300000", "3600000", "86400000", "604800000", "2592000000"},
		},
	})
	RegisterRuntimeSchemaParameterMetadata("chat.update_group_icon", RuntimeSchemaParameterMetadata{
		Formats:  map[string]string{"icon-media-id": "dingtalk-media-id"},
		Examples: map[string]string{"icon-media-id": "@lADP_schema_smoke"},
	})
	RegisterRuntimeSchemaParameterMetadata("chat.update_show_history_msg_option", RuntimeSchemaParameterMetadata{
		Enums: map[string][]string{"option": {"FORBIDDEN", "RECENT_100", "ALL"}},
	})
	RegisterRuntimeSchemaParameterMetadata("contact.get_dept_info_by_dept_id", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{"dept": "numeric-id"},
	})
	RegisterRuntimeSchemaParameterMetadata("contact.get_sub_depts_by_dept_id", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{"dept": "numeric-id"},
	})
	RegisterRuntimeSchemaParameterMetadata("dev.apply_dev_app_permissions", RuntimeSchemaParameterMetadata{
		Required: []string{"scope-values"},
	})
	RegisterRuntimeSchemaParameterMetadata("dev.remove_dev_app_permissions", RuntimeSchemaParameterMetadata{
		Required: []string{"scope-values"},
	})
	RegisterRuntimeSchemaParameterMetadata("dev.subscribe_dev_app_events", RuntimeSchemaParameterMetadata{
		Required: []string{"event-codes"},
	})
	RegisterRuntimeSchemaParameterMetadata("dev.unsubscribe_dev_app_events", RuntimeSchemaParameterMetadata{
		Required: []string{"event-codes"},
	})
	RegisterRuntimeSchemaParameterMetadata("minutes.add_member_permission", RuntimeSchemaParameterMetadata{
		Enums: map[string][]string{"policy": {"0", "1", "2", "3", "4"}},
	})
	RegisterRuntimeSchemaParameterMetadata("pat.browser_policy", RuntimeSchemaParameterMetadata{
		Required: []string{"enabled"},
		Enums:    map[string][]string{"enabled": {"true", "false"}},
	})
	RegisterRuntimeSchemaParameterMetadata("report.create_report", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{"contents": "json"},
		Examples: map[string]string{
			"contents": `[{"content":"schema smoke","sort":"0","key":"work","contentType":"markdown","type":"1"}]`,
		},
	})
	RegisterRuntimeSchemaParameterMetadata("sheet.batch_update", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{"operations": "json"},
		Examples: map[string]string{
			"operations": `[{"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B2","type":"content"}}]`,
		},
	})
	chartPropertiesExample := `{"position":{"row":0,"col":"A"},"dimensions":{"width":600,"height":400},"chart":{"type":"column","series":[{"value":["B2:B10"]}],"category":["A2:A10"]}}`
	for _, canonicalPath := range []string{"sheet.chart_create", "sheet.chart_update"} {
		RegisterRuntimeSchemaParameterMetadata(canonicalPath, RuntimeSchemaParameterMetadata{
			Formats:  map[string]string{"properties": "json"},
			Examples: map[string]string{"properties": chartPropertiesExample},
		})
	}
	RegisterRuntimeSchemaParameterMetadata("sheet.create_pivot_table", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{"properties": "json"},
		Examples: map[string]string{
			"properties": `{"values":[{"field":"Amount","summarize_by":"sum"}]}`,
		},
	})
	RegisterRuntimeSchemaParameterMetadata("sheet.update_pivot_table", RuntimeSchemaParameterMetadata{
		Formats:  map[string]string{"properties": "json"},
		Examples: map[string]string{"properties": `{"show_subtotals":false}`},
	})
	RegisterRuntimeSchemaParameterMetadata("sheet.range_batch_clear", RuntimeSchemaParameterMetadata{
		Formats:  map[string]string{"ranges": "json"},
		Examples: map[string]string{"ranges": `["Sheet1!A1:B2"]`},
	})
	RegisterRuntimeSchemaParameterMetadata("todo.add_todo_reminder", RuntimeSchemaParameterMetadata{
		RequiredWhen: map[string]string{
			"due-date-offset":     "base-time is dueTime",
			"reminder-time-stamp": "base-time is customTime",
		},
		Enums: map[string][]string{"base-time": {"dueTime", "customTime"}},
		Examples: map[string]string{
			"due-date-offset":     "-30",
			"reminder-time-stamp": "2026-03-10T18:00:00+08:00",
		},
	})
	RegisterRuntimeSchemaParameterMetadata("todo.get_user_todos_in_current_org", RuntimeSchemaParameterMetadata{
		Examples: map[string]string{"role-types": "creator,executor"},
	})

	for _, canonicalPath := range []string{
		"sheet.add_dimension",
		"sheet.delete_dimension",
		"sheet.insert_dimension",
		"sheet.move_dimension",
		"sheet.update_dimension",
	} {
		metadata := RuntimeSchemaParameterMetadata{
			Enums: map[string][]string{"dimension": {"ROWS", "COLUMNS"}},
		}
		switch canonicalPath {
		case "sheet.delete_dimension", "sheet.insert_dimension":
			metadata.Formats = map[string]string{"length": "positive-integer"}
			metadata.Examples = map[string]string{"position": "1"}
		case "sheet.update_dimension":
			metadata.Formats = map[string]string{"length": "positive-integer"}
			metadata.Examples = map[string]string{"start-index": "1"}
		}
		RegisterRuntimeSchemaParameterMetadata(canonicalPath, metadata)
	}
	RegisterRuntimeSchemaParameterMetadata("sheet.create_cond_format", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{"ranges": "json", "condition": "json", "cell-style": "json", "data-bar-style": "json"},
		Examples: map[string]string{
			"ranges":     `["A1:B2"]`,
			"condition":  `{"numberCondition":{"operator":"greater","value1":"80"}}`,
			"cell-style": `{"backgroundColor":"#FFCDD2","bold":true}`,
		},
	})
	RegisterRuntimeSchemaParameterMetadata("sheet.update_cond_format", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{"ranges": "json", "condition": "json", "cell-style": "json", "data-bar-style": "json"},
		Examples: map[string]string{
			"ranges":     `["A1:B2"]`,
			"condition":  `{"numberCondition":{"operator":"greater","value1":"80"}}`,
			"cell-style": `{"backgroundColor":"#FFCDD2","bold":true}`,
		},
	})
	RegisterRuntimeSchemaParameterMetadata("sheet.range_set_style", RuntimeSchemaParameterMetadata{
		Formats: map[string]string{
			"bg-colors-json":    "json",
			"font-colors-json":  "json",
			"font-sizes-json":   "json",
			"font-weights-json": "json",
			"h-aligns-json":     "json",
			"v-aligns-json":     "json",
		},
		Enums: map[string][]string{
			"font-weight": {"bold", "normal"},
			"h-align":     {"left", "center", "right", "general"},
			"v-align":     {"top", "middle", "bottom"},
			"word-wrap":   {"overflow", "clip", "autoWrap"},
		},
		Examples: map[string]string{
			"bg-colors-json":    `[["#ffffff","#ffffff"],["#ffffff","#ffffff"]]`,
			"font-colors-json":  `[["#000000","#000000"],["#000000","#000000"]]`,
			"font-sizes-json":   `[[12,12],[12,12]]`,
			"font-weights-json": `[["normal","normal"],["normal","normal"]]`,
			"h-aligns-json":     `[["left","left"],["left","left"]]`,
			"v-aligns-json":     `[["top","top"],["top","top"]]`,
		},
	})
	RegisterRuntimeSchemaParameterMetadata("sheet.set_dropdown_lists", RuntimeSchemaParameterMetadata{
		Formats:  map[string]string{"options": "json"},
		Examples: map[string]string{"options": `[{"value":"option1"},{"value":"option2"}]`},
	})
	RegisterRuntimeSchemaParameterMetadata("wiki.search_wikiSpaces", RuntimeSchemaParameterMetadata{
		Enums: map[string][]string{"type": {"myWikiSpace"}},
	})
}
