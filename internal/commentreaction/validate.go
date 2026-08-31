// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package commentreaction validates the reviewed DingTalk names accepted by
// comment emoji replies. The names mirror the bundled default emoji catalog;
// arbitrary text and raw Unicode emoji must never be persisted as reactions.
package commentreaction

import (
	"strings"
	"unicode/utf8"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Validate rejects values outside DingTalk's reviewed default reaction-name
// catalog. Names are case-sensitive because they are sent verbatim to the
// doc-comment service.
func Validate(value string) error {
	name := strings.TrimSpace(value)
	if value != name || !utf8.ValidString(name) || !supported(name) {
		return apperrors.NewValidation(
			"--reaction/--content 必须是受支持的钉钉表情名称（如 憨笑、鼓掌、比心、赞），不要传 Unicode Emoji 或任意文本",
			apperrors.WithReason("unsupported_comment_reaction"),
		)
	}
	return nil
}

func supported(name string) bool {
	switch name {
	case "微笑", "可爱", "憨笑", "色", "发呆", "老板", "傻笑", "流泪", "害羞", "闭嘴",
		"睡", "大哭", "尴尬", "感谢", "拒绝", "赞", "鼓掌", "打招呼", "666", "抱拳",
		"握手", "OK", "胜利", "向左", "向右", "向上", "向下", "来呀", "一点点", "捏住",
		"比心", "送花花", "加油干", "调皮", "大笑", "惊讶", "流汗", "奋斗", "口罩", "生病",
		"吐", "难过", "抓狂", "右哼哼", "太阳", "月亮", "强", "弱", "彩带", "蛋糕",
		"骷髅", "撇嘴", "鄙视", "嘘", "思考", "亲亲", "无奈", "感冒", "对不起", "再见",
		"投降", "哼", "欠扁", "拜托", "可怜", "舒服", "爱意", "财迷", "迷惑", "委屈",
		"灵感", "天使", "鬼脸", "凄凉", "郁闷", "坏笑", "算账", "PK", "忍者", "衰",
		"炸弹", "笑哭", "嘿嘿", "捂脸哭", "抠鼻", "流鼻血", "呲牙", "吃瓜", "彩虹", "耶",
		"发怒", "捂眼睛", "推眼镜", "暗中观察", "脑暴", "冷笑", "热", "开心", "惊喜", "回头",
		"白眼", "一团乱麻", "黑眼圈", "裂开", "恭喜", "费解", "收到", "快来", "敲打", "捧脸",
		"Get", "客服", "AR", "小蜜蜂", "虎虎生威", "兔飞猛进", "龙头老大", "蛇来运转", "马上来财", "专注",
		"忙疯了", "等一等", "一脸苦笑", "王之蔑视", "洪荒之力", "向左看", "向右看", "YYDS", "这边请", "弹射下班",
		"退退退", "在吗", "让人头大", "摊手", "抱抱", "举手", "开车", "抱大腿", "跪了", "鞠躬",
		"选我", "元气满满", "会议", "猫咪", "二哈", "狗子", "三多", "承让", "撒花", "礼物",
		"生日快乐", "爱心", "心碎", "嘴唇", "鲜花", "残花", "干杯", "咖啡", "奶茶", "茶",
		"OKR", "KPI", "100分", "对勾", "打叉", "气泡", "加一", "Done", "钉子", "出差",
		"高铁", "火箭", "邮件", "文档", "演示", "表格", "废纸篓", "手机", "时间", "静音",
		"公文包", "地球", "碳减排", "回收标志", "幼苗", "红包", "锦鲤", "福", "灯笼", "爆竹",
		"烟花", "恭喜发财", "月饼", "鸡腿", "休假", "火", "点赞", "平安健康", "定胜":
		return true
	default:
		return false
	}
}
