import pytest

from eval_comment_parse import parse

SHA = "0123456789abcdef0123456789abcdef01234567"


def test_single_product():
    assert parse(f"/eval drive sha={SHA}") == ("drive", "", SHA)


def test_multiple_products():
    assert parse(f"/eval drive,doc,edu-app sha={SHA}") == ("drive,doc,edu-app", "", SHA)


def test_cases_ref_escape_hatch():
    assert parse(f"/eval drive sha={SHA} cases=feat/drive-latest") == (
        "drive",
        "feat/drive-latest",
        SHA,
    )


def test_extra_lines_after_command_are_ignored():
    body = f"/eval drive sha={SHA}\n\n顺便说明：这个 PR 只动了 drive 的 --latest。"
    assert parse(body) == ("drive", "", SHA)


def test_missing_products_rejected():
    with pytest.raises(ValueError, match="产品集显式必填"):
        parse("/eval")


def test_omitted_reviewed_sha_defers_to_guard():
    # 是否允许省略由 guard 按“评论者是否 PR 作者”裁决，解析层放行并输出空 reviewed_sha
    assert parse("/eval drive") == ("drive", "", "")


def test_similar_command_prefix_rejected():
    with pytest.raises(ValueError, match="/eval 命令开头"):
        parse(f"/evaluate drive sha={SHA}")


def test_illegal_product_name_rejected():
    with pytest.raises(ValueError, match="产品名非法"):
        parse(f"/eval drive;rm sha={SHA}")


def test_unknown_extra_token_rejected():
    with pytest.raises(ValueError, match="未知参数"):
        parse(f"/eval drive sha={SHA} --force")


def test_illegal_cases_ref_rejected():
    with pytest.raises(ValueError, match="cases 引用非法"):
        parse(f"/eval drive sha={SHA} cases=$(whoami)")


def test_structurally_invalid_cases_refs_rejected():
    # git check-ref-format 结构规则：字符合法但结构非法的引用必须拒绝
    invalid_refs = [
        "..",            # 以点开头且含连续点号
        "/main",         # 首部斜杠
        "feature//x",    # 连续斜杠
        "feature/",      # 尾部斜杠
        "release.",      # 尾部点号
        "a..b",          # 任意位置连续点号
        ".hidden",       # 分量以点开头
        "a/.hidden",     # 非首分量以点开头
        "a.lock",        # 分量以 .lock 结尾
        "a/b.lock",      # 非首分量以 .lock 结尾
        "-flag",         # 以 - 开头，防 git fetch 选项注入
    ]
    for ref in invalid_refs:
        with pytest.raises(ValueError, match="cases 引用非法"):
            parse(f"/eval drive sha={SHA} cases={ref}")


def test_structurally_valid_cases_refs_accepted():
    valid_refs = ["main", "feat/drive-latest", "release/v1.0", "a.b/c-d_e", SHA]
    for ref in valid_refs:
        assert parse(f"/eval drive sha={SHA} cases={ref}") == ("drive", ref, SHA)


def test_short_reviewed_sha_rejected():
    with pytest.raises(ValueError, match="审核 SHA 非法"):
        parse("/eval drive sha=0123456")


def test_empty_comment_rejected():
    with pytest.raises(ValueError, match="评论为空"):
        parse("   \n  ")
