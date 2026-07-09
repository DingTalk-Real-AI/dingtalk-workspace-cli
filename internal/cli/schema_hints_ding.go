// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

func init() {
	RegisterSchemaHints("ding", map[string]ToolSchemaHint{
		"send_ding_message": {
			Description: "发送 DING 消息给指定用户，支持配置机器人编码、接收人、提醒类型和消息内容。",
			Parameters: map[string]ParameterSchemaHint{
				"content": {
					Description: "DING 消息内容。",
					Required:    boolPtr(true),
				},
				"robotCode": {
					FlagName:    "robot-code",
					Description: "DING 机器人编码。通常来自开放平台或应用机器人配置。",
					Required:    boolPtr(true),
				},
				"receiverUserIdList": {
					FlagName:    "users",
					Type:        "array",
					Description: "接收人的 userId 列表。CLI 使用逗号分隔，例如 user1,user2。",
					Required:    boolPtr(true),
				},
				"remindType": {
					FlagName:    "type",
					Description: "提醒类型。CLI 默认使用 app，对应应用内提醒。",
					Default:     "app",
				},
			},
		},
	})
}
