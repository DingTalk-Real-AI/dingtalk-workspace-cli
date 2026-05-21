"""mail parameter regression tests.

Wukong CLI supports sticky flags (--size20) and --sender alias,
so these tests verify they work correctly instead of expecting errors.
"""

import json
import os


class TestMailParamRegression:
    def test_search_sticky_size_flag(self, dws):
        """--size20 (sticky flag) should be accepted by wukong CLI."""
        email = os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")
        result = dws.run_raw(
            "mail", "message", "search",
            "--email", email,
            "--query", 'subject:"测试"',
            "--size20",
        )
        assert result.returncode == 0, f"sticky --size20 should succeed, stderr: {result.stderr}"
        data = json.loads(result.stdout)
        assert "messages" in data or data.get("success") == "true"

    def test_send_wrong_sender_flag(self, dws):
        """--sender alias should be accepted by wukong CLI."""
        email = os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")
        result = dws.run_raw(
            "mail", "message", "send",
            "--sender", email,
            "--to", email,
            "--subject", "X",
            "--body", "X",
        )
        assert result.returncode == 0, f"--sender alias should succeed, stderr: {result.stderr}"
        data = json.loads(result.stdout)
        assert data.get("success") == "true" or "messageId" in data
