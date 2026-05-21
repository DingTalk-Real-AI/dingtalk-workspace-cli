# 测试用例补充说明

## 更新日期
2026-04-26

## 更新文件
`auto-test/cli_to_mcp/testcases/aitable/test_90_aitable_param_regression.py`

---

## 新增测试用例

### TestNewFeatures 类

包含 6 个新增功能的测试用例:

#### 1. test_base_create_with_folder_id
- **测试功能**: `base create` 支持 `--folder-id` 可选参数
- **测试逻辑**: 不传 folder-id 也能成功创建 base
- **预期结果**: 创建成功,返回 baseId

#### 2. test_table_create_with_empty_fields
- **测试功能**: `table create` 支持空 fields 数组
- **测试逻辑**: 传入 `--fields '[]'`
- **预期结果**: 创建成功,系统自动补 primaryDoc 首列

#### 3. test_view_create_with_desc
- **测试功能**: `view create` 支持 `--desc` 参数
- **测试逻辑**: 传入视图描述 JSON
- **预期结果**: 创建成功,返回 viewId

#### 4. test_attachment_upload_requires_size
- **测试功能**: `attachment upload` 的 `--size` 参数必填
- **测试逻辑**: 故意不传 --size 参数
- **预期结果**: CLI 层校验失败,returncode != 0

#### 5. test_field_create_with_new_types
- **测试功能**: `field create` 支持新字段类型
- **测试类型**: address (行政区域)
- **预期结果**: 创建成功

#### 6. test_chart_create_requires_config_and_layout
- **测试功能**: `chart create` 的 config 和 layout 是必填参数
- **测试逻辑**: 不传 --config 和 --layout
- **预期结果**: CLI 层参数校验失败,returncode != 0
- **备注**: 虽然 MCP 定义可能有误,但业务上它们确实是必填的

---

## 运行测试

```bash
cd /Users/hehe/Documents/code/dws-wukong/auto-test/cli_to_mcp/testcases/aitable

# 运行所有回归测试
python3 -m pytest test_90_aitable_param_regression.py -v

# 运行新增功能测试
python3 -m pytest test_90_aitable_param_regression.py::TestNewFeatures -v

# 运行单个测试
python3 -m pytest test_90_aitable_param_regression.py::TestNewFeatures::test_base_create_with_folder_id -v
```

---

## 测试覆盖的新功能

| 功能 | 测试用例 | 状态 |
|------|---------|------|
| base create 支持 folderId | test_base_create_with_folder_id | ✅ |
| table create 支持空 fields | test_table_create_with_empty_fields | ✅ |
| view create 支持 viewDescription | test_view_create_with_desc | ✅ |
| attachment upload 必填 size | test_attachment_upload_requires_size | ✅ |
| field create 支持 address 类型 | test_field_create_with_new_types | ✅ |
| chart create config/layout 必填 | test_chart_create_requires_config_and_layout | ✅ |

---

## 前置条件

运行测试前需要:
1. `dws` CLI 已安装且在 PATH 中
2. `dws auth status` 显示已登录
3. Python 3.9+ 和 pytest 已安装
4. 测试账号下有可用的 AI 表格 Base

---

## 注意事项

1. 所有测试数据使用 `CLI_Test_` 前缀,与生产数据隔离
2. 测试结束后会自动清理创建的测试数据
3. 部分测试依赖 `test_base_id` fixture (在 conftest.py 中定义)
4. 测试用例 4 和 6 测试的是参数校验逻辑,不要求 MCP 调用成功
