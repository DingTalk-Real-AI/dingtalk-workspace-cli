"""将 `dws minutes list mine|shared|all` 的 JSON 规范为 (taskUuid, title) 列表。"""

from __future__ import annotations

import json
from typing import Any, List, Tuple


def _unwrap_rows(payload: Any) -> List[Any]:
    if isinstance(payload, list):
        return payload
    if not isinstance(payload, dict):
        return []
    for key in ('result', 'data', 'list'):
        value = payload.get(key)
        if isinstance(value, list):
            return value
        if isinstance(value, dict):
            for inner_key in (
                'items', 'list', 'records', 'minutes',
            ):
                inner = value.get(inner_key)
                if isinstance(inner, list):
                    return inner
    return []


def uuid_title_pairs_from_payload(payload: Any) -> List[Tuple[str, str]]:
    """列表项可为对象、JSON 字符串、或纯 taskUuid 字符串。"""
    out: List[Tuple[str, str]] = []
    for item in _unwrap_rows(payload):
        if isinstance(item, dict):
            uuid = item.get('taskUuid') or item.get('id') or item.get('task_uuid')
            if not uuid:
                continue
            title = item.get('title') or item.get('name') or '无标题'
            # 确保值是基本类型再转换
            if not isinstance(uuid, (str, int, float, bool)):
                continue
            if not isinstance(title, (str, int, float, bool)):
                title = str(title) if isinstance(title, dict) else '无标题'
            out.append((str(uuid), str(title)))
        elif isinstance(item, str):
            text = item.strip()
            if not text:
                continue
            if text.startswith('{'):
                try:
                    parsed = json.loads(text)
                except json.JSONDecodeError:
                    continue
                if not isinstance(parsed, dict):
                    continue
                uuid = (
                    parsed.get('taskUuid')
                    or parsed.get('id')
                    or parsed.get('task_uuid')
                )
                if not uuid:
                    continue
                title = parsed.get('title') or parsed.get('name') or '无标题'
                out.append((str(uuid), str(title)))
            else:
                out.append((text, text))
    return out
