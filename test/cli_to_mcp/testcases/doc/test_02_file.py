"""
test_02_file.py — 文档文件操作测试 (4 commands × 2-3 cases)

实际返回格式 (2026-04):
  doc create:       {nodeId, docUrl, folderId, name, success:true, ...}
  doc file create:  {nodeId, contentType, extension, success:true, ...}
  doc read --node:  markdown text (via run_raw)
  doc update --node --content: {success:true, ...}
"""

import time
import pytest


class TestDocCreate:
    """dws doc create — 创建在线文档"""

    def test_create_basic(self, dws):
        """创建文档应返回 nodeId。"""
        data = dws.run(
            "doc", "create",
            "--name", f"CLI_StrictTest_{int(time.time())}",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "nodeId" in data, f"响应缺少 nodeId: {data}"
        assert isinstance(data["nodeId"], str) and len(data["nodeId"]) > 0

    def test_create_chinese_name(self, dws):
        """创建中文名文档。"""
        data = dws.run(
            "doc", "create",
            "--name", f"严格测试文档_{int(time.time())}",
        )
        assert data.get("success") is True
        assert "nodeId" in data

    def test_create_with_markdown(self, dws):
        """创建文档并写入初始内容。"""
        data = dws.run(
            "doc", "create",
            "--name", f"CLI_Markdown_{int(time.time())}",
            "--content", "# 标题\n\n测试内容",
        )
        assert data.get("success") is True
        assert "nodeId" in data


class TestFileCreate:
    """dws doc file create — 创建指定类型文件"""

    def test_create_adoc(self, dws):
        """创建在线文档 (type=adoc)。"""
        data = dws.run(
            "doc", "file", "create",
            "--name", f"CLI_File_{int(time.time())}",
            "--type", "adoc",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "nodeId" in data, f"响应缺少 nodeId: {data}"

    def test_create_folder_via_file(self, dws):
        """通过 file create 创建文件夹 (type=folder)。"""
        data = dws.run(
            "doc", "file", "create",
            "--name", f"CLI_Folder_{int(time.time())}",
            "--type", "folder",
        )
        assert data.get("success") is True
        assert "nodeId" in data

    def test_create_missing_type(self, dws):
        """缺少必填 --type 应报错。"""
        result = dws.run_raw(
            "doc", "file", "create",
            "--name", "missing_type",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        )


class TestDocRead:
    """dws doc read — 读取文档内容"""

    def test_read_invalid_node(self, dws):
        """读取无效 node ID 应报错。"""
        result = dws.run_raw(
            "doc", "read",
            "--node", "INVALID_NODE_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestDocUpdate:
    """dws doc update — 更新文档内容"""

    def test_update_invalid_node(self, dws):
        """更新无效 node ID 应报错。"""
        result = dws.run_raw(
            "doc", "update",
            "--node", "INVALID_NODE_99999",
            "--content", "hello",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
