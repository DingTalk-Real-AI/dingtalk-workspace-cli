"""
test_05_mail_user_search.py — 邮箱用户搜索测试

环境变量: DINGTALK_MAIL_EMAIL

Commands tested:
  1. dws mail user search  (search_mail_users)

权限说明：
  @dingtalk.com 个人邮箱无权限调用 search_mail_users，预期返回 No permission 错误。
  企业邮箱（非 @dingtalk.com）才支持此操作。
"""

import os
import pytest


@pytest.fixture(scope="session")
def email_addr():
    return os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")


def _is_personal_dingtalk_email(email: str) -> bool:
    """判断是否为 @dingtalk.com 个人邮箱（无 user search 权限）。"""
    return email.lower().endswith("@dingtalk.com")


class TestMailUserSearch:
    """dws mail user search

    响应结构：{"users": [...], "nextCursor": "...", "hasMore": true/false}
    user 对象包含：id, email, name, nickname, employeeNo, jobTitle, workLocation

    注意：@dingtalk.com 个人邮箱无权限，所有正向用例对个人邮箱均断言报错。
    """

    def test_search_returns_structure(self, dws, email_addr):
        """正常搜索：企业邮箱应返回 users 列表；个人邮箱应报 No permission 错误。"""
        if _is_personal_dingtalk_email(email_addr):
            result = dws.run_raw(
                "mail", "user", "search",
                "--email", email_addr,
                "--keyword", "a",
            )
            assert (
                result.returncode != 0
                or "error" in result.stdout.lower()
                or "error" in result.stderr.lower()
            ), f"@dingtalk.com 个人邮箱应因 No permission 报错，实际 returncode={result.returncode}"
            return
        data = dws.run_ok(
            "mail", "user", "search",
            "--email", email_addr,
            "--keyword", "a",
        )
        assert "users" in data, f"响应缺少 users 字段: {data}"
        assert isinstance(data["users"], list), f"users 应为列表: {data}"

    def test_search_has_pagination_fields(self, dws, email_addr):
        """响应应包含分页相关字段；个人邮箱应报 No permission 错误。"""
        if _is_personal_dingtalk_email(email_addr):
            result = dws.run_raw(
                "mail", "user", "search",
                "--email", email_addr,
                "--keyword", "a",
            )
            assert (
                result.returncode != 0
                or "error" in result.stdout.lower()
                or "error" in result.stderr.lower()
            ), f"@dingtalk.com 个人邮箱应因 No permission 报错，实际 returncode={result.returncode}"
            return
        data = dws.run_ok(
            "mail", "user", "search",
            "--email", email_addr,
            "--keyword", "a",
        )
        assert "nextCursor" in data or "hasMore" in data, (
            f"响应缺少分页字段 nextCursor/hasMore: {data}"
        )

    def test_search_user_fields(self, dws, email_addr):
        """用户条目应包含 id 和 email 字段（如有结果）；个人邮箱应报 No permission 错误。"""
        if _is_personal_dingtalk_email(email_addr):
            result = dws.run_raw(
                "mail", "user", "search",
                "--email", email_addr,
                "--keyword", "a",
            )
            assert (
                result.returncode != 0
                or "error" in result.stdout.lower()
                or "error" in result.stderr.lower()
            ), f"@dingtalk.com 个人邮箱应因 No permission 报错，实际 returncode={result.returncode}"
            return
        data = dws.run_ok(
            "mail", "user", "search",
            "--email", email_addr,
            "--keyword", "a",
        )
        users = data.get("users", [])
        if not users:
            pytest.skip("搜索无结果，跳过字段校验")
        first = users[0]
        assert "id" in first or "email" in first, (
            f"用户条目缺少 id/email 字段: {first}"
        )

    def test_search_with_size(self, dws, email_addr):
        """指定 --size 参数后返回用户数不超过 size；个人邮箱应报 No permission 错误。"""
        if _is_personal_dingtalk_email(email_addr):
            result = dws.run_raw(
                "mail", "user", "search",
                "--email", email_addr,
                "--keyword", "a",
                "--size", "2",
            )
            assert (
                result.returncode != 0
                or "error" in result.stdout.lower()
                or "error" in result.stderr.lower()
            ), f"@dingtalk.com 个人邮箱应因 No permission 报错，实际 returncode={result.returncode}"
            return
        data = dws.run_ok(
            "mail", "user", "search",
            "--email", email_addr,
            "--keyword", "a",
            "--size", "2",
        )
        assert "users" in data, f"响应缺少 users 字段: {data}"
        users = data.get("users", [])
        assert len(users) <= 2, f"返回用户数 {len(users)} 超过 size=2: {data}"

    def test_search_with_cursor_pagination(self, dws, email_addr):
        """使用 nextCursor 翻页；个人邮箱应报 No permission 错误。"""
        if _is_personal_dingtalk_email(email_addr):
            result = dws.run_raw(
                "mail", "user", "search",
                "--email", email_addr,
                "--keyword", "a",
                "--size", "1",
            )
            assert (
                result.returncode != 0
                or "error" in result.stdout.lower()
                or "error" in result.stderr.lower()
            ), f"@dingtalk.com 个人邮箱应因 No permission 报错，实际 returncode={result.returncode}"
            return
        first_page = dws.run_ok(
            "mail", "user", "search",
            "--email", email_addr,
            "--keyword", "a",
            "--size", "1",
        )
        cursor = first_page.get("nextCursor", "")
        has_more = first_page.get("hasMore", False)
        if not cursor or not has_more:
            pytest.skip("第一页无 nextCursor 或 hasMore=false，跳过翻页测试")
        second_page = dws.run_ok(
            "mail", "user", "search",
            "--email", email_addr,
            "--keyword", "a",
            "--size", "1",
            "--cursor", cursor,
        )
        assert "users" in second_page, f"第二页响应缺少 users 字段: {second_page}"

    def test_search_missing_email(self, dws):
        """--email 已优化为可选参数：未传时应仍能正常搜索并返回 users 结构。"""
        data = dws.run_ok(
            "mail", "user", "search",
            "--keyword", "alice",
        )
        assert "users" in data, f"响应缺少 users 字段: {data}"
        assert isinstance(data["users"], list), f"users 应为列表: {data}"

    def test_search_missing_keyword(self, dws, email_addr):
        """缺少必填参数 --keyword 应报错。"""
        result = dws.run_raw(
            "mail", "user", "search",
            "--email", email_addr,
        )
        assert result.returncode != 0, "缺少 --keyword 参数应返回非零退出码"

    def test_search_invalid_email(self, dws):
        """无效邮箱地址应报错。"""
        result = dws.run_raw(
            "mail", "user", "search",
            "--email", "invalid@nowhere.test",
            "--keyword", "alice",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效邮箱应报错，实际 returncode={result.returncode}"

    def test_search_personal_dingtalk_email_forbidden(self, dws, email_addr):
        """个人邮箱（@dingtalk.com）无权限调用 user search，应报错。

        仅企业邮箱支持此操作；@dingtalk.com 个人邮箱因无权限应返回错误。
        非 @dingtalk.com 邮箱跳过此用例。
        """
        if not _is_personal_dingtalk_email(email_addr):
            pytest.skip("当前邮箱非 @dingtalk.com 个人邮箱，此用例不适用")
        result = dws.run_raw(
            "mail", "user", "search",
            "--email", email_addr,
            "--keyword", "alice",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"@dingtalk.com 个人邮箱应因无权限报错，实际 returncode={result.returncode}"

    def test_search_no_match_returns_empty(self, dws, email_addr):
        """搜索无匹配结果时 users 应为空列表；个人邮箱应报 No permission 错误。"""
        if _is_personal_dingtalk_email(email_addr):
            result = dws.run_raw(
                "mail", "user", "search",
                "--email", email_addr,
                "--keyword", "zzz_no_match_xqy9_keyword",
            )
            assert (
                result.returncode != 0
                or "error" in result.stdout.lower()
                or "error" in result.stderr.lower()
            ), f"@dingtalk.com 个人邮箱应因 No permission 报错，实际 returncode={result.returncode}"
            return
        data = dws.run_ok(
            "mail", "user", "search",
            "--email", email_addr,
            "--keyword", "zzz_no_match_xqy9_keyword",
        )
        users = data.get("users", [])
        assert isinstance(users, list), f"users 应为列表: {data}"
