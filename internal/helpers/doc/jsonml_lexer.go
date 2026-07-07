//go:build goexperiment.jsonv2

package doc

import (
	"bytes"
	"encoding/json/jsontext"
	"io"
	"sort"
)

// ──────────────────────────────────────────────────────────────────────────────
// TokenKind — JSON token 类型。
// ──────────────────────────────────────────────────────────────────────────────

// TokenKind 表示 JSON token 类型。
type TokenKind int

const (
	TokenObjectStart TokenKind = iota // {
	TokenObjectEnd                    // }
	TokenArrayStart                   // [
	TokenArrayEnd                     // ]
	TokenString                       // "..."
	TokenNumber                       // 42, 3.14
	TokenTrue                         // true
	TokenFalse                        // false
	TokenNull                         // null
	TokenEOF                          // 输入结束
)

func (k TokenKind) String() string {
	switch k {
	case TokenObjectStart:
		return "{"
	case TokenObjectEnd:
		return "}"
	case TokenArrayStart:
		return "["
	case TokenArrayEnd:
		return "]"
	case TokenString:
		return "string"
	case TokenNumber:
		return "number"
	case TokenTrue:
		return "true"
	case TokenFalse:
		return "false"
	case TokenNull:
		return "null"
	case TokenEOF:
		return "EOF"
	default:
		return "unknown"
	}
}

// Token 表示一个带位置的 JSON token。
type Token struct {
	Kind  TokenKind
	Value []byte // 原始字节（string 含引号, number 含原始文本）
	Range Range  // 精确源位置
}

// StringValue 返回解码后的 string 值。仅 TokenString 有效。
// 对于非 string token 返回空串。
func (t Token) StringValue() string {
	if t.Kind != TokenString || len(t.Value) < 2 {
		return ""
	}
	// Value 含引号，去掉首尾 "
	raw := t.Value
	// 简单路径：如果没有转义符，直接截取
	hasEscape := false
	for _, b := range raw[1 : len(raw)-1] {
		if b == '\\' {
			hasEscape = true
			break
		}
	}
	if !hasEscape {
		return string(raw[1 : len(raw)-1])
	}
	// 有转义符：使用 jsontext.AppendUnquote
	unquoted, err := jsontext.AppendUnquote(nil, raw)
	if err != nil {
		return string(raw[1 : len(raw)-1])
	}
	return string(unquoted)
}

// ──────────────────────────────────────────────────────────────────────────────
// Lexer — 封装 jsontext.Decoder，提供位置感知的 token 流。
// ──────────────────────────────────────────────────────────────────────────────

// Lexer 封装 jsontext.Decoder，提供位置感知的 token 流。
type Lexer struct {
	dec         *jsontext.Decoder
	src         []byte
	lineOffsets []int64 // 每行起始 byte offset (lineOffsets[0] = 0 代表第 1 行)
	diags       DiagnosticList
	done        bool // EOF 已到达
}

// NewLexer 创建 Lexer 实例。预扫描换行符以建立行号索引。
func NewLexer(src []byte) *Lexer {
	l := &Lexer{
		dec:         jsontext.NewDecoder(bytes.NewReader(src)),
		src:         src,
		lineOffsets: buildLineOffsets(src),
	}
	return l
}

// buildLineOffsets 扫描 src 中的 '\n'，返回每行的起始 byte offset。
// lineOffsets[0] = 0（第 1 行从 offset 0 开始）。
func buildLineOffsets(src []byte) []int64 {
	offsets := []int64{0}
	for i, b := range src {
		if b == '\n' {
			offsets = append(offsets, int64(i+1))
		}
	}
	return offsets
}

// offsetToPosition 将 byte offset 转换为 Position (line/col 均 1-based)。
func (l *Lexer) offsetToPosition(offset int64) Position {
	// 二分搜索：找到最后一个 <= offset 的 lineOffset 索引
	line := sort.Search(len(l.lineOffsets), func(i int) bool {
		return l.lineOffsets[i] > offset
	})
	// line 是第一个 > offset 的索引，所以实际行号是 line（1-based）
	col := int(offset-l.lineOffsets[line-1]) + 1
	return Position{Line: line, Col: col, Offset: offset}
}

// Next 返回下一个 Token。
// 遇到 JSON 词法错误时记录 Diagnostic 并返回 TokenEOF。
func (l *Lexer) Next() (Token, error) {
	if l.done {
		return Token{Kind: TokenEOF, Range: l.eofRange()}, nil
	}

	beforeOffset := l.dec.InputOffset()

	tok, err := l.dec.ReadToken()
	if err != nil {
		l.done = true
		if err == io.EOF {
			return Token{Kind: TokenEOF, Range: l.eofRange()}, nil
		}
		// JSON 词法错误
		pos := l.offsetToPosition(beforeOffset)
		l.diags = append(l.diags, Diagnostic{
			Range:    Range{Start: pos, End: pos},
			Severity: SeverityError,
			Code:     classifyJsontextError(err),
			Source:   SourceJSONSyntax,
			Message:  err.Error(),
		})
		return Token{Kind: TokenEOF, Range: Range{Start: pos, End: pos}}, err
	}

	afterOffset := l.dec.InputOffset()

	// 跳过 beforeOffset 到实际 token 之间的空白，得到精确 token 起始位置
	tokenStart := skipJSONWhitespace(l.src, beforeOffset)
	tokenRange := Range{
		Start: l.offsetToPosition(tokenStart),
		End:   l.offsetToPosition(afterOffset),
	}

	kind := mapJsontextKind(tok.Kind())
	var value []byte
	if tokenStart < afterOffset {
		value = cloneBytes(l.src, tokenStart, afterOffset)
	}

	return Token{Kind: kind, Value: value, Range: tokenRange}, nil
}

// Diagnostics 返回 Layer 0 收集的所有诊断。
func (l *Lexer) Diagnostics() DiagnosticList {
	return l.diags
}

// eofRange 返回源文本末尾的 Range。
func (l *Lexer) eofRange() Range {
	pos := l.offsetToPosition(int64(len(l.src)))
	return Range{Start: pos, End: pos}
}

// ──────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────────────────────────────────────

// mapJsontextKind 将 jsontext.Kind 映射到 TokenKind。
func mapJsontextKind(k jsontext.Kind) TokenKind {
	switch k {
	case '{':
		return TokenObjectStart
	case '}':
		return TokenObjectEnd
	case '[':
		return TokenArrayStart
	case ']':
		return TokenArrayEnd
	case '"':
		return TokenString
	case '0':
		return TokenNumber
	case 't':
		return TokenTrue
	case 'f':
		return TokenFalse
	case 'n':
		return TokenNull
	default:
		return TokenEOF
	}
}

// classifyJsontextError 尝试从错误信息中推断错误码。
func classifyJsontextError(err error) string {
	msg := err.Error()
	switch {
	case contains(msg, "unterminated string"):
		return CodeJSONSyntaxUnterminatedStr
	case contains(msg, "escape"):
		return CodeJSONSyntaxInvalidEscape
	case contains(msg, "trailing"):
		return CodeJSONSyntaxTrailingData
	default:
		return CodeJSONSyntaxUnexpectedToken
	}
}

// skipJSONWhitespace 从 offset 开始跳过 JSON 空白字符 (space/tab/LF/CR) 和结构分隔符 (,/:)。
// jsontext.Decoder 内部消费逗号和冒号但不作为 token 返回，
// 所以 InputOffset() 可能停在上一个 token 后的逗号/冒号位置。
func skipJSONWhitespace(src []byte, offset int64) int64 {
	for offset < int64(len(src)) {
		b := src[offset]
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != ',' && b != ':' {
			break
		}
		offset++
	}
	return offset
}

// contains 是 strings.Contains 的内联版本（避免额外 import 开销对 hot path 的影响）。
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstr(s, substr)
}

func searchSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// cloneBytes 从 src 中复制 [start, end) 区间的字节。
func cloneBytes(src []byte, start, end int64) []byte {
	if start < 0 || end > int64(len(src)) || start >= end {
		return nil
	}
	b := make([]byte, end-start)
	copy(b, src[start:end])
	return b
}
