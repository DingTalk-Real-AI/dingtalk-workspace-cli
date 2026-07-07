package doc

import (
	"fmt"
	"strings"
)

// ──────────────────────────────────────────────────────────────────────────────
// Position / Range — 源文本定位，对齐 LSP Diagnostic 规范。
// ──────────────────────────────────────────────────────────────────────────────

// Position 表示源文本中的一个位置点。
type Position struct {
	Line   int   // 1-based 行号
	Col    int   // 1-based 列号 (UTF-8 byte offset within line)
	Offset int64 // 0-based 字节偏移 (相对于整个输入)
}

// Range 表示源文本中的一个区间 [Start, End)。
type Range struct {
	Start Position
	End   Position
}

// ──────────────────────────────────────────────────────────────────────────────
// Severity — 诊断严重程度。
// ──────────────────────────────────────────────────────────────────────────────

// Severity 诊断严重程度，值对齐 LSP Diagnostic (1=Error, 2=Warning, 3=Info, 4=Hint)。
type Severity int

const (
	SeverityError       Severity = 1
	SeverityWarning     Severity = 2
	SeverityInformation Severity = 3
	SeverityHint        Severity = 4
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInformation:
		return "info"
	case SeverityHint:
		return "hint"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Diagnostic — 单条诊断信息。
// ──────────────────────────────────────────────────────────────────────────────

// Diagnostic 表示一条诊断信息（错误/警告/提示）。
type Diagnostic struct {
	Range      Range              // 源位置
	Severity   Severity           // 严重程度
	Code       string             // 错误码 (见 error code 常量)
	Source     string             // "json-syntax" | "jsonml-grammar" | "jsonml-schema"
	Message    string             // 人可读描述
	Suggestion string             // 可粘贴的最小合法片段 (可选)
	Context    *DiagnosticContext // schema 上下文 (可选, Layer 2 填充)
}

// DiagnosticContext 附加的 schema 上下文信息。
type DiagnosticContext struct {
	TagName         string    // 当前/期望 tag
	Description     string    // P1: tag description from schema
	AllowedChildren []string  // 父节点合法子节点全集
	AllValidTags    []string  // schema 全量 tag 列表（unknown tag 时填充）
	AllValidAttrs   []string  // 当前 tag 全量 attr 列表（unknown attr 时填充）
	AttrSpec        *TypeSpec // 当前属性的 schema 定义
}

// String 返回单条诊断的格式化输出。
// 格式: "L{line}:{col} {severity}[{Code}]: {Message}"
func (d Diagnostic) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("L%d:%d %s[%s]: %s",
		d.Range.Start.Line, d.Range.Start.Col,
		d.Severity, d.Code, d.Message))
	if d.Suggestion != "" {
		sb.WriteString(". Suggestion: ")
		sb.WriteString(d.Suggestion)
	}
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// DiagnosticList — 诊断列表，支持聚合操作。
// ──────────────────────────────────────────────────────────────────────────────

// DiagnosticList 诊断列表。
type DiagnosticList []Diagnostic

// HasErrors 返回列表中是否包含 Error 级别诊断。
func (dl DiagnosticList) HasErrors() bool {
	for i := range dl {
		if dl[i].Severity == SeverityError {
			return true
		}
	}
	return false
}

// ErrorCount 返回 Error 级别诊断数量。
func (dl DiagnosticList) ErrorCount() int {
	n := 0
	for i := range dl {
		if dl[i].Severity == SeverityError {
			n++
		}
	}
	return n
}

// WarningCount 返回 Warning 级别诊断数量。
func (dl DiagnosticList) WarningCount() int {
	n := 0
	for i := range dl {
		if dl[i].Severity == SeverityWarning {
			n++
		}
	}
	return n
}

// Filter 按 Source 过滤诊断。
func (dl DiagnosticList) Filter(source string) DiagnosticList {
	var result DiagnosticList
	for i := range dl {
		if dl[i].Source == source {
			result = append(result, dl[i])
		}
	}
	return result
}

// Summary 返回人可读的诊断报告。
func (dl DiagnosticList) Summary() string {
	if len(dl) == 0 {
		return ""
	}
	errCount := dl.ErrorCount()
	warnCount := dl.WarningCount()

	var sb strings.Builder
	if errCount > 0 {
		sb.WriteString(fmt.Sprintf("JSONML 校验失败（%d 个错误", errCount))
		if warnCount > 0 {
			sb.WriteString(fmt.Sprintf(", %d 个警告", warnCount))
		}
		sb.WriteString("）:\n")
	} else if warnCount > 0 {
		sb.WriteString(fmt.Sprintf("JSONML 校验警告（%d 个）:\n", warnCount))
	}

	for i, d := range dl {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, d.String()))
	}
	return sb.String()
}

// ToLegacyResult 将 DiagnosticList 转为旧 JsonMLValidationResult 格式，保持对外兼容。
func (dl DiagnosticList) ToLegacyResult() *JsonMLValidationResult {
	r := &JsonMLValidationResult{}
	for _, d := range dl {
		line := d.String()
		switch d.Severity {
		case SeverityError:
			r.Errors = append(r.Errors, line)
		default:
			r.Warnings = append(r.Warnings, line)
		}
	}
	return r
}

// ──────────────────────────────────────────────────────────────────────────────
// Error Code 常量 — 按 Layer 分组。
// ──────────────────────────────────────────────────────────────────────────────

// Source 常量。
const (
	SourceJSONSyntax    = "json-syntax"
	SourceJSONMLGrammar = "jsonml-grammar"
	SourceJSONMLSchema  = "jsonml-schema"
)

// Layer 0: JSON Syntax errors (reported by jsontext.Decoder).
const (
	CodeJSONSyntaxUnexpectedToken = "JSON_SYNTAX_UNEXPECTED_TOKEN"
	CodeJSONSyntaxUnterminatedStr = "JSON_SYNTAX_UNTERMINATED_STRING"
	CodeJSONSyntaxInvalidEscape   = "JSON_SYNTAX_INVALID_ESCAPE"
	CodeJSONSyntaxTrailingData    = "JSON_SYNTAX_TRAILING_DATA"
)

// Layer 1: JSONML Grammar errors (structural violations).
const (
	CodeJSONMLSyntaxNotArray       = "JSONML_SYNTAX_NOT_ARRAY"
	CodeJSONMLSyntaxEmptyElement   = "JSONML_SYNTAX_EMPTY_ELEMENT"
	CodeJSONMLSyntaxTagNotString   = "JSONML_SYNTAX_TAG_NOT_STRING"
	CodeJSONMLSyntaxInvalidContent = "JSONML_SYNTAX_INVALID_CONTENT"
	CodeJSONMLSyntaxUnexpectedEOF  = "JSONML_SYNTAX_UNEXPECTED_EOF"
	CodeJSONMLSyntaxMaxDepth       = "JSONML_SYNTAX_MAX_DEPTH"
)

// Layer 2: JSONML Schema errors (semantic violations).
const (
	CodeJSONMLSchemaUnknownTag      = "JSONML_SCHEMA_UNKNOWN_TAG"
	CodeJSONMLSchemaUnknownAttr     = "JSONML_SCHEMA_UNKNOWN_ATTR"
	CodeJSONMLSchemaAttrAlias       = "JSONML_SCHEMA_ATTR_ALIAS"
	CodeJSONMLSchemaTypeMismatch    = "JSONML_SCHEMA_TYPE_MISMATCH"
	CodeJSONMLSchemaRequiredMissing = "JSONML_SCHEMA_REQUIRED_MISSING"
	CodeJSONMLSchemaChildNotAllowed = "JSONML_SCHEMA_CHILD_NOT_ALLOWED"
	CodeJSONMLSchemaEnumInvalid     = "JSONML_SCHEMA_ENUM_INVALID"
	CodeJSONMLSchemaDeprecatedAttr  = "JSONML_SCHEMA_DEPRECATED_ATTR"
	CodeJSONMLSchemaNumberRange     = "JSONML_SCHEMA_NUMBER_RANGE"
)

// Layer 2: CustomValidator-specific codes.
const (
	CodeJSONMLSchemaTableMissingColsWidth  = "JSONML_SCHEMA_TABLE_MISSING_COLSWIDTH"
	CodeJSONMLSchemaTableColsWidthMismatch = "JSONML_SCHEMA_TABLE_COLSWIDTH_MISMATCH"
	CodeJSONMLSchemaListStartInvalid       = "JSONML_SCHEMA_LIST_START_INVALID"
	CodeJSONMLSchemaTocContentShape        = "JSONML_SCHEMA_TOC_CONTENT_SHAPE"
)
