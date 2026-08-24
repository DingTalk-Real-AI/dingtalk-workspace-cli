# Recruit CLI 参数幻觉分析（2026-08-21）

## 结论

基于最新线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc`，Recruit 三个公开叶子均已审计。
独立完整候选含 62 个 concept、352 个 command override、691 条 fixture，仅保存在本目录。

## 问题与兜底

| 命令 | 安全归一 | fail-closed 边界 |
|---|---|---|
| `recruit job list` | `q→keyword`、`page-size→size`、`next-cursor→cursor`、`position-ids→job-ids` | 单数 job ID、单数 creator ID、page/offset、泛化 time/type 不转换 |
| `recruit job get` | `position-id`/`recruit-job-id→job-id` | 复数 ID 与无角色 `id` 不转换 |
| `recruit job create` | 明确的 `file/file-path/job-json-file→from` | 原始 `job-json`、通用 payload/data/json、corpId/bizCode/opUserId 与嵌套人员字段不转换 |

`job create --from` 接受本地 UTF-8 JSON 文件。`creatorUserId`、`ownerUserIds` 是 JSON 内业务字段，
不能提升为 CLI flag；corpId、bizCode、opUserId 由连接器/身份上下文负责，禁止猜测或暴露。

## 验证与未解决项

独立候选 fresh generate、嵌入式 PreParse fixture 通过。联合候选新增 3 个有效命令规则、3 个 alias、
4 个 block、8 个 ambiguous。枚举值、时间格式与 JSON 结构校验不属于参数名归一能力，仍由 Schema/Runtime 负责。

分析依据：最新 Cobra/Help、正式 compact/full Schema、`dingtalk-misc/references/recruit.md`、生成器与 Runtime。
