# Aitable DWS 集成测试

## 运行方式

```bash
cd /Users/qinze/dev/work/ai/c3-marketing-service-technology/技术专项/cli/docs/qinze/mcps/testcases/aitable/2026-03-13

# 全部运行
python3 -m pytest -v --tb=short

# 运行单个文件
python3 -m pytest test_01_base.py -v

# 运行单个类
python3 -m pytest test_04_record.py::TestRecordCreate -v
```

## 前置要求

1. `dws` v0.2.3+ 已安装且在 PATH 中
2. `dws auth status` 显示已登录
3. Python 3.9+ 和 `pytest` (`pip install pytest`)

## 文件说明

| 文件 | 覆盖命令 | 用例数 |
|------|---------|--------|
| conftest.py | - | 共享 fixture |
| test_01_base.py | base list/search/get/create/update/delete | 10 |
| test_02_table.py | table get/create/update/delete | 6 |
| test_03_field.py | field get/create/update/delete | 10 |
| test_04_record.py | record query/create/update/delete | 11 |
| test_05_template.py | template search | 4 |

## 数据安全

- 测试数据在 `conftest.py` 的 session fixture 中创建
- 所有数据以 `CLI_Test_` 前缀命名，与生产数据隔离
- 测试结束后自动删除测试 Base（含其下所有 table/field/record）
