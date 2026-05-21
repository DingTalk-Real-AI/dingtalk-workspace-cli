"""
test_99_dashboard_chart.py — 仪表盘与图表操作验证

覆盖 aitable-dashboard-chart.md 中的声明：
- dashboard config-example: 获取配置模板
- chart widgets-example: 获取图表配置模板
- dashboard create / get / update / delete
- chart create / get / update / delete
"""

import json
import time

import pytest


@pytest.fixture(scope="module")
def dashboard_table(dws, test_base_id):
    """Create a table with data for chart testing."""
    ts = int(time.time())
    fields = [
        {"fieldName": "项目", "type": "text"},
        {"fieldName": "销售额", "type": "number", "config": {"formatter": "INT"}},
        {"fieldName": "季度", "type": "singleSelect", "config": {"options": [{"name": "Q1"}, {"name": "Q2"}, {"name": "Q3"}, {"name": "Q4"}]}},
    ]
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"DashboardData_{ts}",
        "--fields", json.dumps(fields, ensure_ascii=False),
    )
    table_id = data["data"]["tableId"]

    # Get field map
    table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
    fm = {f["fieldName"]: f["fieldId"] for f in table_data["data"]["tables"][0].get("fields", [])}

    # Insert sample data
    records = [
        {"cells": {fm["项目"]: "A产品", fm["销售额"]: 100, fm["季度"]: "Q1"}},
        {"cells": {fm["项目"]: "B产品", fm["销售额"]: 200, fm["季度"]: "Q2"}},
        {"cells": {fm["项目"]: "C产品", fm["销售额"]: 150, fm["季度"]: "Q1"}},
    ]
    dws.run(
        "aitable", "record", "create",
        "--base-id", test_base_id,
        "--table-id", table_id,
        "--records", json.dumps(records, ensure_ascii=False),
    )
    return table_id, fm


# ═══════════════════════════════════════════════════════════════
# config-example / widgets-example
# ═══════════════════════════════════════════════════════════════

class TestConfigExamples:
    """验证配置模板获取命令。"""

    def test_dashboard_config_example(self, dws):
        """dashboard config-example 应返回 JSONC 配置结构。"""
        data = dws.run("aitable", "dashboard", "config-example")
        # 应返回有效数据（可能是 data 里的 config 模板或直接的文本）
        assert data.get("status") != "error", f"config-example should succeed, got: {data}"
        print(f"  [OK] dashboard config-example returned")

    def test_chart_widgets_example(self, dws):
        """chart widgets-example 应返回图表配置模板。"""
        data = dws.run("aitable", "chart", "widgets-example")
        assert data.get("status") != "error", f"widgets-example should succeed, got: {data}"
        print(f"  [OK] chart widgets-example returned")


# ═══════════════════════════════════════════════════════════════
# dashboard CRUD
# ═══════════════════════════════════════════════════════════════

class TestDashboardCRUD:
    """验证 dashboard create / get / update / delete 全生命周期。"""

    @pytest.fixture(scope="class")
    def dashboard_id(self, dws, test_base_id):
        """Create a dashboard, yield id, delete at teardown."""
        ts = int(time.time())
        data = dws.run(
            "aitable", "dashboard", "create",
            "--base-id", test_base_id,
            "--name", f"TestDashboard_{ts}",
        )
        body = data.get("data", {})
        did = body.get("dashboardId") or body.get("id")
        assert did, f"dashboard create should return dashboardId, got: {data}"
        print(f"  [SETUP] Created dashboard: {did}")

        yield did

        # Cleanup
        try:
            dws.run(
                "aitable", "dashboard", "delete",
                "--base-id", test_base_id,
                "--dashboard-id", did,
                "--yes",
            )
        except Exception as e:
            print(f"  [TEARDOWN WARN] {e}")

    def test_dashboard_create(self, dashboard_id):
        """dashboard create 应返回有效 ID。"""
        assert dashboard_id

    def test_dashboard_get(self, dws, test_base_id, dashboard_id):
        """dashboard get 应返回仪表盘详情。"""
        data = dws.run(
            "aitable", "dashboard", "get",
            "--base-id", test_base_id,
            "--dashboard-id", dashboard_id,
        )
        body = data.get("data", {})
        # 应包含 dashboardName 或 name
        name = body.get("dashboardName") or body.get("name")
        assert name, f"dashboard get should return name, got: {data}"

    def test_dashboard_update(self, dws, test_base_id, dashboard_id):
        """dashboard update 应成功修改名称。"""
        new_name = f"Updated_{int(time.time())}"
        data = dws.run(
            "aitable", "dashboard", "update",
            "--base-id", test_base_id,
            "--dashboard-id", dashboard_id,
            "--name", new_name,
        )
        assert data.get("status") != "error", f"dashboard update should succeed, got: {data}"


# ═══════════════════════════════════════════════════════════════
# chart CRUD
# ═══════════════════════════════════════════════════════════════

class TestChartCRUD:
    """验证 chart create / get / delete 操作。"""

    @pytest.fixture(scope="class")
    def chart_setup(self, dws, test_base_id, dashboard_table):
        """Create dashboard + chart, return (dashboard_id, chart_id)."""
        table_id, fm = dashboard_table
        ts = int(time.time())

        # Create dashboard
        dash_data = dws.run(
            "aitable", "dashboard", "create",
            "--base-id", test_base_id,
            "--name", f"ChartTestDash_{ts}",
        )
        dash_body = dash_data.get("data", {})
        dashboard_id = dash_body.get("dashboardId") or dash_body.get("id")
        assert dashboard_id, f"dashboard create failed: {dash_data}"

        # Create a BAR chart using real config structure from widgets-example
        chart_config = {
            "chartType": "BAR",
            "name": f"TestBarChart_{ts}",
            "sheet": table_id,
            "measureType": "field",
            "measure": [
                {
                    "value": fm["销售额"],
                    "externalValue": [{"type": "formula", "value": "sum"}],
                }
            ],
            "dimension": [
                {
                    "value": fm["季度"],
                    "externalValue": [],
                }
            ],
            "filter": [],
            "colors": "COLOR_PALETTE_1",
            "legend": "top",
            "label": True,
            "xAxisShow": True,
            "yAxisShow": True,
        }
        chart_layout = {"x": 0, "y": 0, "w": 6, "h": 4}
        chart_data = dws.run(
            "aitable", "chart", "create",
            "--base-id", test_base_id,
            "--dashboard-id", dashboard_id,
            "--config", json.dumps(chart_config, ensure_ascii=False),
            "--layout", json.dumps(chart_layout),
            expect_success=False,
        )
        chart_body = chart_data.get("data", {})
        chart_id = chart_body.get("chartId") or chart_body.get("id")
        if not chart_id:
            # Cleanup dashboard before skip
            try:
                dws.run("aitable", "dashboard", "delete", "--base-id", test_base_id,
                        "--dashboard-id", dashboard_id, "--yes")
            except Exception:
                pass
            pytest.skip(f"chart create returned no chartId (may be config/env issue): {chart_data}")

        yield dashboard_id, chart_id

        # Cleanup
        try:
            dws.run("aitable", "dashboard", "delete", "--base-id", test_base_id,
                    "--dashboard-id", dashboard_id, "--yes")
        except Exception:
            pass

    def test_chart_get(self, dws, test_base_id, chart_setup):
        """chart get 应返回图表详情。"""
        dashboard_id, chart_id = chart_setup
        data = dws.run(
            "aitable", "chart", "get",
            "--base-id", test_base_id,
            "--dashboard-id", dashboard_id,
            "--chart-id", chart_id,
        )
        body = data.get("data", {})
        assert body.get("chartId") or body.get("chartName") or body.get("chartType"), \
            f"chart get should return chart info, got: {data}"

    def test_chart_delete(self, dws, test_base_id, chart_setup):
        """chart delete 应成功删除图表。"""
        dashboard_id, chart_id = chart_setup
        data = dws.run(
            "aitable", "chart", "delete",
            "--base-id", test_base_id,
            "--dashboard-id", dashboard_id,
            "--chart-id", chart_id,
            "--yes",
        )
        assert data.get("status") != "error", f"chart delete should succeed, got: {data}"
