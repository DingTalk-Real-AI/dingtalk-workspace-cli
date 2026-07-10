# DWS Schema 功能设计文档（静态端点架构上的 Schema，对齐旧版与 GWS/Lark）

> 分支：`feat/schema-gws-flat`（内容基于 upstream `main` / v1.0.52 静态端点架构 + 合入旧版历史）
>
> 目标：在 upstream 已移除服务发现的静态端点架构上，为 `dws schema` 提供与旧版一致、并对齐 Lark/GWS 契约的 Schema 能力。

## 1. 背景

upstream `main`（v1.0.52）切换为"静态端点模式"，移除服务发现与动态 Schema 生成体系（删除 `internal/discovery`/`generator`/`ir`/`compat`/`cache`/`market` 等包），`dws schema` 降级为空壳 stub。本方案在不恢复服务发现的前提下，重建与旧版一致的 Schema 能力。

## 2. 设计原则

- **版本内嵌 Command Catalog 优先**：`dws schema` 服务版本内固定的内嵌 Catalog（21 产品 / 504 工具），与旧版发布二进制行为一致；查询不发起网络请求、不做服务发现。
- **GWS 扁平叶子契约**：工具查询输出扁平 leaf 对象，`parameters` 内联 `required`、键为 CLI flag，无参数工具用 `parameters: {}`。
- **Lark 稳定 canonical**：canonical 查找稳定，重复命令路径显式暴露为 alias，不静默择一。
- **回退**：内嵌 Catalog 不可用时，回退到 helper-only 子树与实时 Cobra 命令树渲染。

## 3. 架构与数据流

```
dws schema [path] [--all] [--cli-path P] [--format|--jq|--fields]
  -> NewSchemaCommand (internal/cli/canonical.go)
     1) embeddedSchemaCatalogAvailable() ?
        -> embeddedSchemaPayload(args)                 # 版本内嵌 504 catalog
        -> 无 path 且非 --all: compactSchemaOverviewPayload  # 紧凑总览
        -> output.WriteFiltered (支持 --format/--jq/--fields)
     2) 回退 helper-only 子树: renderHelperSchema(ctx, root, path, fetcher)
     3) 回退实时命令树: runtimeSchemaPayload(root, args)
```

关键文件：`internal/cli/canonical.go`（`NewSchemaCommand` embedded 优先）、`internal/cli/schema_catalog.go`（内嵌 Catalog 读取 `embeddedSchemaPayload`）、`internal/cli/runtime_schema.go`（`compactSchemaOverviewPayload` 与实时树回退）、`internal/cli/schema_agent_metadata/*.json`（Agent 语义）、`internal/cli/schema_catalog.json`（内嵌 504 工具 Catalog）、`internal/ir/catalog.go`（数据结构，去除依赖已删 `discovery` 的构建期 `BuildCatalog`）。

## 4. 与 upstream 的适配点

1. `internal/ir/catalog.go`：删除依赖 `internal/discovery` 的 `BuildCatalog`，保留运行时数据结构。
2. `internal/cli/canonical.go`：`NewSchemaCommand` 由 stub 改为 embedded 优先（复用旧版逻辑），新增 `--all` flag，经 `internal/output` 支持 `--format/--jq/--fields`。
3. `internal/cli/dev_schema.go` / `schema_validate.go`：沿用 upstream 版本，避免重复声明。
4. 新增 `internal/cli/schema_support.go`：补齐 `walkLeafCommands`/`schemaCatalogToolCount`/`helperProductSummaries`。

## 5. 与旧版 / Lark / GWS 对齐

- 与旧版一致：`dws schema` 总览 21 产品 / 504 工具；`dws schema "calendar event create"` 返回 `calendar.create_calendar_event`、20 个扁平参数、`effect=write`；`dws schema --all` 输出完整 21 产品目录。
- 对齐 GWS：扁平 leaf、`parameters` 内联 required、无参数 `parameters: {}`。
- 对齐 Lark：canonical 稳定、重复路径显式 alias（如 `aitable record list` -> `aitable.query_records`）。

## 6. 测试与验证

- `go build ./...` 全量编译通过。
- `go test ./internal/cli/` 通过，含 `TestEmbeddedSchemaCatalogIntegrity`（断言 504 工具 / 21 产品）与 `TestEmbeddedSchemaCatalogProgressiveQueries`（总览/leaf/group/alias 渐进查询）。
- `go test ./...`：schema 相关全部通过；仅 `test/scripts` 3 个 `TestPostGoreleaser*` 失败，为 upstream 固有的 `tar` 打包环境问题（本次未触碰打包脚本），与 schema 无关。
- 运行验证：`dws schema`（21/504）、`dws schema "calendar event create"`（20 参数 / GWS flat）、`dws schema --all`（21 产品）。

## 7. 已知限制与后续

- **构建期 generator 未移植**：`internal/generator/*` 依赖 upstream 已删包，未纳入；运行时直接读版本内嵌 Catalog，不依赖它。重新生成内嵌数据的能力作为后续工作。
- 内嵌 Catalog 为版本内固定快照（与旧版一致的设计），描述发布版本的完整命令面，不随实时命令树变化。
