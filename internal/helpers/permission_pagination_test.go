package helpers

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// P2 评审要求（PR #1085）：三个游标列表命令（drive/doc permission list、
// wiki member list）必须声明 Contract.Pagination（cursor 分页，游标 flag 为
// next-token），且 next-token 是该叶的 declared parameter。
//
// wire 上的 pagination 投影由 output rollout 门控制（schema_runtime_registry
// 对未迁移统一结果 envelope 的命令不发布 result/pagination，避免 Schema 与
// runtime 输出不一致）。本测试固化「声明已就绪」与「当前仍为 legacy 输出」
// 两个事实；命令迁移 unified envelope 后 wire 投影自动生效，届时应更新此
// 断言并用 dws schema 验证最终投影。
// 命名携带 TestCrossPlatformCoverage 前缀：本测试覆盖本次改动的
// Contract.Pagination 声明行，需在 macOS/Windows 平台覆盖率门禁上执行。
func TestCrossPlatformCoveragePermissionListPaginationDeclaration(t *testing.T) {
	cases := []struct {
		name string
		root *cobra.Command
		path []string
	}{
		{"drive permission list", newDriveCommand(), []string{"permission", "list"}},
		{"doc permission list", newDocCommand(), []string{"permission", "list"}},
		{"wiki member list", newWikiCommand(), []string{"member", "list"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, err := tc.root.Find(tc.path)
			if err != nil || cmd == nil || cmd.Name() != tc.path[len(tc.path)-1] {
				t.Fatalf("find leaf %v: cmd=%v err=%v", tc.path, cmd, err)
			}
			final, ok := contractfinal.RuntimeContractFinal(cmd)
			if !ok {
				t.Fatalf("missing RuntimeContractFinal on %v", tc.path)
			}
			pg := final.Pagination
			if pg == nil {
				t.Fatalf("Contract.Pagination not declared on %v", tc.path)
			}
			if pg.Kind != contract.PaginationKindCursor || pg.CursorParameter != "next-token" {
				t.Fatalf("pagination = %+v, want cursor/next-token", pg)
			}
			if pg.MetaPath != contract.PaginationMetaPath ||
				pg.NextTokenPath != contract.PaginationNextTokenPath ||
				pg.EndpointExhaustedPath != contract.PaginationExhaustedPath {
				t.Fatalf("pagination framework-owned paths not normalized: %+v", pg)
			}
			declared := false
			for _, p := range final.Parameters {
				if p.Name == "next-token" {
					declared = true
					break
				}
			}
			if !declared {
				t.Fatalf("cursor flag next-token must be a declared parameter on %v", tc.path)
			}
			if output.ActiveContract(cmd) != output.ContractLegacy {
				t.Fatalf("%v migrated to unified result; update this test and verify wire projection via dws schema", tc.path)
			}
		})
	}
}
