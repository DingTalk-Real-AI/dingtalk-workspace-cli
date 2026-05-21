"""
conftest.py — Mail specific fixtures.
DWSRunner/dws/current_user_id come from root conftest.py.

SKIP: current org does not support Alibaba mailbox; need a non-Alibaba test account.
"""

import pytest

_SKIP_REASON = "当前钉钉组织不支持阿里巴巴邮箱，需要非阿里组织小号测试"


## Skip disabled — mail tests are now enabled.
# def pytest_collection_modifyitems(items):
#     skip = pytest.mark.skip(reason=_SKIP_REASON)
#     for item in items:
#         if "/mail/" in str(item.fspath):
#             item.add_marker(skip)
