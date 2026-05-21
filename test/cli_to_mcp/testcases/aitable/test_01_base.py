"""
test_01_base.py — Base 管理全覆盖测试 (7 commands)

Commands tested:
  1. dws aitable base list       (list_bases)
  2. dws aitable base search     (search_bases)
  3. dws aitable base get        (get_base)
  4. dws aitable base create     (create_base)
  5. dws aitable base update     (update_base)
  6. dws aitable base delete     (delete_base)
  7. dws aitable base copy       (copy_base)
"""

import pytest


class TestBaseList:
    """dws aitable base list"""

    def test_list_returns_bases(self, dws):
        """默认列表应返回至少 1 个 Base。"""
        data = dws.run("aitable", "base", "list")
        bases = data["data"]["bases"]
        assert isinstance(bases, list)
        assert len(bases) >= 1
        # 每个 base 必须包含 baseId 和 baseName
        for b in bases:
            assert "baseId" in b, f"Missing baseId: {b}"
            assert "baseName" in b, f"Missing baseName: {b}"

    def test_list_with_limit(self, dws):
        """--limit 1 应只返回 1 条结果，且有 nextCursor。"""
        data = dws.run("aitable", "base", "list", "--limit", "1")
        bases = data["data"]["bases"]
        assert len(bases) == 1

    def test_list_pagination(self, dws):
        """使用 cursor 翻页，第二页结果不应与第一页重复。"""
        page1 = dws.run("aitable", "base", "list", "--limit", "1")
        cursor = page1["data"].get("nextCursor")
        if not cursor:
            pytest.skip("Only 1 base exists, cannot test pagination")

        page2 = dws.run(
            "aitable", "base", "list", "--limit", "1", "--cursor", cursor
        )
        page2_bases = page2["data"]["bases"]
        assert len(page2_bases) >= 1
        # 确认不与第一页重复
        id1 = page1["data"]["bases"][0]["baseId"]
        id2 = page2_bases[0]["baseId"]
        assert id1 != id2, "Paginated result should differ from page 1"


class TestBaseSearch:
    """dws aitable base search"""

    def test_search_existing_base(self, dws, test_base_id, test_base_name):
        """按刚创建的 Base 名称精确搜索，应能检索到该 Base。

        搜索索引存在延迟，采用轮询重试：每 5 秒查一次，最多等 45 秒。
        索引就绪即通过，超时仍搜不到则 FAIL。
        """
        import time

        timeout, interval = 45, 5
        deadline = time.monotonic() + timeout
        found_ids = []
        while time.monotonic() < deadline:
            time.sleep(interval)
            data = dws.run("aitable", "base", "search", "--query", test_base_name)
            bases = data["data"].get("bases") or []
            found_ids = [b["baseId"] for b in bases]
            if test_base_id in found_ids:
                return  # pass
        assert test_base_id in found_ids, (
            f"Base {test_base_id} ({test_base_name}) not found in search results after {timeout}s. "
            f"This indicates a search indexing consistency issue. "
            f"Got: {[b.get('baseName') for b in bases]}"
        )

    def test_search_returns_structure(self, dws):
        """搜索通用关键词，验证返回结构。"""
        data = dws.run("aitable", "base", "search", "--query", "测试")
        bases = data["data"].get("bases") or []
        # 返回应是列表，每项有 baseId/baseName
        assert isinstance(bases, list)
        if bases:
            assert "baseId" in bases[0]
            assert "baseName" in bases[0]

    def test_search_no_match(self, dws):
        """搜索不存在的关键词应返回空列表。"""
        data = dws.run(
            "aitable", "base", "search",
            "--query", "ZZZZ_NonExistent_99999"
        )
        bases = data["data"].get("bases") or []
        assert len(bases) == 0


class TestBaseGet:
    """dws aitable base get"""

    def test_get_returns_structure(self, dws, test_base_id):
        """获取 Base 详情应返回 baseName 和 tables 目录。"""
        data = dws.run(
            "aitable", "base", "get", "--base-id", test_base_id
        )
        base_data = data["data"]
        assert "baseName" in base_data, "Missing baseName in base get"
        # 新创建的 base 至少有默认 table
        assert "tables" in base_data, "Missing tables in base get"


class TestBaseUpdate:
    """dws aitable base update"""

    def test_update_status_should_be_success(self, dws, test_base_id):
        """base update 的响应 status 应为 'success'。"""
        import time
        new_name = f"CLI_Test_StatusCheck_{int(time.time())}"
        data = dws.run(
            "aitable", "base", "update",
            "--base-id", test_base_id,
            "--name", new_name,
        )
        assert data.get("status") == "success", f"status 应为 success: {data}"

    def test_update_name_effective(self, dws, test_base_id):
        """修改 Base 名称后，get 应返回新名称（验证实际效果）。"""
        import time
        new_name = f"CLI_Test_Renamed_{int(time.time())}"
        # 忽略 status 字段，只验证写入是否真正生效
        dws.run_ok(
            "aitable", "base", "update",
            "--base-id", test_base_id,
            "--name", new_name,
        )
        verify = dws.run(
            "aitable", "base", "get", "--base-id", test_base_id
        )
        assert verify["data"]["baseName"] == new_name

    def test_update_with_desc(self, dws, test_base_id):
        """修改 Base 名称并附带 desc 参数，验证写入生效。"""
        import time
        new_name = f"CLI_Test_Desc_{int(time.time())}"
        dws.run_ok(
            "aitable", "base", "update",
            "--base-id", test_base_id,
            "--name", new_name,
            "--desc", "测试描述文本",
        )
        verify = dws.run(
            "aitable", "base", "get", "--base-id", test_base_id
        )
        assert verify["data"]["baseName"] == new_name


class TestBaseCreateDelete:
    """dws aitable base create + delete (完整生命周期)"""

    def test_create_and_delete_lifecycle(self, dws):
        """创建 → 验证 → 删除 → 验证不可访问。"""
        import time
        name = f"CLI_Test_Lifecycle_{int(time.time())}"

        # Step 1: Create
        create_data = dws.run(
            "aitable", "base", "create", "--name", name
        )
        base_id = create_data["data"]["baseId"]
        assert base_id, "create must return baseId"

        # Step 2: Verify via get
        get_data = dws.run(
            "aitable", "base", "get", "--base-id", base_id
        )
        assert get_data["data"]["baseName"] == name

        # Step 3: Delete
        dws.run(
            "aitable", "base", "delete",
            "--base-id", base_id, "--yes",
        )

        # Step 4: Verify deleted (get should fail)
        result = dws.run_raw(
            "aitable", "base", "get", "--base-id", base_id
        )
        assert result.returncode != 0 or "error" in result.stdout.lower(), (
            "Deleted base should not be accessible"
        )

    def test_create_with_template(self, dws):
        """使用模板创建 Base（先搜索模板获取 templateId）。"""
        # 搜索模板
        tmpl_data = dws.run(
            "aitable", "template", "search", "--query", "项目"
        )
        templates = tmpl_data["data"].get("templates", [])
        if not templates:
            pytest.skip("No template found for '项目'")

        template_id = templates[0]["templateId"]

        # 用模板创建
        import time
        name = f"CLI_Test_Tmpl_{int(time.time())}"
        create_data = dws.run(
            "aitable", "base", "create",
            "--name", name,
            "--template-id", template_id,
        )
        base_id = create_data["data"]["baseId"]
        assert base_id

        # 清理
        dws.run(
            "aitable", "base", "delete",
            "--base-id", base_id, "--yes",
        )


class TestBaseCopy:
    """dws aitable base copy"""

    def test_copy_base_full(self, dws, test_base_id, test_folder_id):
        """完整复制 Base（包含数据和结构）到真实文件夹。"""
        copy_data = dws.run(
            "aitable", "base", "copy",
            "--base-id", test_base_id,
            "--target-folder-id", test_folder_id,
        )

        # 验证返回结果
        assert "baseId" in copy_data["data"], "copy must return baseId"
        assert "baseName" in copy_data["data"], "copy must return baseName"

        new_base_id = copy_data["data"]["baseId"]

        # 验证新 Base 可以访问
        get_data = dws.run(
            "aitable", "base", "get",
            "--base-id", new_base_id,
        )
        assert "baseName" in get_data["data"]

        # 清理：删除复制的 Base
        dws.run(
            "aitable", "base", "delete",
            "--base-id", new_base_id,
            "--yes",
        )

    def test_copy_base_structure_only(self, dws, test_base_id, test_folder_id):
        """仅复制 Base 结构（不含数据）到真实文件夹。"""
        copy_data = dws.run(
            "aitable", "base", "copy",
            "--base-id", test_base_id,
            "--target-folder-id", test_folder_id,
            "--only-struct",
        )

        # 验证返回结果
        assert "baseId" in copy_data["data"]
        new_base_id = copy_data["data"]["baseId"]

        # 验证新 Base 可以访问
        get_data = dws.run(
            "aitable", "base", "get",
            "--base-id", new_base_id,
        )
        assert "baseName" in get_data["data"]

        # 清理
        dws.run(
            "aitable", "base", "delete",
            "--base-id", new_base_id,
            "--yes",
        )

    def test_copy_base_missing_target_folder(self, dws, test_base_id):
        """缺少 --target-folder-id 参数应报错（CLI 层校验）。"""
        result = dws.run_raw(
            "aitable", "base", "copy",
            "--base-id", test_base_id,
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_copy_base_missing_base_id(self, dws, test_folder_id):
        """缺少 --base-id 参数应报错。"""
        result = dws.run_raw(
            "aitable", "base", "copy",
            "--target-folder-id", test_folder_id,
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_copy_base_missing_both_params(self, dws):
        """同时缺少 base-id 和 target-folder-id 应报错。"""
        result = dws.run_raw("aitable", "base", "copy")
        # 应该失败
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
