package encoder

import (
	"testing"
)

func TestEncodeURIComponent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello%20world"},
		{"你好世界", "%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C"},
		{"a!b", "a!b"},
		{"a'b", "a'b"},
		{"a(b)", "a(b)"},
		{"a*b", "a*b"},
		{"key=value&foo=bar", "key%3Dvalue%26foo%3Dbar"},
		{"hello+world", "hello%2Bworld"},
		{"", ""},
		{"abc123", "abc123"},
		{"~test", "~test"},
		{"a b/c", "a%20b%2Fc"},
	}

	for _, tt := range tests {
		result := EncodeURIComponent(tt.input)
		if result != tt.expected {
			t.Errorf("EncodeURIComponent(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestItemToString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
		ok       bool
	}{
		{"non-empty string", "hello", "hello", true},
		{"empty string", "", "", false},
		{"integer", float64(42), "42", true},
		{"float", float64(3.14), "3.14", true},
		{"bool true", true, "true", true},
		{"bool false", false, "false", true},
		{"nil", nil, "", false},
		{"map", map[string]interface{}{"a": float64(1), "b": "hello"}, `{"a":1,"b":"hello"}`, true},
		{"slice", []interface{}{float64(1), "two", float64(3)}, `[1,"two",3]`, true},
	}

	for _, tt := range tests {
		result, ok := ItemToString(tt.input)
		if ok != tt.ok {
			t.Errorf("ItemToString(%v) ok = %v, want %v", tt.input, ok, tt.ok)
			continue
		}
		if ok && result != tt.expected {
			t.Errorf("ItemToString(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestObjToQS(t *testing.T) {
	result := ObjToQS(map[string]string{
		"pid":         "my_app",
		"env":         "prod",
		"sdk_version": "3.3.18",
	})
	expected := "env=prod&pid=my_app&sdk_version=3.3.18"
	if result != expected {
		t.Errorf("ObjToQS basic = %q, want %q", result, expected)
	}

	result = ObjToQS(map[string]string{
		"a": "1",
		"b": "",
		"c": "3",
	})
	expected = "a=1&c=3"
	if result != expected {
		t.Errorf("ObjToQS skip empty = %q, want %q", result, expected)
	}

	result = ObjToQS(map[string]string{
		"msg": "hello world",
	})
	expected = "msg=hello%20world"
	if result != expected {
		t.Errorf("ObjToQS encode value = %q, want %q", result, expected)
	}

	result = ObjToQS(map[string]string{})
	if result != "" {
		t.Errorf("ObjToQS empty = %q, want empty", result)
	}
}

func TestProcessData(t *testing.T) {
	config := map[string]string{
		"pid":      "test_app",
		"platform": "go",
	}

	logs := []map[string]string{
		{"type": "error", "ts": "1716100000000", "message": "something wrong"},
		{"type": "api", "ts": "1716100000001", "url": "/api/user"},
	}
	result := ProcessData(logs, config)
	expected := "pid=test_app&platform=go&msg=" + EncodeURIComponent(
		"message=something%20wrong&ts=1716100000000&type=error"+
			"|"+
			"ts=1716100000001&type=api&url=%2Fapi%2Fuser",
	)
	if result != expected {
		t.Errorf("ProcessData multi logs:\ngot:  %q\nwant: %q", result, expected)
	}

	singleLog := []map[string]string{
		{"type": "custom", "ts": "1716100000002", "action": "click"},
	}
	result = ProcessData(singleLog, config)
	expected = "pid=test_app&platform=go&msg=" + EncodeURIComponent(
		"action=click&ts=1716100000002&type=custom",
	)
	if result != expected {
		t.Errorf("ProcessData single log:\ngot:  %q\nwant: %q", result, expected)
	}

	result = ProcessData([]map[string]string{}, config)
	expected = "pid=test_app&platform=go&msg="
	if result != expected {
		t.Errorf("ProcessData empty logs:\ngot:  %q\nwant: %q", result, expected)
	}
}

func TestToStringMap(t *testing.T) {
	input := map[string]interface{}{
		"name":   "test",
		"count":  float64(42),
		"active": true,
		"empty":  "",
		"null":   nil,
		"nested": map[string]interface{}{"key": "val"},
	}
	result := ToStringMap(input)

	if result["name"] != "test" {
		t.Errorf("name = %q, want 'test'", result["name"])
	}
	if result["count"] != "42" {
		t.Errorf("count = %q, want '42'", result["count"])
	}
	if result["active"] != "true" {
		t.Errorf("active = %q, want 'true'", result["active"])
	}
	if _, ok := result["empty"]; ok {
		t.Error("empty should be filtered out")
	}
	if _, ok := result["null"]; ok {
		t.Error("null should be filtered out")
	}
	if result["nested"] != `{"key":"val"}` {
		t.Errorf("nested = %q, want '{\"key\":\"val\"}'", result["nested"])
	}
}

func TestDoubleEncoding(t *testing.T) {
	config := map[string]string{"pid": "app"}
	logs := []map[string]string{
		{"url": "/api/user?id=1&name=test"},
	}
	result := ProcessData(logs, config)

	innerQS := "url=" + EncodeURIComponent("/api/user?id=1&name=test")
	expected := "pid=app&msg=" + EncodeURIComponent(innerQS)
	if result != expected {
		t.Errorf("double encoding:\ngot:  %q\nwant: %q", result, expected)
	}
}
