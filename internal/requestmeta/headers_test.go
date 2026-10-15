// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package requestmeta

import "testing"

func TestCrossPlatformCoverageDelegatorReservedHeaders(t *testing.T) {
	headers := map[string]string{"DELEGATOR-USER-ID": "u", "Delegator-Corp-Id": "c", "delegator-open-dingtalk-id": "o", "Delegator-Uid": "uid", "X-Other": "keep"}
	for key := range headers {
		if IsDelegatorHeader(key) != (key != "X-Other") {
			t.Fatal(key)
		}
	}
	RemoveDelegatorHeaders(headers)
	if len(headers) != 1 || headers["X-Other"] != "keep" {
		t.Fatal(headers)
	}
	RemoveDelegatorHeaders(nil)
}
