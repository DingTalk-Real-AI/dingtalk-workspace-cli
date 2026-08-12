package errors

// 错误信封 wire-stable 字段集（契约规范 §2.4；B174）。
//
// Agent 可编程分流的字段集合：type/subtype/code/retryable/
// retry_after_seconds/message/hint/actions/trace_id/rpc_code/rpc_data/
// outcome。wireErrors 的每个字段都必须落在此集合内；新增字段 = 契约
// 扩展，需评审。wireErrors 未声明 JSON tag 的字段（如 Cause）是内部
// 字段，序列化缺席，不属于 wire。

// WireStableFields 是错误信封 wire-stable 字段名全集（含 outcome）。
var WireStableFields = []string{
	"type",
	"subtype",
	"code",
	"retryable",
	"retry_after_seconds",
	"message",
	"hint",
	"actions",
	"trace_id",
	"rpc_code",
	"rpc_data",
	"outcome",
}

// WireStableErrorBodyFields 是 error 对象体（不含顶层 outcome）的
// wire-stable 字段名子集。
var WireStableErrorBodyFields = []string{
	"type",
	"subtype",
	"code",
	"retryable",
	"retry_after_seconds",
	"message",
	"hint",
	"actions",
	"trace_id",
	"rpc_code",
	"rpc_data",
}
