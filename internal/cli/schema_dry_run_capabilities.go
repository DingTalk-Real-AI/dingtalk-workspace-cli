// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// dryRunCapabilityGroup is a reviewed positive capability declaration. An
// absent canonical deliberately publishes no dry_run field.
type dryRunCapabilityGroup struct {
	PreviewKind    string
	CanonicalPaths []string
}

// reviewedDryRunCapabilityGroups is a versioned authoring input, not generated
// at runtime. The initial candidates were produced by the exhaustive real-Cobra
// audit and grouped by their observed preview contract, then committed for
// review. CI consumes this list but never mutates it.
var reviewedDryRunCapabilityGroups = []dryRunCapabilityGroup{
	{PreviewKind: DryRunPreviewInvocation, CanonicalPaths: []string{
		"aisearch.enterprise_person_search",
		"aisearch.search_enterprise",
		"aisearch.search_enterprise_behavior",
		"aitable.advperm_disable",
		"aitable.advperm_enable",
		"aitable.advperm_role_create",
		"aitable.advperm_role_get",
		"aitable.advperm_role_list",
		"aitable.advperm_role_update",
		"aitable.attachment_upload",
		"aitable.base_copy",
		"aitable.base_create",
		"aitable.base_get",
		"aitable.base_get_primary_doc_id",
		"aitable.base_list",
		"aitable.base_search",
		"aitable.base_update",
		"aitable.chart_create",
		"aitable.chart_get",
		"aitable.chart_share_get",
		"aitable.chart_share_update",
		"aitable.chart_update",
		"aitable.chart_widgets_example",
		"aitable.create",
		"aitable.dashboard_arrange",
		"aitable.dashboard_config_example",
		"aitable.dashboard_create",
		"aitable.dashboard_get",
		"aitable.dashboard_share_get",
		"aitable.dashboard_share_update",
		"aitable.dashboard_update",
		"aitable.export_data",
		"aitable.field_create",
		"aitable.field_get",
		"aitable.field_list",
		"aitable.field_search_options",
		"aitable.field_update",
		"aitable.form_create",
		"aitable.form_field_hide",
		"aitable.form_field_list",
		"aitable.form_field_update",
		"aitable.form_list",
		"aitable.form_questions_create",
		"aitable.form_share_get",
		"aitable.form_share_update",
		"aitable.form_update",
		"aitable.import_data",
		"aitable.import_upload",
		"aitable.info",
		"aitable.list",
		"aitable.query_records",
		"aitable.record_batch_update",
		"aitable.record_create",
		"aitable.record_get",
		"aitable.record_history_list",
		"aitable.record_primary_doc_create",
		"aitable.record_primary_doc_get",
		"aitable.record_query_empty",
		"aitable.record_share_url",
		"aitable.record_update",
		"aitable.record_upsert",
		"aitable.search",
		"aitable.section_create",
		"aitable.section_delete",
		"aitable.section_list_empty",
		"aitable.section_list_nodes",
		"aitable.section_move_node",
		"aitable.section_rename",
		"aitable.section_reorder",
		"aitable.table_create",
		"aitable.table_get",
		"aitable.table_list",
		"aitable.table_update",
		"aitable.template_search",
		"aitable.view_create",
		"aitable.view_duplicate",
		"aitable.view_get_frozen_cols",
		"aitable.view_get_lock",
		"aitable.view_get_row_height",
		"aitable.view_list",
		"aitable.view_lock",
		"aitable.view_update_fill_color_rule",
		"aitable.view_update_filter",
		"aitable.view_update_frozen_cols",
		"aitable.view_update_group",
		"aitable.view_update_name",
		"aitable.view_update_row_height",
		"aitable.view_update_sort",
		"aitable.view_update_visible_fields",
		"aitable.workflow_enable",
		"aitable.workflow_get",
		"aitable.workflow_list",
		"attendance.adjustment_get",
		"attendance.adjustment_search",
		"attendance.approve_list",
		"attendance.approve_templates",
		"attendance.batch_get_employee_shifts",
		"attendance.check_record",
		"attendance.check_result",
		"attendance.checkin_records",
		"attendance.class_get",
		"attendance.class_search",
		"attendance.get_attendance_summary",
		"attendance.get_user_attendance_record",
		"attendance.globalsetting_get",
		"attendance.group_filtered_get",
		"attendance.group_get",
		"attendance.group_search",
		"attendance.overtime_get",
		"attendance.overtime_search",
		"attendance.query_attendance_group_or_rules",
		"attendance.report_columns",
		"attendance.report_query_data",
		"attendance.report_query_leave",
		"attendance.schedule_get",
		"attendance.selfsetting_get",
		"attendance.vacation_balance",
		"attendance.vacation_records",
		"attendance.vacation_types",
		"calendar.add_attachments",
		"calendar.add_calendar_participant",
		"calendar.add_meeting_room",
		"calendar.create_calendar_event",
		"calendar.delete_calendar_event",
		"calendar.delete_meeting_room",
		"calendar.get_calendar",
		"calendar.get_calendar_detail",
		"calendar.get_calendar_participants",
		"calendar.list_acls",
		"calendar.list_calendar_events",
		"calendar.list_calendars",
		"calendar.list_meeting_room_groups",
		"calendar.list_suggested_event_times",
		"calendar.remove_calendar_participant",
		"calendar.respond",
		"calendar.search_calendar",
		"calendar.update_calendar_event",
		"chat.add_custom_group_role",
		"chat.add_emoji_reaction",
		"chat.add_group_member",
		"chat.add_message_favorite",
		"chat.add_robot_to_group",
		"chat.add_text_emotion",
		"chat.combine_forward_messages",
		"chat.create_and_send_card",
		"chat.create_smart_conv_category",
		"chat.create_text_emotion",
		"chat.dismiss_group",
		"chat.forward_message",
		"chat.forward_topic",
		"chat.get_conv_info_by_group_id",
		"chat.get_conversation_info",
		"chat.get_group_invite_url",
		"chat.list_conversation_message_v2",
		"chat.list_conversations_by_category",
		"chat.list_custom_group_roles",
		"chat.list_group_bots",
		"chat.list_individual_chat_message",
		"chat.list_message_favorites",
		"chat.list_messages_by_ids",
		"chat.list_owned_or_admin_groups",
		"chat.list_pin_messages",
		"chat.list_special_focus_messages",
		"chat.list_top_conversations",
		"chat.list_topic_replies",
		"chat.list_user_define_conv_categories",
		"chat.query_custom_user_roles",
		"chat.query_message_send_status",
		"chat.query_msg_read_status",
		"chat.quit_group",
		"chat.recall_message",
		"chat.recall_robot_message",
		"chat.remove_custom_group_role",
		"chat.remove_custom_user_roles",
		"chat.remove_emoji_reaction",
		"chat.remove_message_favorite",
		"chat.remove_robot_in_group",
		"chat.remove_text_emotion",
		"chat.search_at_me_message",
		"chat.search_bots",
		"chat.search_common_groups",
		"chat.search_groups",
		"chat.search_messages",
		"chat.search_messages_by_keyword",
		"chat.search_messages_by_sender",
		"chat.search_messages_by_time_range",
		"chat.search_my_robots",
		"chat.send_message_by_custom_robot",
		"chat.send_personal_message",
		"chat.send_robot_message",
		"chat.set_custom_user_roles",
		"chat.set_group_member_mute_list",
		"chat.set_group_mute",
		"chat.set_pin_message",
		"chat.set_top_conversation",
		"chat.transfer_group_owner",
		"chat.unread_message_conversation_list",
		"chat.unset_pin_message",
		"chat.update_conv_member_roles",
		"chat.update_custom_group_role",
		"chat.update_group_icon",
		"chat.update_group_name",
		"chat.update_group_settings",
		"chat.update_notification_off",
		"chat.update_show_history_msg_option",
		"chat.update_streaming_card",
		"contact.get_authorized_emp_rosterInfo",
		"contact.get_current_user_profile",
		"contact.get_dept_info_by_dept_id",
		"contact.get_dept_members_by_deptId",
		"contact.get_sub_depts_by_dept_id",
		"contact.get_user_info_by_user_ids",
		"contact.list_authorized_roster_fields",
		"contact.list_my_followings",
		"contact.query_dismission_employee_list",
		"contact.search_contact_by_key_word",
		"contact.search_dept_by_keyword",
		"contact.search_user_by_mobile",
		"dev.search_open_platform_docs_rag",
		"devdoc.search_open_platform_docs_rag",
		"ding.recall_ding_message",
		"ding.send_ding_message",
		"doc.add_permission",
		"doc.copy_document",
		"doc.create_comment",
		"doc.create_document",
		"doc.create_file",
		"doc.create_folder",
		"doc.create_inline_comment",
		"doc.download_doc_attachment",
		"doc.get_document_content",
		"doc.get_document_info",
		"doc.insert_document_block",
		"doc.list_comments",
		"doc.list_document_blocks",
		"doc.list_nodes",
		"doc.list_permission",
		"doc.move_document",
		"doc.remove_permission",
		"doc.rename_document",
		"doc.reply_comment",
		"doc.search_documents",
		"doc.template_apply",
		"doc.template_list",
		"doc.template_search",
		"doc.update_comment",
		"doc.update_document_block",
		"doc.update_permission",
		"doc.version_list",
		"doc.version_save",
		"drive.add_permission",
		"drive.commit_upload",
		"drive.copy_document",
		"drive.create_folder",
		"drive.create_shortcut",
		"drive.get_node_stats",
		"drive.get_upload_info",
		"drive.list_files",
		"drive.list_permission",
		"drive.list_spaces",
		"drive.move_document",
		"drive.permission_remove",
		"drive.permission_update",
		"drive.publish_get",
		"drive.recent",
		"drive.recycle_list",
		"drive.recycle_restore",
		"drive.rename_document",
		"live.get_my_lives",
		"mail.batch_delete_message",
		"mail.batch_delete_user_mail_contacts",
		"mail.batch_move_message",
		"mail.create_draft",
		"mail.create_mail_folder",
		"mail.create_user_mail_contact",
		"mail.create_user_message_template",
		"mail.delete_mail_folder",
		"mail.delete_user_message_template",
		"mail.get_email_by_message_id",
		"mail.get_thread",
		"mail.get_user_message_template",
		"mail.list_emails",
		"mail.list_folders",
		"mail.list_mail_attachments",
		"mail.list_tags",
		"mail.list_user_mail_contacts",
		"mail.list_user_mailboxes",
		"mail.list_user_message_templates",
		"mail.search_emails",
		"mail.search_mail_users",
		"mail.send_draft",
		"mail.send_email",
		"mail.update_draft",
		"mail.update_mail_folder",
		"mail.update_user_mail_contact",
		"mail.update_user_message_template",
		"minutes.add_member_permission",
		"minutes.add_personal_hot_word",
		"minutes.batch_get_minutes_details",
		"minutes.cancel_upload_session",
		"minutes.complete_upload_session",
		"minutes.create_mind_graph",
		"minutes.create_speaker_summary",
		"minutes.create_upload_session",
		"minutes.get_minutes_ai_summary",
		"minutes.get_minutes_basic_info",
		"minutes.get_minutes_keywords",
		"minutes.get_minutes_transcription",
		"minutes.get_speaker_summary",
		"minutes.list_by_keyword_and_time_range",
		"minutes.list_minutes_todos",
		"minutes.list_my_hotwords",
		"minutes.query_mind_graph_status",
		"minutes.query_minutes_audio_url",
		"minutes.query_minutes_by_tag_id",
		"minutes.query_user_tag_list",
		"minutes.record_pause",
		"minutes.record_resume",
		"minutes.record_start",
		"minutes.record_stop",
		"minutes.remove_member_permission",
		"minutes.replace_minutes_text",
		"minutes.replace_speaker",
		"minutes.update_minutes_summary",
		"minutes.update_minutes_title",
		"oa.approve_processInstance",
		"oa.dingflow_comments",
		"oa.get_done_tasks",
		"oa.get_noticed_instances",
		"oa.get_processInstance_detail",
		"oa.get_processInstance_records",
		"oa.get_submitted_instances",
		"oa.list_initiated_instances",
		"oa.list_pending_approvals",
		"oa.list_pending_tasks",
		"oa.list_user_visible_process",
		"oa.oa_cc_noticer",
		"oa.redirect_task",
		"oa.reject_processInstance",
		"report.create_report",
		"report.get_available_report_templates",
		"report.get_received_report_list",
		"report.get_report_entry_details",
		"report.get_report_statistics_by_id",
		"report.get_send_report_list",
		"report.get_template_details_by_name",
		"sheet.add_dimension",
		"sheet.append_rows",
		"sheet.batch_update",
		"sheet.chart_create",
		"sheet.chart_list",
		"sheet.chart_update",
		"sheet.clear_range",
		"sheet.copy_sheet",
		"sheet.create_cond_format",
		"sheet.create_filter",
		"sheet.create_filter_view",
		"sheet.create_float_image",
		"sheet.create_pivot_table",
		"sheet.create_sheet",
		"sheet.create_workspace_sheet",
		"sheet.delete_dimension",
		"sheet.delete_dropdown_lists",
		"sheet.delete_filter",
		"sheet.delete_filter_view",
		"sheet.delete_float_image",
		"sheet.delete_sheet",
		"sheet.fill_range",
		"sheet.filter_clear_criteria",
		"sheet.filter_view_delete_criteria",
		"sheet.filter_view_update_criteria",
		"sheet.find_cells",
		"sheet.get_all_sheets",
		"sheet.get_cond_format",
		"sheet.get_dropdown_lists",
		"sheet.get_filter",
		"sheet.get_filter_views",
		"sheet.get_float_image",
		"sheet.get_range_as_csv",
		"sheet.group_dimension",
		"sheet.hide_gridline",
		"sheet.info",
		"sheet.insert_dimension",
		"sheet.list_float_images",
		"sheet.list_pivot_tables",
		"sheet.merge_cells",
		"sheet.move_dimension",
		"sheet.range_batch_clear",
		"sheet.range_batch_set_style",
		"sheet.range_copy_to",
		"sheet.range_move_to",
		"sheet.range_set_style",
		"sheet.range_update",
		"sheet.replace_all",
		"sheet.set_dropdown_lists",
		"sheet.set_range_from_csv",
		"sheet.show_gridline",
		"sheet.sort_filter",
		"sheet.sort_range",
		"sheet.table_get",
		"sheet.table_put",
		"sheet.template_apply",
		"sheet.template_list",
		"sheet.template_search",
		"sheet.ungroup_dimension",
		"sheet.unmerge_range",
		"sheet.update_cond_format",
		"sheet.update_dimension",
		"sheet.update_filter",
		"sheet.update_filter_view",
		"sheet.update_float_image",
		"sheet.update_pivot_table",
		"sheet.update_sheet",
		"todo.add_task_executors",
		"todo.add_task_participants",
		"todo.add_todo_comment",
		"todo.add_todo_reminder",
		"todo.create_personal_sub_todo",
		"todo.create_personal_todo",
		"todo.get_user_todos_in_current_org",
		"todo.list_todo_comment",
		"todo.remove_task_executors",
		"todo.remove_task_participants",
		"todo.reset_todo_reminder",
		"todo.update_todo_done_status",
		"todo.update_todo_task",
		"wiki.add_member",
		"wiki.create_file",
		"wiki.create_wikiSpace",
		"wiki.get_wikiSpace",
		"wiki.list_member",
		"wiki.list_nodes",
		"wiki.list_wikiSpaces",
		"wiki.node_copy",
		"wiki.node_move",
		"wiki.node_search",
		"wiki.remove_member",
		"wiki.search_wikiSpaces",
		"wiki.update_member",
	}},
	{PreviewKind: DryRunPreviewRequest, CanonicalPaths: []string{
		"dev.add_dev_app_members",
		"dev.apply_dev_app_permissions",
		"dev.create_dev_app",
		"dev.create_dev_app_version",
		"dev.delete_dev_app",
		"dev.disable_dev_app",
		"dev.disable_dev_app_robot",
		"dev.enable_dev_app",
		"dev.enable_dev_app_robot",
		"dev.get_dev_app",
		"dev.get_dev_app_credentials",
		"dev.get_dev_app_version_detail",
		"dev.get_dev_app_version_status",
		"dev.get_extension_robot_config",
		"dev.get_extension_webapp_config",
		"dev.list_dev_app",
		"dev.list_dev_app_events",
		"dev.list_dev_app_members",
		"dev.list_dev_app_permissions",
		"dev.list_dev_app_versions",
		"dev.publish_dev_app_version",
		"dev.query_robot_create_result",
		"dev.remove_dev_app_members",
		"dev.remove_dev_app_permissions",
		"dev.set_extension_robot_config",
		"dev.set_extension_webapp_config",
		"dev.submit_robot_create_task",
		"dev.subscribe_dev_app_events",
		"dev.unsubscribe_dev_app_events",
		"dev.update_dev_app",
		"dev.update_dev_app_security_config",
		"event.stop",
		"pat.batch_grant",
	}},
	{PreviewKind: DryRunPreviewPlan, CanonicalPaths: []string{
		"chat.download_media",
		"doc.download_file",
		"doc.import_get",
		"doc.media_insert",
		"doc.query_export_job",
		"doc.upload",
		"drive.download_file",
		"drive.upload",
		"sheet.filter_view_get_criteria",
		"sheet.filter_view_info",
		"sheet.filter_view_list_criteria",
		"sheet.media_upload",
		"sheet.submit_export_job",
		"sheet.write_image",
		"todo.add_todo_attachment",
	}},
}

var reviewedDryRunCapabilitiesLazy struct {
	once        sync.Once
	byCanonical map[string]DryRunSpec
	err         error
}

func loadReviewedDryRunCapabilities() (map[string]DryRunSpec, error) {
	reviewedDryRunCapabilitiesLazy.once.Do(func() {
		byCanonical := make(map[string]DryRunSpec)
		for _, group := range reviewedDryRunCapabilityGroups {
			spec := DryRunSpec{PreviewKind: group.PreviewKind}
			if err := spec.Validate("<reviewed-dry-run-registry>"); err != nil {
				reviewedDryRunCapabilitiesLazy.err = err
				return
			}
			previous := ""
			for _, raw := range group.CanonicalPaths {
				canonical := strings.TrimSpace(raw)
				if canonical == "" {
					reviewedDryRunCapabilitiesLazy.err = fmt.Errorf("reviewed dry-run capability has empty canonical path")
					return
				}
				if previous != "" && canonical <= previous {
					reviewedDryRunCapabilitiesLazy.err = fmt.Errorf("reviewed dry-run capability paths for %s are not strictly sorted at %q", group.PreviewKind, canonical)
					return
				}
				previous = canonical
				if _, duplicate := byCanonical[canonical]; duplicate {
					reviewedDryRunCapabilitiesLazy.err = fmt.Errorf("duplicate reviewed dry-run capability %s", canonical)
					return
				}
				byCanonical[canonical] = spec
			}
		}
		reviewedDryRunCapabilitiesLazy.byCanonical = byCanonical
	})
	if reviewedDryRunCapabilitiesLazy.err != nil {
		return nil, reviewedDryRunCapabilitiesLazy.err
	}
	out := make(map[string]DryRunSpec, len(reviewedDryRunCapabilitiesLazy.byCanonical))
	for canonical, spec := range reviewedDryRunCapabilitiesLazy.byCanonical {
		out[canonical] = spec
	}
	return out, nil
}

// ReviewedDryRunCapabilities returns a defensive copy of the positive,
// reviewed capability registry for delivery gates.
func ReviewedDryRunCapabilities() (map[string]DryRunSpec, error) {
	return loadReviewedDryRunCapabilities()
}

func reviewedDryRunCapability(canonical string) (*DryRunSpec, error) {
	capabilities, err := loadReviewedDryRunCapabilities()
	if err != nil {
		return nil, err
	}
	spec, ok := capabilities[strings.TrimSpace(canonical)]
	if !ok {
		return nil, nil
	}
	return &spec, nil
}

// ValidateReviewedDryRunCapabilityDelivery proves that every positive source
// entry reaches the final typed registry and no serializer invents one.
func ValidateReviewedDryRunCapabilityDelivery(registry SchemaRegistry) error {
	expected, err := loadReviewedDryRunCapabilities()
	if err != nil {
		return err
	}
	actual := make(map[string]DryRunSpec)
	for _, product := range registry.Products {
		for _, tool := range product.Tools {
			if tool.DryRun != nil {
				actual[tool.Identity.CanonicalPath] = *tool.DryRun
			}
		}
	}
	var problems []string
	for canonical, want := range expected {
		got, ok := actual[canonical]
		if !ok {
			problems = append(problems, fmt.Sprintf("reviewed dry-run capability %s is missing from final Schema", canonical))
			continue
		}
		if got != want {
			problems = append(problems, fmt.Sprintf("Schema dry-run capability %s = %#v, want %#v", canonical, got, want))
		}
	}
	for canonical := range actual {
		if _, ok := expected[canonical]; !ok {
			problems = append(problems, fmt.Sprintf("Schema tool %s publishes an unreviewed dry-run capability", canonical))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}
