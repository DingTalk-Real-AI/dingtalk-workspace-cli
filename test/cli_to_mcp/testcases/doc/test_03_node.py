"""
test_03_node.py — 文件夹创建测试 (2 commands × 2 cases)

注: dws doc node create 已移除。
    文件夹用 dws doc folder create, 文件用 dws doc file create --type。

实际返回格式:
  doc folder create: {createTime, docUrl, folderId, name, nodeId, success:true, ...}
"""

import time
import pytest


class TestFolderCreate:
    """dws doc folder create"""

    def test_create_folder(self, dws):
        """创建文件夹应返回 nodeId。"""
        data = dws.run(
            "doc", "folder", "create",
            "--name", f"CLI_Folder_{int(time.time())}",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "nodeId" in data, f"响应缺少 nodeId: {data}"

    def test_create_folder_chinese_name(self, dws):
        """创建中文名文件夹。"""
        data = dws.run(
            "doc", "folder", "create",
            "--name", f"CLI_测试文件夹_{int(time.time())}",
        )
        assert data.get("success") is True
        assert "nodeId" in data

    def test_create_folder_invalid_parent(self, dws):
        """无效父文件夹应报错。"""
        result = dws.run_raw(
            "doc", "folder", "create",
            "--folder", "INVALID_99999",
            "--name", "error_test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
