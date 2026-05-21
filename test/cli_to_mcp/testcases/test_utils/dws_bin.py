"""
dws 可执行文件路径解析工具。

将路径选择逻辑下沉到 test_utils，避免散落在各 conftest 中重复维护。
"""

from __future__ import annotations

import os
import shutil
from pathlib import Path


def resolve_dws_bin(current_file: str) -> str:
    """
    解析当前测试进程应使用的 dws 二进制路径。

    优先级（从高到低）：
    1. 环境变量 `DWS_BIN`（显式指定，最可控）
    2. 自当前文件起向上逐层目录中的可执行文件 `dws`（常见：项目/单仓根目录下的 ./dws）
    3. 仓库内 `dingtalk-cli_b/dws`（make build 产物）
    4. `PATH` 中的 `dws`（兼容本机已安装场景）
    5. 字面量 `"dws"`（兜底，让调用方沿用系统查找）

    参数：
    - current_file: 调用方文件路径（通常传入 `__file__`），用于反推仓库根目录。
    """
    override = os.environ.get("DWS_BIN", "").strip()
    if override:
        return override

    # 从当前文件目录向上逐层查找，兼容：
    # - testcases/conftest.py
    # - testcases/<product>/conftest.py
    # - 其他更深层级文件
    start = Path(current_file).resolve().parent
    for parent in [start, *start.parents]:
        root_dws = parent / "dws"
        if root_dws.is_file() and os.access(root_dws, os.X_OK):
            return str(root_dws)
    for parent in [start, *start.parents]:
        local = parent / "dingtalk-cli_b" / "dws"
        if local.is_file() and os.access(local, os.X_OK):
            return str(local)

    found = shutil.which("dws")
    if found:
        return found

    return "dws"
