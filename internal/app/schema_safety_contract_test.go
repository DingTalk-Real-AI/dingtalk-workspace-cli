// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"sort"
	"testing"
)

type finalSchemaSafetyWant struct {
	canonical    string
	effect       string
	risk         string
	confirmation string
	idempotency  string
	// provenance is the expected field_provenance precedence; empty defaults
	// to "contract_final" (DeclareLeafMetadata / corecmd.Contract). Production
	// metadata shells no longer carry reviewed tool rows.
	provenance string
}

func TestReviewedMutationSafetyReachesFinalSchema(t *testing.T) {
	const declared = "contract_final"
	wants := []finalSchemaSafetyWant{
		{canonical: "aitable.form_field_hide", effect: "write", risk: "medium", confirmation: "not_required", idempotency: "idempotent", provenance: declared},
		{canonical: "chat.dismiss_group", effect: "destructive", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "chat.shortcut_messages_send", effect: "write", risk: "medium", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "chat.shortcut_messages_send_by_webhook", effect: "write", risk: "medium", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "chat.shortcut_messages_send_card", effect: "write", risk: "medium", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "chat.shortcut_messages_update_card", effect: "write", risk: "medium", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "drive.recycle_restore", effect: "write", risk: "medium", confirmation: "not_required", idempotency: "unknown", provenance: declared},
		{canonical: "minutes.create_speaker_summary", effect: "write", risk: "medium", confirmation: "not_required", idempotency: "unknown", provenance: declared},
		{canonical: "sheet.clear_range", effect: "write", risk: "medium", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "sheet.batch_update", effect: "write", risk: "medium", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "sheet.range_batch_clear", effect: "write", risk: "medium", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "sheet.group_dimension", effect: "write", risk: "medium", confirmation: "not_required", idempotency: "unknown", provenance: declared},
		{canonical: "sheet.sort_filter", effect: "write", risk: "medium", confirmation: "not_required", idempotency: "unknown", provenance: declared},
		{canonical: "sheet.ungroup_dimension", effect: "write", risk: "medium", confirmation: "not_required", idempotency: "unknown", provenance: declared},
	}
	assertFinalSchemaSafety(t, wants)
}

func TestDevAppWriteGuardRequiresFinalSchemaConfirmation(t *testing.T) {
	// devapp 全树已声明化：provenance 为 contract_final；effect/risk 逐字
	// 保持 merge-base 评审值（写操作一律 high，publish 为 write/high），
	// 重分级需要独立的契约变更 PR。
	const declared = "contract_final"
	wants := []finalSchemaSafetyWant{
		{canonical: "dev.add_dev_app_members", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.apply_dev_app_permissions", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.create_dev_app", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.create_dev_app_version", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.delete_dev_app", effect: "destructive", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.disable_dev_app", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.disable_dev_app_robot", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.enable_dev_app", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.enable_dev_app_robot", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.publish_dev_app_version", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.remove_dev_app_members", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.remove_dev_app_permissions", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.set_extension_robot_config", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.set_extension_webapp_config", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.submit_robot_create_task", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.subscribe_dev_app_events", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.unsubscribe_dev_app_events", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.update_dev_app", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
		{canonical: "dev.update_dev_app_security_config", effect: "write", risk: "high", confirmation: "user_required", idempotency: "unknown", provenance: declared},
	}
	assertFinalSchemaSafety(t, wants)
}

func assertFinalSchemaSafety(t *testing.T, wants []finalSchemaSafetyWant) {
	t.Helper()
	canonicals := make([]string, 0, len(wants))
	for _, want := range wants {
		canonicals = append(canonicals, want.canonical)
	}
	sort.Strings(canonicals)
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)

	for _, want := range wants {
		want := want
		t.Run(want.canonical, func(t *testing.T) {
			tool := payload.Tools[want.canonical]
			values := map[string]string{
				"effect":       want.effect,
				"risk":         want.risk,
				"confirmation": want.confirmation,
				"idempotency":  want.idempotency,
			}
			wantProvenance := want.provenance
			if wantProvenance == "" {
				wantProvenance = "contract_final"
			}
			provenance := schemaContractMap(tool["field_provenance"])
			for field, expected := range values {
				if got := schemaContractString(tool[field]); got != expected {
					t.Errorf("%s = %q, want %q", field, got, expected)
				}
				if got := schemaContractString(provenance[field]["precedence"]); got != wantProvenance {
					t.Errorf("%s provenance precedence = %q, want %s", field, got, wantProvenance)
				}
			}
		})
	}
}
