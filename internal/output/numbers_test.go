// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOutputPreservesLargeIntegerIDs(t *testing.T) {
	for _, id := range []string{"9007199254740993", "-9007199254740993", "9223372036854775806", "18446744073709551615", "123456789012345678901234567890"} {
		payload := json.RawMessage(`{"items":[{"uid":` + id + `,"name":"Alice"}],"count":1}`)
		env, err := EnvelopeFromResult(Success(payload))
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			format Format
			fields string
			jq     string
		}{
			{FormatJSON, "", ""}, {FormatJSON, "uid", ""},
			{FormatJSON, "", ".data.items[0].uid"},
			{FormatTable, "", ""}, {FormatPretty, "", ""},
			{FormatCSV, "", ""}, {FormatNDJSON, "", ""}, {FormatRaw, "", ""},
		} {
			var buf bytes.Buffer
			if err := WriteEnvelopeTo(&buf, env, tc.format, tc.fields, tc.jq); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(buf.String(), id) {
				t.Fatalf("id=%s format=%s fields=%s jq=%s: %s", id, tc.format, tc.fields, tc.jq, buf.String())
			}
			if tc.jq != "" && strings.TrimSpace(buf.String()) != id {
				t.Fatalf("UID must remain a number: %s", buf.String())
			}
		}
	}
}

func TestCrossPlatformCoverageOutputJQNumericCompatibility(t *testing.T) {
	payload := json.RawMessage(`{"uid":9223372036854775806,"small":41,"fraction":1.25,"exponent":1e2,"items":[2,1,3],"empty":null,"enabled":true}`)
	for _, tc := range []struct{ query, want string }{
		{".uid + 1", "9223372036854775807"},
		{".uid == 9223372036854775806", "true"},
		{".small + 1", "42"}, {".fraction * 2", "2.5"}, {".exponent + 1", "101"},
		{".items | sort | .[0]", "1"}, {".empty", "null"}, {".enabled", "true"},
	} {
		var buf bytes.Buffer
		if err := ApplyJQ(&buf, payload, tc.query); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(buf.String()); got != tc.want {
			t.Fatalf("%s = %s, want %s", tc.query, got, tc.want)
		}
	}
}

func BenchmarkOutputNumberNormalization(b *testing.B) {
	for _, size := range []struct {
		name  string
		count int
	}{{"single", 1}, {"page100", 100}} {
		record := `{"uid":9223372036854775806,"name":"Alice","count":42,"active":true}`
		data := []byte(`{"items":[` + strings.TrimSuffix(strings.Repeat(record+",", size.count), ",") + `]}`)
		for _, decoder := range []struct {
			name string
			call func([]byte, *any) error
		}{
			{"legacy", func(data []byte, target *any) error { return json.Unmarshal(data, target) }},
			{"preserve", unmarshalJSONUseNumber},
		} {
			b.Run(size.name+"/"+decoder.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var value any
					if err := decoder.call(data, &value); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
