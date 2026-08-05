package helpers

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCrossPlatformCoverageDriveToMillis(t *testing.T) {
	rfc := "2023-11-14T22:13:20Z"
	want := func() int64 {
		tm, err := time.Parse(time.RFC3339, rfc)
		if err != nil {
			t.Fatalf("parse fixture: %v", err)
		}
		return tm.UnixMilli()
	}()

	for _, tc := range []struct {
		name  string
		in    any
		ms    int64
		valid bool
	}{
		{"float64", float64(1700000000000), 1700000000000, true},
		{"float64 non-positive", float64(0), 0, false},
		{"json.Number", json.Number("1700000000000"), 1700000000000, true},
		{"json.Number non-numeric", json.Number("abc"), 0, false},
		{"json.Number non-positive", json.Number("-1"), 0, false},
		{"numeric string", "1700000000000", 1700000000000, true},
		{"rfc3339 string", rfc, want, true},
		{"blank string", "   ", 0, false},
		{"garbage string", "not-a-time", 0, false},
		{"unsupported type", true, 0, false},
		{"nil", nil, 0, false},
	} {
		ms, ok := driveToMillis(tc.in)
		if ok != tc.valid || ms != tc.ms {
			t.Fatalf("%s: driveToMillis(%#v) = (%d, %v), want (%d, %v)", tc.name, tc.in, ms, ok, tc.ms, tc.valid)
		}
	}
}

func TestCrossPlatformCoverageDriveModifiedMillis(t *testing.T) {
	if _, ok := driveModifiedMillis(map[string]any{"name": "no time"}); ok {
		t.Fatal("item without any time key reported a timestamp")
	}

	// 探测顺序即优先级：精确字段名胜出于历史别名。
	priority := map[string]any{
		"updateTime":   float64(1),
		"gmtModified":  float64(2),
		"modifyTime":   float64(3),
		"modifiedTime": float64(4),
	}
	if ms, ok := driveModifiedMillis(priority); !ok || ms != 4 {
		t.Fatalf("priority = (%d, %v), want (4, true)", ms, ok)
	}

	// 靠前的键不可解析时继续回落，不能提前判定为无时间。
	fallback := map[string]any{
		"modifiedTime": "",
		"modifyTime":   float64(0),
		"updateTime":   float64(1700000000000),
	}
	if ms, ok := driveModifiedMillis(fallback); !ok || ms != 1700000000000 {
		t.Fatalf("fallback = (%d, %v)", ms, ok)
	}

	for _, key := range driveModifiedTimeKeys {
		if ms, ok := driveModifiedMillis(map[string]any{key: float64(1234)}); !ok || ms != 1234 {
			t.Fatalf("key %s = (%d, %v)", key, ms, ok)
		}
	}
}
