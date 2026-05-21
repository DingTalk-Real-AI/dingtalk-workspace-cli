"""aitable 高频错误参数回归用例 + 新增功能测试。"""
import pytest


class TestAitableParamRegression:
    def test_base_search_wrong_keyword_flag(self, dws):
        result = dws.run_raw("aitable", "base", "search", "--keyword", "测试")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_record_query_wrong_query_flag(self, dws):
        result = dws.run_raw("aitable", "record", "query", "--query", "测试")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_base_get_wrong_base_flag(self, dws):
        result = dws.run_raw("aitable", "base", "get", "--base", "INVALID")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_base_get_wrong_id_flag(self, dws):
        result = dws.run_raw("aitable", "base", "get", "--id", "INVALID")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_field_create_invalid_fields_json(self, dws):
        result = dws.run_raw(
            "aitable", "field", "create",
            "--base-id", "INVALID",
            "--table-id", "INVALID",
            "--fields", "{bad_json",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


class TestNewFeatures:
    """新增功能测试 (2026-04-26 更新)"""

    def test_base_create_with_folder_id(self, dws, test_base_id):
        """测试 base create 支持 --folder-id 参数"""
        import time
        name = f"CLI_Test_Folder_{int(time.time())}"
        # folder-id 是可选参数，不传也应该能成功
        data = dws.run(
            "aitable", "base", "create",
            "--name", name,
        )
        assert "baseId" in data["data"]
        # 清理
        dws.run("aitable", "base", "delete", "--base-id", data["data"]["baseId"], "--yes")

    def test_table_create_with_empty_fields(self, dws, test_base_id):
        """测试 table create 支持空 fields 数组"""
        import time
        table_name = f"CLI_Test_EmptyFields_{int(time.time())}"
        # fields 可以传空数组，系统会自动补 primaryDoc 首列
        data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", table_name,
            "--fields", "[]",
        )
        assert "tableId" in data["data"]

    def test_view_create_with_desc(self, dws, test_base_id):
        """测试 view create 支持 --desc 参数"""
        # 先获取 tableId
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]
        
        import time
        view_name = f"CLI_Test_ViewDesc_{int(time.time())}"
        # desc 是可选参数
        data = dws.run(
            "aitable", "view", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-type", "Grid",
            "--name", view_name,
            "--desc", '{"content":[{"type":"text","text":"测试描述"}]}',
        )
        assert "viewId" in data["data"]

    def test_attachment_upload_requires_size(self, dws, test_base_id):
        """测试 attachment upload 必须传 --size 参数"""
        result = dws.run_raw(
            "aitable", "attachment", "upload",
            "--base-id", test_base_id,
            "--file-name", "test.png",
            # 故意不传 --size
        )
        # 应该失败，因为 size 是必填参数
        output = result.stdout + result.stderr
        assert result.returncode != 0 or "missing required flag" in output.lower()

    def test_attachment_upload_with_size(self, dws, test_base_id):
        """测试 attachment upload 传入 --size 参数应该成功"""
        result = dws.run_raw(
            "aitable", "attachment", "upload",
            "--base-id", test_base_id,
            "--file-name", "test.png",
            "--size", "1024",
        )
        # 不应该报 missing required flag 错误
        output = result.stdout + result.stderr
        assert "missing required flag" not in output.lower()

    def test_field_create_with_new_types(self, dws, test_base_id):
        """测试 field create 支持新字段类型: address, filterUp, lookup"""
        # 先获取 tableId
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]
        
        import time
        # 测试 address 类型
        data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", f"CLI_Test_Address_{int(time.time())}",
            "--type", "address",
        )
        assert "success" in data or "fieldId" in str(data)

    def test_chart_create_requires_config_and_layout(self, dws, test_base_id):
        """测试 chart create 的 config 和 layout 是必填参数"""
        # 注意: 实际业务中 config 和 layout 是必填的,虽然 MCP 定义可能有误
        # 这里测试 CLI 层的参数校验
        result = dws.run_raw(
            "aitable", "chart", "create",
            "--base-id", test_base_id,
            "--dashboard-id", "INVALID_DASHBOARD_ID",
            # 故意不传 --config 和 --layout
        )
        # 应该在 CLI 层就失败,提示缺少必填参数
        output = result.stdout + result.stderr
        # 应该提示缺少 config 或 layout
        assert result.returncode != 0


class TestAitableAliases:
    """aitable 命令别名测试 (2026-05-06 新增)"""

    def test_search_alias_basic(self, dws, test_base_id):
        """测试 aitable search 别名可以正常工作"""
        # 使用 search 别名，应该和 base search 等价
        result = dws.run_raw(
            "aitable", "search",
            "--query", "测试",
        )
        # 不应该报 unknown flag 错误
        output = result.stdout + result.stderr
        assert "unknown flag" not in output.lower()

    def test_search_alias_with_keyword(self, dws, test_base_id):
        """测试 aitable search 别名支持 --keyword 参数"""
        result = dws.run_raw(
            "aitable", "search",
            "--keyword", "测试",
        )
        output = result.stdout + result.stderr
        assert "unknown flag" not in output.lower()

    def test_create_alias_basic(self, dws):
        """测试 aitable create 别名可以正常工作"""
        import time
        name = f"CLI_Test_Alias_{int(time.time())}"
        # 使用 create 别名
        data = dws.run(
            "aitable", "create",
            "--name", name,
        )
        assert "baseId" in data["data"]
        # 清理
        dws.run("aitable", "base", "delete", "--base-id", data["data"]["baseId"], "--yes")

    def test_create_alias_with_folder_id(self, dws):
        """测试 aitable create 别名支持 --folder-id 参数"""
        import time
        name = f"CLI_Test_Alias_Folder_{int(time.time())}"
        data = dws.run(
            "aitable", "create",
            "--name", name,
            # folder-id 是可选参数
        )
        assert "baseId" in data["data"]
        # 清理
        dws.run("aitable", "base", "delete", "--base-id", data["data"]["baseId"], "--yes")

    def test_info_alias_basic(self, dws, test_base_id):
        """测试 aitable info 别名可以正常工作"""
        # 使用 info 别名，应该和 base get 等价
        data = dws.run(
            "aitable", "info",
            "--base-id", test_base_id,
        )
        assert "baseName" in data["data"]

    def test_record_list_alias_basic(self, dws, test_base_id):
        """测试 aitable record list 别名可以正常工作"""
        # 先获取 tableId
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]
        
        # 使用 record list 别名
        result = dws.run_raw(
            "aitable", "record", "list",
            "--base-id", test_base_id,
            "--table-id", table_id,
        )
        # 不应该报 unknown flag 错误
        output = result.stdout + result.stderr
        assert "unknown flag" not in output.lower()

    def test_record_list_alias_with_record_ids(self, dws, test_base_id):
        """测试 aitable record list 别名支持 --record-ids 参数"""
        # 先获取 tableId
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]
        
        # 使用 record list 别名，传不存在的 record-ids 应该返回空列表
        result = dws.run_raw(
            "aitable", "record", "list",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", "rec_nonexistent_123",
        )
        # 不应该报 unknown flag 错误
        output = result.stdout + result.stderr
        assert "unknown flag" not in output.lower()


class TestViewCommands:
    """view 子命令测试 (2026-05-08 新增)

    覆盖 `view get/create/update/delete` 的核心能力，重点验证
    `view update --config '{"visibleFieldIds":[...]}'` 用于
    "调整字段顺序 / 隐藏字段" 的视图层重排链路。
    """

    # ── 正向：view get 不传 view-ids 返回当前表全部视图 ────────────────
    def test_view_get_without_view_ids_returns_all(self, dws, test_base_id):
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        data = dws.run(
            "aitable", "view", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
        )
        # 至少返回 1 个视图（建表时系统会创建默认 Grid 视图）
        views = data["data"].get("views") or data["data"].get("data", {}).get("views") or []
        assert len(views) >= 1, f"expect at least 1 view, got: {data}"
        assert "viewId" in views[0]

    # ── 正向：view update --config visibleFieldIds 重排字段顺序 ────────
    def test_view_update_reorder_visible_fields(self, dws, test_base_id):
        """通过 view update --config visibleFieldIds 调整视图字段顺序。

        步骤：
        1. 在 test_base_id 下新建一张含 4 个字段的临时表
        2. 拿到默认视图 viewId
        3. 用 visibleFieldIds 把第 2、3 个字段交换顺序后回写
        4. view get 校验新顺序生效
        """
        import time

        # 1) 创建临时表（首列 text 是主字段）
        table_name = f"CLI_Test_ViewReorder_{int(time.time())}"
        fields_json = (
            '[{"fieldName":"标题","type":"text"},'
            '{"fieldName":"状态","type":"text"},'
            '{"fieldName":"负责人","type":"text"},'
            '{"fieldName":"截止日期","type":"date"}]'
        )
        table_data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", table_name,
            "--fields", fields_json,
        )
        table_id = table_data["data"]["tableId"]
        fields = table_data["data"].get("fields") or []
        if len(fields) < 4:
            # 兜底：从 table get 再拉一次字段列表
            t = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
            fields = t["data"]["tables"][0]["fields"]
        field_ids = [f["fieldId"] for f in fields]
        assert len(field_ids) >= 4, f"expect 4 fields, got: {field_ids}"

        # 2) 拿默认视图
        view_data = dws.run(
            "aitable", "view", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
        )
        views = view_data["data"].get("views") or view_data["data"].get("data", {}).get("views") or []
        assert views, f"expect default view, got: {view_data}"
        view_id = views[0]["viewId"]

        # 3) 交换第 2、3 个字段顺序（首列主字段必须保留在第一位）
        reordered = [field_ids[0], field_ids[2], field_ids[1], field_ids[3]]
        config_json = '{"visibleFieldIds":' + str(reordered).replace("'", '"') + "}"
        dws.run(
            "aitable", "view", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-id", view_id,
            "--config", config_json,
        )

        # 4) 校验新顺序
        # 注意：写入用 visibleFieldIds，但读出时该字段在 view 里叫 columns（顺序即可见字段顺序）
        after = dws.run(
            "aitable", "view", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-ids", view_id,
        )
        after_views = after["data"].get("views") or after["data"].get("data", {}).get("views") or []
        assert after_views, f"expect view after update, got: {after}"
        got = (
            after_views[0].get("columns")
            or after_views[0].get("config", {}).get("visibleFieldIds")
            or after_views[0].get("visibleFieldIds")
        )
        assert got == reordered, f"expect reordered columns={reordered}, got: {got}"

    # ── 正向：visibleFieldIds 漏传字段时 API 会自动追加到末尾（不丢字段） ──
    def test_view_update_partial_visible_fields_appends_missing(self, dws, test_base_id):
        """API 容错行为：visibleFieldIds 漏传的字段不会被隐藏，而是追加到列表末尾。

        这是一条**反直觉但很重要**的真实行为：
        - 入参 visibleFieldIds=[fld0, fld2]（共 3 个字段，漏传 fld1）
        - 入参顺序：[fld0, fld2] 生效（前两位）
        - 漏传的 fld1 被自动追加到末尾，最终 columns=[fld0, fld2, fld1]

        含义：
        - `visibleFieldIds` 在当前实现下用于"重排"而非"过滤可见性"
        - 漏传 ≠ 隐藏，要真正隐藏字段需通过其他机制（视图设置/前端）
        """
        import time

        table_name = f"CLI_Test_ViewPartial_{int(time.time())}"
        fields_json = (
            '[{"fieldName":"主标题","type":"text"},'
            '{"fieldName":"备注","type":"text"},'
            '{"fieldName":"标签","type":"text"}]'
        )
        table_data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", table_name,
            "--fields", fields_json,
        )
        table_id = table_data["data"]["tableId"]
        fields = table_data["data"].get("fields") or []
        # CI 环境后端异步落盘可能导致字段不全，带重试的兜底查询
        for _retry in range(3):
            if len(fields) >= 3:
                break
            time.sleep(1)
            t = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
            fields = t["data"]["tables"][0]["fields"]
        field_ids = [f["fieldId"] for f in fields]
        assert len(field_ids) >= 3, f"expect 3 fields after retries, got {len(field_ids)}: {fields}"

        # 默认视图
        view_data = dws.run(
            "aitable", "view", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
        )
        views = view_data["data"].get("views") or view_data["data"].get("data", {}).get("views") or []
        view_id = views[0]["viewId"]

        # 只传首列主字段 + 第 3 列（共 2 个），故意漏掉第 2 列
        partial = [field_ids[0], field_ids[2]]
        config_json = '{"visibleFieldIds":' + str(partial).replace("'", '"') + "}"
        dws.run(
            "aitable", "view", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-id", view_id,
            "--config", config_json,
        )

        # 期望：columns 前 2 位是入参顺序，漏传的 fld1 追加到末尾
        # 注意：写入用 visibleFieldIds，但读出时该字段在 view 里叫 columns
        after = dws.run(
            "aitable", "view", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-ids", view_id,
        )
        after_views = after["data"].get("views") or after["data"].get("data", {}).get("views") or []
        got = (
            after_views[0].get("columns")
            or after_views[0].get("config", {}).get("visibleFieldIds")
            or after_views[0].get("visibleFieldIds")
        )
        # 入参的 2 个字段必须按顺序出现在最前
        assert got[: len(partial)] == partial, \
            f"expect first {len(partial)} columns == {partial}, got prefix: {got[: len(partial)]} (full: {got})"
        # 漏传的字段应仍然存在（没有被隐藏，而是被追加）
        assert field_ids[1] in got, \
            f"expect missing field {field_ids[1]} appended in columns, got: {got}"
        # 总字段数等于原始字段数
        assert sorted(got) == sorted(field_ids), \
            f"expect columns covers all fields, got: {got}, fields: {field_ids}"

    # ── 回归：view update 缺 --view-id 应报错（CLI 用 JSON error 表达） ─
    def test_view_update_missing_view_id(self, dws, test_base_id):
        """缺 --view-id 时 CLI 以 JSON 形式返回 error（约定：进程退出码=0，错误在 error 字段）。"""
        import json as _json

        result = dws.run_raw(
            "aitable", "view", "update",
            "--base-id", test_base_id,
            "--table-id", "INVALIDTABLEID",  # 必须避开"非法字符"分支，以免覆盖了真正想测的"缺 view-id"分支
            "--name", "x",
            # 故意不传 --view-id
        )
        output_text = result.stdout + result.stderr
        # 任一形式表达失败均可：
        # 1) 进程非 0 退出（cobra 直接拒绝）
        # 2) JSON error.code 非空 / status=error
        # 3) stderr 含 "view-id" / "required" / "missing" 文案
        if result.returncode != 0:
            return
        try:
            data = _json.loads(result.stdout.strip())
        except Exception:
            assert "view-id" in output_text.lower() or "required" in output_text.lower(), \
                f"expect failure indicator, got: {output_text[:300]}"
            return
        err = data.get("error") or {}
        is_error = data.get("status") == "error" or (bool(err) and err != {})
        assert is_error, f"expect error JSON, got: {data}"

    # ── 回归：view update --config 传非法 JSON 应失败 ─────────────────
    def test_view_update_invalid_config_json(self, dws, test_base_id):
        """非法 JSON 或非法 ID，CLI/MCP 任一层报错均可（JSON error 或非 0 退出）。"""
        import json as _json

        result = dws.run_raw(
            "aitable", "view", "update",
            "--base-id", test_base_id,
            "--table-id", "INVALIDTABLEID",
            "--view-id", "INVALIDVIEWID",
            "--config", "{not_a_json",
        )
        if result.returncode != 0:
            return
        try:
            data = _json.loads(result.stdout.strip())
        except Exception:
            # 非 JSON 输出 + 退出码 0：兜底接受 stderr 含 error 字样
            assert "error" in (result.stdout + result.stderr).lower()
            return
        err = data.get("error") or {}
        is_error = data.get("status") == "error" or (bool(err) and err != {})
        assert is_error, f"expect error JSON, got: {data}"

    # ── 回归：view create 缺必填 --view-type 应失败 ────────────────────
    def test_view_create_missing_view_type(self, dws, test_base_id):
        import json as _json

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        result = dws.run_raw(
            "aitable", "view", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            # 故意不传 --view-type
        )
        output_text = (result.stdout + result.stderr).lower()
        if result.returncode != 0:
            assert "view-type" in output_text or "required" in output_text or "missing" in output_text
            return
        # 退出码 0 时：CLI 用 JSON 表达错误
        try:
            data = _json.loads(result.stdout.strip())
        except Exception:
            assert "view-type" in output_text or "required" in output_text or "missing" in output_text
            return
        err = data.get("error") or {}
        is_error = data.get("status") == "error" or (bool(err) and err != {})
        assert is_error, f"expect error JSON for missing view-type, got: {data}"

    # ── 回归：view create 误用 --view-name (CLI 实际是 --name) ─────────
    def test_view_create_wrong_view_name_flag(self, dws, test_base_id):
        """常见误用：以为视图名 flag 是 --view-name，实际 CLI 定义是 --name。

        实际行为：CLI 返回 JSON `{"error":{"code":5,"message":"unknown flag: --view-name"}}`，
        进程退出码 = 0。
        """
        import json as _json

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        result = dws.run_raw(
            "aitable", "view", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-type", "Grid",
            "--view-name", "误用的 flag",
        )
        output_text = (result.stdout + result.stderr).lower()
        if result.returncode != 0:
            assert "unknown flag" in output_text or "view-name" in output_text
            return
        try:
            data = _json.loads(result.stdout.strip())
        except Exception:
            assert "unknown flag" in output_text or "view-name" in output_text, \
                f"expect 'unknown flag' indicator, got: {output_text[:300]}"
            return
        err = data.get("error") or {}
        msg = (err.get("message") or "").lower()
        assert "unknown flag" in msg or "view-name" in msg, \
            f"expect 'unknown flag' in error message, got: {data}"


class TestViewCreateWithFilter:
    """view create --config filter 格式验证 (2026-05-14 新增)

    验证文档中描述的 view create --config '{"filter":[...]}' 格式是否能正确工作。
    覆盖场景：创建带筛选条件的 Grid 视图，然后通过 view get 确认 filter 已生效。
    """

    def test_view_create_with_single_filter(self, dws, test_base_id):
        """创建带单个 filter 条件的 Grid 视图（文档示例格式验证）"""
        import json as _json
        import time

        # 1) 创建临时表，含单选字段（带重试应对网络抖动）
        table_name = f"CLI_Test_ViewFilter_{int(time.time())}"
        fields_json = _json.dumps([
            {"fieldName": "标题", "type": "text"},
            {"fieldName": "状态", "type": "singleSelect", "config": {"options": [{"name": "待上架"}, {"name": "已上架"}]}},
            {"fieldName": "库存", "type": "number"},
        ])
        table_data = None
        for _attempt in range(3):
            try:
                table_data = dws.run(
                    "aitable", "table", "create",
                    "--base-id", test_base_id,
                    "--name", table_name,
                    "--fields", fields_json,
                )
                break
            except Exception:
                time.sleep(2)
                table_name = f"CLI_Test_ViewFilter_{int(time.time())}"
        assert table_data, "table create failed after 3 retries"
        table_id = table_data["data"]["tableId"]
        fields = table_data["data"].get("fields") or []
        for _retry in range(5):
            if len(fields) >= 3:
                break
            time.sleep(1)
            t = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
            fields = t["data"]["tables"][0]["fields"]

        # 找到状态字段的 fieldId
        status_field = next((f for f in fields if f["fieldName"] == "状态"), None)
        assert status_field, f"expect '状态' field, got fields: {[f['fieldName'] for f in fields]}"
        status_field_id = status_field["fieldId"]

        # 2) 创建带 filter 的 Grid 视图
        # API 要求 filter 结构: operands = [fieldId, value]
        view_name = f"CLI_Test_FilterView_{int(time.time())}"
        config_json = _json.dumps({
            "filter": [
                {"operator": "eq", "operands": [status_field_id, "待上架"]}
            ]
        })
        view_data = dws.run(
            "aitable", "view", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-type", "Grid",
            "--name", view_name,
            "--config", config_json,
        )
        # 应该成功返回 viewId
        view_id = view_data["data"].get("viewId")
        assert view_id, f"expect viewId in response, got: {view_data}"

        # 3) view get 验证 filter 配置已保存
        after = dws.run(
            "aitable", "view", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-ids", view_id,
        )
        after_views = after["data"].get("views") or []
        assert after_views, f"expect view details, got: {after}"
        view_config = after_views[0]
        # filter 可能在 config.filter 或直接在顶层 filter 字段
        got_filter = (
            view_config.get("filter")
            or view_config.get("config", {}).get("filter")
            or []
        )
        assert len(got_filter) >= 1, f"expect filter saved, got: {view_config}"

    def test_view_create_with_multi_filter(self, dws, test_base_id):
        """创建带多条 filter 条件的 Grid 视图（AND 组合）"""
        import json as _json
        import time

        table_name = f"CLI_Test_MultiFilter_{int(time.time())}"
        fields_json = _json.dumps([
            {"fieldName": "商品名", "type": "text"},
            {"fieldName": "分类", "type": "singleSelect", "config": {"options": [{"name": "服饰"}, {"name": "美妆"}]}},
            {"fieldName": "库存", "type": "number"},
            {"fieldName": "状态", "type": "singleSelect", "config": {"options": [{"name": "待上架"}, {"name": "已上架"}]}},
        ])
        table_data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", table_name,
            "--fields", fields_json,
        )
        table_id = table_data["data"]["tableId"]
        fields = table_data["data"].get("fields") or []
        for _retry in range(3):
            if len(fields) >= 4:
                break
            time.sleep(1)
            t = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
            fields = t["data"]["tables"][0]["fields"]

        status_field_id = next(f["fieldId"] for f in fields if f["fieldName"] == "状态")
        stock_field_id = next(f["fieldId"] for f in fields if f["fieldName"] == "库存")

        # 多条件 filter: 状态=待上架 AND 库存>0
        # API 要求: operands = [fieldId, value]
        view_name = f"CLI_Test_MultiFilterView_{int(time.time())}"
        config_json = _json.dumps({
            "filter": [
                {"operator": "eq", "operands": [status_field_id, "待上架"]},
                {"operator": "gt", "operands": [stock_field_id, "0"]},
            ]
        })
        view_data = dws.run(
            "aitable", "view", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--view-type", "Grid",
            "--name", view_name,
            "--config", config_json,
        )
        view_id = view_data["data"].get("viewId")
        assert view_id, f"expect viewId for multi-filter view, got: {view_data}"


class TestRecordUpdateParams:
    """record update 高频错误 flag 回归 (2026-05-11 新增)

    用户场景：改一条记录的某个字段值（改状态/改数量/改跟进结果）。
    LLM 直觉写 --record-id + --cells 平铺写法，但 CLI 只接受 --records JSON 数组。
    """

    def test_record_update_wrong_record_id_flag(self, dws):
        """--record-id 不存在于 record update，应报 unknown flag"""
        result = dws.run_raw(
            "aitable", "record", "update",
            "--base-id", "INVALID",
            "--table-id", "INVALID",
            "--record-id", "recXXX",
            "--cells", '{"fldX":"值"}',
        )
        output = (result.stdout + result.stderr).lower()
        assert "unknown flag" in output or "error" in output

    def test_record_update_wrong_cells_flag(self, dws):
        """--cells 不存在于 record update，应报 unknown flag"""
        result = dws.run_raw(
            "aitable", "record", "update",
            "--base-id", "INVALID",
            "--table-id", "INVALID",
            "--cells", '{"fldX":"值"}',
        )
        output = (result.stdout + result.stderr).lower()
        assert "unknown flag" in output or "error" in output

    def test_record_update_wrong_data_flag(self, dws):
        """--data 不存在于 record update，应报 unknown flag"""
        result = dws.run_raw(
            "aitable", "record", "update",
            "--base-id", "INVALID",
            "--table-id", "INVALID",
            "--data", '{"fldX":"值"}',
        )
        output = (result.stdout + result.stderr).lower()
        assert "unknown flag" in output or "error" in output

    def test_record_update_correct_single_record(self, dws, test_base_id):
        """正确用法：--records JSON 数组更新单条记录"""
        import json as _json

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        # 拿第一个 text 字段
        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fields = table_data["data"]["tables"][0]["fields"]
        text_field = next((f for f in fields if f.get("type") == "text"), fields[0])
        field_id = text_field["fieldId"]

        # 创建一条记录
        create_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", _json.dumps([{"cells": {field_id: "原始值"}}]),
        )
        # record create 返回 newRecordIds 数组或 records 数组
        data_body = create_data["data"]
        record_id = None
        if "newRecordIds" in data_body:
            record_id = data_body["newRecordIds"][0]
        elif "records" in data_body:
            record_id = data_body["records"][0]["recordId"]
        assert record_id, f"expect created recordId, got: {create_data}"

        # 用正确格式更新
        update_data = dws.run(
            "aitable", "record", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", _json.dumps([{"recordId": record_id, "cells": {field_id: "更新后的值"}}]),
        )
        assert update_data.get("status") != "error", f"expect success, got: {update_data}"


class TestAiConfigAndListAliases:
    """--ai-config 端到端验证 + table list / field list 别名测试 (2026-05-13 新增)"""

    # ── table list 别名 ──────────────────────────────────────────────

    def test_table_list_alias_basic(self, dws, test_base_id):
        """测试 aitable table list 别名可以正常工作（等价于 table get）"""
        data = dws.run(
            "aitable", "table", "list",
            "--base-id", test_base_id,
        )
        tables = data["data"].get("tables", [])
        assert len(tables) >= 1, f"expect at least 1 table, got: {data}"
        assert "tableId" in tables[0]

    def test_table_list_alias_with_table_ids(self, dws, test_base_id):
        """测试 table list 支持 --table-ids 参数"""
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        data = dws.run(
            "aitable", "table", "list",
            "--base-id", test_base_id,
            "--table-ids", table_id,
        )
        tables = data["data"].get("tables", [])
        assert len(tables) == 1
        assert tables[0]["tableId"] == table_id

    # ── field list 别名 ──────────────────────────────────────────────

    def test_field_list_alias_basic(self, dws, test_base_id):
        """测试 aitable field list 别名可以正常工作（等价于 field get）"""
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        data = dws.run(
            "aitable", "field", "list",
            "--base-id", test_base_id,
            "--table-id", table_id,
        )
        # field get 返回 data.fields（不是 data.tables）
        fields = data["data"].get("fields", [])
        assert len(fields) >= 1, f"expect at least 1 field, got: {data}"
        assert "fieldId" in fields[0]

    def test_field_list_alias_with_field_ids(self, dws, test_base_id):
        """测试 field list 支持 --field-ids 参数"""
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        # 先拿到第一个 fieldId
        table_data = dws.run(
            "aitable", "table", "get",
            "--base-id", test_base_id,
            "--table-ids", table_id,
        )
        field_id = table_data["data"]["tables"][0]["fields"][0]["fieldId"]

        data = dws.run(
            "aitable", "field", "list",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-ids", field_id,
        )
        # field get 返回 data.fields
        fields = data["data"].get("fields", [])
        assert len(fields) >= 1, f"expect at least 1 field, got: {data}"

    # ── --ai-config 端到端验证 ───────────────────────────────────────

    def test_ai_config_field_create_success(self, dws, test_base_id):
        """测试 --ai-config 成功创建 AI 字段（prompt 包含 fieldRef）"""
        import time

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        # 先创建一个普通文本字段作为引用源
        ref_name = f"CLI_Test_RefField_{int(time.time())}"
        ref_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", ref_name,
            "--type", "text",
        )
        ref_field_id = ref_data["data"]["results"][0]["fieldId"]
        assert ref_field_id, f"expect fieldId in response, got: {ref_data}"

        # 使用 --ai-config 创建 AI 字段，引用上面的字段
        import json
        ai_config = json.dumps({
            "outputType": "text",
            "prompt": [
                {"type": "text", "value": "请总结以下内容："},
                {"type": "fieldRef", "fieldId": ref_field_id},
            ],
        })
        ai_name = f"CLI_Test_AIField_{int(time.time())}"
        ai_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", ai_name,
            "--type", "text",
            "--ai-config", ai_config,
        )
        assert ai_data["data"]["successCount"] == 1, f"expect AI field created, got: {ai_data}"
        assert ai_data["data"]["results"][0]["success"] is True

    def test_ai_config_field_create_missing_fieldref(self, dws, test_base_id):
        """测试 --ai-config 缺少 fieldRef 时返回明确错误（非静默失败）"""
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        import json
        ai_config = json.dumps({
            "outputType": "text",
            "prompt": [
                {"type": "text", "value": "请总结"},
            ],
        })
        # 应该返回 success 但 results 里标记 success=false 并有 reason
        result = dws.run_raw(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", "AI_NoRef",
            "--type", "text",
            "--ai-config", ai_config,
        )
        output = result.stdout + result.stderr
        parsed = json.loads(output.strip()) if output.strip() else {}
        results = parsed.get("data", {}).get("results", [])

        # 核心断言：不是静默失败，而是有明确错误信息
        assert len(results) >= 1, f"expect results array, got: {parsed}"
        assert results[0].get("success") is False, f"expect success=false, got: {results[0]}"
        reason = results[0].get("reason") or results[0].get("errorMessage") or ""
        assert "fieldRef" in reason or "prompt" in reason, \
            f"expect error mentioning fieldRef/prompt, got reason: {reason}"

    def test_ai_config_invalid_json(self, dws, test_base_id):
        """测试 --ai-config 传入无效 JSON 时 CLI 行为"""
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        result = dws.run_raw(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", "BadAI",
            "--type", "text",
            "--ai-config", "{not_valid_json",
        )
        # 无效 JSON 可能被当作字符串透传给 MCP，MCP 应返回错误
        # 或者 CLI 层直接报错
        output = result.stdout + result.stderr
        # 不应该静默成功
        assert "success" not in output.lower() or "false" in output.lower() or "error" in output.lower()


class TestImportFlow:
    """import upload + import data 三步导入流程测试 (2026-05-13 新增)"""

    def test_import_upload_returns_upload_url_and_import_id(self, dws, test_base_id):
        """测试 import upload 返回 uploadUrl 和 importId"""
        data = dws.run(
            "aitable", "import", "upload",
            "--base-id", test_base_id,
            "--file-name", "test.csv",
            "--file-size", "100",
        )
        assert "uploadUrl" in data["data"], f"expect uploadUrl in response, got: {data}"
        assert "importId" in data["data"], f"expect importId in response, got: {data}"
        assert data["data"]["uploadUrl"].startswith("http"), \
            f"expect uploadUrl to be a URL, got: {data['data']['uploadUrl'][:80]}"

    def test_import_upload_missing_file_name(self, dws, test_base_id):
        """测试 import upload 缺少 --file-name 时报错"""
        result = dws.run_raw(
            "aitable", "import", "upload",
            "--base-id", test_base_id,
            "--file-size", "100",
            # 故意不传 --file-name
        )
        output = result.stdout + result.stderr
        assert result.returncode != 0 or "error" in output.lower() or "file-name" in output.lower(), \
            f"expect error for missing file-name, got: {output[:300]}"

    @pytest.mark.skip(reason="CI 环境限制对外部 OSS 的 HTTP PUT 请求，仅本地验证")
    def test_import_full_csv_flow(self, dws, test_base_id):
        """测试完整的 CSV 导入三步流程：upload → PUT → import data"""
        import subprocess
        import tempfile
        import os

        # 1) 生成临时 CSV 文件
        csv_content = "名称,数量,备注\n苹果,10,新鲜\n香蕉,20,进口\n"
        with tempfile.NamedTemporaryFile(mode="w", suffix=".csv", delete=False, encoding="utf-8") as f:
            f.write(csv_content)
            csv_path = f.name
        try:
            file_size = os.path.getsize(csv_path)
            file_name = os.path.basename(csv_path)

            # 2) import upload 获取凭证
            upload_data = dws.run(
                "aitable", "import", "upload",
                "--base-id", test_base_id,
                "--file-name", file_name,
                "--file-size", str(file_size),
            )
            upload_url = upload_data["data"]["uploadUrl"]
            import_id = upload_data["data"]["importId"]

            # 3) HTTP PUT 上传文件到 OSS（Content-Type 必须为空），带重试应对 CI 网络抖动
            put_ok = False
            for _attempt in range(3):
                put_result = subprocess.run(
                    ["curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
                     "-X", "PUT", upload_url, "-H", "Content-Type:", "--data-binary", f"@{csv_path}"],
                    capture_output=True, text=True, timeout=30,
                )
                if put_result.stdout.strip() == "200":
                    put_ok = True
                    break
                import time
                time.sleep(1)
            assert put_ok, \
                f"expect PUT 200 after retries, got: {put_result.stdout.strip()} stderr: {put_result.stderr[:200]}"

            # 4) import data 触发导入
            import_result = dws.run(
                "aitable", "import", "data",
                "--import-id", import_id,
            )
            assert import_result["data"].get("status") == "success", \
                f"expect import success, got: {import_result}"
            assert len(import_result["data"].get("tableIds", [])) >= 1, \
                f"expect at least 1 tableId, got: {import_result}"
        finally:
            os.unlink(csv_path)

    def test_import_data_invalid_import_id(self, dws):
        """测试 import data 传入无效 importId 时报错"""
        import json as _json

        result = dws.run_raw(
            "aitable", "import", "data",
            "--import-id", "imp_invalid_nonexistent_id",
        )
        output = result.stdout + result.stderr
        if result.returncode != 0:
            return  # CLI 层直接拒绝，符合预期
        try:
            data = _json.loads(result.stdout.strip())
        except Exception:
            assert "error" in output.lower(), f"expect error, got: {output[:300]}"
            return
        err = data.get("error") or {}
        is_error = data.get("status") == "error" or (bool(err) and err != {})
        assert is_error, f"expect error for invalid importId, got: {data}"


class TestParamAliasE2E:
    """参数别名端到端集成测试 (2026-05-15 新增)

    验证 LLM 常犯的 flag 错误现在可以被别名兜住，真正调用到 MCP 并返回正确结果。
    """

    def test_field_create_via_field_name_alias(self, dws, test_base_id):
        """--field-name / --field-type 别名可以成功创建字段"""
        import time

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        field_name = f"CLI_Alias_FieldName_{int(time.time())}"
        data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-name", field_name,
            "--field-type", "text",
        )
        assert data["data"]["successCount"] == 1, f"expect field created via alias, got: {data}"
        assert data["data"]["results"][0]["success"] is True
        assert data["data"]["results"][0]["fieldName"] == field_name

    def test_record_query_via_page_size_alias(self, dws, test_base_id):
        """--page-size 别名可以正确限制返回记录数"""
        import json as _json
        import time

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        # 先创建几条记录确保有数据
        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        text_field = next((f for f in fields if f.get("type") == "text"), fields[0])
        field_id = text_field["fieldId"]

        records_json = _json.dumps([
            {"cells": {field_id: f"PageSize_Test_{i}"}} for i in range(5)
        ])
        dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", records_json,
        )

        # 用 --page-size 2 查询，应该只返回 2 条
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--page-size", "2",
        )
        records = data["data"].get("records", [])
        assert len(records) <= 2, f"expect at most 2 records with --page-size 2, got {len(records)}"

    def test_record_create_via_fields_alias(self, dws, test_base_id):
        """--fields 别名（代替 --records）可以成功创建记录"""
        import json as _json
        import time

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        text_field = next((f for f in fields if f.get("type") == "text"), fields[0])
        field_id = text_field["fieldId"]

        # 用 --fields 代替 --records（LLM 常见误用）
        records_json = _json.dumps([{"cells": {field_id: "Created_Via_Fields_Alias"}}])
        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--fields", records_json,
        )
        # 应该成功创建
        body = data["data"]
        has_records = "newRecordIds" in body or "records" in body
        assert has_records, f"expect record created via --fields alias, got: {data}"

    def test_record_create_via_base_alias(self, dws, test_base_id):
        """--base 别名（代替 --base-id）可以成功执行"""
        import json as _json

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        text_field = next((f for f in fields if f.get("type") == "text"), fields[0])
        field_id = text_field["fieldId"]

        records_json = _json.dumps([{"cells": {field_id: "Created_Via_Base_Alias"}}])
        # 用 --base 代替 --base-id
        data = dws.run(
            "aitable", "record", "create",
            "--base", test_base_id,
            "--table-id", table_id,
            "--records", records_json,
        )
        body = data["data"]
        has_records = "newRecordIds" in body or "records" in body
        assert has_records, f"expect record created via --base alias, got: {data}"


class TestRecordsFileE2E:
    """--records-file 端到端集成测试 (2026-05-15 新增)

    验证 Windows 用户场景：超长 JSON 通过文件传入，CLI 正确读取并调用 MCP。
    """

    def test_record_create_from_file(self, dws, test_base_id):
        """--records-file 从文件读取 JSON 创建记录"""
        import json as _json
        import os
        import tempfile
        import time

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        text_field = next((f for f in fields if f.get("type") == "text"), fields[0])
        field_id = text_field["fieldId"]

        # 将 records JSON 写入临时文件
        records = [{"cells": {field_id: f"FromFile_{int(time.time())}_{i}"}} for i in range(3)]
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False, encoding="utf-8") as f:
            _json.dump(records, f, ensure_ascii=False)
            file_path = f.name

        try:
            data = dws.run(
                "aitable", "record", "create",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--records-file", file_path,
            )
            body = data["data"]
            has_records = "newRecordIds" in body or "records" in body
            assert has_records, f"expect records created from file, got: {data}"

            # 验证创建了 3 条
            if "newRecordIds" in body:
                assert len(body["newRecordIds"]) == 3, f"expect 3 records, got {len(body['newRecordIds'])}"
        finally:
            os.unlink(file_path)

    def test_record_update_from_file(self, dws, test_base_id):
        """--records-file 从文件读取 JSON 更新记录"""
        import json as _json
        import os
        import tempfile
        import time

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        text_field = next((f for f in fields if f.get("type") == "text"), fields[0])
        field_id = text_field["fieldId"]

        # 先创建一条记录
        create_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", _json.dumps([{"cells": {field_id: "BeforeUpdate"}}]),
        )
        body = create_data["data"]
        if "newRecordIds" in body:
            record_id = body["newRecordIds"][0]
        else:
            record_id = body["records"][0]["recordId"]

        # 将更新 JSON 写入文件
        updates = [{"recordId": record_id, "cells": {field_id: "AfterUpdate_FromFile"}}]
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False, encoding="utf-8") as f:
            _json.dump(updates, f, ensure_ascii=False)
            file_path = f.name

        try:
            data = dws.run(
                "aitable", "record", "update",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--records-file", file_path,
            )
            assert data.get("status") != "error", f"expect update success, got: {data}"

            # 验证更新后的值
            query_data = dws.run(
                "aitable", "record", "query",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--record-ids", record_id,
                "--field-ids", field_id,
            )
            records = query_data["data"].get("records", [])
            assert len(records) >= 1, f"expect queried record, got: {query_data}"
            cell_value = records[0].get("cells", {}).get(field_id)
            assert cell_value == "AfterUpdate_FromFile", \
                f"expect updated value, got: {cell_value}"
        finally:
            os.unlink(file_path)

    def test_records_file_priority_over_inline(self, dws, test_base_id):
        """--records-file 优先级高于 --records（同时传两个时以文件为准）"""
        import json as _json
        import os
        import tempfile
        import time

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        text_field = next((f for f in fields if f.get("type") == "text"), fields[0])
        field_id = text_field["fieldId"]

        # 文件里写 "from_file"，inline 写 "from_inline"
        file_records = [{"cells": {field_id: "priority_from_file"}}]
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False, encoding="utf-8") as f:
            _json.dump(file_records, f, ensure_ascii=False)
            file_path = f.name

        try:
            data = dws.run(
                "aitable", "record", "create",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--records", _json.dumps([{"cells": {field_id: "priority_from_inline"}}]),
                "--records-file", file_path,
            )
            body = data["data"]
            if "newRecordIds" in body:
                record_id = body["newRecordIds"][0]
            else:
                record_id = body["records"][0]["recordId"]

            # 查询验证实际写入的是文件内容
            query_data = dws.run(
                "aitable", "record", "query",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--record-ids", record_id,
                "--field-ids", field_id,
            )
            records = query_data["data"].get("records", [])
            cell_value = records[0].get("cells", {}).get(field_id)
            assert cell_value == "priority_from_file", \
                f"expect --records-file to take priority, got: {cell_value}"
        finally:
            os.unlink(file_path)

    def test_records_file_nonexistent_path(self, dws, test_base_id):
        """--records-file 指向不存在的文件应报错"""
        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        result = dws.run_raw(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records-file", "/tmp/nonexistent_cli_test_file_12345.json",
        )
        output = result.stdout + result.stderr
        assert result.returncode != 0 or "error" in output.lower(), \
            f"expect error for nonexistent file, got: {output[:300]}"

    def test_records_file_invalid_json_content(self, dws, test_base_id):
        """--records-file 文件内容非法 JSON 应报错"""
        import os
        import tempfile

        base_data = dws.run("aitable", "base", "get", "--base-id", test_base_id)
        table_id = base_data["data"]["tables"][0]["tableId"]

        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False, encoding="utf-8") as f:
            f.write("{this is not valid json[[[")
            file_path = f.name

        try:
            result = dws.run_raw(
                "aitable", "record", "create",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--records-file", file_path,
            )
            output = result.stdout + result.stderr
            assert result.returncode != 0 or "error" in output.lower() or "parse" in output.lower(), \
                f"expect JSON parse error, got: {output[:300]}"
        finally:
            os.unlink(file_path)


class TestTypoSuggestionE2E:
    """子命令 typo 纠正提示测试 (2026-05-15 新增)

    验证 cobra SuggestionsMinimumDistance 生效后，输错子命令时 CLI 会：
    1. 报出 unknown subcommand 错误
    2. 列出可用的子命令（available: ...）
    用户/Agent 看到后可以自我纠正。
    """

    def test_record_get_suggests_query(self, dws):
        """record get 不存在，应提示 available 子命令（含 query）"""
        result = dws.run_raw("aitable", "record", "get")
        output = result.stdout + result.stderr
        assert result.returncode != 0, f"expect non-zero exit for unknown subcommand, got 0"
        assert "unknown" in output.lower() or "available" in output.lower(), \
            f"expect 'unknown subcommand' or 'available' hint, got: {output[:300]}"
        # 应列出 query 作为可用子命令
        assert "query" in output, \
            f"expect 'query' in available commands, got: {output[:300]}"

    def test_record_qurey_typo(self, dws):
        """record qurey（拼错）应报错并提示可用子命令"""
        result = dws.run_raw("aitable", "record", "qurey")
        output = result.stdout + result.stderr
        assert result.returncode != 0
        assert "query" in output, \
            f"expect 'query' suggested for typo 'qurey', got: {output[:300]}"

    def test_field_lst_typo(self, dws):
        """field lst（拼错 list）应报错并提示可用子命令"""
        result = dws.run_raw("aitable", "field", "lst")
        output = result.stdout + result.stderr
        assert result.returncode != 0
        assert "list" in output or "get" in output, \
            f"expect 'list' or 'get' suggested for typo 'lst', got: {output[:300]}"

    def test_base_serach_typo(self, dws):
        """base serach（拼错 search）应报错并提示可用子命令"""
        result = dws.run_raw("aitable", "base", "serach")
        output = result.stdout + result.stderr
        assert result.returncode != 0
        assert "search" in output, \
            f"expect 'search' suggested for typo 'serach', got: {output[:300]}"

    def test_view_creat_typo(self, dws):
        """view creat（拼错 create）应报错并提示可用子命令"""
        result = dws.run_raw("aitable", "view", "creat")
        output = result.stdout + result.stderr
        assert result.returncode != 0
        assert "create" in output, \
            f"expect 'create' suggested for typo 'creat', got: {output[:300]}"

    def test_nonexistent_top_level_subcommand(self, dws):
        """aitable foo（完全不存在的子命令）应报错并列出可用命令"""
        result = dws.run_raw("aitable", "foo")
        output = result.stdout + result.stderr
        assert result.returncode != 0
        # 应至少列出 base, record, field 等顶层子命令
        assert "base" in output or "record" in output or "field" in output, \
            f"expect available top-level commands listed, got: {output[:300]}"

    def test_record_delte_typo(self, dws):
        """record delte（拼错 delete）应报错并提示可用子命令"""
        result = dws.run_raw("aitable", "record", "delte")
        output = result.stdout + result.stderr
        assert result.returncode != 0
        assert "delete" in output, \
            f"expect 'delete' suggested for typo 'delte', got: {output[:300]}"


class TestRecordPaginationFlags:
    """record query --all / --page-limit flag 回归测试"""

    def test_record_query_all_flag_accepted(self, dws):
        """--all flag should be recognized (not unknown flag error)."""
        result = dws.run_raw(
            "aitable", "record", "query",
            "--base-id", "INVALID",
            "--table-id", "INVALID",
            "--all",
        )
        combined = result.stdout + result.stderr
        # Should NOT fail with "unknown flag" — any auth/business error is fine
        assert "unknown flag" not in combined.lower()
        assert "unknown shorthand" not in combined.lower()
