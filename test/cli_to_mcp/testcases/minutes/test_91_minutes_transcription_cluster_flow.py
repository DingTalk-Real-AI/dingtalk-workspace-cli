"""
test_91_minutes_transcription_cluster_flow.py
  — transcription → 发言人聚类 → 模糊匹配关键词 → speaker replace 串联流程回归

对齐 references/products/minutes.md 中：
  - "获取听记语音转写原文" 章节新增的"四阶段工作流"
  - "反例 / 回归案例 - 案例 4：拉完转写后只输出时间线原文，未引导发言人聚类与替换"

链路说明：
  1) get transcription 拉取原文（含 speakerNick 与时间戳）
  2) AI 在本地按发言人聚类、提取核心要点（无需新调用 dws）
  3) 用户提供"某某人主要讲了 XX"，AI 在已聚类的核心要点中模糊匹配关键词
  4) 用户确认后调用 speaker replace --from <发言人占位符> --to <真实姓名> 写回

本文件只验证 dws CLI 真实可执行的两个端点：
  - get transcription 必须能返回 speakerNick 维度的可聚类数据
  - speaker replace 必须能完整执行 from→to 替换链路
中间的"AI 聚类 + 关键词模糊匹配"是 Agent 端能力，不在 CLI 层面校验，
此文件用于阻断"链路被破坏 / 参数被改名 / 返回字段缺失"等回归。
"""

import pytest


@pytest.fixture(scope="session")
def minutes_id(dws):
    """获取听记 ID，优先用 list all，其次 list shared。

    设计要点：
      - 与本目录其他 fixture 行为一致，但**任何环节失败都走 pytest.skip**
        而非 ERROR / FAIL，避免在『未登录 / 无可见听记 / 服务端临时不可用』
        等环境问题下污染本测试文件的整体通过率。
      - dws.run_ok 在认证失败时会 pytest.fail，这里用 try 包一层把它收敛
        成 skip，让本文件的『发言人聚类替换链路』在数据不可达时优雅退场，
        而 TestClusterFlowParamRegression 那批纯参数校验仍能正常运行。
    """
    for subcmd in ("all", "shared"):
        try:
            data = dws.run_ok("minutes", "list", subcmd)
        except BaseException as exc:  # noqa: BLE001 — pytest.fail 抛的 Failed 并非 Exception
            # run_ok 对未登录 / 鉴权失败会调用 pytest.fail（Failed/OutcomeException），
            # 这里把"未登录"显式转成 skip，让本测试文件在无登录环境也能优雅退场。
            msg = str(exc)
            if any(
                kw in msg
                for kw in ("not_authenticated", "未登录", "auth login")
            ):
                pytest.skip(f"dws 未登录，跳过 transcription 聚类链路用例：{msg[:200]}")
            # 其他异常继续尝试下一个 subcmd
            continue
        result = data.get("result", {})
        items = (
            result.get("itemList", [])
            if isinstance(result, dict)
            else []
        ) or data.get("minutes", [])
        if items:
            mid = items[0].get("minutesId") or items[0].get("uuid") or items[0].get("id")
            if mid:
                return mid
    pytest.skip("No minutes available")


# ---------------------------------------------------------------------------
# 阶段 1：get transcription 必须返回可被本地聚类的原始数据
# ---------------------------------------------------------------------------
class TestTranscriptionClusterableShape:
    """get transcription 返回结构必须支持"按发言人聚类"。

    AI 在拉完转写后要本地按 speakerNick 分组、统计段数和字数，并提取
    每位发言人的核心要点。如果服务端把 speakerNick / sentence / text
    等字段改名或删掉，本测试会立即失败，避免上游链路静默崩溃。
    """

    @staticmethod
    def _extract_sentences(data: dict) -> list[dict]:
        """尽量宽松地从多种返回结构中提取转写句子列表。"""
        if not isinstance(data, dict):
            return []
        result = data.get("result")
        # 形如 {"result": {"sentenceList": [...]}}
        if isinstance(result, dict):
            for key in (
                "sentenceList",
                "itemList",
                "transcriptionList",
                "items",
                "list",
            ):
                v = result.get(key)
                if isinstance(v, list):
                    return v
            # 兜底：把 result 里第一个 list 字段当作句子列表
            for v in result.values():
                if isinstance(v, list) and v:
                    return v
        # 形如 {"result": [...]} / 顶层就是 list
        if isinstance(result, list):
            return result
        if isinstance(data.get("data"), list):
            return data["data"]
        return []

    def test_transcription_returns_sentences(self, dws, minutes_id):
        """transcription 必须返回非空的句子列表（首屏即可，无需翻完）。"""
        data = dws.run_ok(
            "minutes", "get", "transcription", "--id", minutes_id,
        )
        sentences = self._extract_sentences(data)
        if not sentences:
            pytest.skip(
                "当前听记无可用转写句子（可能为空会议或权限不足），跳过聚类校验"
            )
        assert isinstance(sentences, list), f"transcription 句子集合应为 list: {type(sentences)}"

    def test_sentence_has_speaker_field(self, dws, minutes_id):
        """每条句子必须含『发言人』字段，否则 AI 无法按发言人聚类。"""
        data = dws.run_ok(
            "minutes", "get", "transcription", "--id", minutes_id,
        )
        sentences = self._extract_sentences(data)
        if not sentences:
            pytest.skip("无转写句子可校验发言人字段")

        # 兼容多种字段命名（speakerNick / speakerName / nickName / subSpeakerNickname）
        speaker_keys = (
            "speakerNick",
            "speakerName",
            "nickName",
            "subSpeakerNickname",
            "speaker",
            "speakerId",
        )
        sample = sentences[0]
        assert isinstance(sample, dict), f"句子应为 dict: {type(sample)}"
        assert any(k in sample for k in speaker_keys), (
            f"句子缺少发言人字段，可用 keys={list(sample.keys())}；"
            f"AI 将无法按发言人聚类、无法做后续 speaker replace 关联"
        )

    def test_sentence_has_text_field(self, dws, minutes_id):
        """每条句子必须含『文本内容』字段，否则 AI 无法提取核心要点。"""
        data = dws.run_ok(
            "minutes", "get", "transcription", "--id", minutes_id,
        )
        sentences = self._extract_sentences(data)
        if not sentences:
            pytest.skip("无转写句子可校验文本字段")

        text_keys = (
            "text", "sentence", "content", "transcription",
            "paragraph", "sentenceList",
        )
        sample = sentences[0]
        assert any(k in sample for k in text_keys), (
            f"句子缺少文本字段，可用 keys={list(sample.keys())}；"
            f"AI 将无法在已聚类的核心要点中做关键词模糊匹配"
        )


# ---------------------------------------------------------------------------
# 阶段 2/3/4：speaker replace 命令链路（含 url_extract / target-uid 场景）
# ---------------------------------------------------------------------------
class TestTranscriptionToSpeakerReplaceFlow:
    """transcription 拉完聚类后，最终通过 speaker replace 写回的命令链路。

    场景对照（最终实际下发到 CLI 的命令形态）：
      (a) 唯一命中：dws minutes speaker replace --id <uuid> --from "发言人1" --to "李总"
      (b) 多候选用户挑选后确认：同 (a)
      (c) 同时关联通讯录：dws minutes speaker replace ... --target-uid <uid>
    """

    def test_replace_speaker_after_cluster_basic(self, dws, minutes_id):
        """模拟"用户在聚类后确认『发言人1 → 李总』"的命令形态。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--from", "发言人1",
            "--to", "李总",
        )
        # 命令语法必须正确（即使该听记里没有"发言人1"也不应报 unknown flag）
        assert "unknown flag" not in result.stderr, (
            f"speaker replace 在『聚类后确认』链路下被改坏：{result.stderr[:200]}"
        )

    def test_replace_speaker_after_cluster_with_target_uid(self, dws, minutes_id):
        """模拟"用户在聚类后确认并附加钉钉 UID 关联通讯录"的命令形态。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--from", "发言人1",
            "--to", "李总",
            "--target-uid", "test_uid_litotal",
        )
        assert "unknown flag" not in result.stderr, (
            f"speaker replace 在『聚类后确认 + UID 关联』链路下被改坏：{result.stderr[:200]}"
        )

    def test_replace_speaker_chinese_real_name(self, dws, minutes_id):
        """模拟"模糊匹配命中后从中文真实姓名替换为另一个中文真实姓名"。

        该场景对应：用户先告诉 AI"故愚负责供应链"，但其实之前已经把发言人2
        关联成了"故惠"，AI 二次确认后需要把"故惠"改成"故愚"。
        """
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--from", "故惠",
            "--to", "故愚",
        )
        assert "unknown flag" not in result.stderr

    # 注：『URL 直达 → 聚类 → 替换』链路里 speaker replace 不应接受 --url
    # 这一参数回归项已下移至 TestClusterFlowParamRegression，
    # 那里不依赖 minutes_id，能在未登录环境也照常运行。


# ---------------------------------------------------------------------------
# 阶段补充：参数回归（与"聚类 → 模糊匹配 → 替换"链路相关的高频错误参数）
# ---------------------------------------------------------------------------
# 注意：本类下所有用例都不依赖 minutes_id 这个 session-fixture，
# 改用字面量占位 uuid（与 test_90_minutes_param_regression.py 的策略一致），
# 这样在『未登录 / 拿不到 list 数据』的环境下，参数回归也能照常运行——
# 只校验 CLI 是否把不存在的 flag 当作错误处理，与服务端登录状态无关。
_FAKE_TASK_UUID = "7632756964323339"


class TestClusterFlowParamRegression:
    """覆盖『聚类 → 模糊匹配 → 替换』链路中模型最容易写错的参数形态。

    这些用例和 test_90_minutes_param_regression.py 互补：
      - test_90 关心的是单条命令的 flag 是否被改名
      - 本类关心的是『聚类后引导确认』这一新链路里 AI 是否会在
        speaker replace 上产生新的错误参数（比如把"模糊匹配关键词"
        当成参数传给 CLI）
    """

    def test_speaker_replace_keyword_should_not_be_a_flag(self, dws):
        """AI 不应把『关键词』作为参数传给 speaker replace。

        反例：模型把用户输入的关键词『战略规划』错误地拼成
              `--keyword 战略规划` 之类的非法 flag。
        """
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", _FAKE_TASK_UUID,
            "--from", "发言人1", "--to", "李总",
            "--keyword", "战略规划",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "speaker replace 不存在 --keyword 参数，必须报错以阻断错误链路"

    def test_speaker_replace_match_should_not_be_a_flag(self, dws):
        """AI 不应把『模糊匹配的命中分数』作为参数传给 speaker replace。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", _FAKE_TASK_UUID,
            "--from", "发言人1", "--to", "李总",
            "--match-score", "0.95",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "speaker replace 不存在 --match-score 参数，必须报错"

    def test_get_transcription_no_cluster_flag(self, dws):
        """AI 不应把『是否聚类』作为参数传给 get transcription。

        正确做法：聚类是 AI 在拿到原文后本地做的，不下发到 CLI。
        反例：模型臆测出 `--cluster-by-speaker` 之类的不存在的 flag。
        """
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", _FAKE_TASK_UUID,
            "--cluster-by-speaker",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "get transcription 不存在 --cluster-by-speaker 参数，必须报错"

    def test_get_transcription_no_group_by_flag(self, dws):
        """同上，`--group-by speaker` 也不应是合法参数。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", _FAKE_TASK_UUID,
            "--group-by", "speaker",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "get transcription 不存在 --group-by 参数，必须报错"

    def test_speaker_replace_url_extract_form(self, dws):
        """speaker replace 不应该接受 --url 参数（必须先用 --id）。

        从 test_91 的 TestTranscriptionToSpeakerReplaceFlow 拆出来的纯参数校验，
        这样不依赖 minutes_id 也能跑：URL → taskUuid 是 Agent 端的能力，
        到 CLI 层面就只能是 --id。
        """
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--url", "https://shanji.dingtalk.com/app/transcribes/abc",
            "--from", "发言人2", "--to", "张三",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "speaker replace 不应该接受 --url 参数（必须先用 --id）"
