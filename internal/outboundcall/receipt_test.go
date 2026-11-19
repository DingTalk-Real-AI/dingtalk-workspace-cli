// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package outboundcall

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCrossPlatformCoverageOutboundCallReceipts(t *testing.T) {
	for _, tc := range []struct {
		text   string
		id     string
		failed bool
		reason string
	}{
		{`{"taskId":"t"}`, "t", false, ""},
		{`{"success":true,"result":{"taskId":"t"}}`, "t", false, ""},
		{`{"data":{"taskId":"t"}}`, "t", false, ""},
		{`{"taskId":"a","data":{"taskId":"b"}}`, "", false, ""},
		{`{"taskId":"a\nb"}`, "", false, ""},
		{`{"success":false,"message":"denied","code":"403"}`, "", true, ""},
		{`{"isError":true}`, "", true, ""},
		{`{"callStatus":4,"errMsg":10}`, "", false, ""},
		{`[1,"accepted"]`, "", false, ""},
		{`"accepted"`, "", false, ""},
		{`9007199254740993`, "", false, ""},
		{" \n{\"taskId\":\"t\"}\t\r\n", "t", false, ""},
		{`accepted`, "", false, "invalid_json"},
		{`{"taskId":"a","taskId":"b"}`, "", false, "duplicate_fields"},
		{`{"data":{"taskId":"a","taskId":"b"}}`, "", false, "duplicate_fields"},
		{`{"taskId":"t"} {}`, "", false, "invalid_json"},
		{`{"taskId":"t"} garbage`, "", false, "invalid_json"},
		{"{\"taskId\":\"t\"}\u00a0", "", false, "invalid_json"},
		{"{\"taskId\":\"t\"}\v", "", false, "invalid_json"},
		{`{"taskId":`, "", false, "invalid_json"},
		{"", "", false, "missing_text"},
		{`null`, "", false, "empty_value"},
		{"  ", "", false, "missing_text"},
	} {
		r, err := ParseReceipt(tc.text)
		if r.TaskID != tc.id || r.Failed != tc.failed {
			t.Errorf("receipt %s = %#v err=%v", tc.text, r, err)
		}
		if tc.reason != "" {
			var receiptErr *ReceiptError
			if !errors.As(err, &receiptErr) || receiptErr.Reason != tc.reason {
				t.Errorf("receipt %s: reason=%v, want %s", tc.text, err, tc.reason)
			} else {
				messages := map[string]string{
					"missing_text":     "外呼接口未返回文本内容",
					"invalid_json":     "外呼接口返回无效 JSON",
					"duplicate_fields": "外呼接口返回重复 JSON 字段",
					"empty_value":      "外呼接口返回空值",
				}
				if err.Error() != messages[tc.reason] {
					t.Errorf("receipt %s: message=%q, want %q", tc.text, err.Error(), messages[tc.reason])
				}
			}
			if r.Data != nil {
				t.Errorf("invalid receipt retained business data: %#v", r.Data)
			}
			continue
		}
		if err != nil || r.Data == nil {
			t.Errorf("business data lost: %s, err=%v", tc.text, err)
		}
	}
}

func TestCrossPlatformCoverageOutboundCallReceiptPreservesBusinessData(t *testing.T) {
	const text = `{"taskId":"t","callStartTs":9007199254740993,"callStatus":4,"errMsg":10}`
	r, err := ParseReceipt(text)
	if err != nil {
		t.Fatal(err)
	}
	object := r.Data.(map[string]any)
	if r.TaskID != "t" || r.Failed || object["callStartTs"] != json.Number("9007199254740993") || object["callStatus"] != json.Number("4") || object["errMsg"] != json.Number("10") {
		t.Fatalf("business values changed: %#v", r)
	}
}
