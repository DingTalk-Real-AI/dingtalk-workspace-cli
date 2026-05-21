"""
test_15_export.py — 导出表格为 xlsx 测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet export --node NODE_ID                      仅返回 downloadUrl
  2. dws sheet export --node NODE_ID --output <FILE>      下载到本地文件
  3. dws sheet export --node NODE_ID --output <DIR>       下载到目录，文件名自动推断

注意：export 是单命令一站式（内部编排：提交 → 渐进式退避轮询 → 可选下载），
输出混合了 [INFO] 进度行 + "jobId:"/"downloadUrl:" KV 纯文本，不是结构化 JSON，
因此正向测试必须使用 subprocess.run 并通过关键字/正则解析 stdout。

export 服务端真实导出可能耗时数十秒（甚至更久），超出框架 run_raw 的 60s 默认超时，
所以正向用例直接用 subprocess.run + 大超时（_EXPORT_TIMEOUT = 360s）而不走 dws.run_raw。
若仍然 timeout，pytest.skip 以免污染其他用例结果。
"""

import os
import re
import subprocess
import tempfile

import pytest
from test_utils import resolve_dws_bin


# ─── 工具函数 ──────────────────────────────────────────────

# export 服务端真实导出可能耗时数十秒甚至更久，默认给 360s；
# 必要时可通过环境变量 DWS_EXPORT_TIMEOUT 覆盖（CI 环境较慢时适当放大）。
_EXPORT_TIMEOUT = int(os.environ.get("DWS_EXPORT_TIMEOUT", "360"))

# 统一使用框架的 DWS_BIN 解析逻辑；入口取 __file__ 用于从当前文件向上定位仓库根。
_DWS_BIN = resolve_dws_bin(__file__)

_DOWNLOAD_URL_RE = re.compile(r"downloadUrl:\s*(https?://\S+)", re.IGNORECASE)
_JOB_ID_RE = re.compile(r"jobId:\s*(\S+)", re.IGNORECASE)


def _extract_download_url(stdout: str) -> str:
    """从 export 命令的 stdout 中提取 downloadUrl。"""
    m = _DOWNLOAD_URL_RE.search(stdout or "")
    return m.group(1) if m else ""


def _extract_job_id(stdout: str) -> str:
    """从 export 命令的 stdout 中提取 jobId。"""
    m = _JOB_ID_RE.search(stdout or "")
    return m.group(1) if m else ""


def _run_export_or_skip(dws, *args: str):
    """直接用 subprocess 调用 DWS_BIN 运行 export（绕开 run_raw 的 60s 超时）。

    若超出 _EXPORT_TIMEOUT（默认 360s，可由环境变量 DWS_EXPORT_TIMEOUT 覆盖），
    则 pytest.skip 该用例。
    """
    cmd = [_DWS_BIN, "sheet", "export", *args, "-f", "json"]
    print(f"DWS_CMD: {' '.join(cmd)}")
    try:
        return subprocess.run(
            cmd, capture_output=True, text=True, timeout=_EXPORT_TIMEOUT
        )
    except subprocess.TimeoutExpired as e:
        pytest.skip(
            f"sheet export 超过 {_EXPORT_TIMEOUT}s 未完成（环境较慢或服务端处理中），skip: {e}"
        )


def _skip_if_permission_denied(result):
    """若命令因权限/灰度返回被拒，则 skip 该用例。"""
    combined = (result.stdout or "") + (result.stderr or "")
    for kw in ("AUTH_PERMISSION_DENIED", "权限不足", "PAT_MEDIUM_RISK_NO_PERMISSION"):
        if kw in combined:
            pytest.skip(f"dws 权限不足/灰度未开，skip export 用例：{combined[:200]}")


# ─── 正向测试 ──────────────────────────────────────────────


class TestSheetExport:
    """dws sheet export — 正向测试（一站式：提交 → 轮询 → 可选下载）"""

    def test_export_help(self, dws):
        """--help 应正常输出，包含命令说明关键字。"""
        result = dws.run_raw("sheet", "export", "--help")
        assert result.returncode == 0, f"--help 应成功: {result.stderr}"
        combined = result.stdout + result.stderr
        assert "--node" in combined, f"--help 应描述 --node: {combined[:400]}"
        assert "--output" in combined, f"--help 应描述 --output: {combined[:400]}"
        # Short/Long 文本里应该提到 xlsx
        assert "xlsx" in combined.lower(), f"--help 应提到 xlsx: {combined[:400]}"

    def test_export_returns_download_url(self, dws, sheet_node_id):
        """不传 --output，命令应成功并在 stdout 中输出 jobId + downloadUrl。"""
        result = _run_export_or_skip(dws, "--node", sheet_node_id)
        _skip_if_permission_denied(result)
        assert result.returncode == 0, (
            f"export 应成功: returncode={result.returncode}\n"
            f"stdout={result.stdout[:800]}\nstderr={result.stderr[:400]}"
        )

        job_id = _extract_job_id(result.stdout)
        assert job_id, f"stdout 应含 jobId: {result.stdout[:600]}"

        download_url = _extract_download_url(result.stdout)
        assert download_url.startswith("http"), (
            f"downloadUrl 应为有效 HTTP(S) 链接: stdout={result.stdout[:800]}"
        )

    def test_export_with_file_output(self, dws, sheet_node_id):
        """传 --output 文件路径，命令应下载 xlsx 到该路径，文件非空。"""
        with tempfile.TemporaryDirectory(prefix="cli_sheet_export_") as tmpdir:
            out_path = os.path.join(tmpdir, "export_result.xlsx")
            result = _run_export_or_skip(
                dws, "--node", sheet_node_id, "--output", out_path
            )
            _skip_if_permission_denied(result)
            assert result.returncode == 0, (
                f"export --output 应成功: returncode={result.returncode}\n"
                f"stdout={result.stdout[:800]}\nstderr={result.stderr[:400]}"
            )

            assert os.path.isfile(out_path), (
                f"--output 指定文件应存在: {out_path}\nstdout={result.stdout[:800]}"
            )
            size = os.path.getsize(out_path)
            assert size > 0, f"下载的 xlsx 文件不应为空: size={size}"
            # stdout 应打印"导出完成: <path>"
            assert out_path in result.stdout, (
                f"stdout 应提示已保存到 {out_path}: {result.stdout[:800]}"
            )

    def test_export_with_dir_output(self, dws, sheet_node_id):
        """传 --output 目录，命令应自动按下载链接推断文件名保存到该目录。"""
        with tempfile.TemporaryDirectory(prefix="cli_sheet_export_dir_") as tmpdir:
            before = set(os.listdir(tmpdir))
            result = _run_export_or_skip(
                dws, "--node", sheet_node_id, "--output", tmpdir
            )
            _skip_if_permission_denied(result)
            assert result.returncode == 0, (
                f"export --output <dir> 应成功: returncode={result.returncode}\n"
                f"stdout={result.stdout[:800]}\nstderr={result.stderr[:400]}"
            )

            after = set(os.listdir(tmpdir))
            new_files = after - before
            assert new_files, (
                f"目录下应新增至少 1 个文件: {tmpdir}\nstdout={result.stdout[:800]}"
            )
            # 至少一个文件应非空
            for name in new_files:
                path = os.path.join(tmpdir, name)
                if os.path.isfile(path) and os.path.getsize(path) > 0:
                    break
            else:
                pytest.fail(f"新增文件均为空: {new_files} in {tmpdir}")


# ─── 反向测试 ──────────────────────────────────────────────


class TestSheetExportErrors:
    """dws sheet export — 反向测试（参数校验与错误处理）"""

    def test_missing_node(self, dws):
        """缺少必填 --node 应报错。"""
        result = dws.run_raw("sheet", "export")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "required" in combined, (
            f"缺少 --node 应报错: stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )

    def test_empty_node(self, dws):
        """--node 传空字符串应报错。"""
        result = dws.run_raw("sheet", "export", "--node", "")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "required" in combined, (
            f"--node '' 应报错: stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )

    def test_invalid_node_id(self, dws):
        """无效 nodeId 应报错（MCP 侧返回 invalidRequest）。"""
        result = _run_export_or_skip(dws, "--node", "INVALID_NODE_99999")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "invalid" in combined, (
            f"无效 nodeId 应报错: stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )

    def test_output_parent_not_exist(self, dws, sheet_node_id):
        """--output 指向的父目录不存在，命令应最终失败（下载阶段写文件失败）。"""
        bogus_path = "/nonexistent_dir_99999/sheet_export.xlsx"
        result = _run_export_or_skip(
            dws, "--node", sheet_node_id, "--output", bogus_path
        )
        _skip_if_permission_denied(result)
        # 命令应失败，或即便 returncode=0 也不应真的在 /nonexistent_dir_99999 下生成文件
        file_created = os.path.exists(bogus_path)
        combined = (result.stdout + result.stderr).lower()
        assert (result.returncode != 0 or "error" in combined) and not file_created, (
            f"不可写 --output 应报错且不创建文件: "
            f"returncode={result.returncode}, file_created={file_created}, "
            f"stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )
