// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package errors

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"reflect"
	"testing"
)

func TestCrossPlatformCoveragePrintJSONWithNoticePreservesLegacyError(t *testing.T) {
	err := NewValidation("unknown flag", WithReason("unknown_flag"), WithCause(stderrors.New("parser failure")), WithHint("Run 'dws --help'"), WithActions("read help"), WithDetails(map[string]any{"input": "missing"}))
	notice := map[string]any{"update": map[string]any{"current": "1.0.60", "latest": "1.0.63", "command": "dws upgrade"}}
	var legacy, nilNotice, withNotice bytes.Buffer
	if writeErr := PrintJSON(&legacy, err); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := PrintJSONWithNotice(&nilNotice, err, nil); writeErr != nil {
		t.Fatal(writeErr)
	}
	if !bytes.Equal(legacy.Bytes(), nilNotice.Bytes()) {
		t.Fatalf("nil notice changed legacy bytes: %s != %s", &legacy, &nilNotice)
	}
	if writeErr := PrintJSONWithNotice(&withNotice, err, notice); writeErr != nil {
		t.Fatal(writeErr)
	}
	var originalPayload, noticePayload map[string]any
	if decodeErr := json.Unmarshal(legacy.Bytes(), &originalPayload); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if decodeErr := json.Unmarshal(withNotice.Bytes(), &noticePayload); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if !reflect.DeepEqual(noticePayload["_notice"], notice) {
		t.Fatalf("notice changed: %#v", noticePayload["_notice"])
	}
	delete(noticePayload, "_notice")
	if !reflect.DeepEqual(originalPayload, noticePayload) {
		t.Fatalf("notice changed legacy error fields: original=%#v with_notice=%#v", originalPayload, noticePayload)
	}
}
