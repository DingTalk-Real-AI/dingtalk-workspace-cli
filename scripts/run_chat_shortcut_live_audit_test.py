#!/usr/bin/env python3
"""Deterministic regression checks for the live Chat audit summarizer."""

from __future__ import annotations

from run_chat_shortcut_live_audit import (
    Capture,
    distinct_lower_collection_count,
    distinct_lower_message_count,
    primary_projection_for_message_metrics,
    reaction_message_count,
    summarize_capture,
    thread_message_count,
)


def check(name: str, condition: bool) -> None:
    if not condition:
        raise AssertionError(f"FAIL: {name}")
    print(f"  ok: {name}")


def test_compatibility_aliases_are_counted_once() -> None:
    rows = [{"threadId": "thread-1", "reactions": [{"type": "LIKE"}]}]
    payload = {"messages": rows, "items": rows, "count": 1}
    projection = primary_projection_for_message_metrics(payload)

    check("messages is the canonical projection", projection is rows)
    check("reaction-bearing message counted once", reaction_message_count(projection, upper=True) == 1)
    check("thread-bearing message counted once", thread_message_count(projection, upper=True) == 1)


def test_projection_fallback_order() -> None:
    replies = [{"threadId": "thread-2"}]
    items = [{"threadId": "thread-3"}]
    check(
        "replies precedes items",
        primary_projection_for_message_metrics({"replies": replies, "items": items}) is replies,
    )
    check("items remains a supported fallback", primary_projection_for_message_metrics({"items": items}) is items)


def test_non_envelope_passthrough() -> None:
    rows = [{"threadId": "thread-4"}]
    check("bare list passes through", primary_projection_for_message_metrics(rows) is rows)


def test_thread_reply_pages_are_deduplicated() -> None:
    raw_calls = [
        (
            "chat",
            "list_topic_replies",
            {
                "result": {
                    "hasMore": True,
                    "messages": [
                        {
                            "openMessageId": "m1",
                            "openConvThreadId": "thread-1",
                            "emotionReplyList": [{"type": "LIKE"}],
                            "createTime": "2026-08-05 16:48:20",
                        }
                    ],
                }
            },
        ),
        (
            "chat",
            "list_topic_replies",
            {
                "result": {
                    "hasMore": False,
                    "messages": [
                        {
                            "openMessageId": "m1",
                            "openConvThreadId": "thread-1",
                            "emotionReplyList": [{"type": "LIKE"}],
                            "createTime": "2026-08-05 16:48:20",
                        },
                        {"openMessageId": "m0", "createTime": "2026-08-05 16:47:20"},
                    ],
                }
            },
        ),
    ]
    check(
        "boundary duplicate counted once",
        distinct_lower_message_count(raw_calls, "list_topic_replies") == 2,
    )

    capture = Capture(
        command="+thread-replies",
        argv=[],
        exit_code=0,
        stdout=(
            '{"replies":[{"messageId":"m1","threadId":"thread-1",'
            '"reactions":[{"type":"LIKE"}]},{"messageId":"m0"}],'
            '"count":2,"complete":true,"hasMore":false,"pagesFetched":2}'
        ),
        stderr="",
        raw_calls=raw_calls,
        duration_ms=1,
    )
    summary = summarize_capture(capture)
    check("multi-page projection passes", summary["status"] == "pass")
    check("all lower calls reported", summary["lower"]["call_count"] == 2)
    check("unique lower count reported", summary["lower"]["count"] == 2)
    check("boundary reaction counted once", summary["lower"]["reaction_message_count"] == 1)
    check("boundary thread counted once", summary["lower"]["thread_message_count"] == 1)


def test_group_pages_are_counted_across_all_lower_calls() -> None:
    raw_calls = [
        (
            "im",
            "list_my_groups_pagination",
            {"result": {"groups": [{"openConversationId": "g1"}], "hasMore": True}},
        ),
        (
            "im",
            "list_my_groups_pagination",
            {
                "result": {
                    "groups": [
                        {"openConversationId": "g1"},
                        {"openConversationId": "g2"},
                    ],
                    "hasMore": False,
                }
            },
        ),
    ]
    check(
        "group pages deduplicated",
        distinct_lower_collection_count(
            raw_calls,
            "list_my_groups_pagination",
            ("openConversationId", "conversationId", "id"),
        )
        == 2,
    )
    capture = Capture(
        command="+chat-list-all",
        argv=[],
        exit_code=0,
        stdout=(
            '{"groups":[{"openConversationId":"g1"},'
            '{"openConversationId":"g2"}],"count":2,'
            '"complete":true,"hasMore":false,"pagesFetched":2}'
        ),
        stderr="",
        raw_calls=raw_calls,
        duration_ms=1,
    )
    summary = summarize_capture(capture)
    check("multi-page group projection passes", summary["status"] == "pass")
    check("all group rows reported", summary["lower"]["count"] == 2)


def test_thread_reply_empty_terminal_page_passes() -> None:
    raw_calls = [
        (
            "chat",
            "list_topic_replies",
            {
                "result": {
                    "hasMore": True,
                    "messages": [
                        {"openMessageId": "m1", "createTime": "2026-08-05 16:48:20"}
                    ],
                }
            },
        ),
        ("chat", "list_topic_replies", {"result": {"hasMore": False, "messages": []}}),
    ]
    capture = Capture(
        command="+thread-replies",
        argv=[],
        exit_code=0,
        stdout=(
            '{"replies":[{"messageId":"m1"}],"count":1,'
            '"complete":true,"hasMore":false,"pagesFetched":2}'
        ),
        stderr="",
        raw_calls=raw_calls,
        duration_ms=1,
    )
    summary = summarize_capture(capture)
    check("empty terminal page passes", summary["status"] == "pass")
    check("first-page item retained", summary["lower"]["count"] == 1)


def test_empty_projection_is_not_promoted_to_non_empty_pass() -> None:
    capture = Capture(
        command="+bot-search",
        argv=[],
        exit_code=0,
        stdout='{"bots":[],"count":0}',
        stderr="",
        raw_calls=[("bot", "search_my_robots", {"result": {"robots": []}})],
        duration_ms=1,
    )
    summary = summarize_capture(capture)
    check("empty projected collection is explicit", summary["status"] == "pass_empty")

    dropped_projection = Capture(
        command="+bot-search",
        argv=[],
        exit_code=0,
        stdout='{"bots":[],"count":0}',
        stderr="",
        raw_calls=[
            ("bot", "search_my_robots", {"result": {"robots": [{"robotId": "r1"}]}})
        ],
        duration_ms=1,
    )
    summary = summarize_capture(dropped_projection)
    check("non-empty lower projection cannot pass empty", summary["status"] == "projection_mismatch")

    unknown_lower_shape = Capture(
        command="+bot-search",
        argv=[],
        exit_code=0,
        stdout='{"bots":[],"count":0}',
        stderr="",
        raw_calls=[("bot", "search_my_robots", {"result": {"unexpected": []}})],
        duration_ms=1,
    )
    summary = summarize_capture(unknown_lower_shape)
    check("unknown lower collection is not a success", summary["status"] == "projection_unverified")

    no_upper_count = Capture(
        command="+unread-chats",
        argv=[],
        exit_code=0,
        stdout='{"result":[]}',
        stderr="",
        raw_calls=[
            (
                "chat",
                "unread_message_conversation_list",
                {"result": {"conversations": []}},
            )
        ],
        duration_ms=1,
    )
    summary = summarize_capture(no_upper_count)
    check(
        "lower-confirmed empty collection without upper count is explicit",
        summary["status"] == "pass_empty",
    )


def test_incomplete_projection_is_not_promoted_to_pass() -> None:
    capture = Capture(
        command="+thread-replies",
        argv=[],
        exit_code=0,
        stdout=(
            '{"replies":[{"messageId":"m1"}],"count":1,'
            '"complete":false,"hasMore":true,"truncatedByPageLimit":true,'
            '"pagesFetched":1,"nextPage":{"time":"2026-08-05T08:48:19.136Z"}}'
        ),
        stderr="",
        raw_calls=[(
            "chat",
            "list_topic_replies",
            {"result": {"hasMore": True, "nextCursor": 1785919699136,
                        "messages": [{"openMessageId": "m1"}]}},
        )],
        duration_ms=1,
    )
    summary = summarize_capture(capture)
    check("bounded partial read is explicit", summary["status"] == "incomplete")


def main() -> None:
    tests = [
        test_compatibility_aliases_are_counted_once,
        test_projection_fallback_order,
        test_non_envelope_passthrough,
        test_thread_reply_pages_are_deduplicated,
        test_group_pages_are_counted_across_all_lower_calls,
        test_thread_reply_empty_terminal_page_passes,
        test_empty_projection_is_not_promoted_to_non_empty_pass,
        test_incomplete_projection_is_not_promoted_to_pass,
    ]
    for test in tests:
        print(f"{test.__name__}:")
        test()
    print(f"\nAll {len(tests)} test groups passed.")


if __name__ == "__main__":
    main()
