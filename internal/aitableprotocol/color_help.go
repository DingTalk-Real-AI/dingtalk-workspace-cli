// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

// GanttColorHelp describes the public color input for CLI help and Schema.
const GanttColorHelp = "颜色使用 colorConfigs 规则数组，[] 清空。\n固定红色：[{\"type\":\"FILTER\",\"value\":[],\"color\":\"#FFFFFF\",\"bgColor\":\"\",\"bgColorKey\":\"red_5\"}]。\n跟随单选字段：[{\"type\":\"SINGLE_OPTION\",\"value\":\"fldStatus\"}]，使用已有选项颜色。\nFILTER 的 value 是视图筛选条件数组，[] 匹配全部；按规则顺序匹配。\n推荐键：red_5 红、orange_3 橙、yellow_3 黄、green_5 绿、teal_4 青、blue_5 蓝、purple_5 紫、grey_5 灰。\n背景键为 red/orange/yellow/lime/green/teal/lightblue/blue/purple/pink/grey 加 _0 至 _6，0 最浅、6 最浓，实际色值跟随主题。\n精确固定颜色可填 bgColor=\"#FFF1F0\"，bgColorKey 留空。bgColorKey 不接受 HEX、条件填色语义名或数字字符串。"

// AppThemeHelp describes the public color input for CLI help and Schema.
const AppThemeHelp = "可选，主题名称：default=默认蓝，default-indigo=灰靛蓝，normal-blue=商务蓝，normal-grey=黑白色，default-teal=青石色，default-purple=神秘紫，default-green=清新绿，default-orange=明亮橙，default-red=渐变红，default-pink=可爱粉。推荐传完整名称，例如 themeType=\"normal-blue\"、appearance=\"light\"；不传保持不变。"

// AppNavThemeHelp describes the public color input for CLI help and Schema.
const AppNavThemeHelp = "可选，导航主题变体 default/normal，供简写主题名称组合查找；完整 themeType 优先。不是明暗模式开关，明暗由 appearance 控制；不传保持不变。"

// AppAppearanceHelp describes the public color input for CLI help and Schema.
const AppAppearanceHelp = "可选，推荐外观模式：light=浅色，dark=深色，auto=跟随系统；其他字符串保持原样交由服务端处理，不传保持不变。"

// AppBackgroundHelp describes the public color input for CLI help and Schema.
const AppBackgroundHelp = "页面背景 JSON 对象，type=color|image。颜色示例 {\"type\":\"color\",\"value\":\"#FFF1F0\",\"image\":\"\"}；value 为 CSS 颜色，支持 HEX/rgb()/rgba()，空值跟随主题。创建省略使用主题；更新省略保持原值，提供时整体替换。恢复主题用 {\"type\":\"color\",\"value\":\"\",\"image\":\"\"}。图片 URL 独立放 image；不使用条件填色语义名。"
