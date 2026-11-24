// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package outboundcall owns outbound-call payload validation without CLI or I/O dependencies.
package outboundcall

import (
	"encoding/json"
	"fmt"
	"strings"
)

const MaxInputBytes = 1 << 20

type CalleeInfo map[string]any

type CreateRequest struct {
	CalleeInfos []CalleeInfo `json:"calleeInfos"`
}

func Single(callee, prompt, greeting string) (CreateRequest, error) {
	r := CreateRequest{CalleeInfos: []CalleeInfo{{"callee": callee, "prompt": prompt, "greeting": greeting}}}
	return r, r.Validate()
}

func (r CreateRequest) Validate() error {
	if len(r.CalleeInfos) != 1 {
		return fmt.Errorf("每次外呼必须且只能指定一名被呼叫人")
	}
	for i, info := range r.CalleeInfos {
		for _, key := range []string{"callee", "prompt", "greeting"} {
			value, ok := info[key].(string)
			if !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("calleeInfos[%d].%s 必须为非空字符串", i, key)
			}
		}
	}
	encoded, err := json.Marshal(r.CalleeInfos)
	if err != nil || len(encoded) > MaxInputBytes {
		return fmt.Errorf("外呼输入不能超过 %d 字节", MaxInputBytes)
	}
	return nil
}

func (r CreateRequest) Arguments() map[string]any {
	return map[string]any{"calleeInfos": r.CalleeInfos}
}
