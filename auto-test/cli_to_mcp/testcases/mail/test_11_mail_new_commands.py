import os

from test_utils import combined_output, dry_run_args


def mail_email() -> str:
    return os.environ.get("DINGTALK_MAIL_EMAIL", "user@example.com")


def assert_ok(result):
    output = combined_output(result)
    assert result.returncode == 0, output
    return output


def assert_fails(result, expected: str):
    output = combined_output(result)
    assert result.returncode != 0, output
    assert expected in output


def test_mailbox_profile_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("mail", "mailbox", "profile", "--help"))
    assert "dws mail mailbox profile" in output
    assert "--email string" in output

    assert_fails(dws.run_raw("mail", "mailbox", "profile"), "email")

    output = assert_ok(
        dws.run_raw("mail", "mailbox", "profile", "--email", mail_email(), "--dry-run")
    )
    assert "get_mailbox_profile" in output
    assert dry_run_args(output) == {"email": mail_email()}


def test_message_batch_get_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("mail", "message", "batch-get", "--help"))
    assert "dws mail message batch-get" in output
    assert "--email string" in output
    assert "--ids string" in output

    assert_fails(
        dws.run_raw("mail", "message", "batch-get", "--email", mail_email()),
        "ids",
    )

    too_many_ids = ",".join(f"msg_{i:02d}" for i in range(21))
    assert_fails(
        dws.run_raw(
            "mail",
            "message",
            "batch-get",
            "--email",
            mail_email(),
            "--ids",
            too_many_ids,
            "--dry-run",
        ),
        "20",
    )

    output = assert_ok(
        dws.run_raw(
            "mail",
            "message",
            "batch-get",
            "--email",
            mail_email(),
            "--ids",
            "msg_001,msg_002",
            "--dry-run",
        )
    )
    assert "get_email_by_message_id" in output
    assert "msg_001" in output
    assert "msg_002" in output


def test_sent_message_recall_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("mail", "sent-message", "recall", "--help"))
    assert "dws mail sent-message recall" in output
    assert "--subject string" in output
    assert "--yes" in output

    assert_fails(
        dws.run_raw(
            "mail",
            "sent-message",
            "recall",
            "--email",
            mail_email(),
            "--id",
            "msg_001",
            "--subject",
            "subject",
        ),
        "--yes",
    )

    output = assert_ok(
        dws.run_raw(
            "mail",
            "sent-message",
            "recall",
            "--email",
            mail_email(),
            "--id",
            "msg_001",
            "--subject",
            "subject",
            "--yes",
            "--dry-run",
        )
    )
    assert "recall_sent_message" in output
    assert dry_run_args(output) == {
        "email": mail_email(),
        "id": "msg_001",
        "subject": "subject",
    }


def test_sent_message_recall_detail_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("mail", "sent-message", "recall-detail", "--help"))
    assert "dws mail sent-message recall-detail" in output
    assert "--email string" in output
    assert "--id string" in output
    assert "FINISHED" in output

    output = assert_ok(
        dws.run_raw(
            "mail",
            "sent-message",
            "recall-detail",
            "--email",
            mail_email(),
            "--id",
            "task_001",
            "--dry-run",
        )
    )
    assert "get_recall_detail" in output
    assert dry_run_args(output) == {"email": mail_email(), "id": "task_001"}
