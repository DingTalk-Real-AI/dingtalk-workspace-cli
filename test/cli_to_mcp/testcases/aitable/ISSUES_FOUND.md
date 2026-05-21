# aitable.go 详细比对发现的问题

## 检查日期
2026-04-26 第二次详细检查

---

## ❌ 发现的问题

### 1. create_chart - config 和 layout 应保持必填 ⚠️ MCP配置问题

**MCP 定义** (可能有误):
```json
{
  "required": ["baseId", "dashboardId"],
  "properties": {
    "baseId": {...},
    "dashboardId": {...},
    "config": {...},  // 不在 required 中,但描述写“必传参数”
    "layout": {...}   // 不在 required 中,但描述写“必传参数”
  }
}
```

**当前代码** (第 1229 行):
```go
if err := validateRequiredFlags(cmd, "dashboard-id", "config", "layout"); err != nil {
```

**问题**: MCP 的 required 中没有 config 和 layout,但业务逻辑上它们是必填的

**处理**: ✅ 保持代码中 config 和 layout 为必填,不按 MCP 的 required 修改。
这是 MCP Server 配置的问题,应该反馈给他们修复。

**备注**: MCP 描述中也写着“必传参数”,说明业务上确实是必填的,只是 required 数组配置遗漏了。

---

### 2. update_chart - config 不应强制必填

**MCP 定义**:
```json
{
  "required": ["baseId", "dashboardId", "chartId"],
  "properties": {
    "config": {...},  // 不在 required 中
    "layout": {...}   // 不在 required 中
  }
}
```

**当前代码** (第 1253 行):
```go
if err := validateRequiredFlags(cmd, "dashboard-id", "chart-id", "config"); err != nil {
```

**问题**: 代码强制要求 config,但 MCP 中它不是 required

**修复**: 移除 config 的必填校验

---

### 3. prepare_attachment_upload - size 应该必填

**MCP 定义**:
```json
{
  "required": ["baseId", "fileName", "size"],
  "properties": {
    "size": {...}  // 在 required 中
  }
}
```

**当前代码** (第 818 行):
```go
if err := validateRequiredFlags(cmd, "base-id", "file-name"); err != nil {
```

**问题**: 代码没有校验 size,但 MCP 中它是 required

**修复**: 添加 --size 的必填校验

---

### 4. create_view - viewSubType 参数缺失

**MCP 定义**:
```json
{
  "properties": {
    "viewSubType": {...}  // 代码中没有处理
  }
}
```

**问题**: MCP 有 viewSubType 参数,但代码中未实现

**修复**: 添加 --view-sub-type 参数支持

---

## 总结

| 问题 | 严重程度 | 影响 |
|------|---------|------|
| create_chart 强制要求 config/layout | 中 | 限制了灵活性 |
| update_chart 强制要求 config | 中 | 无法只更新 layout |
| prepare_attachment_upload 缺少 size 校验 | 高 | 可能导致 MCP 调用失败 |
| create_view 缺少 viewSubType | 低 | 缺少可选功能 |
