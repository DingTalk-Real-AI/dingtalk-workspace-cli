"""
test_07_doc_delete.py — 文档节点删除到回收站测试

命令: dws doc delete --node <DOC_ID_OR_URL> --yes
MCP tool: delete_document (参数: dentryUuid)

注意:
  - 这是危险操作，需要 --yes / -y 确认。
  - --node 支持别名 --url / --id / --node-id / --doc-id（见 doc.go 的
    nodeAliasCmds 注册）。
  - 同时支持直接传入 nodeId 字符串，或传入 alidocs URL（CLI 会自动从
    /i/nodes/<UUID> 解析出 nodeId 后透传给 MCP）。
"""

import time
import uuid

import pytest


def _create_temp_doc(dws, prefix: str = "CLI_DocDelete_Test") -> str:
    """创建一个临时文档并返回 nodeId（提取失败时 pytest.skip）。"""
    doc_name = f"{prefix}_{int(time.time())}_{uuid.uuid4().hex[:6]}"
    create_data = dws.run("doc", "create", "--name", doc_name)
    result_data = create_data.get("result", create_data)
    node_id = (
        result_data.get("nodeId")
        or result_data.get("id")
        or result_data.get("dentryUuid")
    )
    if not node_id:
        pytest.skip(f"doc create 未返回 nodeId，跳过删除测试: {create_data}")
    return node_id


# ─────────────────────────────────────────────────────────────
# 反向测试：参数校验
# ─────────────────────────────────────────────────────────────

class TestDocDeleteParamErrors:
    """dws doc delete — 参数校验类反向用例"""

    def test_delete_requires_node(self, dws):
        """缺少 --node 应报错且非零退出。"""
        result = dws.run_raw("doc", "delete", "--yes")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"缺少 --node 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "node" in combined or "required" in combined or "error" in combined

    def test_delete_invalid_node_id(self, dws):
        """无效 nodeId 时 MCP server 返回业务错误，CLI 非零退出且无 panic。"""
        result = dws.run_raw(
            "doc", "delete",
            "--node", "INVALID_NODE_99999",
            "--yes",
        )
        combined = (result.stdout + result.stderr).lower()
        assert "panic" not in combined, f"不应 panic: {combined[:300]}"
        assert "runtime error" not in combined, f"不应 runtime error: {combined[:300]}"
        assert result.returncode != 0, (
            f"无效 nodeId 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_delete_unknown_flag_rejected(self, dws):
        """未注册的 flag 应被 cobra 拒绝。

        使用 --definitely-not-a-real-flag 作为 sentinel flag 名，避免选用任何
        可能在未来被业务复用的 flag 名（如 --dentry-uuid 历史上是真实字段名）。
        """
        result = dws.run_raw(
            "doc", "delete",
            "--definitely-not-a-real-flag", "any_value",
            "--yes",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"未注册 flag 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "unknown flag" in combined or "definitely-not-a-real-flag" in combined


# ─────────────────────────────────────────────────────────────
# 正向测试：基础删除 + 字段断言
# ─────────────────────────────────────────────────────────────

class TestDocDeleteBasic:
    """dws doc delete — 正向用例（创建 → 删除 → 字段断言）"""

    def test_delete_creates_then_deletes_with_yes(self, dws):
        """先创建文档再用 --yes 删除，断言删除成功且响应字段合法。"""
        node_id = _create_temp_doc(dws)

        # dws.run() 内部已断言 success=True / 无 error 字段，正向用例直接复用
        delete_data = dws.run("doc", "delete", "--node", node_id, "--yes")
        assert isinstance(delete_data, dict), (
            f"delete 应返回 JSON 对象，实际: {type(delete_data).__name__}={delete_data!r}"
        )

    def test_delete_with_short_yes_alias(self, dws):
        """confirmDelete 同时支持 --yes 和 -y，验证 -y 短别名亦可跳过交互。"""
        node_id = _create_temp_doc(dws)
        delete_data = dws.run("doc", "delete", "--node", node_id, "-y")
        assert isinstance(delete_data, dict)


# ─────────────────────────────────────────────────────────────
# 正向测试：--node 的全部别名（--url / --id / --node-id / --doc-id）
# 对应 doc.go 中 nodeAliasCmds 的注册
# ─────────────────────────────────────────────────────────────

class TestDocDeleteNodeAliases:
    """dws doc delete — 验证 --node 的所有隐藏别名均可用"""

    @pytest.mark.parametrize("alias_flag", ["--url", "--id", "--node-id", "--doc-id"])
    def test_delete_via_node_alias(self, dws, alias_flag):
        """传入 --url / --id / --node-id / --doc-id 应等价于 --node。"""
        node_id = _create_temp_doc(dws)
        delete_data = dws.run("doc", "delete", alias_flag, node_id, "--yes")
        assert isinstance(delete_data, dict), (
            f"通过 {alias_flag} 删除应返回 JSON 对象: {delete_data!r}"
        )


# ─────────────────────────────────────────────────────────────
# 正向测试：alidocs URL 形式的 --node
# README 显式承诺该形式可用，CLI 应能从 URL 中解析出 nodeId
# ─────────────────────────────────────────────────────────────

class TestDocDeleteWithUrl:
    """dws doc delete — 通过 alidocs URL 删除"""

    def test_delete_with_alidocs_url(self, dws):
        """传入 https://alidocs.dingtalk.com/i/nodes/<UUID> 形式的 URL 应能正确删除。"""
        node_id = _create_temp_doc(dws)
        url = f"https://alidocs.dingtalk.com/i/nodes/{node_id}"
        delete_data = dws.run("doc", "delete", "--node", url, "--yes")
        assert isinstance(delete_data, dict), (
            f"通过 URL 形式删除应返回 JSON 对象: {delete_data!r}"
        )


# ─────────────────────────────────────────────────────────────
# 反向测试：删除已删除节点的幂等性 / 二次删除行为
# ─────────────────────────────────────────────────────────────

class TestDocDeleteIdempotency:
    """dws doc delete — 重复删除场景"""

    def test_delete_twice_does_not_panic(self, dws):
        """对同一个 nodeId 连续删除两次，第二次不应 panic（业务报错可接受）。

        说明：服务端可能将"已在回收站"识别为业务错误返回非零退出，也可能幂等
        成功返回 success=true。本用例只断言 CLI 不崩溃、不产生 runtime error，
        将真正的"幂等/报错"语义判断留给服务端契约约束，避免与服务端实现细节耦合。
        """
        node_id = _create_temp_doc(dws)

        # 第一次删除：必须成功
        first = dws.run("doc", "delete", "--node", node_id, "--yes")
        assert isinstance(first, dict)

        # 第二次删除：用 run_raw 容忍业务错误，仅校验不崩溃
        second = dws.run_raw("doc", "delete", "--node", node_id, "--yes")
        combined = (second.stdout + second.stderr).lower()
        assert "panic" not in combined, f"二次删除不应 panic: {combined[:300]}"
        assert "runtime error" not in combined, (
            f"二次删除不应 runtime error: {combined[:300]}"
        )
