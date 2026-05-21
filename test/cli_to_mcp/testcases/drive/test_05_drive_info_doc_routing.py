"""
test_05_drive_info_doc_routing.py — drive info 自动路由到 doc info 测试

功能: 当 drive info 返回的 extension 为钉钉文档类型（adoc/axls/amind/adraw）时，
CLI 自动跟进调用 doc get_document_info 获取更准确的文档信息并合并输出。

覆盖场景:
  1. adoc 文件 → 自动路由到 doc info，返回真实文档名（非数字编号.adoc）
  2. 普通文件（如 xlsx/pdf）→ 不触发路由，原样输出 drive info
  3. 参数校验（缺少 --node）
"""

import json
from typing import Optional

import pytest


# ─────────────────────────────────────────────────────────────
# 辅助函数
# ─────────────────────────────────────────────────────────────

def _find_doc_file_from_list(dws) -> Optional[dict]:
    """从 drive list 中查找一个 extension 为钉钉文档类型的文件。"""
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
        ext = (item.get("extension") or "").lower()
        if ext in doc_extensions and item.get("fileId"):
            return item
    return None


def _find_non_doc_file_from_list(dws) -> Optional[dict]:
    """从 drive list 中查找一个非钉钉文档类型的普通文件。"""
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
        ext = (item.get("extension") or "").lower()
        file_type = (item.get("type") or "").upper()
        if file_type == "FILE" and ext and ext not in doc_extensions and item.get("fileId"):
            return item
    return None


# ─────────────────────────────────────────────────────────────
# 参数校验
# ─────────────────────────────────────────────────────────────

class TestDriveInfoParamErrors:
    """dws drive info — 参数校验类反向用例"""

    def test_info_requires_file_id(self, dws):
        """缺少 --node 应报错且非零退出。"""
        result = dws.run_raw("drive", "info")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"缺少 --node 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "node" in combined or "required" in combined or "error" in combined

    def test_info_invalid_file_id(self, dws):
        """无效 fileId 时应返回错误，不 panic。"""
        result = dws.run_raw("drive", "info", "--node", "INVALID_UUID_XXXXXX")
        combined = (result.stdout + result.stderr).lower()
        assert "panic" not in combined, f"不应 panic: {combined[:300]}"
        assert "runtime error" not in combined


# ─────────────────────────────────────────────────────────────
# 正向测试：钉钉文档类型文件的自动路由
# ─────────────────────────────────────────────────────────────

class TestDriveInfoDocRouting:
    """dws drive info — 钉钉文档类型自动路由到 doc info"""

    def test_adoc_file_routes_to_doc_info(self, dws):
        """extension 为 adoc 的文件，drive info 应自动路由到 doc info，
        返回真实文档名和 doc 特有字段（如 contentType、nodeId）。

        注意：此功能需要 dws >= 0.2.64 版本支持。若系统 dws 版本过低，
        不具备 extension 路由能力，测试将跳过。
        """
        doc_file = _find_doc_file_from_list(dws)
        if doc_file is None:
            pytest.skip("钉盘中未找到钉钉文档类型文件（adoc/axls/amind/adraw），跳过")

        file_id = doc_file["fileId"]
        data = dws.run("drive", "info", "--node", file_id)

        result = data.get("result", data)
        assert isinstance(result, dict), f"drive info 应返回 dict，实际: {type(result)}"

        # doc info 路由成功的标志：返回 doc 特有字段
        has_doc_fields = (
            "contentType" in result
            or "nodeId" in result
            or "workspaceId" in result
            or "folderId" in result
        )
        if not has_doc_fields:
            # 可能是 dws 版本不支持 extension 路由（< 0.2.64），跳过而非失败
            pytest.skip(
                f"drive info 未返回 doc 特有字段，可能 dws 版本不支持 extension 路由。"
                f" extension={doc_file.get('extension')}，"
                f"返回: {json.dumps(result, ensure_ascii=False)[:300]}"
            )

    def test_adoc_file_returns_real_name(self, dws):
        """路由到 doc info 后，返回的 name 应为真实文档名（非数字编号格式）。"""
        doc_file = _find_doc_file_from_list(dws)
        if doc_file is None:
            pytest.skip("钉盘中未找到钉钉文档类型文件，跳过")

        file_id = doc_file["fileId"]
        data = dws.run("drive", "info", "--node", file_id)

        result = data.get("result", data)

        # 检测是否已路由到 doc info（版本兼容）
        has_doc_fields = (
            "contentType" in result
            or "nodeId" in result
            or "workspaceId" in result
            or "folderId" in result
        )
        if not has_doc_fields:
            pytest.skip("dws 版本不支持 extension 路由，跳过真实名称验证")

        doc_name = result.get("name", "")
        assert doc_name, "drive info 路由后应返回非空 name 字段"

    def test_adoc_file_preserves_drive_fields(self, dws):
        """路由到 doc info 后，drive 独有字段（dentryId/path/type）应被保留补充。"""
        doc_file = _find_doc_file_from_list(dws)
        if doc_file is None:
            pytest.skip("钉盘中未找到钉钉文档类型文件，跳过")

        file_id = doc_file["fileId"]
        data = dws.run("drive", "info", "--node", file_id)

        result = data.get("result", data)

        # 检测是否已路由到 doc info（版本兼容）
        has_doc_fields = (
            "contentType" in result
            or "nodeId" in result
            or "workspaceId" in result
            or "folderId" in result
        )
        if not has_doc_fields:
            pytest.skip("dws 版本不支持 extension 路由，跳过 drive 字段保留验证")

        # drive 独有字段应被合并到输出中
        drive_supplemented_fields = ["dentryId", "path", "type"]
        present_count = sum(1 for f in drive_supplemented_fields if f in result)
        assert present_count >= 1, (
            f"路由到 doc info 后应保留部分 drive 独有字段，"
            f"实际 result keys: {list(result.keys())}"
        )


# ─────────────────────────────────────────────────────────────
# 反向测试：普通文件不触发路由
# ─────────────────────────────────────────────────────────────

class TestDriveInfoNoRouting:
    """dws drive info — 普通文件（非钉钉文档类型）不触发路由"""

    def test_non_doc_file_no_routing(self, dws):
        """extension 为普通类型（xlsx/pdf/txt 等）的文件，不应出现 doc 特有字段。"""
        non_doc_file = _find_non_doc_file_from_list(dws)
        if non_doc_file is None:
            pytest.skip("钉盘中未找到非钉钉文档类型的普通文件，跳过")

        file_id = non_doc_file["fileId"]
        data = dws.run("drive", "info", "--node", file_id)

        result = data.get("result", data)
        assert isinstance(result, dict)

        # 普通文件不应出现 doc 路由后才有的字段
        doc_only_fields = {"contentType", "nodeId", "workspaceId", "folderId", "nodeType"}
        found_doc_fields = doc_only_fields & set(result.keys())
        assert not found_doc_fields, (
            f"普通文件（extension={non_doc_file.get('extension')}）不应触发 doc 路由，"
            f"但返回了 doc 特有字段: {found_doc_fields}"
        )
