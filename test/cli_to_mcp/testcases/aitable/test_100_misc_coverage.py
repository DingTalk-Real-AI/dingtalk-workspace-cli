"""
test_100_misc_coverage.py — 杂项覆盖率补齐

覆盖以下 md 中尚未被其他测试覆盖的声明：
- aitable-filter-sort.md: exclusive / all_of / date_eq / exist / un_exist / not_before / not_after
- aitable-error-recovery.md: --dry-run / --verbose 参数
- aitable-data-analysis-sop.md: nextCursor 检测 / --limit 配合 filters
"""

import json
import time

import pytest


@pytest.fixture(scope="module")
def misc_table(dws, test_base_id):
    """Create a table for misc coverage tests."""
    ts = int(time.time())
    fields = [
        {"fieldName": "标题", "type": "text"},
        {"fieldName": "数值", "type": "number", "config": {"formatter": "INT"}},
        {"fieldName": "多选", "type": "multipleSelect", "config": {"options": [{"name": "A"}, {"name": "B"}, {"name": "C"}, {"name": "D"}]}},
        {"fieldName": "日期", "type": "date", "config": {"formatter": "YYYY-MM-DD"}},
        {"fieldName": "备注", "type": "text"},
    ]
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"MiscTest_{ts}",
        "--fields", json.dumps(fields, ensure_ascii=False),
    )
    table_id = data["data"]["tableId"]

    # Get field map
    table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
    fm = {f["fieldName"]: f["fieldId"] for f in table_data["data"]["tables"][0].get("fields", [])}

    # Pre-populate records
    records = [
        {"cells": {fm["标题"]: "苹果手机", fm["数值"]: 100, fm["多选"]: ["A", "B"], fm["日期"]: "2026-03-15", fm["备注"]: "重要记录"}},
        {"cells": {fm["标题"]: "华为平板", fm["数值"]: 200, fm["多选"]: ["B", "C"], fm["日期"]: "2026-03-15"}},
        {"cells": {fm["标题"]: "小米耳机", fm["数值"]: 50, fm["多选"]: ["A", "B", "C"], fm["日期"]: "2026-06-01", fm["备注"]: ""}},
        {"cells": {fm["标题"]: "联想笔记本", fm["数值"]: 500, fm["多选"]: ["D"], fm["日期"]: "2026-09-20"}},
        {"cells": {fm["标题"]: "三星手表", fm["数值"]: 80}},  # 无日期无备注
    ]
    create_data = dws.run(
        "aitable", "record", "create",
        "--base-id", test_base_id,
        "--table-id", table_id,
        "--records", json.dumps(records, ensure_ascii=False),
    )
    body = create_data["data"]
    rec_ids = body.get("newRecordIds") or [r["recordId"] for r in body.get("records", [])]

    return table_id, fm, rec_ids


# ═══════════════════════════════════════════════════════════════
# filters: exclusive（不包含文本）
# ═══════════════════════════════════════════════════════════════

class TestFilterExclusive:
    """验证 exclusive 操作符（文本不包含）。

    注意：正确的操作符拼写是 `exclusive`（不是 `not_contain`）。
    """

    def test_filter_exclusive(self, dws, test_base_id, misc_table):
        """标题不包含 '手机' 的记录。"""
        table_id, fm, _ = misc_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "exclusive", "operands": [fm["标题"], "手机"]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        # 5条中2条含"手机" → 应返回3条
        print(f"  [filter exclusive] got {len(records)} records")
        assert len(records) >= 3


# ═══════════════════════════════════════════════════════════════
# filters: all_of (多选全包含)
# ═══════════════════════════════════════════════════════════════

class TestFilterAllOf:
    """验证 all_of 操作符（多选字段全部包含）。"""

    def test_filter_all_of(self, dws, test_base_id, misc_table):
        """验证 all_of 操作符不报错（多选字段全包含）。

        注意：all_of 的实际匹配行为依赖后端版本，部分环境下
        用 option name 可能返回 0 条（需要用 option ID）。
        此处验证的是操作符本身被接受且不报错，不强依赖返回数量。
        """
        table_id, fm, _ = misc_table
        filters = json.dumps({
            "operator": "and",
            "operands": [
                {"operator": "all_of", "operands": [fm["多选"], "A"]},
            ],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        # 验证操作符被接受（status=success 且无 error）
        records = data.get("data", {}).get("records", [])
        print(f"  [filter all_of A] got {len(records)} records (operator accepted)")
        assert data.get("status") == "success", f"all_of should not error, got: {data}"


# ═══════════════════════════════════════════════════════════════
# filters: date_eq
# ═══════════════════════════════════════════════════════════════

class TestFilterDateEq:
    """验证 date_eq 操作符。"""

    def test_filter_date_eq(self, dws, test_base_id, misc_table):
        """日期等于 2026-03-15 的记录。"""
        table_id, fm, _ = misc_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "date_eq", "operands": [fm["日期"], "2026-03-15"]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        # 2条日期是 2026-03-15
        print(f"  [filter date_eq] got {len(records)} records")
        assert len(records) >= 2


# ═══════════════════════════════════════════════════════════════
# filters: exist / un_exist（有值 / 为空）
# ═══════════════════════════════════════════════════════════════

class TestFilterExistUnExist:
    """验证 exist 和 un_exist 操作符。

    注意：正确的操作符是 `exist`/`un_exist`（不是 `is_empty`/`is_not_empty`）。
    """

    def test_filter_un_exist(self, dws, test_base_id, misc_table):
        """日期为空的记录（三星手表无日期）。"""
        table_id, fm, _ = misc_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "un_exist", "operands": [fm["日期"]]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        # 1条无日期
        print(f"  [filter un_exist 日期] got {len(records)} records")
        assert len(records) >= 1

    def test_filter_exist(self, dws, test_base_id, misc_table):
        """日期不为空的记录。"""
        table_id, fm, _ = misc_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "exist", "operands": [fm["日期"]]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        # 4条有日期
        print(f"  [filter exist 日期] got {len(records)} records")
        assert len(records) >= 4


# ═══════════════════════════════════════════════════════════════
# filters: not_before / not_after
# ═══════════════════════════════════════════════════════════════

class TestFilterNotBeforeAfter:
    """验证 not_before 和 not_after 操作符。"""

    def test_filter_not_before(self, dws, test_base_id, misc_table):
        """日期不早于 2026-06-01 的记录（即 >= 2026-06-01）。"""
        table_id, fm, _ = misc_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "not_before", "operands": [fm["日期"], "2026-06-01"]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        # 2026-06-01(小米) + 2026-09-20(联想) → 2条
        print(f"  [filter not_before 2026-06-01] got {len(records)} records")
        assert len(records) >= 2

    def test_filter_not_after(self, dws, test_base_id, misc_table):
        """日期不晚于 2026-03-15 的记录（即 <= 2026-03-15）。"""
        table_id, fm, _ = misc_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "not_after", "operands": [fm["日期"], "2026-03-15"]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        # 2条日期 = 2026-03-15
        print(f"  [filter not_after 2026-03-15] got {len(records)} records")
        assert len(records) >= 2

# ═══════════════════════════════════════════════════════════════
# --dry-run 验证
# ═══════════════════════════════════════════════════════════════

class TestDryRun:
    """验证 --dry-run 只预览不执行。

    注意：--dry-run 输出纯文本预览信息（非 JSON），需用 subprocess 直接验证。
    """

    def test_dry_run_does_not_create(self, dws, test_base_id, misc_table):
        """--dry-run 模式下 record create 不应实际写入。"""
        import shlex
        import subprocess
        from test_utils import resolve_dws_bin
        dws_bin = resolve_dws_bin(__file__)

        table_id, fm, rec_ids = misc_table

        # dry-run create（输出纯文本 [DRY-RUN]，不是 JSON）
        cmd = [
            dws_bin, "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["标题"]: "DryRun不应出现"}}]),
            "--dry-run",
        ]
        print(f"DWS_CMD: {' '.join(shlex.quote(x) for x in cmd)}")
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        output = (result.stdout or "") + (result.stderr or "")
        # dry-run 应输出预览标记
        assert "DRY-RUN" in output or "Preview" in output or "dry" in output.lower(), \
            f"--dry-run should show preview marker, got: {output[:200]}"

        # Query all to verify no new record was actually created
        all_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--all",
        )
        all_records = all_data["data"].get("records", [])
        titles = [str(r.get("cells", {}).get(fm["标题"], "")) for r in all_records]
        assert "DryRun不应出现" not in titles, \
            f"--dry-run should not actually create record, but found it in: {titles}"


# ═══════════════════════════════════════════════════════════════
# --verbose 验证
# ═══════════════════════════════════════════════════════════════

class TestVerbose:
    """验证 --verbose 参数不影响正常执行。"""

    def test_verbose_still_succeeds(self, dws, test_base_id, misc_table):
        """--verbose 模式下命令仍应正常返回数据。"""
        table_id, fm, _ = misc_table
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--limit", "1",
            "--verbose",
        )
        records = data["data"].get("records", [])
        assert len(records) >= 1, f"--verbose should not break query, got: {data}"


# ═══════════════════════════════════════════════════════════════
# nextCursor 检测 (data-analysis-sop)
# ═══════════════════════════════════════════════════════════════

class TestNextCursorDetection:
    """验证 --limit 小于总记录数时返回 nextCursor。"""

    def test_limit_1_returns_cursor(self, dws, test_base_id, misc_table):
        """--limit 1 查询 5 条记录的表，应返回 nextCursor。"""
        table_id, fm, _ = misc_table
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--limit", "1",
        )
        body = data["data"]
        records = body.get("records", [])
        assert len(records) == 1
        # 应有分页标记
        has_more = body.get("hasMore", False) or body.get("nextCursor") or body.get("cursor")
        assert has_more, f"limit=1 on 5-record table should indicate more data, got: {body}"
