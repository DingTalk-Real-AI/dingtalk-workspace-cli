# AI 表格 Section 与内部节点

只在用户操作 Base 内文件夹（Section）或把 Table/Dashboard 移入、移出 Section 时读取。这里的节点是 nsheet 业务节点，不是独立 Drive dentry；不要加载 Drive Skill 或尝试 Drive move。

## 高频闭环

创建 Section 并移动节点：

```bash
dws aitable +section-create --base-id <B> --name "归档区" --format json
dws aitable +section-move-node --base-id <B> --node-id <TABLE_OR_DASHBOARD_ID> --new-parent-section-id <S> --format json
dws aitable +section-list-nodes --base-id <B> --format json
```

移动回 Base 根目录时显式传空字符串，不能省略该参数或改用 Drive：

```bash
dws aitable +section-move-node --base-id <B> --node-id <N> --new-parent-section-id '' --format json
```

后续操作只使用本次创建真实返回的 sectionId。若列表、移动或重命名对同一 sectionId 返回不存在或父节点无效，保留创建返回与后续错误并停止；不要创建替代 Section、更换 ID、重读本文或查询 Help/Catalog，也不要修改无关节点。

## 删除空 Section

1. 用 `+section-list-nodes` 核对目标 Section 内节点；需要移出的节点逐个 `+section-move-node`。
2. 用 `+section-list-empty --base-id <B>` 验证目标 sectionId 确实为空。
3. 执行 `+section-delete --base-id <B> --section-id <S>`；按 Runtime confirmation 处理。
4. 再次 `+section-list-nodes` 或 `+section-list-empty`，确认 Section 已不存在且被移动节点仍在预期父级。

Table 本身的创建、复制、改名、删除分别使用 `+table-*`；Section 只管理 Base 内目录关系。
