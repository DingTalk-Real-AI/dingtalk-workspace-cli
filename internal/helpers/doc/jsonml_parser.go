//go:build goexperiment.jsonv2

package doc

import (
	"encoding/json"
	"fmt"
)

// MaxDepth 是 JSONML 节点嵌套的最大深度。
const MaxDepth = 64

// ──────────────────────────────────────────────────────────────────────────────
// AST 模型
// ──────────────────────────────────────────────────────────────────────────────

// JsonMLDocument 表示一次 Parse 的完整结果。
type JsonMLDocument struct {
	Nodes       []*JsonMLNode  // 顶层节点列表
	Source      []byte         // 原始输入
	Diagnostics DiagnosticList // Layer 0 + Layer 1 诊断
}

// ToAnySlice 将顶层节点列表转为 []any（body 形态），兼容 ValidateJsonMLBodyV2。
func (doc *JsonMLDocument) ToAnySlice() []any {
	var result []any
	for _, n := range doc.Nodes {
		if a := n.ToAny(); a != nil {
			result = append(result, a)
		}
	}
	return result
}

// JsonMLNode 表示 AST 中的一个节点。
type JsonMLNode struct {
	Tag      string         // tag 名称（IsText 时为空）
	Attrs    map[string]any // 属性 map（无 attrs 时为 nil）
	Children []*JsonMLNode  // 子节点列表
	Text     string         // 文本内容（仅 IsText == true 时有值）

	IsText  bool // true = 文本节点
	IsError bool // true = 错误占位节点 (error recovery 产生)

	Range    Range // 整个节点的源位置
	TagRange Range // tag string 的源位置
}

// ToAny 将 AST 节点转回 []any/string 形式。IsError 节点返回 nil。
func (n *JsonMLNode) ToAny() any {
	if n == nil || n.IsError {
		return nil
	}
	if n.IsText {
		return n.Text
	}
	arr := []any{n.Tag}
	if n.Attrs != nil {
		arr = append(arr, n.Attrs)
	}
	for _, child := range n.Children {
		if a := child.ToAny(); a != nil {
			arr = append(arr, a)
		}
	}
	return arr
}

// Walk 深度优先遍历。fn 返回 false 时跳过子树。
func (n *JsonMLNode) Walk(fn func(node *JsonMLNode, depth int) bool) {
	walkNode(n, 0, fn)
}

func walkNode(n *JsonMLNode, depth int, fn func(*JsonMLNode, int) bool) {
	if n == nil {
		return
	}
	if !fn(n, depth) {
		return
	}
	for _, child := range n.Children {
		walkNode(child, depth+1, fn)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Parser
// ──────────────────────────────────────────────────────────────────────────────

// parser 内部状态。
type parser struct {
	lexer   *Lexer
	current Token // 当前 token（peek 用）
	peeked  bool  // 是否有 peeked token
	diags   DiagnosticList
}

// Parse 解析 JSONML 源文本，返回 AST + 诊断（Layer 0 + Layer 1）。
func Parse(src []byte) *JsonMLDocument {
	lexer := NewLexer(src)
	p := &parser{lexer: lexer}

	doc := &JsonMLDocument{Source: src}

	tok := p.peek()
	switch tok.Kind {
	case TokenEOF:
		// 空输入，不报错
	case TokenArrayStart:
		// 判断是单个 element 还是 element-array (body)
		if p.isElementStart() {
			node := p.parseElement(0)
			doc.Nodes = append(doc.Nodes, node)
		} else {
			// body 形态: [[elem1], [elem2], ...]
			p.advance() // consume outer [
			for p.peek().Kind != TokenArrayEnd && p.peek().Kind != TokenEOF {
				node := p.parseElement(0)
				doc.Nodes = append(doc.Nodes, node)
			}
			if p.peek().Kind == TokenArrayEnd {
				p.advance() // consume outer ]
			} else {
				p.emit(Diagnostic{
					Range:    p.peek().Range,
					Severity: SeverityError,
					Code:     CodeJSONMLSyntaxUnexpectedEOF,
					Source:   SourceJSONMLGrammar,
					Message:  "unexpected end of input while parsing body array",
				})
			}
		}
	default:
		// 非数组输入
		p.emit(Diagnostic{
			Range:    tok.Range,
			Severity: SeverityError,
			Code:     CodeJSONMLSyntaxNotArray,
			Source:   SourceJSONMLGrammar,
			Message:  fmt.Sprintf("element must be a JSON array, got %s", tok.Kind),
		})
	}

	// 合并 Layer 0 + Layer 1 诊断
	doc.Diagnostics = append(lexer.Diagnostics(), p.diags...)
	return doc
}

// isElementStart 判断当前 '[' 开头的结构是否是 JSONML element。
// 启发式: 如果 '[' 后紧跟的是 '[' → body-array；其他情况→ element。
func (p *parser) isElementStart() bool {
	tok := p.peek()
	if tok.Kind != TokenArrayStart {
		return false
	}
	// 看 '[' 之后 src 中下一个非空白/分隔符 token 的首字符
	afterBracket := skipJSONWhitespace(p.lexer.src, tok.Range.End.Offset)
	if afterBracket >= int64(len(p.lexer.src)) {
		return true // truncated — 当作 element 处理（会报 EOF 错）
	}
	// 仅当下一个字符是 '[' 时，认为是 body-array [[elem], [elem], ...]
	// 其他情况（"、数字、bool、null、]、{）都当作 element 处理
	return p.lexer.src[afterBracket] != '['
}

// parseElement 解析一个 JSONML element: [tag, attrs?, child*]
func (p *parser) parseElement(depth int) *JsonMLNode {
	if depth >= MaxDepth {
		tok := p.peek()
		p.emit(Diagnostic{
			Range:    tok.Range,
			Severity: SeverityError,
			Code:     CodeJSONMLSyntaxMaxDepth,
			Source:   SourceJSONMLGrammar,
			Message:  fmt.Sprintf("element nesting depth exceeds maximum (%d)", MaxDepth),
		})
		p.skipToArrayEnd()
		return &JsonMLNode{IsError: true, Range: tok.Range}
	}

	// Expect '['
	tok := p.advance()
	if tok.Kind != TokenArrayStart {
		p.emit(Diagnostic{
			Range:    tok.Range,
			Severity: SeverityError,
			Code:     CodeJSONMLSyntaxNotArray,
			Source:   SourceJSONMLGrammar,
			Message:  fmt.Sprintf("element must be a JSON array, got %s", tok.Kind),
		})
		return &JsonMLNode{IsError: true, Range: tok.Range}
	}
	startRange := tok.Range

	// Check empty array
	if p.peek().Kind == TokenArrayEnd {
		endTok := p.advance()
		p.emit(Diagnostic{
			Range:    Range{Start: startRange.Start, End: endTok.Range.End},
			Severity: SeverityError,
			Code:     CodeJSONMLSyntaxEmptyElement,
			Source:   SourceJSONMLGrammar,
			Message:  "element array must not be empty",
		})
		return &JsonMLNode{IsError: true, Range: Range{Start: startRange.Start, End: endTok.Range.End}}
	}

	// Expect tag (string)
	tagTok := p.advance()
	if tagTok.Kind != TokenString {
		p.emit(Diagnostic{
			Range:    tagTok.Range,
			Severity: SeverityError,
			Code:     CodeJSONMLSyntaxTagNotString,
			Source:   SourceJSONMLGrammar,
			Message:  fmt.Sprintf("first element must be a string tag, got %s", tagTok.Kind),
		})
		p.skipToArrayEnd()
		return &JsonMLNode{IsError: true, Range: Range{Start: startRange.Start, End: p.lastRange().End}}
	}

	node := &JsonMLNode{
		Tag:      tagTok.StringValue(),
		TagRange: tagTok.Range,
	}

	// Optional attrs (object at position [1])
	if p.peek().Kind == TokenObjectStart {
		node.Attrs = p.parseAttrsObject()
	}

	// Children: string or array until ']'
	for p.peek().Kind != TokenArrayEnd && p.peek().Kind != TokenEOF {
		childTok := p.peek()
		switch childTok.Kind {
		case TokenArrayStart:
			child := p.parseElement(depth + 1)
			node.Children = append(node.Children, child)
		case TokenString:
			tok := p.advance()
			node.Children = append(node.Children, &JsonMLNode{
				IsText: true,
				Text:   tok.StringValue(),
				Range:  tok.Range,
			})
		default:
			// number, bool, null, object in content position → invalid
			tok := p.advance()
			idx := len(node.Children)
			p.emit(Diagnostic{
				Range:    tok.Range,
				Severity: SeverityError,
				Code:     CodeJSONMLSyntaxInvalidContent,
				Source:   SourceJSONMLGrammar,
				Message:  fmt.Sprintf("content at position [%d] must be string or element array, got %s", idx, tok.Kind),
			})
			// Recovery: skip this token, continue processing siblings
		}
	}

	// Expect ']'
	if p.peek().Kind == TokenArrayEnd {
		endTok := p.advance()
		node.Range = Range{Start: startRange.Start, End: endTok.Range.End}
	} else {
		// EOF
		p.emit(Diagnostic{
			Range:    p.peek().Range,
			Severity: SeverityError,
			Code:     CodeJSONMLSyntaxUnexpectedEOF,
			Source:   SourceJSONMLGrammar,
			Message:  "unexpected end of input while parsing element",
		})
		node.Range = Range{Start: startRange.Start, End: p.lastRange().End}
	}

	return node
}

// parseAttrsObject 解析 attrs 对象 { key: value, ... }。
// 消费 '{' 到 '}' 之间的所有 token，返回 map[string]any。
func (p *parser) parseAttrsObject() map[string]any {
	// 消费 '{'
	p.advance()
	attrs := make(map[string]any)

	for p.peek().Kind != TokenObjectEnd && p.peek().Kind != TokenEOF {
		// key
		keyTok := p.advance()
		if keyTok.Kind != TokenString {
			// 不应该发生（JSON 对象的 key 必须是 string），跳过
			continue
		}
		key := keyTok.StringValue()
		// value
		val := p.parseValue()
		attrs[key] = val
	}

	if p.peek().Kind == TokenObjectEnd {
		p.advance() // consume '}'
	}

	if len(attrs) == 0 {
		return nil // 空对象不保留
	}
	return attrs
}

// parseValue 解析一个 JSON value（递归，用于 attrs 内嵌结构）。
func (p *parser) parseValue() any {
	tok := p.advance()
	switch tok.Kind {
	case TokenString:
		return tok.StringValue()
	case TokenNumber:
		var n json.Number
		n = json.Number(string(tok.Value))
		// 尝试解析为 float64（保持和 json.Unmarshal 行为一致）
		if f, err := n.Float64(); err == nil {
			return f
		}
		return n
	case TokenTrue:
		return true
	case TokenFalse:
		return false
	case TokenNull:
		return nil
	case TokenArrayStart:
		var arr []any
		for p.peek().Kind != TokenArrayEnd && p.peek().Kind != TokenEOF {
			arr = append(arr, p.parseValue())
		}
		if p.peek().Kind == TokenArrayEnd {
			p.advance()
		}
		return arr
	case TokenObjectStart:
		obj := make(map[string]any)
		for p.peek().Kind != TokenObjectEnd && p.peek().Kind != TokenEOF {
			keyTok := p.advance()
			if keyTok.Kind != TokenString {
				continue
			}
			obj[keyTok.StringValue()] = p.parseValue()
		}
		if p.peek().Kind == TokenObjectEnd {
			p.advance()
		}
		return obj
	default:
		return nil
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Parser 辅助方法
// ──────────────────────────────────────────────────────────────────────────────

// peek 返回下一个 token 但不消费。
func (p *parser) peek() Token {
	if !p.peeked {
		tok, _ := p.lexer.Next()
		p.current = tok
		p.peeked = true
	}
	return p.current
}

// advance 消费当前 token 并返回。
func (p *parser) advance() Token {
	tok := p.peek()
	p.peeked = false
	return tok
}

// emit 记录一条诊断。
func (p *parser) emit(d Diagnostic) {
	p.diags = append(p.diags, d)
}

// lastRange 返回最近消费 token 的 Range（用于 error recovery 定位）。
func (p *parser) lastRange() Range {
	return p.current.Range
}

// skipToArrayEnd 跳过 token 直到遇到匹配的 ']' 或 EOF。
// 处理嵌套 [] 以正确匹配层级。
func (p *parser) skipToArrayEnd() {
	depth := 1
	for depth > 0 {
		tok := p.advance()
		switch tok.Kind {
		case TokenArrayStart:
			depth++
		case TokenArrayEnd:
			depth--
		case TokenEOF:
			return
		}
	}
}
