"""
test_06_export.py — 文档导出测试

Commands tested:
  1. dws doc export
  2. dws doc export get

注意: dws doc export 是多步骤命令（提交→轮询→下载），stdout 混合了 [INFO] 进度行和 JSON，
因此正向测试使用 run_raw() 并从 stdout 中提取信息。

前置条件:
  - submit_export_job / query_export_job MCP Tool 已在服务端注册并上线
  - 当前用户在测试文档所在组织下有阅读权限
  - dws doc export 子命令已注册（该命令在部分分支可能尚未合入）
"""

import json
import os
import re
import subprocess
import tempfile
import time

import pytest


def _dws_has_export_command() -> bool:
    """检测当前 dws 二进制是否包含 doc export 子命令。"""
    try:
        result = subprocess.run(
            ["dws", "doc", "export", "--help"],
            capture_output=True, text=True, timeout=10,
        )
        # 如果 export 不存在，dws doc --help 会被打印出来（returncode 0）
        # 或者直接报错；通过检查 stdout 是否包含 export 相关关键字来判断
        return "export" in result.stdout.lower() and "output" in result.stdout.lower()
    except Exception:
        return False


# 模块级跳过：如果当前 dws 二进制不包含 doc export 命令，跳过整个文件
pytestmark = pytest.mark.skipif(
    not _dws_has_export_command(),
    reason="dws doc export 子命令在当前二进制中不存在（可能尚未合入当前分支）",
)


def _extract_json_from_output(stdout: str) -> dict:
    """从混合了 [INFO] 行的 stdout 中提取最后一个 JSON 对象。"""
    # export get 可能直接返回 JSON，也可能混合 [INFO] 行
    matches = re.findall(r"\{[^{}]*\}", stdout, re.DOTALL)
    if matches:
        return json.loads(matches[-1])
    raise ValueError(f"Cannot extract JSON from stdout: {stdout[:500]}")


def _stdout_contains_info(stdout: str, keyword: str) -> bool:
    """检查 stdout 中是否包含指定的 [INFO] 关键字。"""
    return keyword.lower() in stdout.lower()


# ──────────────────────────────────────────────────────────
# dws doc export — 一体化命令（提交→轮询→下载）
# ──────────────────────────────────────────────────────────


class TestExportBasic:
    """dws doc export — 正向测试"""

    def test_export_to_file_path(self, dws, export_doc_node_id):
        """导出文档到指定文件路径，应成功下载 docx 文件。"""
        with tempfile.TemporaryDirectory(prefix="cli_export_") as tmpdir:
            output_path = os.path.join(tmpdir, "exported.docx")
            result = dws.run_raw(
                "doc", "export",
                "--node", export_doc_node_id,
                "--output", output_path,
            )
            assert result.returncode == 0, (
                f"导出命令应成功: stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
            )
            # 验证进度输出
            assert _stdout_contains_info(result.stdout, "提交导出任务"), (
                f"stdout 应包含提交任务的进度信息: {result.stdout[:300]}"
            )
            assert _stdout_contains_info(result.stdout, "导出完成"), (
                f"stdout 应包含导出完成信息: {result.stdout[:300]}"
            )
            # 验证文件已下载
            assert os.path.exists(output_path), (
                f"导出文件应存在: {output_path}"
            )
            assert os.path.getsize(output_path) > 0, (
                f"导出文件不应为空: {output_path}"
            )

    def test_export_polling_progress(self, dws, export_doc_node_id):
        """验证导出命令的三阶段轮询进度输出完整且格式正确。"""
        with tempfile.TemporaryDirectory(prefix="cli_export_") as tmpdir:
            output_path = os.path.join(tmpdir, "poll_test.docx")
            result = dws.run_raw(
                "doc", "export",
                "--node", export_doc_node_id,
                "--output", output_path,
            )
            assert result.returncode == 0, (
                f"导出命令应成功: stdout={result.stdout[:500]}, stderr={result.stderr[:300]}"
            )
            stdout = result.stdout

            # Step 1: 应包含提交任务进度
            assert "[1/3]" in stdout, f"stdout 应包含 [1/3] 步骤标记: {stdout[:300]}"
            assert _stdout_contains_info(stdout, "提交导出任务"), (
                f"stdout 应包含提交导出任务: {stdout[:300]}"
            )

            # 应输出 jobId
            job_id_match = re.search(r"jobId:\s*(\d+)", stdout)
            assert job_id_match, f"stdout 应包含 jobId: {stdout[:300]}"

            # Step 2: 应包含轮询进度（至少 1 次）
            assert "[2/3]" in stdout, f"stdout 应包含 [2/3] 步骤标记: {stdout[:300]}"
            poll_matches = re.findall(r"第 (\d+)/30 次查询", stdout)
            assert len(poll_matches) >= 1, (
                f"stdout 应包含至少 1 次轮询记录: {stdout[:500]}"
            )
            # 轮询计数应从 1 开始
            assert poll_matches[0] == "1", (
                f"轮询应从第 1 次开始，实际: {poll_matches[0]}"
            )

            # Step 3: 应包含下载进度和完成信息
            assert "[3/3]" in stdout, f"stdout 应包含 [3/3] 步骤标记: {stdout[:300]}"
            assert _stdout_contains_info(stdout, "下载文件到"), (
                f"stdout 应包含下载文件信息: {stdout[:300]}"
            )
            assert _stdout_contains_info(stdout, "导出完成"), (
                f"stdout 应包含导出完成: {stdout[:300]}"
            )

    def test_export_to_directory(self, dws, export_doc_node_id):
        """导出文档到目录（自动推断文件名），应成功下载。"""
        with tempfile.TemporaryDirectory(prefix="cli_export_") as tmpdir:
            result = dws.run_raw(
                "doc", "export",
                "--node", export_doc_node_id,
                "--output", tmpdir,
            )
            assert result.returncode == 0, (
                f"导出到目录应成功: stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
            )
            # 目录下应有一个文件被下载
            files = os.listdir(tmpdir)
            assert len(files) > 0, (
                f"目录下应有下载的文件: {tmpdir}"
            )

    def test_export_with_url(self, dws, export_doc_node_id):
        """使用文档 URL 进行导出（先获取 URL 再导出）。"""
        # 先获取文档 URL
        info = dws.run("doc", "info", "--node", export_doc_node_id)
        doc_url = info.get("docUrl", "")
        if not doc_url:
            pytest.skip("无法获取文档 URL，跳过 URL 导出测试")

        with tempfile.TemporaryDirectory(prefix="cli_export_") as tmpdir:
            output_path = os.path.join(tmpdir, "url_export.docx")
            result = dws.run_raw(
                "doc", "export",
                "--node", doc_url,
                "--output", output_path,
            )
            assert result.returncode == 0, (
                f"URL 导出应成功: stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
            )
            assert os.path.exists(output_path), (
                f"导出文件应存在: {output_path}"
            )


class TestExportErrors:
    """dws doc export — 反向测试"""

    def test_missing_node_flag(self, dws):
        """缺少 --node 应报错。"""
        result = dws.run_raw(
            "doc", "export",
            "--output", "/tmp/test.docx",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"缺少 --node 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_missing_output_flag(self, dws, test_doc_node_id):
        """缺少 --output 应报错。"""
        result = dws.run_raw(
            "doc", "export",
            "--node", test_doc_node_id,
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"缺少 --output 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_invalid_node_id(self, dws):
        """无效 nodeId 应报错。"""
        with tempfile.TemporaryDirectory(prefix="cli_export_") as tmpdir:
            result = dws.run_raw(
                "doc", "export",
                "--node", "INVALID_NODE_99999",
                "--output", os.path.join(tmpdir, "test.docx"),
            )
            combined = (result.stdout + result.stderr).lower()
            assert result.returncode != 0 or "error" in combined, (
                f"无效 nodeId 应报错: stdout={result.stdout}, stderr={result.stderr}"
            )


# ──────────────────────────────────────────────────────────
# dws doc export get — 手动查询兜底命令
# ──────────────────────────────────────────────────────────


class TestExportGetBasic:
    """dws doc export get — 正向测试"""

    def test_get_with_valid_job_id(self, dws, export_doc_node_id):
        """先提交导出任务拿到 jobId，再用 export get 查询状态。"""
        # Step 1: 用 export 命令拿到 jobId（通过 stdout 提取）
        # 由于 export 是一体化命令，我们直接用 run_raw 执行并从输出中提取 jobId
        with tempfile.TemporaryDirectory(prefix="cli_export_") as tmpdir:
            export_result = dws.run_raw(
                "doc", "export",
                "--node", export_doc_node_id,
                "--output", os.path.join(tmpdir, "test.docx"),
            )
            # 从输出中提取 jobId
            job_id_match = re.search(r"jobId:\s*(\S+)", export_result.stdout)
            if not job_id_match:
                pytest.skip("无法从 export 输出中提取 jobId，跳过 export get 测试")
            job_id = job_id_match.group(1)

        # Step 2: 用 export get 查询
        get_result = dws.run_raw(
            "doc", "export", "get",
            "--job-id", job_id,
        )
        # 任务可能已完成或仍在处理中，都算正常
        assert get_result.returncode == 0, (
            f"export get 应成功: stdout={get_result.stdout[:300]}, stderr={get_result.stderr[:300]}"
        )
        data = _extract_json_from_output(get_result.stdout)
        assert "status" in data, f"响应应包含 status 字段: {data}"
        assert data["status"].upper() in ("SUCCESS", "PROCESSING", "FAILED"), (
            f"status 应为合法值: {data}"
        )


class TestExportGetErrors:
    """dws doc export get — 反向测试"""

    def test_missing_job_id_flag(self, dws):
        """缺少 --job-id 应报错。"""
        result = dws.run_raw(
            "doc", "export", "get",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"缺少 --job-id 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_invalid_job_id(self, dws):
        """无效 jobId 应报错。"""
        result = dws.run_raw(
            "doc", "export", "get",
            "--job-id", "INVALID_JOB_99999",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"无效 jobId 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_empty_job_id(self, dws):
        """空 jobId 应报错。"""
        result = dws.run_raw(
            "doc", "export", "get",
            "--job-id", "",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined, (
            f"空 jobId 应报错: stdout={result.stdout}, stderr={result.stderr}"
        )
