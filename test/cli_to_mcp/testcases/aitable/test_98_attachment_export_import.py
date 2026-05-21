"""
test_98_attachment_export_import.py — 附件上传/导出/导入流程验证

覆盖 aitable-attachment.md / aitable-export-import.md 中的声明：
- attachment upload: 获取上传凭证（返回 uploadUrl + fileToken）
- export data: 异步任务创建 + taskId 轮询
- import upload: 三步导入流程（申请凭证）
"""

import json
import os
import subprocess
import time

import pytest
from test_utils import resolve_dws_bin

DWS_BIN = resolve_dws_bin(__file__)


# ═══════════════════════════════════════════════════════════════
# attachment upload — 上传凭证获取
# ═══════════════════════════════════════════════════════════════

class TestAttachmentUpload:
    """验证 attachment upload 命令返回上传凭证。"""

    def test_upload_returns_upload_url_and_token(self, dws, test_base_id):
        """attachment upload 应返回 uploadUrl 和 fileToken。"""
        data = dws.run(
            "aitable", "attachment", "upload",
            "--base-id", test_base_id,
            "--file-name", "test_report.pdf",
            "--size", "1024",
            "--mime-type", "application/pdf",
        )
        body = data.get("data", {})
        # 应该包含 uploadUrl 或 resourceUrl
        upload_url = body.get("uploadUrl") or body.get("resourceUrl")
        file_token = body.get("fileToken") or body.get("resourceId")
        assert upload_url, f"attachment upload should return uploadUrl, got: {data}"
        assert file_token, f"attachment upload should return fileToken/resourceId, got: {data}"
        print(f"  [OK] uploadUrl={upload_url[:60]}... fileToken={file_token}")

    def test_upload_infers_mime_from_extension(self, dws, test_base_id):
        """不传 mime-type 时，应根据扩展名推断。"""
        data = dws.run(
            "aitable", "attachment", "upload",
            "--base-id", test_base_id,
            "--file-name", "photo.png",
            "--size", "2048",
        )
        body = data.get("data", {})
        upload_url = body.get("uploadUrl") or body.get("resourceUrl")
        assert upload_url, f"upload without mime-type should still succeed, got: {data}"


# ═══════════════════════════════════════════════════════════════
# export data — 异步导出任务
# ═══════════════════════════════════════════════════════════════

class TestExportData:
    """验证 export data 异步导出流程。"""

    @pytest.fixture(scope="class")
    def export_table_id(self, dws, test_base_id):
        """Create a small table for export testing."""
        ts = int(time.time())
        fields = [
            {"fieldName": "名称", "type": "text"},
            {"fieldName": "金额", "type": "number", "config": {"formatter": "INT"}},
        ]
        data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", f"ExportTest_{ts}",
            "--fields", json.dumps(fields, ensure_ascii=False),
        )
        table_id = data["data"]["tableId"]
        # Insert a record
        fm_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fm = {f["fieldName"]: f["fieldId"] for f in fm_data["data"]["tables"][0].get("fields", [])}
        dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["名称"]: "测试导出", fm["金额"]: 100}}]),
        )
        return table_id

    def _run_export(self, dws, *args):
        """export data 的 --format 用于指定导出格式（excel 等），不能追加全局 --format json。

        该子命令默认输出 JSON，无需额外指定输出格式。
        """
        import shlex
        cmd = [DWS_BIN, *args]
        print(f"DWS_CMD: {' '.join(shlex.quote(x) for x in cmd)}")
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
        for text in ((result.stdout or "").strip(), (result.stderr or "").strip()):
            if not text:
                continue
            try:
                return json.loads(text)
            except json.JSONDecodeError:
                continue
        pytest.fail(f"export returned non-JSON:\n  stdout: {result.stdout[:300]}\n  stderr: {result.stderr[:300]}")

    def test_export_creates_task(self, dws, test_base_id, export_table_id):
        """export data --scope table 应创建任务并返回 taskId 或 downloadUrl。"""
        data = self._run_export(
            dws,
            "aitable", "export", "data",
            "--base-id", test_base_id,
            "--scope", "table",
            "--table-id", export_table_id,
            "--format", "excel",
            "--timeout-ms", "5000",
        )
        body = data.get("data", {})
        download_url = body.get("downloadUrl")
        task_id = body.get("taskId")
        assert download_url or task_id, f"export should return downloadUrl or taskId, got: {data}"

        if task_id and not download_url:
            poll_data = self._run_export(
                dws,
                "aitable", "export", "data",
                "--base-id", test_base_id,
                "--task-id", task_id,
                "--timeout-ms", "10000",
            )
            poll_body = poll_data.get("data", {})
            download_url = poll_body.get("downloadUrl")
            print(f"  [POLL] taskId={task_id}, downloadUrl={download_url}")

    def test_export_scope_all(self, dws, test_base_id):
        """export data --scope all 导出整个 Base。"""
        data = self._run_export(
            dws,
            "aitable", "export", "data",
            "--base-id", test_base_id,
            "--scope", "all",
            "--format", "excel",
            "--timeout-ms", "10000",
        )
        body = data.get("data", {})
        download_url = body.get("downloadUrl")
        task_id = body.get("taskId")
        assert download_url or task_id, f"export --scope all should work, got: {data}"


# ═══════════════════════════════════════════════════════════════
# import upload — 导入凭证获取
# ═══════════════════════════════════════════════════════════════

class TestImportUpload:
    """验证 import upload 返回上传凭证。"""

    def test_import_upload_returns_credentials(self, dws, test_base_id):
        """import upload 应返回 uploadUrl 和 importId。"""
        data = dws.run(
            "aitable", "import", "upload",
            "--base-id", test_base_id,
            "--file-name", "test_data.xlsx",
            "--file-size", "4096",
        )
        body = data.get("data", {})
        upload_url = body.get("uploadUrl") or body.get("resourceUrl")
        import_id = body.get("importId")
        assert upload_url, f"import upload should return uploadUrl, got: {data}"
        assert import_id, f"import upload should return importId, got: {data}"
        print(f"  [OK] importId={import_id}, uploadUrl={upload_url[:60]}...")
