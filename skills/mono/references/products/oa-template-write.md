# 创建和更新审批模板

本文件包含模板创建（`template create`）、更新（`template update`）的完整参数说明、控件/节点/规则参考索引、`--from-document` 文档模式以及同名冲突处理。查询模板列表和模板详情仍在 [oa.md](oa.md) 中。

## 逐项参数模式

```bash
dws oa approval template create --name '出差申请' --schema-content @form.json --format json
dws oa approval template detail --process-code PROC-EXAMPLE --format json
dws oa approval template update --process-code PROC-EXAMPLE --name '出差申请' --schema-content @form.json --process-config @process.json --format json
```

- `create` 调用 `oa/create_process_template`，逐项参数模式必填 `--name`、`--schema-content`。不传 `--process-config` 时创建包含发起人节点、一个审批人节点和一个抄送人节点的默认流程；不传 `--visible-range` 时全员可见。
- `update` 调用 `oa/update_process_template`，逐项参数模式必填 `--process-code`、`--name`、`--schema-content`、`--process-config`。更新前必须读取 `template detail`，基于返回配置修改并保留已有控件 `props.id`，每个流程节点包含 `nodeId`。更新接口不接受 `--expected-version`；该数值参数仅创建接口可选。
- `--schema-content`、`--process-config` 输入 JSON 对象，但以 JSON **字符串**发送给 MCP。`--form-config`、`--node-config`、`--condition-rule` 同样以 JSON 字符串发送。这些参数支持直接 JSON、`@文件` 或 `-` 标准输入；一次命令只能有一个参数读取标准输入。
- `--plugin-configs`、`--visible-range` 接收对象数组 JSON，`--manager-user-ids` 接收字符串数组 JSON，支持相同文件/标准输入方式；MCP 接收到数组而非字符串。显式 `[]` 保留并提交，不等同于未传。
- 其他可选参数：`--description`、`--icon-url`、`--static-workflow`（`0` 动态 / `1` 静态）、`--append-enable`（`y` / `n`）、`--duplicate-removal`（字符串 `true` / `false`）。更新不传可选字段时沿用原配置；显式空描述或空图标字符串会提交给服务端。
- 两个写命令支持 `--dry-run`，仅做本地校验并展示请求参数，返回 `executed=false`，不调用 MCP。
- 成功通过统一输出 `data.processCode` 返回模板编码；服务端 `success=false` 或非零错误码作为失败输出，保留错误码及错误信息。创建/更新后再次读取 `template detail` 核对实际状态，不将本地 Schema 查询当作业务验证。

## 流程节点审批人默认规则

构造 `processConfig` 时，如果用户没有明确指定审批人类型，审批节点的选人规则默认使用**部门主管**（`target_management`，`level: 1`），而不是直属主管（`target_formula` / `reportLineManager`）。大部分审批流程使用的是部门主管。只有用户明确要求直属主管直属领导汇报线主管时才使用直属主管规则。详见 [rules/target_management.md](oa/template/rules/target_management.md)。

## 详细控件 / 节点 / 规则参考（构造 schemaContent 与 processConfig 时按需读取）

构造 `--schema-content` 和 `--process-config` 的 JSON 结构时，按需加载以下参考文档。**不要一次性预读全部控件/节点文档**——先看总览确定需要的控件类型或节点类型，再按 `componentName` 或节点类型定位单个文件。

| 需要了解 | 参考文档 | 说明 |
|---------|---------|------|
| 控件总览与表单顶层结构 | [form-components.md](oa/template/form-components.md) | 38 种控件分类、`schemaContent` 顶层字段、控件通用字段、约束速查 |
| 某个具体控件的 props 结构与示例 | [components/\<ComponentName\>.md](oa/template/components/) | 如 `TextField.md`、`DDSelectField.md`、`TableField.md` 等，共 31 个 |
| 流程节点总览与 processConfig 树结构 | [process-nodes.md](oa/template/process-nodes.md) | 节点类型、嵌套树结构、节点通用字段、选人规则总览、约束速查 |
| 某类节点（审批人/抄送/条件/并行等） | [nodes/\<node\>.md](oa/template/nodes/) | 如 `approver.md`、`cc.md`、`condition.md`、`parallel.md` 等，共 7 个 |
| 审批人选人规则 | [rules/\<rule\>.md](oa/template/rules/) | 如 `target_management.md`、`target_label.md`、`target_managers_labels.md` 等，共 10 个 |
| 完整表单示例 | [example/form_components.json](oa/template/example/form_components.json) | 包含多种控件的报销申请表单 |
| 完整流程示例 | [example/process_config.json](oa/template/example/process_config.json) | 包含多级审批、条件分支、抄送的流程树 |
| 生成控件唯一 ID | `python3 scripts/generate_form_component_id.py <ComponentType>` | 输出格式 `{ComponentName}_{随机字符串}` |
| 生成节点唯一 ID | `python3 scripts/generate_node_id.py` | 输出格式 `{4位hex}_{4位hex}` |

## 从完整 JSON 文档创建或更新模板

`--from-document` 接收本地普通 JSON 文件的**绝对路径**（不加 `@`，最大 4 MiB），不需要模型服务。文件顶层使用 MCP 参数名；不接受自由文字描述、未知字段、重复顶层字段或 `null`。

`/绝对路径/template.json` 示例：

```json
{
  "name": "出差申请",
  "description": "员工出差申请",
  "schemaContent": {
    "title": "出差申请",
    "items": [
      {
        "componentName": "TextField",
        "props": {"id": "TextField-reason", "label": "出差事由", "required": true}
      }
    ]
  },
  "processConfig": {
    "type": "start",
    "name": "发起人",
    "nodeId": "sid-startevent",
    "properties": {}
  }
}
```

```bash
dws oa approval template create --from-document /绝对路径/template.json --dry-run --format json
dws oa approval template create --from-document /绝对路径/template.json --format json
dws oa approval template update --process-code PROC-EXAMPLE --from-document /绝对路径/template.json --dry-run --format json
```

- 创建文档必填 `name`、`schemaContent`；更新文档还需 `processConfig`，以及文档中的 `processCode` 或单独的 `--process-code`。同时提供两处编码时必须一致。
- `schemaContent`、`processConfig` 可以写成对象或已有 JSON 字符串，CLI 统一发送为 MCP 要求的 JSON 字符串；其他 JSON 配置字段也会转换为 JSON 字符串。`pluginConfigs`、`visibleRange` 必须是对象数组，`managerUserIds` 必须是字符串数组。
- `staticWorkflow` 使用字符串 `"0"`/`"1"`，`appendEnable` 使用 `"y"`/`"n"`，`duplicateRemoval` 使用字符串 `"true"`/`"false"`；`expectedVersion` 仅创建文档可用，必须是数值。
- 文档模式与逐项配置参数互斥，只有更新的 `--process-code` 可以同时使用。未传可选字段保持省略，显式空描述、空图标和空数组原样提交。
- `schemaContent.items` 必须是控件数组，每个控件包含 `componentName` 和唯一 `props.id`；流程中的 `childNode`/`conditionNodes` 节点须有 `type` 和唯一 `nodeId`。这些结构问题在调用 MCP 前报错。
- 更新文档前先读取真实 `template detail`，基于原配置修改并保留已有控件 ID；CLI 不会根据 JSON 中缺少的信息推测业务需求。`--dry-run` 只读取本地文件并展示最终请求，不调用 MCP。

## 创建时的同名模板处理

名称唯一性由 create_process_template 校验，不预查模板列表。若 MCP 返回同名错误，明确告知创建失败，让用户选择重新命名后创建、更新已有模板或取消创建。不得自行追加名称后缀重试，也不得自动切换为更新。选择更新后再定位已有模板并读取 template detail，保留已有字段 ID 和未修改配置；错误未返回 processCode 时不得编造。

`--dry-run` 不调用创建 MCP，不能证明名称可用。其他错误保留服务端原因，不得一律判为同名冲突。

创建 MCP 返回 `dingOpenErrcode=810001` 表示同名冲突，`810002` 表示模板数量达到上限。数量超限应由用户清理无效模板、联系管理员调整容量，或选择更新已有模板/取消创建；不能通过改名解决，也不得自动删除模板。CLI 保留服务端错误码和文案，不自动重试。
