// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package outboundcall

import (
	"encoding/json"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
)

// Receipt preserves the upstream business value; only explicit facts affect status.
type Receipt struct {
	Data    any
	TaskID  string
	Failed  bool
	Message string
	Code    any
}

// ReceiptError separates a stable diagnostic reason from user-facing text.
type ReceiptError struct {
	Reason  string
	message string
}

func (e *ReceiptError) Error() string { return e.message }

func ParseReceipt(text string) (Receipt, error) {
	var receipt Receipt
	if strings.TrimSpace(text) == "" {
		return receipt, &ReceiptError{Reason: "missing_text", message: "外呼接口未返回文本内容"}
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var data any
	// Decode accepts a JSON prefix; reject any trailing non-whitespace too.
	if err := decoder.Decode(&data); err != nil || strings.Trim(text[decoder.InputOffset():], " \t\r\n") != "" {
		return receipt, &ReceiptError{Reason: "invalid_json", message: "外呼接口返回无效 JSON"}
	}
	if err := jsonutil.RejectDuplicateObjectKeys([]byte(text)); err != nil {
		return receipt, &ReceiptError{Reason: "duplicate_fields", message: "外呼接口返回重复 JSON 字段"}
	}
	receipt.Data = data
	if receipt.Data == nil {
		return receipt, &ReceiptError{Reason: "empty_value", message: "外呼接口返回空值"}
	}
	receipt.TaskID = taskID(receipt.Data)
	if object, ok := receipt.Data.(map[string]any); ok {
		success, hasSuccess := object["success"].(bool)
		isError, _ := object["isError"].(bool)
		receipt.Failed = (hasSuccess && !success) || isError
		if receipt.Failed {
			for _, key := range []string{"errorMsg", "message", "msg"} {
				if message, ok := object[key].(string); ok && message != "" {
					receipt.Message = message
					break
				}
			}
			for _, key := range []string{"errorCode", "code"} {
				if code, ok := object[key]; ok {
					receipt.Code = code
					break
				}
			}
		}
	}
	return receipt, nil
}

func taskID(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	id, _ := object["taskId"].(string)
	// Recognize common Connector envelopes without modifying their business data.
	for _, key := range []string{"result", "data"} {
		if nested, ok := object[key].(map[string]any); ok {
			if nestedID, ok := nested["taskId"].(string); ok && strings.TrimSpace(nestedID) != "" {
				if strings.TrimSpace(id) != "" && id != nestedID {
					return ""
				}
				id = nestedID
			}
		}
	}
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\r\n\x00") {
		return ""
	}
	return id
}
