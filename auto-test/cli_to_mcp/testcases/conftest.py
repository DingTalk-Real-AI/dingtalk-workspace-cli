import pytest

from test_utils import DWSRunner


@pytest.fixture(scope="session")
def dws():
    return DWSRunner()
