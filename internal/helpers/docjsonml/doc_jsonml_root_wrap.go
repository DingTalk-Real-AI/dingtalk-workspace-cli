package docjsonml

// ──────────────────────────────────────────────────────────
// JSONML body root wrapping (protocol-level coercion)
//
// 服务端 writeAsJsonML 路径要求 body 形态为
//
//	["root", {attrs?}, ...blockNodes]
//
// 即「单棵以 root 为顶点的树」。早先 dws-wukong 文档/校验器注释里曾声称「裸
// block 数组也是服务端可接受的形态」，但 cross-stack 验证（we-word-open-api
// jsonmlNodes.ts 的 getChildStart 启发式 + packSerializer.deserialize 反向
// 期望）确认：bare-array 形态会触发节点静默丢失或反序列化失败。
//
// 本函数仅做最薄的协议层修整，与 NormalizeJsonMLBody 关注点分离：
//
//	NormalizeJsonMLBody  — 单 block 内部修复（uuid 注入、文本包裹、…）
//	EnsureRootWrappedBody — body 整体形态修整（缺 root 时补包裹）
//
// 这样既保留 normalize 测试矩阵（仍以 [block, ...] 形态作为基线），又能保证
// 落地协议层符合服务端约束。
// ──────────────────────────────────────────────────────────

// EnsureRootWrappedBody returns body wrapped as ["root", {}, ...body] when
// the input is not already root-rooted. Returns the input unchanged when:
//
//   - body is empty
//   - body[0] is the literal string "root"
//
// notes is non-empty only when wrapping was applied.
//
// The function does not mutate its input.
func EnsureRootWrappedBody(body []any) (wrapped []any, notes []string) {
	if len(body) == 0 {
		return body, nil
	}
	if tag, ok := body[0].(string); ok && tag == "root" {
		return body, nil
	}
	out := make([]any, 0, len(body)+2)
	out = append(out, "root", map[string]any{})
	out = append(out, body...)
	return out, []string{`$: wrap bare body with ["root", {}, ...] to satisfy server writeAsJsonML contract`}
}
