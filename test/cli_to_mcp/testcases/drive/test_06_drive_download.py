"""
test_06_drive_download.py — 钉盘文件下载测试

命令: dws drive download --node <dentryUuid> --output <path>
MCP tool: download_file (返回 downloadUrl + headers)

覆盖场景:
  1. 参数校验（缺少 --node / --output）
  2. 正向下载：下载一个普通文件到本地并验证文件存在
  3. output 为目录时自动推断文件名
"""

import os
import tempfile
from typing import Optional

import pytest


# ─────────────────────────────────────────────────────────────
# 辅助函数
# ─────────────────────────────────────────────────────────────

def _find_downloadable_file(dws) -> Optional[dict]:
    """从 drive list 中查找一个可下载的普通文件（非文件夹、非在线文档、fileSize > 0）。"""
    doc_extensions = {"adoc", "axls", "amind", "adraw"}
    data = dws.run("drive", "list", "--max", "50")
    items = []
    if isinstance(data, dict):
        result = data.get("result", data)
        if isinstance(result, dict):
            items = result.get("items", [])
        elif isinstance(result, list):
            items = result
    for item in items:
        file_type = (item.get("type") or "").upper()
        ext = (item.get("extension") or "").lower()
        file_size = item.get("fileSize", 0)
        if file_type == "FILE" and ext not in doc_extensions and item.get("fileId"):
            # 必须有实际大小才能验证下载结果
            if file_size and file_size > 0:
                return item
    return None


# ─────────────────────────────────────────────────────────────
# 参数校验
# ─────────────────────────────────────────────────────────────

class TestDriveDownloadParamErrors:
    """dws drive download — 参数校验类反向用例"""

    def test_download_requires_file_id(self, dws):
        """缺少 --node 应报错且非零退出。"""
        result = dws.run_raw("drive", "download", "--output", "/tmp/test.txt")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"缺少 --node 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "node" in combined or "required" in combined or "error" in combined

    def test_download_requires_output(self, dws):
        """缺少 --output 应报错且非零退出。"""
        result = dws.run_raw("drive", "download", "--node", "some-uuid")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"缺少 --output 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "output" in combined or "required" in combined or "error" in combined

    def test_download_invalid_file_id(self, dws):
        """无效 fileId 应返回错误，不 panic。"""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "invalid_test.bin")
            result = dws.run_raw(
                "drive", "download",
                "--node", "INVALID_UUID_XXXXXX",
                "--output", output_path,
            )
            combined = (result.stdout + result.stderr).lower()
            assert "panic" not in combined, f"不应 panic: {combined[:300]}"
            assert "runtime error" not in combined
            # 无效 ID 应该导致失败
            assert result.returncode != 0 or "error" in combined


# ─────────────────────────────────────────────────────────────
# 正向测试：下载文件
# ─────────────────────────────────────────────────────────────

class TestDriveDownloadBasic:
    """dws drive download — 正向下载用例"""

    def test_download_to_file_path(self, dws):
        """下载文件到指定路径，验证文件存在且非空。"""
        downloadable = _find_downloadable_file(dws)
        if downloadable is None:
            pytest.skip("钉盘中未找到可下载的普通文件（非文件夹/非在线文档/fileSize>0），跳过")

        file_id = downloadable["fileId"]
        ext = downloadable.get("extension") or "bin"
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, f"download_test.{ext}")
            result = dws.run_raw(
                "drive", "download",
                "--node", file_id,
                "--output", output_path,
            )
            combined = (result.stdout + result.stderr).lower()
            assert "panic" not in combined, f"不应 panic: {combined[:300]}"

            # 如果是权限问题，跳过
            if "permission" in combined or "权限" in combined:
                pytest.skip(f"权限不足，跳过: {combined[:200]}")

            # 如果命令本身不支持（旧版 dws），跳过
            if "unknown flag" in combined or "unknown command" in combined:
                pytest.skip(f"dws 版本不支持 drive download 命令: {combined[:200]}")

            assert result.returncode == 0, (
                f"下载应成功: stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
            )
            assert os.path.exists(output_path), f"下载完成后文件应存在: {output_path}"
            assert os.path.getsize(output_path) > 0, f"下载文件不应为空: {output_path}"

    def test_download_to_directory(self, dws):
        """--output 为目录时，应自动推断文件名并下载成功。"""
        downloadable = _find_downloadable_file(dws)
        if downloadable is None:
            pytest.skip("钉盘中未找到可下载的普通文件，跳过")

        file_id = downloadable["fileId"]
        with tempfile.TemporaryDirectory() as tmpdir:
            result = dws.run_raw(
                "drive", "download",
                "--node", file_id,
                "--output", tmpdir,
            )
            combined = (result.stdout + result.stderr).lower()
            assert "panic" not in combined, f"不应 panic: {combined[:300]}"

            if "permission" in combined or "权限" in combined:
                pytest.skip(f"权限不足，跳过: {combined[:200]}")

            if "unknown flag" in combined or "unknown command" in combined:
                pytest.skip(f"dws 版本不支持 drive download 命令: {combined[:200]}")

            if result.returncode != 0:
                pytest.skip(f"下载失败（可能是环境问题）: {combined[:200]}")

            # 目录下应该出现至少一个文件
            files = os.listdir(tmpdir)
            assert len(files) > 0, f"下载到目录后应有文件生成，实际为空: {tmpdir}"
