// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// canonicalOutcomes 是契约规范 §1/§2.5 的四值（顺序即 wire 顺序）。
var canonicalOutcomes = []Outcome{
	OutcomeSuccess,
	OutcomePending,
	OutcomePartialFailure,
	OutcomeFailure,
}

// --- B1：ParseOutcome 校验 ---

func TestParseOutcomeAcceptsCanonicalValues(t *testing.T) {
	for _, want := range canonicalOutcomes {
		got, err := ParseOutcome(string(want))
		if err != nil {
			t.Fatalf("ParseOutcome(%q) error = %v, want nil", want, err)
		}
		if got != want {
			t.Fatalf("ParseOutcome(%q) = %q, want %q", want, got, want)
		}
	}
}

func TestParseOutcomeRejectsNonCanonical(t *testing.T) {
	rejected := []string{
		"",                  // 空串
		"Success",           // 首字母大写
		"SUCCESS",           // 全大写
		"success ",          // 尾部空白
		" success",          // 头部空白
		"partial-failure",   // 连字符变体
		"partialfailure",    // 缺下划线
		"PARTIAL_FAILURE",   // 大写变体
		"Pending",           // 大小写变体
		"failure ",          // 尾部空白
		"ok",                // 历史键名，非 outcome 值
		"error",             // 历史键名，非 outcome 值
		"partial_failure\n", // 控制字符
	}
	for _, s := range rejected {
		got, err := ParseOutcome(s)
		if err == nil {
			t.Fatalf("ParseOutcome(%q) = %q, want error", s, got)
		}
		if got != "" {
			t.Fatalf("ParseOutcome(%q) returned %q on error, want zero value", s, got)
		}
	}
}

func TestParseOutcomeErrorMessageNamesAllFourValues(t *testing.T) {
	_, err := ParseOutcome("Success")
	if err == nil {
		t.Fatal("ParseOutcome(\"Success\") error = nil, want non-nil")
	}
	msg := err.Error()
	for _, want := range []string{`"Success"`, `"success"`, `"pending"`, `"partial_failure"`, `"failure"`} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message %q missing %q", msg, want)
		}
	}
}

// --- B2：Outcome.String() 与 JSON 序列化恒为四小写字符串 ---

func TestOutcomeStringIsCanonicalLowercase(t *testing.T) {
	want := []string{"success", "pending", "partial_failure", "failure"}
	for i, o := range canonicalOutcomes {
		if got := o.String(); got != want[i] {
			t.Fatalf("Outcome(%q).String() = %q, want %q", string(o), got, want[i])
		}
	}
}

func TestOutcomeJSONSerializesToExactLowercaseStrings(t *testing.T) {
	want := []string{`"success"`, `"pending"`, `"partial_failure"`, `"failure"`}
	for i, o := range canonicalOutcomes {
		raw, err := json.Marshal(o)
		if err != nil {
			t.Fatalf("json.Marshal(%q) error = %v", o, err)
		}
		if string(raw) != want[i] {
			t.Fatalf("json.Marshal(%q) = %s, want %s", o, raw, want[i])
		}
	}
}

func TestOutcomeJSONRejectsInvalidValues(t *testing.T) {
	for _, invalid := range []Outcome{"", "Success", "SUCCESS", "ok", "pending "} {
		if _, err := json.Marshal(invalid); err == nil {
			t.Fatalf("json.Marshal(%q) error = nil, want marshal-time rejection", string(invalid))
		}
	}
}

// --- B3：Envelope 结构体 json tag 对齐契约规范 §2.5 字段总表 ---

func TestEnvelopeJSONTagsMatchFieldTable(t *testing.T) {
	// §2.5 字段总表：字段名 + 是否 omitempty（ok/outcome 恒序列化）。
	want := []struct {
		name    string
		tag     string
		omitted bool // true = 契约要求 omitempty
	}{
		{"OK", "ok", false},
		{"Outcome", "outcome", false},
		{"Identity", "identity", true},
		{"DryRun", "dry_run", true},
		{"Data", "data", true},
		{"Meta", "meta", true},
		{"Error", "error", true},
		{"Notice", "_notice", true},
	}

	typ := reflect.TypeOf(Envelope{})
	if typ.NumField() != len(want) {
		t.Fatalf("Envelope has %d fields, want %d (§2.5 field table)", typ.NumField(), len(want))
	}
	for i, w := range want {
		f := typ.Field(i)
		if f.Name != w.name {
			t.Fatalf("Envelope field %d = %q, want %q (§2.5 order)", i, f.Name, w.name)
		}
		tag := f.Tag.Get("json")
		if tag != w.tag && tag != w.tag+",omitempty" {
			t.Fatalf("Envelope.%s json tag = %q, want %q(,omitempty)", f.Name, tag, w.tag)
		}
		hasOmit := strings.HasSuffix(tag, ",omitempty")
		if hasOmit != w.omitted {
			t.Fatalf("Envelope.%s omitempty = %v, want %v (§2.5)", f.Name, hasOmit, w.omitted)
		}
	}
}

func TestEnvelopeFieldTypesMatchFieldTable(t *testing.T) {
	typ := reflect.TypeOf(Envelope{})
	byName := map[string]reflect.Type{
		"OK":       reflect.TypeOf(false),
		"Outcome":  reflect.TypeOf(OutcomeSuccess),
		"Identity": reflect.TypeOf(""),
		"DryRun":   reflect.TypeOf(false),
		"Meta":     reflect.TypeOf((*Meta)(nil)),
		"Error":    reflect.TypeOf((*ErrorInfo)(nil)),
	}
	for name, want := range byName {
		f, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("Envelope missing field %q", name)
		}
		if f.Type != want {
			t.Fatalf("Envelope.%s type = %v, want %v", name, f.Type, want)
		}
	}
}

// --- B4：Meta 结构体与嵌套序列化 ---

func TestMetaJSONTagsMatchFieldTable(t *testing.T) {
	want := []struct {
		name string
		tag  string
	}{
		{"Count", "count,omitempty"},
		{"Operation", "operation,omitempty"},
		{"Pagination", "pagination,omitempty"},
	}
	typ := reflect.TypeOf(Meta{})
	if typ.NumField() != len(want) {
		t.Fatalf("Meta has %d fields, want %d (§2.5 meta.* rows)", typ.NumField(), len(want))
	}
	for i, w := range want {
		f := typ.Field(i)
		if f.Name != w.name {
			t.Fatalf("Meta field %d = %q, want %q", i, f.Name, w.name)
		}
		if tag := f.Tag.Get("json"); tag != w.tag {
			t.Fatalf("Meta.%s json tag = %q, want %q", f.Name, tag, w.tag)
		}
	}
}

func TestEnvelopeNestedMetaSerialization(t *testing.T) {
	env := Envelope{
		OK:      true,
		Outcome: OutcomeSuccess,
		Meta: &Meta{
			Count: NewCount(60),
			Operation: &OperationInfo{
				ID:          "task_abc",
				State:       "processing",
				NextCommand: "dws wiki task-result --task task_abc",
			},
			Pagination: &Pagination{
				EndpointExhausted: false,
				Pages:             3,
				Items:             60,
				NextToken:         "cursor_xyz",
			},
		},
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal(envelope) error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}

	meta, ok := decoded["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing or not an object in %s", raw)
	}
	if got, ok := meta["count"].(float64); !ok || got != 60 {
		t.Fatalf("meta.count = %v, want 60", meta["count"])
	}
	op, ok := meta["operation"].(map[string]any)
	if !ok {
		t.Fatalf("meta.operation missing or not an object in %s", raw)
	}
	if op["id"] != "task_abc" || op["state"] != "processing" ||
		op["next_command"] != "dws wiki task-result --task task_abc" {
		t.Fatalf("meta.operation = %v, want id/state/next_command preserved", op)
	}
	pg, ok := meta["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("meta.pagination missing or not an object in %s", raw)
	}
	if pg["endpoint_exhausted"] != false {
		t.Fatalf("meta.pagination.endpoint_exhausted = %v, want false (explicit)", pg["endpoint_exhausted"])
	}
	if got, ok := pg["pages"].(float64); !ok || got != 3 {
		t.Fatalf("meta.pagination.pages = %v, want 3", pg["pages"])
	}
	if pg["next_token"] != "cursor_xyz" {
		t.Fatalf("meta.pagination.next_token = %v, want cursor_xyz", pg["next_token"])
	}
}

// --- B7：OperationInfo json tag（§2.2，全部 omitempty）---

func TestOperationInfoJSONTagsAllOmitempty(t *testing.T) {
	want := map[string]string{
		"ID":          "id,omitempty",
		"State":       "state,omitempty",
		"TimedOut":    "timed_out,omitempty",
		"NextCommand": "next_command,omitempty",
	}
	typ := reflect.TypeOf(OperationInfo{})
	if typ.NumField() != len(want) {
		t.Fatalf("OperationInfo has %d fields, want %d (§2.2)", typ.NumField(), len(want))
	}
	for i := range typ.NumField() {
		f := typ.Field(i)
		w, ok := want[f.Name]
		if !ok {
			t.Fatalf("OperationInfo has unexpected field %q", f.Name)
		}
		if tag := f.Tag.Get("json"); tag != w {
			t.Fatalf("OperationInfo.%s json tag = %q, want %q", f.Name, tag, w)
		}
	}
}

// --- B8：Pagination json 字段（§3）---

func TestPaginationEndpointExhaustedAlwaysPresent(t *testing.T) {
	// endpoint_exhausted 两态都必须显式出现（无 omitempty）：
	// false+next_token = 可续跑；true = 观察到服务端耗尽。
	raw, err := json.Marshal(Pagination{EndpointExhausted: true})
	if err != nil {
		t.Fatalf("json.Marshal(Pagination) error = %v", err)
	}
	if !strings.Contains(string(raw), `"endpoint_exhausted":true`) {
		t.Fatalf("endpoint_exhausted:true must be explicit, got %s", raw)
	}
	raw, err = json.Marshal(Pagination{NextToken: "cursor_xyz"})
	if err != nil {
		t.Fatalf("json.Marshal(Pagination) error = %v", err)
	}
	if !strings.Contains(string(raw), `"endpoint_exhausted":false`) {
		t.Fatalf("endpoint_exhausted:false must be explicit, got %s", raw)
	}
	if !strings.Contains(string(raw), `"next_token":"cursor_xyz"`) {
		t.Fatalf("next_token missing, got %s", raw)
	}
}

func TestPaginationJSONTagsMatchContract(t *testing.T) {
	want := []struct {
		name string
		tag  string
	}{
		{"EndpointExhausted", "endpoint_exhausted"},
		{"Pages", "pages,omitempty"},
		{"Items", "items,omitempty"},
		{"NextToken", "next_token,omitempty"},
	}
	typ := reflect.TypeOf(Pagination{})
	if typ.NumField() != len(want) {
		t.Fatalf("Pagination has %d fields, want %d (§3)", typ.NumField(), len(want))
	}
	for i, w := range want {
		f := typ.Field(i)
		if f.Name != w.name {
			t.Fatalf("Pagination field %d = %q, want %q", i, f.Name, w.name)
		}
		if tag := f.Tag.Get("json"); tag != w.tag {
			t.Fatalf("Pagination.%s json tag = %q, want %q", f.Name, tag, w.tag)
		}
	}
}

// --- B9/B10：ErrorInfo 字段组序列化（§2.4）---

func TestErrorInfoWireStableFieldsSerialize(t *testing.T) {
	retryAfter := int64(30)
	info := ErrorInfo{
		Type:              "api",
		Subtype:           "rate_limit",
		Code:              90018,
		Retryable:         true,
		RetryAfterSeconds: &retryAfter,
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal(ErrorInfo) error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	if decoded["type"] != "api" || decoded["subtype"] != "rate_limit" {
		t.Fatalf("type/subtype = %v/%v, want api/rate_limit", decoded["type"], decoded["subtype"])
	}
	if got, ok := decoded["code"].(float64); !ok || got != 90018 {
		t.Fatalf("code = %v, want 90018", decoded["code"])
	}
	if decoded["retryable"] != true {
		t.Fatalf("retryable = %v, want true present", decoded["retryable"])
	}
	if got, ok := decoded["retry_after_seconds"].(float64); !ok || got != 30 {
		t.Fatalf("retry_after_seconds = %v, want 30", decoded["retry_after_seconds"])
	}
}

func TestErrorInfoRetryableAbsentWhenFalse(t *testing.T) {
	raw, err := json.Marshal(ErrorInfo{Type: "validation", Message: "bad flag"})
	if err != nil {
		t.Fatalf("json.Marshal(ErrorInfo) error = %v", err)
	}
	s := string(raw)
	if strings.Contains(s, "retryable") {
		t.Fatalf("retryable:false must be omitted from wire, got %s", s)
	}
}

func TestErrorInfoRetryAfterSecondsPreservesZero(t *testing.T) {
	zero := int64(0)
	raw, err := json.Marshal(ErrorInfo{Type: "api", Retryable: true, RetryAfterSeconds: &zero})
	if err != nil {
		t.Fatalf("json.Marshal(ErrorInfo) error = %v", err)
	}
	if !strings.Contains(string(raw), `"retry_after_seconds":0`) {
		t.Fatalf("retry_after_seconds:0 must serialize explicitly (0s is meaningful), got %s", raw)
	}

	absent, err := json.Marshal(ErrorInfo{Type: "api"})
	if err != nil {
		t.Fatalf("json.Marshal(ErrorInfo) error = %v", err)
	}
	if strings.Contains(string(absent), "retry_after_seconds") {
		t.Fatalf("nil retry_after_seconds must be omitted, got %s", absent)
	}
}

func TestErrorInfoValidationFieldsSerialize(t *testing.T) {
	info := ErrorInfo{
		Type:    "validation",
		Subtype: "confirmation_required",
		Message: "confirmation needed",
		Hint:    "re-run with --yes",
		Param:   "--user-id",
		Params:  []string{"--user-id", "--dept-id"},
		Actions: []string{"dws dev app delete --app-id a1 --yes"},
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal(ErrorInfo) error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	if decoded["param"] != "--user-id" {
		t.Fatalf("param = %v, want --user-id", decoded["param"])
	}
	params, ok := decoded["params"].([]any)
	if !ok || len(params) != 2 {
		t.Fatalf("params = %v, want 2 entries", decoded["params"])
	}
	actions, ok := decoded["actions"].([]any)
	if !ok || len(actions) != 1 {
		t.Fatalf("actions = %v, want 1 entry", decoded["actions"])
	}
}

func TestMetaCountDistinguishesZeroFromAbsent(t *testing.T) {
	withZero, err := json.Marshal(Envelope{OK: true, Outcome: OutcomeSuccess, Meta: &Meta{Count: NewCount(0)}})
	if err != nil {
		t.Fatalf("json.Marshal(count:0) error = %v", err)
	}
	if !strings.Contains(string(withZero), `"count":0`) {
		t.Fatalf("count:0 must serialize explicitly, got %s", withZero)
	}

	absent, err := json.Marshal(Envelope{OK: true, Outcome: OutcomeSuccess, Meta: &Meta{}})
	if err != nil {
		t.Fatalf("json.Marshal(empty meta) error = %v", err)
	}
	if strings.Contains(string(absent), "count") {
		t.Fatalf("unset count must be omitted, got %s", absent)
	}
}

// --- B11：omitempty 语义——空值一律省略、不输出 null（契约规范 §2.5 末段）---

// marshalEnvelope 序列化信封并在失败时终止测试。
func marshalEnvelope(t *testing.T, env Envelope) []byte {
	t.Helper()
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal(envelope) error = %v", err)
	}
	return raw
}

func TestEnvelopeOmitEmptyNeverEmitsNull(t *testing.T) {
	// 全零值信封：除恒序列化的 ok/outcome 外，所有可选字段缺席，
	// wire 中不得出现任何 null（token 效率 + 对齐 lark）。
	raw := marshalEnvelope(t, Envelope{OK: true, Outcome: OutcomeSuccess})
	if s := string(raw); s != `{"ok":true,"outcome":"success"}` {
		t.Fatalf("empty-value envelope = %s, want exactly {\"ok\":true,\"outcome\":\"success\"}", s)
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("empty values must be omitted, never null: %s", raw)
	}
}

func TestEnvelopeOmitEmptyPerFieldTable(t *testing.T) {
	raw := marshalEnvelope(t, Envelope{OK: true, Outcome: OutcomePending})
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	for _, key := range []string{"identity", "dry_run", "data", "meta", "error", "_notice"} {
		if v, present := decoded[key]; present {
			t.Fatalf("empty %s must be omitted (§2.5 omitempty), got %v in %s", key, v, raw)
		}
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("no null values allowed on empty fields: %s", raw)
	}
}

func TestEnvelopeMetaOmitEmptyNeverEmitsNull(t *testing.T) {
	// meta 非空但内部字段全部未设置：count/operation/pagination 一律缺席。
	raw := marshalEnvelope(t, Envelope{OK: true, Outcome: OutcomeSuccess, Meta: &Meta{}})
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	meta, ok := decoded["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing in %s", raw)
	}
	if len(meta) != 0 {
		t.Fatalf("empty meta must serialize as {} with no null entries, got %s", raw)
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("empty meta fields must be omitted, never null: %s", raw)
	}
}

func TestEnvelopeEmptySliceDataIsPreservedNotOmitted(t *testing.T) {
	// 空数组是合法业务载荷（空结果 data:[] + count:0，AC-06）：
	// omitempty 对非 nil 空切片不生效，必须原样输出 []，且不是 null。
	raw := marshalEnvelope(t, Envelope{OK: true, Outcome: OutcomeSuccess, Data: []any{}})
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	data, ok := decoded["data"].([]any)
	if !ok {
		t.Fatalf("data:[] must be present as an empty array, got %s", raw)
	}
	if len(data) != 0 {
		t.Fatalf("data = %v, want empty array", decoded["data"])
	}
	if !strings.Contains(string(raw), `"data":[]`) {
		t.Fatalf("empty-array data must serialize as [], got %s", raw)
	}
}

// --- B12：Envelope JSON 顶层键集合 golden 断言（契约规范 §2 / §2.5）---

func envelopeTopLevelKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	// json.Decoder 保序读入对象键，顺序即 Go 声明序（golden 键序前提）。
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		t.Fatalf("envelope JSON must be an object, got %v (err %v) from %s", tok, err, raw)
	}
	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatalf("read key token from %s: %v", raw, err)
		}
		key, ok := keyTok.(string)
		if !ok {
			t.Fatalf("non-string key token %v in %s", keyTok, raw)
		}
		keys = append(keys, key)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("decode value for key %q in %s: %v", key, raw, err)
		}
	}
	return keys
}

func TestEnvelopeTopLevelKeySetGoldenFullEnvelope(t *testing.T) {
	env := Envelope{
		OK:       true,
		Outcome:  OutcomeSuccess,
		Identity: "user",
		DryRun:   true,
		Data:     map[string]any{"todoId": "t_123"},
		Meta:     &Meta{Count: NewCount(1)},
		Error:    nil,
		Notice:   map[string]any{},
	}
	raw := marshalEnvelope(t, env)

	// golden 键集合（§2.5 字段总表）；含 error 键的完整集合在失败信封测试断言。
	want := []string{"ok", "outcome", "identity", "dry_run", "data", "meta", "_notice"}
	got := envelopeTopLevelKeys(t, raw)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("top-level keys = %v, want golden %v (declaration order, §2.5)", got, want)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	if len(decoded) != len(want) {
		t.Fatalf("decoded key count = %d, want %d", len(decoded), len(want))
	}
}

func TestEnvelopeTopLevelKeySetGoldenFailureEnvelope(t *testing.T) {
	env := Envelope{
		OK:      false,
		Outcome: OutcomeFailure,
		Error:   &ErrorInfo{Type: "api", Subtype: "rate_limit", Code: 90018},
	}
	raw := marshalEnvelope(t, env)
	want := []string{"ok", "outcome", "error"}
	if got := envelopeTopLevelKeys(t, raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("top-level keys = %v, want golden %v", got, want)
	}
}

func TestEnvelopeTopLevelKeySetIsClosedWhitelist(t *testing.T) {
	// 顶层键只允许 §2.5 字段总表白名单；任何非白名单键（含历史键名）都不得出现。
	allowed := map[string]bool{
		"ok": true, "outcome": true, "identity": true, "dry_run": true,
		"data": true, "meta": true, "error": true, "_notice": true,
	}
	retryAfter := int64(30)
	env := Envelope{
		OK:       false,
		Outcome:  OutcomeFailure,
		Identity: "app",
		DryRun:   true,
		Data:     []any{},
		Meta: &Meta{
			Count:     NewCount(0),
			Operation: &OperationInfo{ID: "task_abc", State: "processing"},
			Pagination: &Pagination{
				EndpointExhausted: false,
				NextToken:         "cursor_xyz",
				Pages:             3,
				Items:             60,
			},
		},
		Error: &ErrorInfo{
			Type: "api", Subtype: "rate_limit", Code: 90018, Message: "too many requests",
			Hint: "retry", Retryable: true, RetryAfterSeconds: &retryAfter, RequestID: "req_xxx",
			Param: "--user-id", Params: []string{"--user-id"}, Actions: []string{"dws dev app list --yes"},
		},
		Notice: map[string]any{},
	}
	raw := marshalEnvelope(t, env)
	for _, key := range envelopeTopLevelKeys(t, raw) {
		if !allowed[key] {
			t.Fatalf("unexpected top-level key %q outside §2.5 whitelist in %s", key, raw)
		}
	}
}

// --- B13：ok 恒为 JSON bool（AC-02，禁止字符串布尔）---

func TestEnvelopeOKSerializesAsJSONBoolean(t *testing.T) {
	for _, ok := range []bool{true, false} {
		raw := marshalEnvelope(t, Envelope{OK: ok, Outcome: OutcomeSuccess})
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
		}
		got, present := decoded["ok"]
		if !present {
			t.Fatalf("ok must always be present (no omitempty), got %s", raw)
		}
		boolVal, isBool := got.(bool)
		if !isBool {
			t.Fatalf("ok = %v (%T), want JSON boolean (§2.5 禁止字符串布尔)", got, got)
		}
		if boolVal != ok {
			t.Fatalf("ok = %v, want %v", boolVal, ok)
		}
	}
}

func TestEnvelopeOKWireBytesAreBooleanLiterals(t *testing.T) {
	// wire 层断言：ok 只能以 true/false 字面量出现，绝不出现 "true"/"false"。
	if raw := marshalEnvelope(t, Envelope{OK: true, Outcome: OutcomeSuccess}); !strings.Contains(string(raw), `"ok":true`) {
		t.Fatalf("ok:true must be a bare boolean, got %s", raw)
	}
	if raw := marshalEnvelope(t, Envelope{OK: false, Outcome: OutcomeFailure}); !strings.Contains(string(raw), `"ok":false`) {
		t.Fatalf("ok:false must be a bare boolean, got %s", raw)
	}
	for _, banned := range []string{`"ok":"true"`, `"ok":"false"`} {
		for _, ok := range []bool{true, false} {
			if raw := marshalEnvelope(t, Envelope{OK: ok, Outcome: OutcomeSuccess}); strings.Contains(string(raw), banned) {
				t.Fatalf("string boolean %s must never appear, got %s", banned, raw)
			}
		}
	}
}

func TestEnvelopeOKFieldGoTypeIsBool(t *testing.T) {
	// 类型层断言：Go 字段类型为 bool，字符串布尔在编译期即不可表达。
	f, ok := reflect.TypeOf(Envelope{}).FieldByName("OK")
	if !ok {
		t.Fatal("Envelope missing field OK")
	}
	if f.Type.Kind() != reflect.Bool {
		t.Fatalf("Envelope.OK kind = %v, want bool (AC-02)", f.Type.Kind())
	}
	if f.Tag.Get("json") != "ok" {
		t.Fatalf("Envelope.OK json tag = %q, want \"ok\" (恒序列化，无 omitempty)", f.Tag.Get("json"))
	}
}

func TestEnvelopeDecoderRejectsStringBooleanOK(t *testing.T) {
	// wire 守卫断言：若外部产出字符串布尔信封（契约禁止形态），
	// 严格解码进 Envelope 必须失败——证明信封类型无法吞下该违规形态。
	for _, wire := range []string{
		`{"ok":"true","outcome":"success"}`,
		`{"ok":"false","outcome":"failure"}`,
	} {
		var env Envelope
		if err := json.Unmarshal([]byte(wire), &env); err == nil {
			t.Fatalf("json.Unmarshal(%s) into Envelope error = nil, want strict rejection of string boolean", wire)
		}
	}
}

// --- B14：ParseOutcome 非法值拒绝与错误信息（契约规范 §1）---

func TestParseOutcomeRejectsNonCanonicalWithDiagnosticMessage(t *testing.T) {
	for _, invalid := range []string{"", "Success", "SUCCESS", "Pending", "PARTIAL_FAILURE", "success ", " partial_failure", "ok"} {
		outcome, err := ParseOutcome(invalid)
		if err == nil {
			t.Fatalf("ParseOutcome(%q) = %q, want rejection (§1 仅四值)", invalid, outcome)
		}
		msg := err.Error()
		// 错误信息必须回显非法输入（含空串），Agent/开发者可直接定位。
		if !strings.Contains(msg, fmt.Sprintf("%q", invalid)) {
			t.Fatalf("error %q must echo the invalid input %q", msg, invalid)
		}
		// 错误信息必须列全四个合法值，避免二次试错。
		for _, canonical := range []string{`"success"`, `"pending"`, `"partial_failure"`, `"failure"`} {
			if !strings.Contains(msg, canonical) {
				t.Fatalf("error %q for input %q must list legal value %s", msg, invalid, canonical)
			}
		}
	}
}

func TestParseOutcomeCanonicalValuesRoundTripWithNoError(t *testing.T) {
	for _, canonical := range canonicalOutcomes {
		got, err := ParseOutcome(canonical.String())
		if err != nil {
			t.Fatalf("ParseOutcome(%q) error = %v, want acceptance", canonical, err)
		}
		if got != canonical {
			t.Fatalf("ParseOutcome(%q) = %q, want %q", canonical, got, canonical)
		}
	}
}

// --- B15：ErrorInfo.Retryable 仅 true 时出现（契约规范 §2.4；重试设计 §2.1）---

func TestErrorInfoRetryableAppearsOnlyWhenTrue(t *testing.T) {
	// Retryable:false（零值）必须缺席：omitempty 语义，wire 上不出现 false。
	raw, err := json.Marshal(ErrorInfo{Type: "api", Subtype: "not_found", Retryable: false})
	if err != nil {
		t.Fatalf("json.Marshal(ErrorInfo) error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	if v, present := decoded["retryable"]; present {
		t.Fatalf("retryable:false must be absent from wire (§2.4), got %v in %s", v, raw)
	}
	if strings.Contains(string(raw), "retryable") {
		t.Fatalf("retryable must be omitted when false, got %s", raw)
	}

	// Retryable:true 必须显式出现，且恰为布尔 true。
	raw, err = json.Marshal(ErrorInfo{Type: "api", Subtype: "rate_limit", Retryable: true})
	if err != nil {
		t.Fatalf("json.Marshal(ErrorInfo) error = %v", err)
	}
	if !strings.Contains(string(raw), `"retryable":true`) {
		t.Fatalf("retryable:true must appear exactly, got %s", raw)
	}
	if strings.Contains(string(raw), `"retryable":false`) {
		t.Fatalf("retryable:false must never serialize, got %s", raw)
	}
}

func TestErrorInfoRetryableTagIsOmitempty(t *testing.T) {
	f, ok := reflect.TypeOf(ErrorInfo{}).FieldByName("Retryable")
	if !ok {
		t.Fatal("ErrorInfo missing field Retryable")
	}
	if tag := f.Tag.Get("json"); tag != "retryable,omitempty" {
		t.Fatalf("ErrorInfo.Retryable json tag = %q, want \"retryable,omitempty\" (§2.4)", tag)
	}
}

func TestEnvelopeFailureEnvelopeOmitsAbsentRetryable(t *testing.T) {
	// 完整失败信封视角：error 未标可重试时，整个 wire 不出现 retryable 键。
	env := Envelope{
		OK:      false,
		Outcome: OutcomeFailure,
		Error:   &ErrorInfo{Type: "validation", Message: "bad flag"},
	}
	raw := marshalEnvelope(t, env)
	if strings.Contains(string(raw), "retryable") {
		t.Fatalf("retryable must not leak into wire when false, got %s", raw)
	}
}

// --- B16：data/error 互斥校验（契约规范 §1 不变量 I3）---

func TestEnvelopeDataErrorExclusivityRejectsBothPresent(t *testing.T) {
	// error 非空时 data 必须缺席：同时出现即违反 I3 字段级推论。
	env := &Envelope{
		OK:      false,
		Outcome: OutcomeFailure,
		Data:    map[string]any{"id": "a"},
		Error:   &ErrorInfo{Type: "api", Code: 1},
	}
	if err := env.ValidateDataErrorExclusivity(); err == nil {
		t.Fatal("ValidateDataErrorExclusivity() = nil, want I3 violation error for data+error")
	} else if !strings.Contains(err.Error(), "I3") {
		t.Fatalf("error %q must reference invariant I3", err.Error())
	}
}

func TestEnvelopeDataErrorExclusivityAcceptsFailureShape(t *testing.T) {
	// §2.4 失败形态：error 非空、data 缺席，合法。
	env := &Envelope{
		OK:      false,
		Outcome: OutcomeFailure,
		Error:   &ErrorInfo{Type: "api", Subtype: "rate_limit"},
	}
	if err := env.ValidateDataErrorExclusivity(); err != nil {
		t.Fatalf("ValidateDataErrorExclusivity() = %v, want nil for error-only envelope", err)
	}
}

func TestEnvelopeDataErrorExclusivityAcceptsDataShapes(t *testing.T) {
	cases := map[string]*Envelope{
		"success with data":        {OK: true, Outcome: OutcomeSuccess, Data: map[string]any{"todoId": "t_1"}},
		"success empty-array data": {OK: true, Outcome: OutcomeSuccess, Data: []any{}},
		"partial_failure channels": {OK: false, Outcome: OutcomePartialFailure, Data: map[string]any{"total": 3}},
		"pending with operation":   {OK: true, Outcome: OutcomePending, Meta: &Meta{Operation: &OperationInfo{ID: "task_abc"}}},
		"nil envelope":             nil,
	}
	for name, env := range cases {
		if err := env.ValidateDataErrorExclusivity(); err != nil {
			t.Fatalf("%s: ValidateDataErrorExclusivity() = %v, want nil", name, err)
		}
	}
}

func TestEnvelopeDataErrorExclusivityEmptyErrorStillViolates(t *testing.T) {
	// 只要 Error 指针非 nil 即视为 error 非空（零值 ErrorInfo 也算），
	// 与 I3「error 字段非空 ⇔ outcome==failure」的指针语义一致。
	env := &Envelope{OK: false, Outcome: OutcomeFailure, Data: []any{}, Error: &ErrorInfo{}}
	if err := env.ValidateDataErrorExclusivity(); err == nil {
		t.Fatal("ValidateDataErrorExclusivity() = nil, want error when Error pointer is non-nil alongside data")
	}
}

// --- B17：_notice 系统级通知注入位（契约规范 §2.5）---

func TestEnvelopeNoticeInjectionSlotSerializesAsObject(t *testing.T) {
	// _notice 是系统级通知注入位：设置时以 object 形态出现在 wire 上。
	notice := map[string]any{"upgrade": map[string]any{"available": true}}
	raw := marshalEnvelope(t, Envelope{OK: true, Outcome: OutcomeSuccess, Notice: notice})
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	got, present := decoded["_notice"]
	if !present {
		t.Fatalf("_notice injection slot missing when Notice set: %s", raw)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("_notice = %v (%T), want JSON object (§2.5 注入位)", got, got)
	}
	if _, ok := obj["upgrade"]; !ok {
		t.Fatalf("_notice payload lost in %s", raw)
	}
	// 注入位是顶层最后一个键（§2.5 表行序，声明序=wire 序）。
	keys := envelopeTopLevelKeys(t, raw)
	if keys[len(keys)-1] != "_notice" {
		t.Fatalf("top-level keys = %v, want _notice last (§2.5 field table order)", keys)
	}
}

func TestEnvelopeNoticeOmittedWhenUnset(t *testing.T) {
	// 未注入通知时 _notice 缺席（omitempty），绝不输出 null。
	raw := marshalEnvelope(t, Envelope{OK: true, Outcome: OutcomeSuccess})
	if strings.Contains(string(raw), "_notice") {
		t.Fatalf("unset _notice must be omitted, got %s", raw)
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("_notice must never serialize as null, got %s", raw)
	}
}

// --- B18：Envelope.Validate() 聚合校验入口（契约规范 §1）---

func TestEnvelopeValidateAcceptsCanonicalShapes(t *testing.T) {
	cases := map[string]*Envelope{
		"success":         NewSuccessEnvelope(map[string]any{"todoId": "t_123"}),
		"success empty":   NewSuccessEnvelope([]any{}),
		"pending":         NewPendingEnvelope(&OperationInfo{ID: "task_abc", State: "processing"}),
		"pending no meta": NewPendingEnvelope(nil),
		"failure":         NewFailureEnvelope(&ErrorInfo{Type: "api", Subtype: "rate_limit"}),
		"partial":         NewPartialEnvelope(map[string]any{"total": 3}),
		"nil envelope":    nil,
	}
	for name, env := range cases {
		if err := env.Validate(); err != nil {
			t.Fatalf("%s: Validate() = %v, want nil", name, err)
		}
	}
}

func TestEnvelopeValidateRejectsInvalidOutcome(t *testing.T) {
	for _, invalid := range []Outcome{"", "Success", "ok"} {
		env := &Envelope{OK: true, Outcome: invalid}
		err := env.Validate()
		if err == nil {
			t.Fatalf("Validate() = nil for outcome %q, want rejection (§1 仅四值)", invalid)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("%q", string(invalid))) {
			t.Fatalf("error %q must echo invalid outcome %q", err.Error(), invalid)
		}
	}
}

func TestEnvelopeValidateRejectsI1Violations(t *testing.T) {
	cases := map[string]*Envelope{
		"failure with ok=true":  {OK: true, Outcome: OutcomeFailure},
		"partial with ok=true":  {OK: true, Outcome: OutcomePartialFailure},
		"success with ok=false": {OK: false, Outcome: OutcomeSuccess, Data: map[string]any{}},
		"pending with ok=false": {OK: false, Outcome: OutcomePending},
	}
	for name, env := range cases {
		err := env.Validate()
		if err == nil {
			t.Fatalf("%s: Validate() = nil, want I1 violation", name)
		}
		if !strings.Contains(err.Error(), "I1") {
			t.Fatalf("%s: error %q must reference invariant I1", name, err.Error())
		}
	}
}

func TestEnvelopeValidateRejectsI3Violations(t *testing.T) {
	cases := map[string]*Envelope{
		"failure without error": {OK: false, Outcome: OutcomeFailure},
		"success with error":    {OK: true, Outcome: OutcomeSuccess, Error: &ErrorInfo{Type: "api"}},
		"pending with error":    {OK: true, Outcome: OutcomePending, Error: &ErrorInfo{}},
		"partial with error":    {OK: false, Outcome: OutcomePartialFailure, Error: &ErrorInfo{Type: "api"}},
	}
	for name, env := range cases {
		err := env.Validate()
		if err == nil {
			t.Fatalf("%s: Validate() = nil, want I3 violation", name)
		}
		if !strings.Contains(err.Error(), "I3") {
			t.Fatalf("%s: error %q must reference invariant I3", name, err.Error())
		}
	}
}

func TestEnvelopeValidateAggregatesMultipleViolations(t *testing.T) {
	// 聚合入口：一次报全，不因首个违反项提前返回。
	// ok=true 配 failure（I1）+ data 与 error 并存（I3 互斥）+ error 配 failure 合法，
	// 此处构造 I1 + I3 互斥两项同时违反。
	env := &Envelope{
		OK:      true,
		Outcome: OutcomeFailure,
		Data:    map[string]any{"id": "a"},
		Error:   &ErrorInfo{Type: "api"},
	}
	err := env.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want aggregated violations")
	}
	msg := err.Error()
	if !strings.Contains(msg, "I1") {
		t.Fatalf("aggregated error %q must include I1 violation", msg)
	}
	if !strings.Contains(msg, "I3") {
		t.Fatalf("aggregated error %q must include I3 exclusivity violation", msg)
	}
}

func TestEnvelopeValidateFailureConstructorRequiresErrorInfo(t *testing.T) {
	// 构造器不做前置拦截（保持简单装配），但 nil error 的失败信封
	// 无法通过 Validate()：I3「error 非空 ⇔ failure」双向约束。
	env := NewFailureEnvelope(nil)
	if err := env.Validate(); err == nil {
		t.Fatal("Validate() = nil for failure envelope without error, want I3 violation")
	}
}

// --- B19/B20：四类信封构造函数（契约规范 §2.1/§2.2/§2.3/§2.4）---

func TestNewSuccessEnvelopeShape(t *testing.T) {
	env := NewSuccessEnvelope(map[string]any{"todoId": "t_123"})
	if !env.OK || env.Outcome != OutcomeSuccess {
		t.Fatalf("NewSuccessEnvelope = ok:%v outcome:%q, want true/success (§2.1)", env.OK, env.Outcome)
	}
	if env.Error != nil || env.Meta != nil {
		t.Fatalf("success envelope must not carry error/meta by default: %+v", env)
	}
	raw := marshalEnvelope(t, *env)
	if s := string(raw); s != `{"ok":true,"outcome":"success","data":{"todoId":"t_123"}}` {
		t.Fatalf("success envelope wire = %s, want ok/outcome/data only", s)
	}
}

func TestNewPendingEnvelopeShape(t *testing.T) {
	op := &OperationInfo{
		ID:          "task_abc",
		State:       "processing",
		NextCommand: "dws wiki task-result --task task_abc",
	}
	env := NewPendingEnvelope(op)
	if !env.OK || env.Outcome != OutcomePending {
		t.Fatalf("NewPendingEnvelope = ok:%v outcome:%q, want true/pending (§2.2)", env.OK, env.Outcome)
	}
	if env.Meta == nil || env.Meta.Operation != op {
		t.Fatalf("pending envelope must carry meta.operation: %+v", env.Meta)
	}
	raw := marshalEnvelope(t, *env)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", raw, err)
	}
	meta, ok := decoded["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing in %s", raw)
	}
	operation, ok := meta["operation"].(map[string]any)
	if !ok {
		t.Fatalf("meta.operation missing in %s", raw)
	}
	if operation["next_command"] != "dws wiki task-result --task task_abc" {
		t.Fatalf("next_command = %v, want preserved", operation["next_command"])
	}
}

func TestNewPendingEnvelopeNilOperationOmitsMeta(t *testing.T) {
	env := NewPendingEnvelope(nil)
	if env.Meta != nil {
		t.Fatalf("nil operation must leave Meta unset, got %+v", env.Meta)
	}
	raw := marshalEnvelope(t, *env)
	if strings.Contains(string(raw), "meta") || strings.Contains(string(raw), "null") {
		t.Fatalf("pending envelope without operation must omit meta and null: %s", raw)
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil for bare pending envelope", err)
	}
}

func TestNewFailureEnvelopeShape(t *testing.T) {
	info := &ErrorInfo{Type: "api", Subtype: "rate_limit", Code: 90018}
	env := NewFailureEnvelope(info)
	if env.OK || env.Outcome != OutcomeFailure {
		t.Fatalf("NewFailureEnvelope = ok:%v outcome:%q, want false/failure (§2.4)", env.OK, env.Outcome)
	}
	if env.Error != info || env.Data != nil {
		t.Fatalf("failure envelope must carry error only: %+v", env)
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil for §2.4 shape", err)
	}
	raw := marshalEnvelope(t, *env)
	if got := envelopeTopLevelKeys(t, raw); !reflect.DeepEqual(got, []string{"ok", "outcome", "error"}) {
		t.Fatalf("failure envelope keys = %v, want [ok outcome error]", got)
	}
}

func TestErrorInfoValidateRejectsUnknownWireType(t *testing.T) {
	for _, errorType := range []string{"network", "confirmation", "mystery"} {
		info := &ErrorInfo{Type: errorType, Message: "must use a governed type"}
		if err := info.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported failure error.type") {
			t.Fatalf("ErrorInfo.Validate(%q) = %v, want unsupported type error", errorType, err)
		}
	}
	for _, errorType := range []string{"api", "auth", "validation", "permission", "discovery", "internal"} {
		if err := (&ErrorInfo{Type: errorType}).Validate(); err != nil {
			t.Fatalf("ErrorInfo.Validate(%q) = %v, want governed type accepted", errorType, err)
		}
	}
}

func TestNewPartialEnvelopeShape(t *testing.T) {
	data := map[string]any{
		"total":     3,
		"succeeded": []any{map[string]any{"id": "a"}},
		"failed":    []any{map[string]any{"id": "b"}},
		"unknown":   []any{map[string]any{"id": "c", "reason": "timeout after submit"}},
	}
	env := NewPartialEnvelope(data)
	if env.OK || env.Outcome != OutcomePartialFailure {
		t.Fatalf("NewPartialEnvelope = ok:%v outcome:%q, want false/partial_failure (§2.3)", env.OK, env.Outcome)
	}
	if env.Error != nil {
		t.Fatalf("partial envelope must not carry top-level error (§2.3/I3): %+v", env)
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil for §2.3 shape", err)
	}
}

func TestEnvelopeConstructorsComputeOKFromOutcome(t *testing.T) {
	// I1 框架计算：四类构造器的 OK 值与 outcome 映射恒一致，命令层不可写。
	want := []struct {
		env *Envelope
		ok  bool
	}{
		{NewSuccessEnvelope(nil), true},
		{NewPendingEnvelope(nil), true},
		{NewPartialEnvelope(nil), false},
		{NewFailureEnvelope(&ErrorInfo{Type: "internal"}), false},
	}
	for i, c := range want {
		if c.env.OK != c.ok {
			t.Fatalf("constructor %d OK = %v, want %v (I1)", i, c.env.OK, c.ok)
		}
	}
}
