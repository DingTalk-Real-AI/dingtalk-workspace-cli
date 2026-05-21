"""
ISO-8601 时间工具（东八区 +08:00）。

约定：
- 含时间：`YYYY-MM-DDTHH:MM:SS+08:00`（RFC 3339 / ISO-8601）
- 仅日期：`YYYY-MM-DD`
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

# 固定中国时区，避免测试运行机器时区不同导致断言波动。
TZ_CN = timezone(timedelta(hours=8))


def iso8601_cn(dt: datetime | None = None) -> str:
    """
    将 datetime 转为东八区 ISO-8601 时间字符串。

    行为说明：
    - dt 为 None：取当前东八区时间；
    - dt 为 naive datetime：按“东八区本地时间”解释；
    - dt 为 aware datetime：先转换到东八区再格式化。
    """
    if dt is None:
        dt = datetime.now(TZ_CN)
    elif dt.tzinfo is None:
        dt = dt.replace(tzinfo=TZ_CN)
    else:
        dt = dt.astimezone(TZ_CN)
    return dt.strftime("%Y-%m-%dT%H:%M:%S+08:00")


def iso8601_cn_offset(
    *,
    days: float = 0,
    hours: float = 0,
    minutes: float = 0,
    seconds: float = 0,
) -> str:
    """
    基于“当前东八区时间”做相对偏移，返回 ISO-8601 时间字符串。

    常用于构造“未来 1~2 小时”的会议/日程测试参数。
    """
    base = datetime.now(TZ_CN)
    delta = timedelta(
        days=days,
        hours=hours,
        minutes=minutes,
        seconds=seconds,
    )
    return iso8601_cn(base + delta)


def iso8601_date_cn(*, days_offset: int = 0) -> str:
    """
    返回东八区日期字符串 `YYYY-MM-DD`。

    主要用于仅接受日期参数的命令（例如 attendance 的 date/from/to）。
    """
    dt = datetime.now(TZ_CN) + timedelta(days=days_offset)
    return dt.strftime("%Y-%m-%d")


def iso8601_calendar_midnight_cn(*, days_offset: int = 0) -> str:
    """
    返回某天东八区零点时间字符串 `YYYY-MM-DDT00:00:00+08:00`。

    主要用于需要“当天起点时刻”的接口参数。
    """
    dt = datetime.now(TZ_CN) + timedelta(days=days_offset)
    return dt.strftime("%Y-%m-%dT00:00:00+08:00")
