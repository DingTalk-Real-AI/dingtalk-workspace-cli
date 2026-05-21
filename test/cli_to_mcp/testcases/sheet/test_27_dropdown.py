"""
test_27_dropdown.py — 下拉列表测试

依赖 conftest.py 自建的测试表格。
正向用例使用默认工作表的空闲区域（Q~S 列），操作完成后清理下拉列表，
避免影响其他测试用例。

Commands tested:
  1. dws sheet set-dropdown --node NODE_ID --sheet-id SHEET_ID --range RANGE --options JSON [--multi-select]
  2. dws sheet get-dropdown --node NODE_ID --sheet-id SHEET_ID --range RANGE
  3. dws sheet delete-dropdown --node NODE_ID --sheet-id SHEET_ID --range RANGE
"""

import json


def _cleanup_dropdown(dws, node_id, sheet_id, cell_range):
    """删除指定范围的下拉列表（best-effort 清理）。"""
    try:
        dws.run(
            "sheet", "delete-dropdown",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--range", cell_range,
            expect_success=False,
        )
    except Exception as exc:
        print(f"[CLEANUP] Failed to delete dropdown at {cell_range}: {exc}")


class TestSetDropdown:
    """dws sheet set-dropdown"""

    def test_set_basic_dropdown(self, dws, sheet_node_id, sheet_id):
        """设置基本单选下拉列表，验证 success。"""
        options = json.dumps([
            {"value": "选项A"},
            {"value": "选项B"},
            {"value": "选项C"},
        ], ensure_ascii=False)
        data = dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q1:Q5",
            "--options", options,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "Q1:Q5")

    def test_set_dropdown_with_colors(self, dws, sheet_node_id, sheet_id):
        """设置带颜色的下拉列表，验证 success 和 optionCount。"""
        options = json.dumps([
            {"value": "高", "color": "#ff0000"},
            {"value": "中", "color": "#ffaa00"},
            {"value": "低", "color": "#00ff00"},
        ], ensure_ascii=False)
        data = dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q6:Q10",
            "--options", options,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        option_count = data.get("optionCount")
        if option_count is not None:
            assert option_count == 3, f"optionCount 应为 3: {data}"

        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "Q6:Q10")

    def test_set_dropdown_multi_select(self, dws, sheet_node_id, sheet_id):
        """设置多选下拉列表，验证 enableMultiSelect。"""
        options = json.dumps([
            {"value": "标签1"},
            {"value": "标签2"},
            {"value": "标签3"},
        ], ensure_ascii=False)
        data = dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "R1:R5",
            "--options", options,
            "--multi-select",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        multi = data.get("enableMultiSelect")
        if multi is not None:
            assert multi is True, f"enableMultiSelect 应为 True: {data}"

        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "R1:R5")

    def test_set_dropdown_overwrite(self, dws, sheet_node_id, sheet_id):
        """覆盖已有下拉列表，验证新配置生效。"""
        # 先设置初始下拉
        initial_options = json.dumps([
            {"value": "旧选项1"},
            {"value": "旧选项2"},
        ], ensure_ascii=False)
        dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "S1:S3",
            "--options", initial_options,
        )

        # 覆盖为新下拉
        new_options = json.dumps([
            {"value": "新A"},
            {"value": "新B"},
            {"value": "新C"},
            {"value": "新D"},
        ], ensure_ascii=False)
        data = dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "S1:S3",
            "--options", new_options,
        )
        assert data.get("success") is True, f"覆盖设置应成功: {data}"
        option_count = data.get("optionCount")
        if option_count is not None:
            assert option_count == 4, f"覆盖后 optionCount 应为 4: {data}"

        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "S1:S3")


class TestGetDropdown:
    """dws sheet get-dropdown"""

    def test_get_existing_dropdown(self, dws, sheet_node_id, sheet_id):
        """设置下拉后查询，验证返回的选项值。"""
        options = json.dumps([
            {"value": "查询A"},
            {"value": "查询B"},
        ], ensure_ascii=False)
        dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q11:Q15",
            "--options", options,
        )

        data = dws.run(
            "sheet", "get-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q11:Q15",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("hasDropdown") is True, f"hasDropdown 应为 True: {data}"

        validations = data.get("dataValidations")
        assert isinstance(validations, list), f"dataValidations 应为 list: {data}"
        assert len(validations) >= 1, f"应至少有 1 组下拉配置: {data}"

        # 验证选项值包含设置的值
        all_values = []
        for validation in validations:
            condition_values = validation.get("conditionValues", [])
            all_values.extend(condition_values)
        assert "查询A" in all_values, f"conditionValues 应包含 '查询A': {all_values}"
        assert "查询B" in all_values, f"conditionValues 应包含 '查询B': {all_values}"

        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "Q11:Q15")

    def test_get_dropdown_with_color(self, dws, sheet_node_id, sheet_id):
        """设置带颜色的下拉后查询，验证 colorValueMap。"""
        options = json.dumps([
            {"value": "红色", "color": "#ff0000"},
            {"value": "蓝色", "color": "#0000ff"},
        ], ensure_ascii=False)
        dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q16:Q20",
            "--options", options,
        )

        data = dws.run(
            "sheet", "get-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q16:Q20",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("hasDropdown") is True, f"hasDropdown 应为 True: {data}"

        validations = data.get("dataValidations", [])
        assert len(validations) >= 1, f"应至少有 1 组下拉配置: {data}"

        # 验证颜色映射
        first_validation = validations[0]
        color_map = (first_validation.get("options") or {}).get("colorValueMap")
        if color_map is not None:
            assert isinstance(color_map, dict), f"colorValueMap 应为 dict: {color_map}"

        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "Q16:Q20")

    def test_get_dropdown_multi_select_flag(self, dws, sheet_node_id, sheet_id):
        """设置多选下拉后查询，验证 enableMultiSelect 字段。"""
        options = json.dumps([
            {"value": "多选A"},
            {"value": "多选B"},
        ], ensure_ascii=False)
        dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "R6:R10",
            "--options", options,
            "--multi-select",
        )

        data = dws.run(
            "sheet", "get-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "R6:R10",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        validations = data.get("dataValidations", [])
        assert len(validations) >= 1, f"应至少有 1 组下拉配置: {data}"

        first_options = (validations[0].get("options") or {})
        multi_select = first_options.get("enableMultiSelect")
        if multi_select is not None:
            assert multi_select is True, f"enableMultiSelect 应为 True: {first_options}"

        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "R6:R10")

    def test_get_no_dropdown(self, dws, sheet_node_id, sheet_id):
        """查询无下拉列表的范围，验证 hasDropdown 为 false。"""
        # 先确保 S10:S15 无下拉列表
        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "S10:S15")

        data = dws.run(
            "sheet", "get-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "S10:S15",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("hasDropdown") is not True, f"无下拉范围 hasDropdown 应为 false: {data}"


class TestDeleteDropdown:
    """dws sheet delete-dropdown"""

    def test_delete_existing_dropdown(self, dws, sheet_node_id, sheet_id):
        """设置下拉后删除，验证删除成功。"""
        options = json.dumps([
            {"value": "待删A"},
            {"value": "待删B"},
        ], ensure_ascii=False)
        dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q21:Q25",
            "--options", options,
        )

        data = dws.run(
            "sheet", "delete-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q21:Q25",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        # 验证删除后查询不到下拉列表
        get_data = dws.run(
            "sheet", "get-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "Q21:Q25",
        )
        assert get_data.get("hasDropdown") is not True, (
            f"删除后 hasDropdown 应为 false: {get_data}"
        )

    def test_delete_nonexistent_dropdown(self, dws, sheet_node_id, sheet_id):
        """删除不存在的下拉列表，应仍返回成功。"""
        # 先确保该范围无下拉
        _cleanup_dropdown(dws, sheet_node_id, sheet_id, "S16:S20")

        data = dws.run(
            "sheet", "delete-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "S16:S20",
        )
        assert data.get("success") is True, f"删除不存在的下拉应成功: {data}"


class TestDropdownE2EFlow:
    """端到端流程：设置 → 查询 → 删除"""

    def test_full_lifecycle(self, dws, sheet_node_id, sheet_id):
        """完整流程：设置带颜色多选下拉 → 查询验证 → 删除 → 确认已删除。"""
        target_range = "R11:R20"

        # 1. 设置带颜色的多选下拉
        options = json.dumps([
            {"value": "进行中", "color": "#4285f4"},
            {"value": "已完成", "color": "#0f9d58"},
            {"value": "已取消", "color": "#db4437"},
        ], ensure_ascii=False)
        set_data = dws.run(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", target_range,
            "--options", options,
            "--multi-select",
        )
        assert set_data.get("success") is True, f"set-dropdown 应成功: {set_data}"

        # 2. 查询验证配置
        get_data = dws.run(
            "sheet", "get-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", target_range,
        )
        assert get_data.get("success") is True, f"get-dropdown 应成功: {get_data}"
        assert get_data.get("hasDropdown") is True, f"hasDropdown 应为 True: {get_data}"

        validations = get_data.get("dataValidations", [])
        assert len(validations) >= 1, f"应有下拉配置: {get_data}"
        all_values = []
        for validation in validations:
            all_values.extend(validation.get("conditionValues", []))
        assert "进行中" in all_values, f"应包含 '进行中': {all_values}"
        assert "已完成" in all_values, f"应包含 '已完成': {all_values}"
        assert "已取消" in all_values, f"应包含 '已取消': {all_values}"

        # 3. 删除下拉列表
        del_data = dws.run(
            "sheet", "delete-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", target_range,
        )
        assert del_data.get("success") is True, f"delete-dropdown 应成功: {del_data}"

        # 4. 确认已删除
        verify_data = dws.run(
            "sheet", "get-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", target_range,
        )
        assert verify_data.get("hasDropdown") is not True, (
            f"删除后 hasDropdown 应为 false: {verify_data}"
        )


class TestDropdownError:
    """dws sheet set-dropdown / get-dropdown / delete-dropdown — 错误路径"""

    # ─── set-dropdown 错误 ──────────────────────────────────

    def test_set_missing_node(self, dws):
        """set-dropdown 缺少 --node 应报错。"""
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--sheet-id", "Sheet1",
            "--range", "A1:A5",
            "--options", '[{"value":"x"}]',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_set_missing_sheet_id(self, dws, sheet_node_id):
        """set-dropdown 缺少 --sheet-id 应报错。"""
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--range", "A1:A5",
            "--options", '[{"value":"x"}]',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_set_missing_range(self, dws, sheet_node_id, sheet_id):
        """set-dropdown 缺少 --range 应报错。"""
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--options", '[{"value":"x"}]',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --range 应报错: {result.stdout[:200]}"

    def test_set_missing_options(self, dws, sheet_node_id, sheet_id):
        """set-dropdown 缺少 --options 应报错。"""
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A5",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --options 应报错: {result.stdout[:200]}"

    def test_set_empty_options(self, dws, sheet_node_id, sheet_id):
        """set-dropdown --options 空数组应报错。"""
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A5",
            "--options", "[]",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"空 options 应报错: {result.stdout[:200]}"

    def test_set_invalid_options_json(self, dws, sheet_node_id, sheet_id):
        """set-dropdown --options 非法 JSON 应报错。"""
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A5",
            "--options", "not-json",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"非法 JSON 应报错: {result.stdout[:200]}"

    def test_set_option_with_comma(self, dws, sheet_node_id, sheet_id):
        """set-dropdown 选项值包含英文逗号应报错。"""
        options = json.dumps([{"value": "A,B"}])
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A5",
            "--options", options,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"逗号选项值应报错: {result.stdout[:200]}"

    def test_set_option_empty_value(self, dws, sheet_node_id, sheet_id):
        """set-dropdown 选项缺少 value 应报错。"""
        options = json.dumps([{"color": "#ff0000"}])
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A5",
            "--options", options,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 value 应报错: {result.stdout[:200]}"

    def test_set_invalid_node(self, dws):
        """set-dropdown 无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "set-dropdown",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--range", "A1:A5",
            "--options", '[{"value":"x"}]',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    # ─── get-dropdown 错误 ──────────────────────────────────

    def test_get_missing_node(self, dws):
        """get-dropdown 缺少 --node 应报错。"""
        result = dws.run_raw(
            "sheet", "get-dropdown",
            "--sheet-id", "Sheet1",
            "--range", "A1:A5",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_get_missing_range(self, dws, sheet_node_id, sheet_id):
        """get-dropdown 缺少 --range 应报错。"""
        result = dws.run_raw(
            "sheet", "get-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --range 应报错: {result.stdout[:200]}"

    # ─── delete-dropdown 错误 ───────────────────────────────

    def test_delete_missing_node(self, dws):
        """delete-dropdown 缺少 --node 应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dropdown",
            "--sheet-id", "Sheet1",
            "--range", "A1:A5",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_delete_missing_range(self, dws, sheet_node_id, sheet_id):
        """delete-dropdown 缺少 --range 应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dropdown",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --range 应报错: {result.stdout[:200]}"

    def test_delete_invalid_node(self, dws):
        """delete-dropdown 无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dropdown",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--range", "A1:A5",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"
