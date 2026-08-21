// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package commentreaction

import "testing"

func TestCrossPlatformCoverageValidate(t *testing.T) {
	for _, value := range []string{"憨笑", "鼓掌", "比心", "赞", "OK", "Done", "平安健康"} {
		if err := Validate(value); err != nil {
			t.Errorf("supported reaction %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "😄", "👏", "�", "garbled", "乱码", "憨笑\n"} {
		if err := Validate(value); err == nil {
			t.Errorf("unsupported reaction %q accepted", value)
		}
	}
}
