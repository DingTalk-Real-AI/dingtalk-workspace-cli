"""
conftest.py — Shared fixtures for aitable DWS integration tests.
DWSRunner/dws/current_user_id come from root conftest.py.

Provides:
  - dws(): subprocess runner that returns parsed JSON
  - test_base_id: session-scoped Base created once, deleted at teardown
  - test_table_id / test_field_ids: session-scoped Table with typed fields
"""
from __future__ import annotations

import json
import os
import shlex
import subprocess

import pytest
from test_utils import resolve_dws_bin, unique_name

# ─── 扩展测试跳过控制 ──────────────────────────────────────
# CI 默认只跑核心测试 (test_01~test_05)，扩展测试 (test_90~test_100) 需要
# 设置 AITABLE_EXTENDED=1 才运行，避免 237 个 case 超时。
_RUN_EXTENDED = os.environ.get("AITABLE_EXTENDED", "0") == "1"

_EXTENDED_FILES = [
    "test_90_aitable_param_regression.py",
    "test_91_record_pagination.py",
    "test_92_field_properties.py",
    "test_93_cell_value_write_read.py",
    "test_94_field_edge_cases.py",
    "test_95_coverage_gaps.py",
    "test_96_record_operations.py",
    "test_97_advanced_fields.py",
    "test_98_attachment_export_import.py",
    "test_99_dashboard_chart.py",
    "test_100_misc_coverage.py",
]


def pytest_ignore_collect(collection_path, config):
    """Skip extended test files unless AITABLE_EXTENDED=1."""
    if not _RUN_EXTENDED and collection_path.name in _EXTENDED_FILES:
        return True

# 与根 conftest 保持一致：优先 DWS_BIN 环境变量与仓库本地构建产物。
DWS_BIN = resolve_dws_bin(__file__)
TEST_PREFIX = "CLI_Test"


# ─── Core helper ────────────────────────────────────────────

class DWSRunner:
    """Thin wrapper around `dws` CLI invocation."""

    @staticmethod
    def _log_cmd(cmd: list[str]) -> None:
        """将真实执行命令写入 pytest 日志，便于报告阶段提取。"""
        print(f"DWS_CMD: {' '.join(shlex.quote(x) for x in cmd)}")

    @staticmethod
    def _parse_completed_json(result: subprocess.CompletedProcess, cmd: list[str]) -> dict:
        for text in ((result.stdout or "").strip(), (result.stderr or "").strip()):
            if not text:
                continue
            try:
                return json.loads(text)
            except json.JSONDecodeError:
                continue
        stderr = (result.stderr or "").strip()
        if "AUTH_PERMISSION_DENIED" in stderr or "权限不足" in stderr:
            pytest.skip(f"dws 权限不足，跳过用例：{stderr[:120]}")
        pytest.fail(
            f"dws returned non-JSON:\n"
            f"  cmd:    {' '.join(cmd)}\n"
            f"  stdout: {result.stdout[:500]}\n"
            f"  stderr: {result.stderr[:500]}"
        )

    def run(self, *args: str, expect_success: bool = True) -> dict:
        """Execute dws with given args, return parsed JSON dict."""
        cmd = [DWS_BIN, *args, "--format", "json"]
        self._log_cmd(cmd)
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=60,
        )
        data = self._parse_completed_json(result, cmd)

        if expect_success:
            # "error": {} 是 aitable 正常返回，只有非空 error 才算错误
            err = data.get("error")
            has_real_error = bool(err) and err != {}
            is_error = (
                data.get("status") == "error"
                or has_real_error
            )
            if is_error:
                pytest.fail(
                    f"Expected success but got: {json.dumps(data, ensure_ascii=False, indent=2)[:500]}"
                )
        return data

    def run_ok(self, *args: str) -> dict:
        """Like run(), but only asserts no error (tolerates empty status)."""
        cmd = [DWS_BIN, *args, "--format", "json"]
        self._log_cmd(cmd)
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=60,
        )
        data = self._parse_completed_json(result, cmd)
        err = data.get("error")
        has_real_error = bool(err) and err != {}
        is_error = (
            data.get("status") == "error"
            or has_real_error
        )
        if is_error:
            pytest.fail(
                f"Command returned error: {json.dumps(data, ensure_ascii=False, indent=2)[:500]}"
            )
        return data

    def run_raw(self, *args: str) -> subprocess.CompletedProcess:
        """Execute dws, return raw CompletedProcess (for error-path tests)."""
        cmd = [DWS_BIN, *args, "--format", "json"]
        self._log_cmd(cmd)
        return subprocess.run(cmd, capture_output=True, text=True, timeout=30)


@pytest.fixture(scope="session")
def dws():
    """Session-scoped DWS runner."""
    return DWSRunner()


# ─── Test Base lifecycle ────────────────────────────────────

@pytest.fixture(scope="session")
def test_base_info(dws: DWSRunner):
    """Create one shared test Base and expose both id/name."""
    name = unique_name("CLI_Test_Base")
    data = dws.run("aitable", "base", "create", "--name", name)
    base_id = data["data"]["baseId"]
    assert base_id, "base create must return baseId"
    print(f"\n[SETUP] Created test Base: {base_id} ({name})")

    yield {"base_id": base_id, "base_name": name}

    # Teardown: delete the test base
    print(f"\n[TEARDOWN] Deleting test Base: {base_id}")
    try:
        dws.run("aitable", "base", "delete", "--base-id", base_id, "--yes")
    except Exception as e:
        print(f"[TEARDOWN WARNING] Failed to delete base {base_id}: {e}")


@pytest.fixture(scope="session")
def test_base_id(test_base_info):
    """Backward-compatible fixture: return test base id."""
    return test_base_info["base_id"]


@pytest.fixture(scope="session")
def test_base_name(test_base_info):
    """Return the exact created base name used in this run."""
    return test_base_info["base_name"]


@pytest.fixture(scope="session")
def test_folder_id(dws: DWSRunner):
    """Create a temporary folder for copy tests, return its nodeId (dentryUuid).

    MCP copy_base expects targetFolderId = dentryUuid, which corresponds to
    the ``nodeId`` returned by ``doc folder create``, NOT ``folderId``.
    """
    name = unique_name("CLI_Test_Folder")
    data = dws.run("doc", "folder", "create", "--name", name)
    # nodeId is the dentryUuid accepted by MCP copy_base as targetFolderId
    node_id = data.get("nodeId") or data.get("data", {}).get("nodeId")
    assert node_id, f"folder create must return nodeId, got: {data}"
    print(f"\n[SETUP] Created test Folder: nodeId={node_id} ({name})")

    yield node_id

    # Teardown: 尝试清理测试文件夹（CLI 暂无 doc delete 命令，静默跳过）
    # 文件夹会随 Base 删除被级联清理，此处仅做 best-effort
    print(f"\n[TEARDOWN] Test Folder {node_id} will be cleaned up with Base deletion")
