// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import "testing"

func TestCrossPlatformCoverageRecordUpsertWireContext(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	params := map[string]any{"baseId": "b", "tableId": "t", "records": []any{}, "clientToken": token}
	wire, header, err := RecordUpsertWireParams("aitable-helper", "record_upsert", params)
	if err != nil || header != token || len(wire) != 3 || wire["clientToken"] != nil || params["clientToken"] != token {
		t.Fatalf("wire=%v header=%s err=%v original=%v", wire, header, err, params)
	}
	for _, value := range []any{"invalid", 12} {
		if _, _, err := RecordUpsertWireParams("aitable", "record_upsert", map[string]any{"clientToken": value}); err == nil {
			t.Fatalf("accepted invalid token %v", value)
		}
	}
	for _, pair := range [][2]string{{"custom", "record_upsert"}, {"aitable", "create_records"}} {
		got, header, err := RecordUpsertWireParams(pair[0], pair[1], params)
		if err != nil || header != "" || got["clientToken"] != token {
			t.Fatal("changed other tool")
		}
	}
	if _, header, err := RecordUpsertWireParams("aitable-helper", "record_upsert", map[string]any{}); err != nil || header != "" {
		t.Fatal("changed legacy call")
	}
}
