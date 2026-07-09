import os

from test_utils import combined_output, dry_run_args


def mail_email() -> str:
    return os.environ.get("DINGTALK_MAIL_EMAIL", "user@example.com")


def assert_ok(result):
    output = combined_output(result)
    assert result.returncode == 0, output
    return output


def test_auto_reply_update_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("mail", "auto-reply", "update", "--help"))
    for flag in ("--email string", "--enabled string", "--start string", "--end string", "--scope string", "--content string"):
        assert flag in output

    output = assert_ok(
        dws.run_raw(
            "mail",
            "auto-reply",
            "update",
            "--email",
            mail_email(),
            "--enabled",
            "true",
            "--start",
            "2026/07/01 09:00:00 +0800",
            "--end",
            "2026/07/07 18:00:00 +0800",
            "--scope",
            "all",
            "--content",
            "out of office",
            "--dry-run",
        )
    )
    assert "update_auto_reply" in output
    assert dry_run_args(output) == {
        "email": mail_email(),
        "enabled": True,
        "startTime": "2026/07/01 09:00:00 +0800",
        "endTime": "2026/07/07 18:00:00 +0800",
        "scope": "all",
        "content": "out of office",
    }


def test_allow_list_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("mail", "allow-list", "list", "--email", mail_email(), "--dry-run"))
    assert "list_mailbox_allowlist" in output
    assert dry_run_args(output) == {"email": mail_email()}

    output = assert_ok(
        dws.run_raw(
            "mail",
            "allow-list",
            "add",
            "--email",
            mail_email(),
            "--entries",
            "partner@example.com,@example.org",
            "--dry-run",
        )
    )
    assert "add_mailbox_allowlist" in output
    assert dry_run_args(output) == {
        "email": mail_email(),
        "entries": ["partner@example.com", "@example.org"],
    }

    output = assert_ok(
        dws.run_raw(
            "mail",
            "allow-list",
            "remove",
            "--email",
            mail_email(),
            "--entries",
            "partner@example.com",
            "--dry-run",
        )
    )
    assert "remove_mailbox_allowlist" in output
    assert dry_run_args(output) == {
        "email": mail_email(),
        "entries": ["partner@example.com"],
    }


def test_block_list_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("mail", "block-list", "list", "--email", mail_email(), "--dry-run"))
    assert "list_mailbox_blocklist" in output
    assert dry_run_args(output) == {"email": mail_email()}

    output = assert_ok(
        dws.run_raw(
            "mail",
            "block-list",
            "add",
            "--email",
            mail_email(),
            "--entries",
            "spam@example.com,@junk.example",
            "--dry-run",
        )
    )
    assert "add_mailbox_blocklist" in output
    assert dry_run_args(output) == {
        "email": mail_email(),
        "entries": ["spam@example.com", "@junk.example"],
    }

    output = assert_ok(
        dws.run_raw(
            "mail",
            "block-list",
            "remove",
            "--email",
            mail_email(),
            "--entries",
            "spam@example.com",
            "--dry-run",
        )
    )
    assert "remove_mailbox_blocklist" in output
    assert dry_run_args(output) == {
        "email": mail_email(),
        "entries": ["spam@example.com"],
    }
