"""
针对不同命令响应结构的 ID 提取工具。

目的：
- 把“多分支兼容解析”集中维护，避免散落在各产品 fixture 中；
- 当后端响应结构变化时，仅需在这里改动。
"""

from __future__ import annotations

from typing import Any

from .core_utils import first_non_empty, get_in


def extract_calendar_event_id(resp: dict[str, Any]) -> str | None:
    """
    从 `calendar event create` 响应中提取 event id。

    兼容历史上常见位置：
    - 根层：`eventId` / `id`
    - result 层：`result.id` / `result.eventId`
    - data 层：`data.id` / `data.eventId`
    """
    return first_non_empty(
        resp.get("eventId"),
        resp.get("id"),
        get_in(resp, ("result", "id")),
        get_in(resp, ("result", "eventId")),
        get_in(resp, ("data", "eventId")),
        get_in(resp, ("data", "id")),
    )


def extract_todo_task_id(resp: dict[str, Any]) -> str | None:
    """
    从 `todo task create` 响应中提取 task id。

    兼容历史上常见位置：
    - result 层：`result.taskId` / `result.id`
    - 根层：`taskId` / `id`
    - data.result 层：`data.result.id`
    """
    return first_non_empty(
        get_in(resp, ("result", "taskId")),
        get_in(resp, ("result", "id")),
        resp.get("taskId"),
        resp.get("id"),
        get_in(resp, ("data", "result", "id")),
    )
