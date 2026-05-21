"""
test_06_book.py — 用户日历列表测试 (1 command × 3 cases)

Commands tested:
  1. dws calendar book list  (list_calendars)
"""


def _flatten(value):
    """递归把任意嵌套结构铺开成可序列化的字符串，方便检索 'primary'。"""
    return str(value)


class TestBookList:
    """dws calendar book list"""

    def test_list_books_returns_data(self, dws):
        """查询日历本列表应成功返回。"""
        data = dws.run_ok("calendar", "book", "list")
        assert data is not None

    def test_list_books_contains_primary(self, dws):
        """返回的日历本中应包含主日历 id == 'primary'。"""
        data = dws.run_ok("calendar", "book", "list")
        flat = _flatten(data)
        assert "primary" in flat, (
            f"book list 必须包含主日历 id 'primary'，实际返回: {flat[:400]}"
        )

    def test_list_books_idempotent(self, dws):
        """多次调用应返回一致结构。"""
        d1 = dws.run_ok("calendar", "book", "list")
        d2 = dws.run_ok("calendar", "book", "list")
        assert _flatten(d1.get("data", d1)) == _flatten(d2.get("data", d2))
