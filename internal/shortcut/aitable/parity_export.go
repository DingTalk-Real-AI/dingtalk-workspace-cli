// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const maxRecordArtifactBytes = 64 << 20

func outputRecordQuery(rt *shortcut.RuntimeContext, records []map[string]any, payload map[string]any) error {
	if !rt.Changed("output") {
		return rt.Output(payload)
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	for _, r := range records {
		if err := encoder.Encode(r); err != nil {
			return err
		}
		if buffer.Len() > maxRecordArtifactBytes {
			return fmt.Errorf("NDJSON artifact exceeds 64 MiB; narrow the query")
		}
	}
	fields, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id")})
	if err != nil {
		return err
	}
	list, found := findNamedObjectList(fields, "fields", "fieldList")
	if !found {
		return fmt.Errorf("get_fields is missing the column catalog")
	}
	wanted := map[string]bool{}
	for _, id := range rt.StrSlice("field-ids") {
		wanted[id] = true
	}
	projected := len(wanted) > 0
	columns := make([]map[string]any, 0, len(list))
	for _, f := range list {
		id := stringValue(f, "fieldId", "id")
		name := stringValue(f, "fieldName", "name")
		typ := stringValue(f, "type", "fieldType")
		if id == "" || name == "" || typ == "" {
			return fmt.Errorf("column catalog lacks identity/name/type")
		}
		if projected {
			if !wanted[id] {
				continue
			}
			delete(wanted, id)
		}
		columns = append(columns, map[string]any{"fieldId": id, "name": name, "type": typ})
	}
	if len(wanted) > 0 {
		return fmt.Errorf("projected fields are absent from the column catalog")
	}
	cwd, err := aitableWorkingDirectory()
	if err != nil {
		return err
	}
	file, err := localio.PublishBytes(buffer.Bytes(), localio.PublishBytesOptions{BaseDir: cwd, Output: rt.Str("output"), PreferredName: "records.ndjson", MaxBytes: maxRecordArtifactBytes})
	if err != nil {
		return err
	}
	hash := sha256.Sum256(buffer.Bytes())
	return rt.Output(map[string]any{"format": "ndjson", "output": file.RelativePath, "recordCount": len(records), "sizeBytes": file.SizeBytes, "sha256": hex.EncodeToString(hash[:]), "complete": true, "columns": columns})
}
