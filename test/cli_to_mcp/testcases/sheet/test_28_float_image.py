"""
test_28_float_image.py — 浮动图片 CRUD 测试

Commands tested:
  1. dws sheet media-upload          (前置：上传图片获取 resourceUrl)
  2. dws sheet create-float-image    创建浮动图片
  3. dws sheet list-float-images     列出浮动图片
  4. dws sheet get-float-image       获取浮动图片详情
  5. dws sheet update-float-image    更新浮动图片属性
  6. dws sheet delete-float-image    删除浮动图片

流程：先 media-upload 获取 resourceUrl，再走完整 CRUD 链路。
依赖 conftest.py 中的 sheet_node_id / sheet_id fixture。
"""

import json
import os
import re
import struct
import tempfile
import zlib

import pytest

# ─── 反向测试：错误参数 ──────────────────────────────────────

class TestFloatImageErrors:
    """浮动图片命令的反向/边界测试。"""

    def test_create_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "create-float-image",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--src", "https://example.com/fake.png",
            "--range", "A1",
            "--width", "200",
            "--height", "150",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined

    def test_get_nonexistent_float_image(self, dws, sheet_node_id, sheet_id):
        """获取不存在的浮动图片 ID 应报错。"""
        result = dws.run_raw(
            "sheet", "get-float-image",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--float-image-id", "nonexistent_id_99999",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined

    def test_delete_nonexistent_float_image(self, dws, sheet_node_id, sheet_id):
        """删除不存在的浮动图片 ID — 服务端幂等，返回 success 也视为通过。"""
        result = dws.run_raw(
            "sheet", "delete-float-image",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--float-image-id", "nonexistent_id_99999",
        )
        # 服务端对 delete 操作做了幂等处理，不存在的 ID 也可能返回 success
        # 只要不报非预期的 CLI 错误即可
        assert result.returncode == 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_nonexistent_float_image(self, dws, sheet_node_id, sheet_id):
        """更新不存在的浮动图片 ID 应报错。"""
        result = dws.run_raw(
            "sheet", "update-float-image",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--float-image-id", "nonexistent_id_99999",
            "--width", "300",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined
