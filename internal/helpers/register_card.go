// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"

func init() {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "card",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "构建、校验、预览、发送和增量更新有层次且交互可靠的 A2UI 卡片",
			UseWhen:      []string{"回复内容适合用结构化卡片表达，或需要发送、更新 A2UI 卡片时"},
			AvoidWhen:    []string{"普通文本或媒体消息使用 chat；卡片平台模板资产生命周期使用专门的 Card Platform CLI"},
		},
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-chat"},
			Documentation: []contract.HelpDocumentation{{
				Label: "A2UI Agent 卡片创作技术方案",
				URL:   "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/docs/a2ui-agent-card-authoring-technical-design.md",
			}},
		},
	})
	RegisterPublicNamed("card", func() Handler { return wukongHandler{name: "card", buildFn: newCardCommand} })
}
