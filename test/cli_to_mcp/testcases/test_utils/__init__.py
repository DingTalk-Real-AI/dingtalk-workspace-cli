"""
test_utils 包：集中存放 cli_to_mcp 测试复用工具函数。

设计目标：
- 所有跨产品复用的轻量工具统一放在一个目录下，便于维护与检索；
- 通过 __init__ 对外暴露稳定导入路径，业务用例侧只需:
  `from test_utils import ...`
"""

from .core_utils import first_non_empty, get_in, unique_name
from .dws_bin import resolve_dws_bin
from .id_extractors import extract_calendar_event_id, extract_todo_task_id
from .time_utils import (
    TZ_CN,
    iso8601_calendar_midnight_cn,
    iso8601_cn,
    iso8601_cn_offset,
    iso8601_date_cn,
)

__all__ = [
    "TZ_CN",
    "extract_calendar_event_id",
    "extract_todo_task_id",
    "first_non_empty",
    "get_in",
    "iso8601_calendar_midnight_cn",
    "iso8601_cn",
    "iso8601_cn_offset",
    "iso8601_date_cn",
    "resolve_dws_bin",
    "unique_name",
]
