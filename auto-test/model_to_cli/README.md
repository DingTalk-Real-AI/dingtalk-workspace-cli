# model_to_cli — 模型 → CLI 命令评测

测试 AI 模型能否根据自然语言意图正确生成 `dws` CLI 命令。用于回归检验
skills 包文档质量（产品参考是否清晰、命令名是否易被推理）和模型选型。

> 当前 testcases 覆盖**玉渊域 4 个产品**（aitable / aiapp / doc / drive），
> 共 46 个 testcase 文件 / ~4300 行。其余产品由各产品 owner 自行追加。

## 目录结构

```
auto-test/
└── model_to_cli/                          # 本目录
    ├── README.md
    ├── run_evaluation.py                  # 模型评测引擎
    ├── generate_cli_unified_testcases.py  # 从 skills/references/products/*.md 生成 testcase
    ├── .env                               # API Key 配置（本地，不提交）
    ├── testcases/                         # 测试用例
    │   ├── index.json                     # 全局索引（按产品聚合）
    │   └── <product>/
    │       ├── index.json                 # 产品级索引
    │       └── <command>_testcases.json   # 具体 testcase
    └── evaluation_reports/                # 评测报告输出（gitignore）
        └── open_report/
            └── <model>_<N>cases_<ts>_failures.json
```

---

## 脚本说明

### `run_evaluation.py` — 模型评测引擎

读取测试用例，向 AI 模型发送用户意图，评估模型输出的 CLI 命令是否正确。

**评分维度：**

| 维度 | 说明 |
|------|------|
| 命令准确率 | 模型输出的动词路径是否与期望完全一致 |
| 参数均分 | 模型输出的 flags 与期望 flags 的匹配程度（0~100） |

**参数评分规则：**

- 完全匹配或值为占位符 → +1.0
- 字符串包含关系 → +0.8
- flag 缺失但值为占位符 → +0.3
- 额外多余的 flag → 每个 -0.2

**失败用例输出：**
仅将命令错误或参数未满分（< 100）的用例写入 JSON，供人工或 AI 分析。

**前置条件：**

1. 在 `.env` 中配置 API Key（或通过环境变量）：
   ```
   DASHSCOPE_API_KEY=sk-xxxxxxxx
   ```
2. 确保 `testcases/` 目录下已有测试用例

**使用方式：**

```bash
cd auto-test/model_to_cli
python run_evaluation.py                          # 默认 --edition open，跑所有模型 + 全部用例
python run_evaluation.py --models qwen3-max       # 指定模型
python run_evaluation.py --cases 50               # 限制用例数（随机采样）
python run_evaluation.py --dry-run                # 只加载用例，不调用 API
python run_evaluation.py --output /tmp/reports    # 自定义输出目录
```

默认输出目录：`evaluation_reports/open_report/`，传 `--output` 则以参数为准。

---

## 快速开始

```bash
cd auto-test/model_to_cli

# 运行评测
python3 run_evaluation.py
```

---

## 依赖

```bash
pip3 install openai
```

模型调用通过 DashScope 兼容接口统一接入，目前支持 `qwen3-max`。
