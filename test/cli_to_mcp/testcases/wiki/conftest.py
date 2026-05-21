import time
import uuid

import pytest


@pytest.fixture(scope="session")
def test_workspace_id(dws):
    """
    创建一个临时知识库供 member 用例复用，返回 workspaceId。

    依赖：
      - 当前账号在组织内具备「创建知识库」权限；
      - dws-wukong v0.2.55+，包含 wiki space create 命令。
    """
    name = f"CLI_WikiTest_{int(time.time())}_{uuid.uuid4().hex[:6]}"
    data = dws.run("wiki", "space", "create", "--name", name)
    inner = data.get("result", data)
    workspace_id = (
        inner.get("workspaceId")
        or inner.get("id")
        or inner.get("wikiSpaceId")
    )
    assert workspace_id, (
        f"Failed to extract workspaceId from wiki space create response: {data}"
    )
    yield workspace_id
