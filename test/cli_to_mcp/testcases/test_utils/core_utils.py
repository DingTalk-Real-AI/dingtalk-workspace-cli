"""
通用基础工具函数（与具体产品无关）。

这里的函数尽量保持“纯函数 + 无业务耦合”，方便各测试模块复用。
"""

from __future__ import annotations

import time
import uuid
from typing import Any


def unique_name(prefix: str = "CLI_Test") -> str:
    """
    生成低冲突、可读性高的测试资源名。

    命名格式：`<prefix>_<unix秒级时间戳>_<短UUID>`
    这样做的好处：
    - 出问题时可从名称快速判断资源创建时间；
    - 并发跑测时减少重名冲突概率；
    - 保留 prefix，方便按业务类型筛查资源。
    """
    short_id = uuid.uuid4().hex[:6]
    return f"{prefix}_{int(time.time())}_{short_id}"


def get_in(data: Any, path: tuple[str, ...], default: Any = None) -> Any:
    """
    安全读取嵌套 dict 中的值，缺失时返回 default。

    示例：
    - get_in(resp, ("result", "id"))
    - get_in(resp, ("data", "result", "taskId"))

    适用于 dws 各子命令返回结构不完全一致的场景。
    """
    cur = data
    for key in path:
        if not isinstance(cur, dict):
            return default
        cur = cur.get(key)
        if cur is None:
            return default
    return cur


def first_non_empty(*values: Any) -> Any:
    """
    返回第一个“非空”值；如果都为空则返回 None。

    这里的“空”遵循 Python 真值语义：
    - None、""、[]、{}、0、False 都会被视为无效值。
    在测试 ID 提取场景中，这样的语义更贴近“找到可用标识”的需求。
    """
    for value in values:
        if value:
            return value
    return None
