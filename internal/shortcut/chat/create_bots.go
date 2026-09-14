// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"fmt"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func addCreatedGroupBots(rt *shortcut.RuntimeContext, data map[string]any, bots []string) error {
	cidValue := replyResponseValue(data, "openConversationId", "openCid")
	cid, ok := cidValue.(string)
	if !ok || cid == "" {
		if err := rt.Output(map[string]any{"createdGroup": data, "botsAdded": false, "reason": "created_conversation_id_missing"}); err != nil {
			return err
		}
		return apperrors.NewAPI("建群已返回，但缺少稳定会话 ID；未添加机器人，请先定位已建群，勿重复建群", apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false))
	}
	items := make([]map[string]any, 0, len(bots))
	failed := 0
	for _, bot := range bots {
		result, err := rt.CallMCPWriteData("bot", "add_robot_to_group", map[string]any{"openConversationId": cid, "robotCode": bot})
		item := map[string]any{"robotCode": bot, "success": err == nil}
		if err != nil {
			failed++
			item["error"] = err.Error()
		} else {
			item["result"] = result
		}
		items = append(items, item)
	}
	data["openConversationId"] = cid
	data["botResults"] = items
	data["failedBotCount"] = failed
	data["atomic"] = false
	if failed > 0 {
		data["recovery"] = "仅对失败机器人执行 dws chat +chat-add-bot --id " + cid + " --robot-code <failedRobotCode>；不要重新建群"
	}
	if err := rt.Output(data); err != nil {
		return err
	}
	if failed > 0 {
		return apperrors.NewAPI(fmt.Sprintf("群已创建，%d 个机器人未加入", failed), apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false), apperrors.WithReason("initial_bots_partial_failure"))
	}
	return nil
}
