"""
test_20_set_style.py — dws sheet range set-style / batch-set-style 集成测试

依赖 conftest.py 自建的测试表格（与 test_04_range 共用 fixture）。

Commands tested:
  1. dws sheet range set-style ... --bg-color / --font-weight / --h-align（单值刷整个 range）
  2. dws sheet range set-style ... --bg-colors-json（逐单元格二维 JSON）
  3. dws sheet range set-style ... --word-wrap（整个 range 共用的单值样式）
  4. dws sheet range set-style 缺样式参数应报错
  5. dws sheet range set-style --*-json 维度与 --range 不一致应报错
  6. dws sheet range batch-set-style --batch <file>（CLI 侧顺序循环）

说明：MCP 侧 update_range 新字段依赖 lippi-doc-solution 预发落地后才会真正生效；
若预发未上线，下面 TestSetStyleSuccess / TestBatchSetStyle 的 happy-path 会返回
服务端错误，此时请在 MCP 部署完成后重跑本文件。纯 CLI 校验（TestSetStyleValidation）
不依赖 MCP，随时可跑。
"""

import json


class TestSetStyleSuccess:
    """happy-path：依赖 MCP 预发落地。"""

    def test_bg_color_and_font_weight_and_h_align(self, dws, sheet_node_id, sheet_id):
        """一次性设置背景色 + 字体粗细 + 水平对齐（单值刷整个 range）。"""
        data = dws.run(
            "sheet", "range", "set-style",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:D1",
            "--bg-color", "#FFF2CC",
            "--font-weight", "bold",
            "--h-align", "center",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_bg_colors_json_per_cell(self, dws, sheet_node_id, sheet_id):
        """逐单元格设置背景色（二维 JSON）。"""
        bg_colors = json.dumps(
            [["#FF0000"], ["#00FF00"], ["#0000FF"]]
        )
        data = dws.run(
            "sheet", "range", "set-style",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "E1:E3",
            "--bg-colors-json", bg_colors,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_word_wrap_only(self, dws, sheet_node_id, sheet_id):
        """仅设置 wordWrap（整个 range 共用的单值样式）。"""
        data = dws.run(
            "sheet", "range", "set-style",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:D5",
            "--word-wrap", "autoWrap",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestSetStyleValidation:
    """CLI 本地校验，不依赖 MCP。"""

    def test_missing_all_style_flags(self, dws, sheet_node_id, sheet_id):
        """--bg-color/--font-*/--h-align/--v-align/--word-wrap/... 全部不传应报错。"""
        result = dws.run_raw(
            "sheet", "range", "set-style",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:B2",
        )
        assert result.returncode != 0, (
            f"缺少所有样式参数应报错: "
            f"returncode={result.returncode}, "
            f"stderr={result.stderr[:200]}"
        )
        combined = (result.stdout + result.stderr).lower()
        assert "至少" in (result.stdout + result.stderr) or "at least" in combined, (
            f"错误信息应提示至少一项: stderr={result.stderr[:300]}"
        )

    def test_bg_colors_json_dimension_mismatch(self, dws, sheet_node_id, sheet_id):
        """--range=A1:B2（2 行 2 列）但 --bg-colors-json 传 3x1 应报错。"""
        bad = json.dumps([["#FF0000"], ["#00FF00"], ["#0000FF"]])
        result = dws.run_raw(
            "sheet", "range", "set-style",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:B2",
            "--bg-colors-json", bad,
        )
        assert result.returncode != 0, (
            f"维度不一致应报错: "
            f"returncode={result.returncode}, "
            f"stderr={result.stderr[:200]}"
        )
        assert "维度" in (result.stdout + result.stderr) or "dimension" in (result.stdout + result.stderr).lower(), (
            f"错误信息应指向维度不一致: stderr={result.stderr[:300]}"
        )

    def test_invalid_enum_h_align(self, dws, sheet_node_id, sheet_id):
        """--h-align 传非法枚举值应报错。"""
        result = dws.run_raw(
            "sheet", "range", "set-style",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1",
            "--h-align", "middle",  # middle 不是 h-align 的合法值（是 v-align 的）
        )
        assert result.returncode != 0, (
            f"非法枚举应报错: "
            f"returncode={result.returncode}, "
            f"stderr={result.stderr[:200]}"
        )

    def test_scalar_and_json_conflict(self, dws, sheet_node_id, sheet_id):
        """--bg-color 和 --bg-colors-json 同时指定应报错。"""
        result = dws.run_raw(
            "sheet", "range", "set-style",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A2",
            "--bg-color", "#FF0000",
            "--bg-colors-json", '[["#00FF00"],["#0000FF"]]',
        )
        assert result.returncode != 0, (
            f"标量与 JSON 冲突应报错: "
            f"returncode={result.returncode}, "
            f"stderr={result.stderr[:200]}"
        )


class TestBatchSetStyle:
    """batch-set-style：CLI 侧顺序循环。"""

    def test_batch_happy_path(self, dws, sheet_node_id, sheet_id, tmp_path):
        """多条不同 range 的样式一次性下发。依赖 MCP 预发落地。"""
        items = [
            {
                "sheetId": sheet_id,
                "range": "A1:D1",
                "bgColor": "#FFF2CC",
                "fontWeight": "bold",
            },
            {
                "sheetId": sheet_id,
                "range": "A2:D5",
                "hAlign": "left",
                "vAlign": "middle",
            },
        ]
        batch_file = tmp_path / "styles.json"
        batch_file.write_text(json.dumps(items), encoding="utf-8")

        result = dws.run_raw(
            "sheet", "range", "batch-set-style",
            "--node", sheet_node_id,
            "--batch", str(batch_file),
        )
        # batch 命令最后一步 update_range 成功即退出 0；预发未上线则 returncode != 0
        assert result.returncode == 0, (
            f"批量样式设置应成功（若预发未上线请忽略）: "
            f"stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )

    def test_batch_empty_file(self, dws, sheet_node_id, tmp_path):
        """批次 JSON 为空数组应报错。"""
        batch_file = tmp_path / "empty.json"
        batch_file.write_text("[]", encoding="utf-8")
        result = dws.run_raw(
            "sheet", "range", "batch-set-style",
            "--node", sheet_node_id,
            "--batch", str(batch_file),
        )
        assert result.returncode != 0, (
            f"空批次应报错: returncode={result.returncode}, "
            f"stderr={result.stderr[:200]}"
        )

    def test_batch_missing_sheet_id(self, dws, sheet_node_id, tmp_path):
        """批次项缺 sheetId，默认 continue-on-error=false 时遇错即停。"""
        items = [
            {"range": "A1", "bgColor": "#FF0000"},  # 缺 sheetId
        ]
        batch_file = tmp_path / "bad.json"
        batch_file.write_text(json.dumps(items), encoding="utf-8")
        result = dws.run_raw(
            "sheet", "range", "batch-set-style",
            "--node", sheet_node_id,
            "--batch", str(batch_file),
        )
        assert result.returncode != 0, (
            f"缺 sheetId 应报错: returncode={result.returncode}, "
            f"stderr={result.stderr[:200]}"
        )

    def test_batch_continue_on_error(self, dws, sheet_node_id, sheet_id, tmp_path):
        """开启 --continue-on-error：坏条目被跳过，跑完后统一返回首错。"""
        items = [
            {"sheetId": sheet_id, "range": "A1"},  # 缺样式字段，校验会挂
            {"sheetId": sheet_id, "range": "B1", "bgColor": "#FFFFFF"},  # 这条合法
        ]
        batch_file = tmp_path / "mix.json"
        batch_file.write_text(json.dumps(items), encoding="utf-8")
        result = dws.run_raw(
            "sheet", "range", "batch-set-style",
            "--node", sheet_node_id,
            "--batch", str(batch_file),
            "--continue-on-error",
        )
        # 即便第 2 条可能成功，整体因为第 1 条失败仍 returncode != 0
        assert result.returncode != 0, (
            f"continue-on-error 时首错应在最终返回: "
            f"returncode={result.returncode}, "
            f"stderr={result.stderr[:300]}"
        )
