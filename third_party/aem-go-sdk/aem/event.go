package aem

// Event 是一次上报的载荷。Type 决定字段语义（api/event 等），
// Fields 携带 p1~p20、c1~c10、ext 以及 url、status 等业务维度。
//
// 字段命名约定：
//   - 通用字段：type、ts（毫秒）
//   - 自定义事件：type=event，p1 为事件 ID，p4 为事件类型（EXP/CLK/SLD/INPUT/SYS/OTHER）
//   - 平台保留：p1~p20（不同 type 含义不同，详见 AES 协议文档）
//   - 用户自定义：c1~c10
//   - 扩展 JSON：ext
//
// Fields 中值为空字符串的键会在序列化时被自动丢弃。
type Event struct {
	// Type 事件类型，如 "api"、"event"。必填。
	Type string

	// Fields 业务字段。Track 时会补全 ts；其余键的语义由调用方决定。
	Fields map[string]string
}

// toMap 把 Event 序列化为 SDK 内部使用的 map 形式，过滤空值。
//
// 注意：返回的 map 是新分配的，调用方修改不会影响原 Event。
func (e Event) toMap() map[string]string {
	m := make(map[string]string, len(e.Fields)+1)
	if e.Type != "" {
		m["type"] = e.Type
	}
	for k, v := range e.Fields {
		if v != "" {
			m[k] = v
		}
	}
	return m
}
