from test_utils import combined_output, dry_run_args


def assert_ok(result):
    output = combined_output(result)
    assert result.returncode == 0, output
    return output


def test_group_notice_cli_to_mcp(dws):
    output = assert_ok(
        dws.run_raw(
            "chat",
            "group",
            "notice",
            "create",
            "--group",
            "cid123",
            "--content",
            "maintenance tonight",
            "--sticky",
            "--send-ding",
            "--dry-run",
        )
    )
    assert "create_group_notice" in output
    assert dry_run_args(output) == {
        "openConversationId": "cid123",
        "content": "maintenance tonight",
        "sticky": True,
        "sendDing": True,
    }

    output = assert_ok(
        dws.run_raw(
            "chat",
            "group",
            "notice",
            "edit",
            "--group",
            "cid123",
            "--notice-id",
            "notice123",
            "--content",
            "updated",
            "--dry-run",
        )
    )
    assert "edit_group_notice" in output
    assert dry_run_args(output) == {
        "openConversationId": "cid123",
        "dataId": "notice123",
        "content": "updated",
    }

    output = assert_ok(
        dws.run_raw(
            "chat",
            "group",
            "notice",
            "get",
            "--group",
            "cid123",
            "--notice-id",
            "notice123",
            "--dry-run",
        )
    )
    assert "get_group_notice" in output
    assert dry_run_args(output) == {
        "openConversationId": "cid123",
        "dataId": "notice123",
    }

    output = assert_ok(
        dws.run_raw(
            "chat",
            "group",
            "notice",
            "list",
            "--group",
            "cid123",
            "--limit",
            "20",
            "--cursor",
            "next",
            "--scheduled",
            "--dry-run",
        )
    )
    assert "list_group_notices" in output
    assert dry_run_args(output) == {
        "openConversationId": "cid123",
        "limit": 20,
        "cursor": "next",
        "scheduled": True,
    }


def test_chat_misc_new_commands_cli_to_mcp(dws):
    output = assert_ok(
        dws.run_raw(
            "chat",
            "group",
            "share-invite",
            "--source",
            "sourceCid",
            "--target",
            "targetCid",
            "--expires-seconds",
            "3600",
            "--uuid",
            "uuid-1",
            "--dry-run",
        )
    )
    assert "share_group_invite_url" in output
    assert dry_run_args(output) == {
        "sourceOpenConversationId": "sourceCid",
        "targetOpenConversationId": "targetCid",
        "expiresSeconds": 3600,
        "uuid": "uuid-1",
    }

    output = assert_ok(
        dws.run_raw("chat", "text", "translate", "--query", "hello", "--to", "zh_CN", "--dry-run")
    )
    assert "translate" in output
    assert dry_run_args(output) == {"query": "hello", "to": "zh_CN"}

    output = assert_ok(
        dws.run_raw(
            "chat",
            "category",
            "create-smart",
            "--name",
            "priority",
            "--keywords",
            "alpha,beta",
            "--members",
            "uid1,uid2",
            "--dry-run",
        )
    )
    assert "create_smart_conv_category" in output
    assert dry_run_args(output) == {
        "title": "priority",
        "keywords": ["alpha", "beta"],
        "memberOpenDingTalkIds": ["uid1", "uid2"],
    }

    output = assert_ok(
        dws.run_raw(
            "chat",
            "message",
            "list-emotion-replies",
            "--msg-ids",
            "msg1,msg2",
            "--dry-run",
        )
    )
    assert "list_message_emotion_replies" in output
    assert dry_run_args(output) == {"openMessageIds": ["msg1", "msg2"]}
