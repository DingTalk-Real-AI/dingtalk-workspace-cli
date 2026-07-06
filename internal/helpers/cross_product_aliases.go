package helpers

import "github.com/spf13/cobra"

// ──────────────────────────────────────────────────────────
// Cross-product flag alias registration
//
// doc/drive/wiki/sheet 四个产品对相同语义的参数使用了不同的 flag 名称，
// 导致大模型跨产品操作时容易混淆参数名。本文件集中维护「语义 → 等价参数名」
// 的映射关系，新命令只需调用一次 RegisterCrossProductAliases 即可自动补全
// 跨产品 hidden alias。
//
// ── 新增 alias 的准入标准 ──
//   1. API 字段驱动：alias 来源于 API 返回字段名的 camelCase → kebab-case
//      转换（如 workspaceId → --workspace-id），或跨产品同语义不同命名。
//   2. 真实幻觉验证：需有 LLM trace 或 eval case 证明大模型确实会猜错。
//   3. 不扩散无关名称：仅注册语义完全等价的名称，不要把近义词都加进来。
//
// ── 使用方式 ──
//   1. 在本文件 crossProductAliases 表中新增一行 {semantic, names}。
//   2. 在读取该 flag 值的代码中使用 flagOrFallback(cmd, primary, fallbacks...)。
//   3. 在 cross_product_aliases_test.go 中补充对应的单测。
//   4. 现有命令已在各产品 .go 文件末尾批量调用 RegisterCrossProductAliases,
//      无需手动给每个子命令注册 hidden flag。
// ──────────────────────────────────────────────────────────

// crossProductAliasGroup defines a set of equivalent flag names across products.
type crossProductAliasGroup struct {
	semantic string   // human-readable description (for debug/logging)
	names    []string // equivalent flag names within this group
}

// crossProductAliases is the global registry of equivalent parameter names.
// To add a new equivalence, just append here — all commands that call
// RegisterCrossProductAliases will automatically pick it up.
var crossProductAliases = []crossProductAliasGroup{
	{semantic: "节点标识", names: []string{"node", "file-id", "node-id", "doc-id"}},
	{semantic: "父文件夹", names: []string{"folder", "parent-id", "parent-folder", "parent-node-id", "parent-folder-id"}},
	{semantic: "知识库", names: []string{"workspace", "workspace-id"}},
	{semantic: "搜索关键词", names: []string{"query", "keyword"}},
	{semantic: "单页大小", names: []string{"limit", "page-size"}},
	{semantic: "续页标识", names: []string{"cursor", "page-token", "next-token"}},
	{semantic: "描述", names: []string{"desc", "description"}},
	{semantic: "用户单数", names: []string{"user", "user-id", "uid"}},
	{semantic: "用户复数", names: []string{"users", "user-ids", "user-list", "user-id-list"}},
	{semantic: "本地文件路径", names: []string{"file", "file-path"}},
	{semantic: "消息正文", names: []string{"content", "markdown"}},
	{semantic: "内容文件路径", names: []string{"content-file", "content-path"}},
}

// RegisterCrossProductAliases automatically registers hidden alias flags for
// cross-product compatibility. Call this once after all primary flags are registered.
//
// For each alias group, it finds the first flag already registered on cmd (the
// "primary"), then registers all other names in the group as hidden String flags.
// Existing flags are never overwritten.
func RegisterCrossProductAliases(cmd *cobra.Command) {
	for _, group := range crossProductAliases {
		// Find the primary: first name in the group that already exists on cmd.
		var primaryName string
		for _, name := range group.names {
			if cmd.Flags().Lookup(name) != nil {
				primaryName = name
				break
			}
		}
		if primaryName == "" {
			continue // this command doesn't use any flag in this semantic group
		}
		// Register remaining names as hidden aliases.
		for _, name := range group.names {
			if name == primaryName {
				continue
			}
			if cmd.Flags().Lookup(name) != nil {
				continue // already exists (manually registered or another primary)
			}
			cmd.Flags().String(name, "", "")
			_ = cmd.Flags().MarkHidden(name)
		}
	}
}
