// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDingReferencePublishesStableFailureAndCompletionRules(t *testing.T) {
	for _, path := range []string{
		"../../skills/mono/references/products/ding.md",
		"../../skills/multi/dingtalk-misc/references/ding.md",
	} {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			assertDingReferenceContract(t, string(data))
		})
	}
}

func assertDingReferenceContract(t *testing.T, content string) {
	t.Helper()
	for _, required := range []string{
		"本页是 DING 唯一默认 Reference",
		"业务失败不是命令漂移",
		"## 机器人写前门禁",
		"robot_credentials_missing",
		"robot_not_in_org",
		"用户指定某个机器人显示名但没给",
		"显示名不是 code",
		"不能用环境中的默认机器人替代",
		"没有指定具体机器人时，省略 `--robot-code` 并执行一次",
		"Agent 不读取、打印或探测该变量",
		"`dws auth login`",
		"也不再发第二次请求",
		"禁止切换为个人身份、其他机器人或其他通道",
		"不枚举候选 robot-code",
		"不使用另一个机器人的凭据补偿本次失败",
		"## 完成条件",
		"同一 `openDingId` 的精确接收状态",
		"同一主体的 DING recall 返回成功终态",
		"明确分页终止证据",
		"实际主体、实际通道、费用属性",
		"## 调用与上下文预算",
		"同一逻辑请求的 send、status、recall 各最多一次",
		"正文、收件人、时间等用户要求字段必须原样保留",
		"投影只减少重复上下文，不删除业务信息",
		"不得为恢复已丢字段重发远端请求",
		"list 项已含 `content`、`openDingId` 和状态",
		"## 消息转 DING 的资源生命周期",
		"`sourceMessageId`",
		"`sourceConversationId`",
		"`openDingId`",
		"不透明标识",
		"不依据 `msg` / `cid` 前缀",
		"裸 `--id` 的资源有效性由服务端校验",
		"## 临时群消息转 DING 的有界状态链",
		"禁止在 Chat 内再次按",
		"dws chat +chat-create",
		"dws chat +messages-send --as user",
		"`sendReceipt`",
		"`messageRef.openMessageId/openConversationId`",
		"dws chat +messages-query-send-status --open-task-id",
		"至多一次发送状态",
		"不执行 `+chat-messages`",
		"必须等于建群回执的",
		"dws chat +chat-dismiss --group <OPEN_CONVERSATION_ID>",
		"不能用减少上下文掩盖部分成功",
		"dws ding message send-personal",
		"dws ding message send-by-message",
		"dws ding message recall-personal",
		"openDingTalkId",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("DING reference missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"ding-intent-guide.md",
		"ding-lite-recipes.md",
		"## 命令总览",
		"CLI 会拒绝 `msg...` / `cid...`",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("DING reference retains redundant content %q", forbidden)
		}
	}
	for _, fence := range bashFences(content) {
		for _, forbidden := range []string{
			"dws auth login", "dws dev ", "dws devapp ", "dws profile ", "dws chat +chat-bots",
			"dws chat +messages-recall", "--verbose", "env |", "echo $DINGTALK_DING_ROBOT_CODE",
			"--robot-code <ROBOT_CODE>",
		} {
			if strings.Contains(fence, forbidden) {
				t.Errorf("DING stored route contains forbidden discovery/replay %q:\n%s", forbidden, fence)
			}
		}
	}
}

func TestDingReferencesKeepTheSameContractAcrossSkillSurfaces(t *testing.T) {
	monoPath := "../../skills/mono/references/products/ding.md"
	mono, err := os.ReadFile(monoPath)
	if err != nil {
		t.Fatal(err)
	}
	multi, err := os.ReadFile("../../skills/multi/dingtalk-misc/references/ding.md")
	if err != nil {
		t.Fatal(err)
	}
	// Only packaging/navigation differs. The generated public-shortcut block
	// belongs to multi; all authored command, safety and completion rules match.
	common, _, _ := strings.Cut(string(multi), "<!-- VISIBLE_SHORTCUTS_START -->")
	normalized := strings.NewReplacer(
		"完成 shared 前置读取后", "完成根 Skill 前置要求后",
		"| `dingtalk-chat` | Chat 拥有群资源", "| [chat.md](./chat.md) | Chat 拥有群资源",
		"这类跨产品任务只为群生命周期加载 `dingtalk-chat` 的 `group-admin.md`；不加载聊天消息\n查询、搜索或消息动作 Reference，除非用户另外要求操作源聊天消息。人员只使用\n`dingtalk-aisearch` 根 Skill 的 person 入口，不加载其二级 Reference；按“身份与稳定 ID”",
		"这类跨产品任务只按需读取 [Chat Reference](./chat.md) 的群生命周期章节；不加载聊天消息\n查询、搜索或消息动作章节，除非用户另外要求操作源聊天消息。人员只使用本页已给出的\nperson 入口；其他搜人意图才读取 [AISearch Reference](./aisearch.md)。按“身份与稳定 ID”",
	).Replace(common)
	if strings.TrimSpace(normalized) != strings.TrimSpace(string(mono)) {
		t.Fatal("mono/multi DING authored contracts diverged beyond packaging/navigation")
	}
	for _, target := range []string{"chat.md", "aisearch.md"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(monoPath), target)); err != nil {
			t.Fatalf("mono navigation target %s: %v", target, err)
		}
	}
	for _, multiOnly := range []string{"dingtalk-chat", "dingtalk-aisearch", "group-admin.md"} {
		if strings.Contains(string(mono), multiOnly) {
			t.Errorf("mono depends on unavailable multi-only navigation %q", multiOnly)
		}
	}
}
