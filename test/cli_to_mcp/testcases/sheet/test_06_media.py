"""
test_06_media.py — 表格附件上传与图片写入测试

Commands tested:
  1. dws sheet media-upload
  2. dws sheet write-image

注意: 两个命令都是多步骤命令，stdout 混合了 [INFO] 进度行和 JSON/文本，
因此正向测试使用 run_raw() 并检查 returncode 和输出关键字。

依赖 conftest.py 中的 sheet_node_id / sheet_id fixture（自动创建测试表格）。
"""

import json
import os
import re
import tempfile
import time

import pytest


def _create_temp_text_file(content="CLI 集成测试内容", suffix=".txt"):
    """创建一个临时文本文件，返回路径。调用方负责删除。"""
    tmp = tempfile.NamedTemporaryFile(
        suffix=suffix, prefix="cli_sheet_test_", delete=False, mode="w"
    )
    tmp.write(content)
    tmp.close()
    return tmp.name


def _create_temp_image_file():
    """创建一个最小的合法 PNG 文件，返回路径。调用方负责删除。"""
    import struct
    import zlib

    width, height = 2, 2
    raw = b""
    for _ in range(height):
        raw += b"\x00" + b"\xff\x00\x00" * width

    def chunk(ctype, data):
        c = ctype + data
        return (
            struct.pack(">I", len(data))
            + c
            + struct.pack(">I", zlib.crc32(c) & 0xFFFFFFFF)
        )

    tmp = tempfile.NamedTemporaryFile(
        suffix=".png", prefix="cli_sheet_test_", delete=False, mode="wb"
    )
    tmp.write(b"\x89PNG\r\n\x1a\n")
    tmp.write(
        chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
    )
    tmp.write(chunk(b"IDAT", zlib.compress(raw)))
    tmp.write(chunk(b"IEND", b""))
    tmp.close()
    return tmp.name


def _stdout_contains_keyword(stdout: str, keyword: str) -> bool:
    """检查 stdout 中是否包含指定关键词（大小写不敏感）。"""
    return keyword.lower() in stdout.lower()


class TestSheetMediaUploadErrors:
    """dws sheet media-upload — 反向测试"""

    def test_missing_node_flag(self, dws):
        """缺少 --node 应报错。"""
        tmp_path = _create_temp_text_file()
        try:
            result = dws.run_raw(
                "sheet", "media-upload",
                "--file", tmp_path,
            )
            combined = (result.stdout + result.stderr).lower()
            assert result.returncode != 0 or "error" in combined, (
                f"缺少 --node 应报错: stdout={result.stdout}, stderr={result.stderr}"
            )
        finally:
            os.unlink(tmp_path)

    def test_missing_file_flag(self, dws, sheet_node_id):
        """缺少 --file 应报错。"""
        result = dws.run_raw(
            "sheet", "media-upload",
            "--node", sheet_node_id,
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"缺少 --file 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_nonexistent_file_path(self, dws, sheet_node_id):
        """--file 指向不存在的文件应报错。"""
        result = dws.run_raw(
            "sheet", "media-upload",
            "--node", sheet_node_id,
            "--file", "/tmp/nonexistent_sheet_file_99999.txt",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"不存在的文件路径应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_directory_as_file(self, dws, sheet_node_id):
        """--file 传入目录而非文件应报错。"""
        result = dws.run_raw(
            "sheet", "media-upload",
            "--node", sheet_node_id,
            "--file", "/tmp",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"目录作为 --file 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_invalid_node_id(self, dws):
        """无效 nodeId 应报错。"""
        tmp_path = _create_temp_text_file()
        try:
            result = dws.run_raw(
                "sheet", "media-upload",
                "--node", "INVALID_NODE_99999",
                "--file", tmp_path,
            )
            combined = (result.stdout + result.stderr).lower()
            assert result.returncode != 0 or "error" in combined, (
                f"无效 nodeId 应报错: stdout={result.stdout}, stderr={result.stderr}"
            )
        finally:
            os.unlink(tmp_path)

class TestSheetWriteImageErrors:
    """dws sheet write-image — 反向测试"""

    def test_missing_node_flag(self, dws, sheet_id):
        """缺少 --node 应报错。"""
        tmp_path = _create_temp_image_file()
        try:
            result = dws.run_raw(
                "sheet", "write-image",
                "--sheet-id", sheet_id,
                "--range", "A1:A1",
                "--file", tmp_path,
            )
            combined = (result.stdout + result.stderr).lower()
            assert result.returncode != 0 or "error" in combined, (
                f"缺少 --node 应报错: stdout={result.stdout}, stderr={result.stderr}"
            )
        finally:
            os.unlink(tmp_path)

    def test_missing_sheet_id_flag(self, dws, sheet_node_id):
        """缺少 --sheet-id 应报错。"""
        tmp_path = _create_temp_image_file()
        try:
            result = dws.run_raw(
                "sheet", "write-image",
                "--node", sheet_node_id,
                "--range", "A1:A1",
                "--file", tmp_path,
            )
            combined = (result.stdout + result.stderr).lower()
            assert result.returncode != 0 or "error" in combined, (
                f"缺少 --sheet-id 应报错: stdout={result.stdout}, stderr={result.stderr}"
            )
        finally:
            os.unlink(tmp_path)

    def test_missing_range_flag(self, dws, sheet_node_id, sheet_id):
        """缺少 --range 应报错。"""
        tmp_path = _create_temp_image_file()
        try:
            result = dws.run_raw(
                "sheet", "write-image",
                "--node", sheet_node_id,
                "--sheet-id", sheet_id,
                "--file", tmp_path,
            )
            combined = (result.stdout + result.stderr).lower()
            assert result.returncode != 0 or "error" in combined, (
                f"缺少 --range 应报错: stdout={result.stdout}, stderr={result.stderr}"
            )
        finally:
            os.unlink(tmp_path)

    def test_missing_file_flag(self, dws, sheet_node_id, sheet_id):
        """缺少 --file 应报错。"""
        result = dws.run_raw(
            "sheet", "write-image",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A1",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"缺少 --file 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_nonexistent_file_path(self, dws, sheet_node_id, sheet_id):
        """--file 指向不存在的文件应报错。"""
        result = dws.run_raw(
            "sheet", "write-image",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A1",
            "--file", "/tmp/nonexistent_sheet_image_99999.png",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"不存在的文件路径应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_invalid_node_id(self, dws, sheet_id):
        """无效 nodeId 应报错。"""
        tmp_path = _create_temp_image_file()
        try:
            result = dws.run_raw(
                "sheet", "write-image",
                "--node", "INVALID_NODE_99999",
                "--sheet-id", sheet_id,
                "--range", "A1:A1",
                "--file", tmp_path,
            )
            combined = (result.stdout + result.stderr).lower()
            assert result.returncode != 0 or "error" in combined, (
                f"无效 nodeId 应报错: stdout={result.stdout}, stderr={result.stderr}"
            )
        finally:
            os.unlink(tmp_path)

    def test_directory_as_file(self, dws, sheet_node_id, sheet_id):
        """--file 传入目录而非文件应报错。"""
        result = dws.run_raw(
            "sheet", "write-image",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:A1",
            "--file", "/tmp",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"目录作为 --file 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )
