"""
conftest.py — Doc-specific fixtures.
DWSRunner/dws come from root conftest.py.

实际 API 格式 (2026-04):
  doc search:        {"documents": [...], "logId": "...", "success": true}
  doc list:          {"hasMore": bool, "logId": "...", "nodes": [...], "success": bool}
  doc create:        {"nodeId": "...", "docUrl": "...", "success": true, ...}
  doc file create:   {"nodeId": "...", "contentType": "...", "success": true, ...}
  doc folder create: {"nodeId": "...", "success": true, ...}
  doc read:          markdown text
  doc update:        {"success": true, ...}

注: doc space get-root 已移除, 所有创建命令默认写入"我的文档"根目录.
"""

import json
import sys
import time
import uuid

import pytest

@pytest.fixture(scope="session")
def root_uuid(dws):
    """Get my docs root dentry UUID."""
    data = dws.run("doc", "space", "get-root")
    uid = data.get("rootDentryUuid")
    assert uid and isinstance(uid, str), (
        f"doc space get-root must return rootDentryUuid, got: {data}"
    )
    return uid

def _print_login_identity(dws) -> str:
    """打印当前 dws 登录身份（userId / 姓名 / corpId），返回 userId。

    用途：让 CI 日志一眼能看到"测试运行时到底是哪个账号在操作"，
    在出现 AUTH_PERMISSION_DENIED 时可立即区分是【代码 bug】
    还是【CI 端 token 注入到了错误账号】。
    """
    result = dws.run_raw("contact", "user", "get-self")
    if result.returncode != 0:
        print(
            f"\n[doc/conftest] ⚠️ contact user get-self failed: rc={result.returncode}, "
            f"stderr={(result.stderr or '')[:200]}",
            file=sys.stderr,
        )
        return ""
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        print(
            f"\n[doc/conftest] ⚠️ contact user get-self returned non-JSON: "
            f"{(result.stdout or '')[:200]}",
            file=sys.stderr,
        )
        return ""

    items = data.get("result") or data.get("data", {}).get("result") or []
    if not items:
        print(
            f"\n[doc/conftest] ⚠️ contact user get-self response has empty result: "
            f"{json.dumps(data, ensure_ascii=False)[:300]}",
            file=sys.stderr,
        )
        return ""

    org = items[0].get("orgEmployeeModel", {})
    user_id = org.get("userId", "")
    name = org.get("name", "")
    corp_id = org.get("corpId", "")
    print(
        f"\n[doc/conftest] 🪪 dws current login identity: "
        f"userId={user_id} name={name!r} corpId={corp_id}"
    )
    return user_id

@pytest.fixture(scope="session")
def test_doc_node_id(dws):
    """Create a temporary document for media insert / permission tests, yield its nodeId."""
    # Step 1: 打印身份（仅用于 CI 日志排障，失败不阻塞 fixture）
    _print_login_identity(dws)

    # Step 2: 创建临时文档
    doc_name = f"CLI_MediaTest_{int(time.time())}_{uuid.uuid4().hex[:6]}"
    data = dws.run("doc", "create", "--name", doc_name)
    result = data.get("result", data)
    node_id = result.get("nodeId") or result.get("id") or result.get("dentryUuid")
    assert node_id, f"Failed to extract nodeId from doc create response: {data}"
    print(f"[doc/conftest] 📄 created test doc: nodeId={node_id} name={doc_name!r}")

    yield node_id


@pytest.fixture(scope="session")
def export_doc_node_id(dws):
    """动态创建带内容的文档用于导出测试（避免使用写死的 nodeId）。

    线上环境导出空白文档会返回业务错误，因此创建时带上初始内容。
    """
    doc_name = f"CLI_ExportTest_{int(time.time())}_{uuid.uuid4().hex[:6]}"
    data = dws.run(
        "doc", "create",
        "--name", doc_name,
        "--content", "export test content",
    )
    result = data.get("result", data)
    node_id = result.get("nodeId") or result.get("id") or result.get("dentryUuid")
    assert node_id, f"Failed to extract nodeId from doc create response: {data}"
    print(f"[doc/conftest] 📄 created export test doc: nodeId={node_id} name={doc_name!r}")

    # 等待服务端索引完成，避免刚创建就导出时服务端尚未就绪
    time.sleep(3)

    yield node_id
