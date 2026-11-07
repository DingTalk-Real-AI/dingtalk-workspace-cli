// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

package helpers

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func mindCreateResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure, contract.ResultOutcomePending}, SensitivePaths: []string{"nodeId"}, DataSchema: json.RawMessage(`{
  "type": "object",
  "description": "创建脑图的返回结构；结合 CLI outcome 检查业务状态及确认字段。 dry-run 仅返回请求摘要，不表示操作完成。",
  "properties": {
    "requestId": {
      "type": "string",
      "description": "本次操作的幂等请求标识，与输入一致；重试或恢复时复用。",
      "title": "请求ID"
    },
    "nodeId": {
      "type": "string",
      "description": "脑图文件的真实节点 ID；用于后续查询和节点增删改请求的 nodeId，不是内部主题 ID。创建结果不确定且文件身份尚未知时可能不返回。",
      "title": "脑图ID"
    },
    "nodeUrl": {
      "type": "string",
      "description": "已创建脑图的访问链接，资源身份已知时返回。",
      "title": "脑图链接"
    },
    "name": {
      "type": "string",
      "description": "本次请求创建的脑图文件名称。",
      "title": "脑图名称"
    },
    "status": {
      "type": "string",
      "description": "创建状态：READY 表示初始内容已确认；INITIALIZING 表示初始化尚未完成；UNKNOWN 表示结果不确定。后两种状态使用相同 requestId 和参数恢复，避免重复创建。",
      "title": "操作状态"
    },
    "requestedContentApplied": {
      "type": "boolean",
      "description": "初始内容是否已经确认写入。仅 status=READY 且此字段为 true 时，可以确认携带内容创建完成。",
      "title": "初始内容是否已应用"
    },
    "idempotentReplay": {
      "type": "boolean",
      "description": "是否复用了相同 requestId 的已有创建结果。",
      "title": "是否为幂等重试"
    },
    "revision": {
      "type": "string",
      "description": "已经确认的脑图初始内容版本，使用十进制字符串表示；内容未确认时可能不返回。",
      "title": "初始内容版本"
    },
    "message": {
      "type": "string",
      "description": "创建成功的使用提示，幂等重放同样保留；不替代错误信息或后续读写权限校验。",
      "title": "使用提示"
    },
    "pageIdMap": {
      "type": "object",
      "description": "客户端临时页面 ID 到真实页面 ID 的映射。键来自 source.pages[].id，值可用于后续请求的 pageId。映射的键和值均为字符串。",
      "title": "页面ID映射"
    },
    "nodeIdMap": {
      "type": "object",
      "description": "客户端临时主题 ID 到真实主题 ID 的映射。值可用于后续请求的 topicId 或 parentTopicId；不是文件 nodeId。映射的键和值均为字符串。",
      "title": "主题ID映射"
    },
    "errorDetail": {
      "type": "object",
      "properties": {
        "stage": {
          "type": "string",
          "description": "操作未完成、结果未确认或冲突发生的处理阶段，用于定位失败环节。",
          "title": "处理阶段"
        },
        "code": {
          "type": "string",
          "description": "该处理阶段的诊断错误码；与顶层请求失败的 errorCode 分开使用。",
          "title": "诊断错误码"
        },
        "message": {
          "type": "string",
          "description": "该阶段的具体原因与诊断信息，已做敏感内容处理；结合 status 判断操作结果。",
          "title": "诊断信息"
        },
        "category": {
          "type": "string",
          "description": "有限业务错误类别：TIMEOUT、VALIDATION、DOWNSTREAM；不表示可自动重试，不返回内部异常类名。",
          "title": "错误类别"
        },
        "operationId": {
          "type": "string",
          "description": "关联操作标识，存在且可安全展示时返回，用于关联诊断；恢复操作仍应复用请求的 requestId。",
          "title": "关联操作ID"
        }
      },
      "description": "未决结果、提交未验证或冲突的阶段诊断信息；可能随业务结果返回，不能仅凭顶层 success 判断内容已完成。",
      "title": "操作诊断信息"
    },
    "executed": {
      "type": "boolean",
      "description": "dry-run 时固定为 false，表示没有调用远端工具"
    },
    "tool": {
      "type": "string",
      "description": "dry-run 预览的 MCP 工具 key"
    },
    "sourceBytes": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：UTF-8 字节数"
    },
    "pageCount": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：页面数"
    },
    "topicCount": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：主题数"
    }
  },
  "additionalProperties": true
}`)}
}

func mindQueryResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, SensitivePaths: []string{"nodeId"}, DataSchema: json.RawMessage(`{
  "type": "object",
  "description": "查询脑图节点的返回结构；结合 CLI outcome 检查业务状态及确认字段。 dry-run 仅返回请求摘要，不表示操作完成。",
  "properties": {
    "nodeId": {
      "type": "string",
      "description": "已解析的脑图文件节点 ID，与主题 ID 分开使用；用于后续请求的 nodeId。",
      "title": "脑图ID"
    },
    "schemaVersion": {
      "type": "string",
      "description": "返回主题内容的 OpenNodes 格式版本，当前为 1.0。",
      "title": "内容格式版本"
    },
    "catalogVersion": {
      "type": "string",
      "description": "返回主题内容的脑图类型版本，当前为 mind-v1。",
      "title": "脑图类型版本"
    },
    "readVersion": {
      "type": "string",
      "description": "本次查询或节点操作读取的固定内容版本，使用十进制字符串表示。可用于后续写入的 expectedVersion；查询续页保持同一读取版本。",
      "title": "读取版本"
    },
    "items": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string",
            "description": "脑图页面的真实 ID，用于后续请求的 pageId。",
            "title": "页面ID"
          },
          "title": {
            "type": "string",
            "description": "脑图页面标题，不是脑图文件名称。",
            "title": "页面标题"
          },
          "rootId": {
            "type": "string",
            "description": "本页面根主题的真实 ID。",
            "title": "根主题ID"
          },
          "nodes": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "id": {
                  "type": "string",
                  "description": "脑图主题的真实 ID，可作为后续请求的 topicId 或 parentTopicId；不是文件 nodeId，也不是创建输入的临时 ID。",
                  "title": "主题ID"
                },
                "type": {
                  "type": "string",
                  "description": "公开主题类型，当前返回 topic。",
                  "title": "主题类型"
                },
                "parentId": {
                  "type": "string",
                  "description": "父主题的真实 ID；根主题没有父主题时不返回。",
                  "title": "父主题ID"
                },
                "text": {
                  "type": "string",
                  "description": "主题纯文本，空字符串表示空内容；受保护主题不返回正文。",
                  "title": "主题文本"
                },
                "index": {
                  "type": "integer",
                  "description": "主题在父主题全部直接子节点中的位置，从 0 开始；不是当前分页结果中的下标。",
                  "title": "兄弟节点位置"
                },
                "childCount": {
                  "type": "integer",
                  "description": "直接子主题数量，不包含更深层后代；受保护主题不返回此字段。",
                  "title": "直接子主题数量"
                },
                "writeSupport": {
                  "type": "string",
                  "description": "主题写入支持程度：readWrite 表示可通过节点接口写入，readOnly 表示只读；只读原因见 unsupportedFeatures。根主题和受限主题的可执行操作仍由服务端校验。",
                  "title": "主题写入支持程度"
                },
                "unsupportedFeatures": {
                  "type": "array",
                  "items": {
                    "type": "string",
                    "title": "限制原因标识",
                    "description": "一个限制原因标识，例如 mind.rootMutation 或 mind.protected。"
                  },
                  "description": "导致主题只读或部分内容不公开的原因标识列表，例如 mind.rootMutation 表示根主题禁止修改，mind.protected 表示内容受保护。受保护主题不会返回 text 和 childCount。",
                  "title": "不支持写入的原因"
                }
              },
              "title": "主题信息",
              "description": "一个主题的公开内容及写入支持信息。"
            },
            "description": "本页在当前查询范围内返回的主题列表；按模型的主题顺序返回，可能只包含部分主题。",
            "title": "主题列表"
          }
        },
        "title": "页面信息",
        "description": "一个页面及本次查询返回的主题投影。"
      },
      "description": "本次查询返回的页面列表，每项包含页面标识、根主题标识和本页返回的主题内容。",
      "title": "页面列表"
    },
    "truncated": {
      "type": "boolean",
      "description": "当前主题投影是否因深度或单次结果数量限制而截断。true 不一定表示可续页，应同时检查 hasMore 和 nextToken。",
      "title": "结果是否被截断"
    },
    "executed": {
      "type": "boolean",
      "description": "dry-run 时固定为 false，表示没有调用远端工具"
    },
    "tool": {
      "type": "string",
      "description": "dry-run 预览的 MCP 工具 key"
    },
    "sourceBytes": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：UTF-8 字节数"
    },
    "pageCount": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：页面数"
    },
    "topicCount": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：主题数"
    }
  },
  "additionalProperties": true
}`)}
}

func mindMutationResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure, contract.ResultOutcomePending}, SensitivePaths: []string{"nodeId"}, DataSchema: json.RawMessage(`{
  "type": "object",
  "description": "创建脑图节点的返回结构；结合 CLI outcome 检查业务状态及确认字段。 dry-run 仅返回请求摘要，不表示操作完成。",
  "properties": {
    "requestId": {
      "type": "string",
      "description": "本次操作的幂等请求标识，与输入一致；重试或恢复时复用。",
      "title": "请求ID"
    },
    "nodeId": {
      "type": "string",
      "description": "已解析的脑图文件节点 ID，与主题 ID 分开使用；用于后续请求的 nodeId。",
      "title": "脑图ID"
    },
    "pageId": {
      "type": "string",
      "description": "本次节点操作所在页面的真实 ID。",
      "title": "页面ID"
    },
    "topicId": {
      "type": "string",
      "description": "本次操作目标主题的真实 ID；新增时为实际分配的主题 ID，结果尚未知时可能不返回。",
      "title": "主题ID"
    },
    "status": {
      "type": "string",
      "description": "节点操作状态：APPLIED 表示已确认应用；NO_CHANGE 表示无需新增内容变更；UNKNOWN 表示提交结果不确定；COMMITTED_UNVERIFIED 表示已提交但实际内容尚未验证；CONFLICT 表示操作冲突。未决结果按原 requestId 和参数恢复。",
      "title": "操作状态"
    },
    "applied": {
      "type": "boolean",
      "description": "是否已经确认操作持久化且实际内容符合预期；不能仅凭顶层 success 判断写入成功。",
      "title": "操作是否已确认应用"
    },
    "committedVersion": {
      "type": "string",
      "description": "已知的实际提交版本，使用十进制字符串表示；结果不确定时可能不返回。",
      "title": "提交版本"
    },
    "readVersion": {
      "type": "string",
      "description": "本次查询或节点操作读取的固定内容版本，使用十进制字符串表示。可用于后续写入的 expectedVersion；查询续页保持同一读取版本。",
      "title": "读取版本"
    },
    "nodeIdMap": {
      "type": "object",
      "description": "客户端临时主题 ID 到真实主题 ID 的映射。值可用于后续请求的 topicId 或 parentTopicId；不是文件 nodeId。映射的键和值均为字符串。",
      "title": "主题ID映射"
    },
    "node": {
      "type": "object",
      "properties": {
        "id": {
          "type": "string",
          "description": "脑图主题的真实 ID，可作为后续请求的 topicId 或 parentTopicId；不是文件 nodeId，也不是创建输入的临时 ID。",
          "title": "主题ID"
        },
        "type": {
          "type": "string",
          "description": "公开主题类型，当前返回 topic。",
          "title": "主题类型"
        },
        "parentId": {
          "type": "string",
          "description": "父主题的真实 ID；根主题没有父主题时不返回。",
          "title": "父主题ID"
        },
        "text": {
          "type": "string",
          "description": "主题纯文本，空字符串表示空内容；受保护主题不返回正文。",
          "title": "主题文本"
        },
        "index": {
          "type": "integer",
          "description": "主题在父主题全部直接子节点中的位置，从 0 开始；不是当前分页结果中的下标。",
          "title": "兄弟节点位置"
        },
        "childCount": {
          "type": "integer",
          "description": "直接子主题数量，不包含更深层后代；受保护主题不返回此字段。",
          "title": "直接子主题数量"
        },
        "writeSupport": {
          "type": "string",
          "description": "主题写入支持程度：readWrite 表示可通过节点接口写入，readOnly 表示只读；只读原因见 unsupportedFeatures。根主题和受限主题的可执行操作仍由服务端校验。",
          "title": "主题写入支持程度"
        },
        "unsupportedFeatures": {
          "type": "array",
          "items": {
            "type": "string",
            "title": "限制原因标识",
            "description": "一个限制原因标识，例如 mind.rootMutation 或 mind.protected。"
          },
          "description": "导致主题只读或部分内容不公开的原因标识列表，例如 mind.rootMutation 表示根主题禁止修改，mind.protected 表示内容受保护。受保护主题不会返回 text 和 childCount。",
          "title": "不支持写入的原因"
        }
      },
      "description": "已确认的主题内容投影，可用时返回；不会返回完整脑图快照。",
      "title": "主题内容"
    },
    "deletedCount": {
      "type": "integer",
      "description": "已确认删除的主题数量，包含目标主题及后代；仅 deletedCountKnown=true 时可据此判断删除数量。",
      "title": "已删除主题数量"
    },
    "deletedCountKnown": {
      "type": "boolean",
      "description": "删除数量是否已经确认。false 时不能把缺失的 deletedCount 当作 0。",
      "title": "删除数量是否已确认"
    },
    "errorDetail": {
      "type": "object",
      "properties": {
        "stage": {
          "type": "string",
          "description": "操作未完成、结果未确认或冲突发生的处理阶段，用于定位失败环节。",
          "title": "处理阶段"
        },
        "code": {
          "type": "string",
          "description": "该处理阶段的诊断错误码；与顶层请求失败的 errorCode 分开使用。",
          "title": "诊断错误码"
        },
        "message": {
          "type": "string",
          "description": "该阶段的具体原因与诊断信息，已做敏感内容处理；结合 status 判断操作结果。",
          "title": "诊断信息"
        },
        "category": {
          "type": "string",
          "description": "有限业务错误类别：TIMEOUT、VALIDATION、DOWNSTREAM；不表示可自动重试，不返回内部异常类名。",
          "title": "错误类别"
        },
        "operationId": {
          "type": "string",
          "description": "关联操作标识，存在且可安全展示时返回，用于关联诊断；恢复操作仍应复用请求的 requestId。",
          "title": "关联操作ID"
        }
      },
      "description": "未决结果、提交未验证或冲突的阶段诊断信息；可能随业务结果返回，不能仅凭顶层 success 判断内容已完成。",
      "title": "操作诊断信息"
    },
    "executed": {
      "type": "boolean",
      "description": "dry-run 时固定为 false，表示没有调用远端工具"
    },
    "tool": {
      "type": "string",
      "description": "dry-run 预览的 MCP 工具 key"
    },
    "sourceBytes": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：UTF-8 字节数"
    },
    "pageCount": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：页面数"
    },
    "topicCount": {
      "type": "integer",
      "description": "dry-run 的初始内容统计：主题数"
    }
  },
  "additionalProperties": true
}`)}
}
