"""
conftest.py — Ding specific fixtures.
DWSRunner/dws/current_user_id come from root conftest.py.

SKIP: robotCode not registered in current test org; ding send APIs unavailable.
Set DWS_FORCE_PRODUCTS=ding (or comma-separated list) to override.
"""

import os

import pytest

_SKIP_REASON = "dws ding robotCode 不属于当前测试组织，发送类接口不可用"


def pytest_collection_modifyitems(items):
    forced = {
        p.strip().lower()
        for p in os.environ.get("DWS_FORCE_PRODUCTS", "").split(",")
        if p.strip()
    }
    if "ding" in forced:
        return  # skip disabled by DWS_FORCE_PRODUCTS
    skip = pytest.mark.skip(reason=_SKIP_REASON)
    for item in items:
        if "/ding/" in str(item.fspath):
            item.add_marker(skip)
