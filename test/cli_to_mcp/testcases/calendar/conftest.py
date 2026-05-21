"""
conftest.py — Calendar-specific fixtures.
DWSRunner and dws/current_user_id come from root conftest.py.
"""

import pytest

from test_utils import extract_calendar_event_id, iso8601_cn_offset, unique_name


@pytest.fixture(scope="session")
def test_event_id(dws):
    """Create a test calendar event; yield eventId; delete on teardown."""
    title = unique_name("CLI_Test_Event")
    start = iso8601_cn_offset(hours=2)
    end = iso8601_cn_offset(hours=3)
    data = dws.run(
        "calendar", "event", "create",
        "--title", title, "--start", start, "--end", end,
    )
    # 不同后端实现可能把 id 放在不同层级，统一用公共提取函数兼容。
    event_id = extract_calendar_event_id(data)
    assert event_id, f"Event create must return eventId, got: {data}"
    print(f"\n[SETUP] Created test event: {event_id} ({title})")

    yield event_id

    print(f"\n[TEARDOWN] Deleting test event: {event_id}")
    try:
        dws.run(
            "calendar", "event", "delete",
            "--id", event_id, "--yes",
        )
    except Exception as e:
        print(f"[TEARDOWN WARNING] {e}")
