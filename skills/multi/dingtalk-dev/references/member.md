# 成员管理

> 概念锚点：成员=谁能改这个应用（DEVELOPER 等角色）；见 SKILL.md 概念地图。

## 查询成员

```bash
dws dev app member list --unified-app-id UNIFIED_APP_ID --format json
```

## 添加成员

```bash
dws dev app member add --unified-app-id UNIFIED_APP_ID --users userId1,userId2 --member-type DEVELOPER --dry-run --format json
dws dev app member add --unified-app-id UNIFIED_APP_ID --users userId1,userId2 --member-type DEVELOPER --yes --format json
```

## 移除成员

```bash
dws dev app member remove --unified-app-id UNIFIED_APP_ID --users userId1 --member-type DEVELOPER --dry-run --format json
dws dev app member remove --unified-app-id UNIFIED_APP_ID --users userId1 --member-type DEVELOPER --yes --format json
```

| CLI | 说明 |
|-----|------|
| `--unified-app-id` | 统一应用 ID（必填） |
| `--users` | userId 列表，逗号/分号分隔（必填） |
| `--member-type` | 成员类型，如 `DEVELOPER`（必填） |
