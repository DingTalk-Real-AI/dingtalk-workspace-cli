"""
test_04_drive_delete.py — 钉盘删除节点到回收站测试

命令: dws drive delete --node <dentryUuid> --yes
MCP tool: delete_document (参数: dentryUuid)

约束:
  - 这是危险操作，需要 --yes / -y 确认。
  - drive list 返回同时包含 dentryId（数字格式）和 fileId（UUID 格式）两个字段；
    delete 命令的 --node 必须传入 fileId（即 dentryUuid），传入数字格式
    dentryId 服务端会拒绝。drive.md 反复强调该约束，本文件包含对应反向用例。
"""

import time
import uuid

import pytest


def _create_temp_folder(dws, prefix: str = "CLI_DriveDelete_Test") -> str:
    """创建一个临时钉盘文件夹并返回 dentryUuid（提取失败时 pytest.skip）。"""
    folder_name = f"{prefix}_{int(time.time())}_{uuid.uuid4().hex[:6]}"
    create_data = dws.run("drive", "mkdir", "--name", folder_name)
    inner = create_data.get("result", {}) if isinstance(create_data, dict) else {}
    if not isinstance(inner, dict):
        inner = {}
    file_id = (
        create_data.get("dentryUuid")
        or create_data.get("fileId")
        or inner.get("dentryUuid")
        or inner.get("fileId")
    )
    if not file_id:
        pytest.skip(
            f"drive mkdir 未返回 dentryUuid/fileId，跳过删除测试: {create_data}"
        )
    return file_id


# ─────────────────────────────────────────────────────────────
# 反向测试：参数校验
# ─────────────────────────────────────────────────────────────

class TestDriveDeleteParamErrors:
    """dws drive delete — 参数校验类反向用例"""

    def test_delete_requires_file_id(self, dws):
        """缺少 --node 应报错且非零退出。"""
        result = dws.run_raw("drive", "delete", "--yes")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"缺少 --node 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "node" in combined or "required" in combined or "error" in combined

    def test_delete_invalid_file_id(self, dws):
        """无效 fileId 时 MCP server 返回业务错误，CLI 非零退出且无 panic。"""
        result = dws.run_raw(
            "drive", "delete",
            "--node", "INVALID_UUID_99999",
            "--yes",
        )
        combined = (result.stdout + result.stderr).lower()
        assert "panic" not in combined, f"不应 panic: {combined[:300]}"
        assert "runtime error" not in combined, f"不应 runtime error: {combined[:300]}"
        assert result.returncode != 0, (
            f"无效 fileId 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_delete_wrong_flag_dentry_id(self, dws):
        """使用错误的 --dentry-id flag 应被 cobra 拒绝（正确应该是 --node）。"""
        result = dws.run_raw(
            "drive", "delete",
            "--dentry-id", "some-uuid",
            "--yes",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "unknown flag" in combined or "dentry-id" in combined

    def test_delete_with_numeric_dentry_id_should_fail(self, dws):
        """drive.md 显式约束：必须用 fileId (UUID)，传入 dentryId（数字格式）应被服务端拒绝。

        说明：
          - CLI 层不区分 UUID/数字（都是 string 透传），服务端 delete_document
            要求 fileId 是 UUID 格式，传入纯数字会被识别为非法节点。
          - 本用例覆盖 drive.md 中"严禁使用 dentryId"的核心踩坑点。

        跨账号兼容性：
          - 使用极长的纯数字串（17 位）作为 sentinel 值，远超真实 dentryId 长度，
            不可能在任何企业/账号下命中真实节点；
          - 断言只判定 CLI 不 panic、且非 success（returncode != 0 或返回体含 error），
            不强绑定具体 server_error_code，避免服务端错误码调整时假红/假绿。
        """
        sentinel_numeric_id = "99999999999999999"  # 17 位纯数字，绝对不会命中真实 dentryUuid
        result = dws.run_raw(
            "drive", "delete",
            "--node", sentinel_numeric_id,
            "--yes",
        )
        combined = (result.stdout + result.stderr).lower()
        assert "panic" not in combined, f"不应 panic: {combined[:300]}"
        assert "runtime error" not in combined, (
            f"不应 runtime error: {combined[:300]}"
        )
        # 非零退出 或 响应体含 error/success=false 之一即可，避免与具体 server_error_code 耦合
        is_failed = (
            result.returncode != 0
            or "\"error\"" in combined
            or "success\":false" in combined.replace(" ", "")
            or "success\": false" in combined
        )
        assert is_failed, (
            f"非 UUID 格式的 fileId 应被服务端拒绝: "
            f"stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )


# ─────────────────────────────────────────────────────────────
# 正向测试：基础删除 + 字段断言
# ─────────────────────────────────────────────────────────────

class TestDriveDeleteBasic:
    """dws drive delete — 正向用例（创建 → 删除 → 字段断言）"""

    def test_delete_creates_then_deletes_with_yes(self, dws):
        """先创建文件夹再用 --yes 删除，断言删除成功且响应字段合法。"""
        file_id = _create_temp_folder(dws)

        # dws.run() 内部已断言 success=True / 无 error 字段，正向用例直接复用
        delete_data = dws.run("drive", "delete", "--node", file_id, "--yes")
        assert isinstance(delete_data, dict), (
            f"delete 应返回 JSON 对象，实际: {type(delete_data).__name__}={delete_data!r}"
        )

    def test_delete_with_short_yes_alias(self, dws):
        """confirmDelete 同时支持 --yes 和 -y，验证 -y 短别名亦可跳过交互。"""
        file_id = _create_temp_folder(dws)
        delete_data = dws.run("drive", "delete", "--node", file_id, "-y")
        assert isinstance(delete_data, dict)


# ─────────────────────────────────────────────────────────────
# 反向测试：删除已删除节点的幂等性 / 二次删除行为
# ─────────────────────────────────────────────────────────────

class TestDriveDeleteIdempotency:
    """dws drive delete — 重复删除场景"""

    def test_delete_twice_does_not_panic(self, dws):
        """对同一个 fileId 连续删除两次，第二次不应 panic（业务报错可接受）。

        说明：服务端可能将"已在回收站"识别为业务错误返回非零退出，也可能幂等
        成功返回 success=true。本用例只断言 CLI 不崩溃、不产生 runtime error，
        将真正的"幂等/报错"语义判断留给服务端契约约束，避免与服务端实现细节耦合。
        """
        file_id = _create_temp_folder(dws)

        # 第一次删除：必须成功
        first = dws.run("drive", "delete", "--node", file_id, "--yes")
        assert isinstance(first, dict)

        # 第二次删除：用 run_raw 容忍业务错误，仅校验不崩溃
        second = dws.run_raw("drive", "delete", "--node", file_id, "--yes")
        combined = (second.stdout + second.stderr).lower()
        assert "panic" not in combined, f"二次删除不应 panic: {combined[:300]}"
        assert "runtime error" not in combined, (
            f"二次删除不应 runtime error: {combined[:300]}"
        )
