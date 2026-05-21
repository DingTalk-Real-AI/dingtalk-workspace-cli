"""
test_92_minutes_speaker_identify_flow.py
  — 发言人识别与总结链路回归 (transcription → 身份推断 → 确认 → 总结 → speaker replace)

对齐 references/products/minutes.md 中：
  - "发言人识别与总结执行链路（用户指定人名查发言时必须遵循）"
  - "查某人在听记中说了什么（发言人识别与总结）" 意图判断
  - "反例 / 回归案例 - 案例 5：用户查某人在听记中说了什么，总结完不引导替换发言人"

链路说明（7 步）：
  Step 1: 定位听记并读取转写原文 (get transcription)
  Step 2: 声纹标注检查 — speakerNick 是否已为真实姓名
  Step 3: 转写原文内推断 — 称呼/自我介绍/上下文指代/内容特征
  Step 4: 多路并发身份推断 — 通讯录/文档/日程/聊天记录 (Agent 端能力)
  Step 5: 定向匹配 + 置信度分支 — 文本确认 vs 多候选展示
  Step 6: 结构化总结输出
  Step 7: 引导用户替换发言人 → speaker replace 写回

本文件验证 CLI 层面两个关键端点在"查某人说了什么"链路中的可用性：
  - get transcription 必须返回含 speakerNick 的结构化数据（支持声纹检查 + 原文推断）
  - speaker replace 必须在"识别 → 确认"后能正确执行替换

中间的 Step 3-6（AI 推断/多路查询/置信度判断/总结）是 Agent 端能力，
不在 CLI 层面校验。本文件聚焦于阻断"CLI 参数改名/返回字段缺失"等回归。
"""

import pytest


@pytest.fixture(scope="session")
def minutes_id(dws):
    """获取听记 ID，优先用 list all，其次 list shared。"""
    for subcmd in ("all", "shared"):
        try:
            data = dws.run_ok("minutes", "list", subcmd)
        except BaseException as exc:  # noqa: BLE001
            msg = str(exc)
            if any(
                kw in msg
                for kw in ("not_authenticated", "未登录", "auth login")
            ):
                pytest.skip(f"dws 未登录，跳过发言人识别链路用例：{msg[:200]}")
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
# Step 1 + 2: get transcription 必须返回含声纹标注信息的结构化数据
# ---------------------------------------------------------------------------
class TestTranscriptionForSpeakerIdentify:
    """get transcription 返回结构必须支持"发言人识别与总结"链路。

    在"查某人说了什么"链路中，AI 拿到转写后需要：
      Step 2: 检查 speakerNick 是否已标注为真实姓名（声纹标注检查）
      Step 3: 利用称呼/自我介绍/上下文指代等线索推断发言人身份
    这两步都依赖转写数据中的 speakerNick + text + timestamp 字段。
    """

    @staticmethod
    def _extract_sentences(data: dict) -> list[dict]:
        """从多种返回结构中提取转写句子列表。"""
        if not isinstance(data, dict):
            return []
        result = data.get("result")
        if isinstance(result, dict):
            for key in (
                "sentenceList", "itemList", "transcriptionList",
                "items", "list",
            ):
                val = result.get(key)
                if isinstance(val, list):
                    return val
            for val in result.values():
                if isinstance(val, list) and val:
                    return val
        if isinstance(result, list):
            return result
        if isinstance(data.get("data"), list):
            return data["data"]
        return []

    def test_transcription_has_speaker_for_identify(self, dws, minutes_id):
        """转写必须含发言人字段，支持 Step 2 声纹标注检查。

        如果 speakerNick 字段缺失，AI 无法判断目标人物是否已被系统标注，
        也无法在 Step 3 做基于称呼/上下文的推断。
        """
        data = dws.run_ok(
            "minutes", "get", "transcription", "--id", minutes_id,
        )
        sentences = self._extract_sentences(data)
        if not sentences:
            pytest.skip("当前听记无可用转写句子，跳过发言人识别链路校验")

        speaker_keys = (
            "speakerNick", "speakerName", "nickName",
            "subSpeakerNickname", "speaker", "speakerId",
        )
        sample = sentences[0]
        assert isinstance(sample, dict), f"句子应为 dict: {type(sample)}"
        assert any(k in sample for k in speaker_keys), (
            f"句子缺少发言人字段 (keys={list(sample.keys())})；"
            f"Step 2 声纹标注检查和 Step 3 原文推断均无法执行"
        )

    def test_transcription_has_timestamp_for_identify(self, dws, minutes_id):
        """转写必须含时间戳字段，支持 Step 3 上下文指代推断。

        时间戳用于判断发言顺序（如"张三你来说一下"后紧接着的发言人），
        也用于 Step 5 展示代表性片段时定位对应时间点。
        """
        data = dws.run_ok(
            "minutes", "get", "transcription", "--id", minutes_id,
        )
        sentences = self._extract_sentences(data)
        if not sentences:
            pytest.skip("无转写句子可校验时间戳字段")

        time_keys = (
            "startTime", "endTime", "timestamp", "begin", "end",
            "startTimeMs", "endTimeMs",
        )
        sample = sentences[0]
        assert any(k in sample for k in time_keys), (
            f"句子缺少时间戳字段 (keys={list(sample.keys())})；"
            f"AI 无法基于发言顺序做 Step 3 上下文指代推断"
        )


# ---------------------------------------------------------------------------
# Step 7: speaker replace 在"发言人识别确认后"的命令链路
# ---------------------------------------------------------------------------
class TestSpeakerReplaceAfterIdentify:
    """Step 7 引导用户替换发言人后，speaker replace 命令必须能正确执行。

    场景对照（从"查某人说了什么"链路最终下发的命令形态）：
      (a) AI 推断 + 用户确认：--from "发言人X" --to "真实姓名"
      (b) 同时关联通讯录：附加 --target-uid
      (c) 已标注但名字错误的二次替换：--from "旧姓名" --to "新姓名"
    """

    def test_replace_after_identify_basic(self, dws, minutes_id):
        """模拟 Step 7：AI 推断发言人2是张三，用户确认后替换。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--from", "发言人2",
            "--to", "张三",
        )
        assert "unknown flag" not in result.stderr, (
            f"speaker replace 在发言人识别链路下语法报错：{result.stderr[:200]}"
        )

    def test_replace_after_identify_with_uid(self, dws, minutes_id):
        """模拟 Step 7：用户确认替换并同时关联通讯录。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--from", "发言人1",
            "--to", "故愚",
            "--target-uid", "test_uid_guyu",
        )
        assert "unknown flag" not in result.stderr, (
            f"speaker replace + target-uid 在发言人识别链路下语法报错：{result.stderr[:200]}"
        )

    def test_replace_already_labeled_speaker(self, dws, minutes_id):
        """模拟声纹已标注但名字识别错的二次修正。

        例如系统声纹标注为"故惠"，但实际应该是"故愚"，
        用户通过发言人识别链路确认后要求修正。
        """
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--from", "故惠",
            "--to", "故愚",
        )
        assert "unknown flag" not in result.stderr


# ---------------------------------------------------------------------------
# 参数回归：发言人识别链路中模型最容易产生的错误参数
# ---------------------------------------------------------------------------
_FAKE_TASK_UUID = "7632756964323430"


class TestSpeakerIdentifyParamRegression:
    """覆盖"发言人识别与总结"链路中 AI 最容易写错的参数形态。

    与 test_91 的 TestClusterFlowParamRegression 互补：
      - test_91 关心"聚类 → 模糊匹配"链路的错误参数
      - 本类关心"查某人说了什么 → 推断 → 确认 → 替换"链路的错误参数
    """

    def test_speaker_replace_no_speaker_name_flag(self, dws):
        """AI 不应把目标人名作为 --speaker-name 传给 speaker replace。

        反例：模型臆测 `--speaker-name 张三` 而非正确的 `--to 张三`。
        """
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", _FAKE_TASK_UUID,
            "--from", "发言人1",
            "--speaker-name", "张三",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "speaker replace 不存在 --speaker-name 参数，必须报错"

    def test_speaker_replace_no_identify_flag(self, dws):
        """AI 不应把 --identify / --infer 之类的推断指令传给 speaker replace。

        推断是 Agent 端能力（Step 3-5），到 CLI 层面只能是确定性的
        --from + --to 替换。
        """
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", _FAKE_TASK_UUID,
            "--identify", "张三",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "speaker replace 不存在 --identify 参数，必须报错"

    def test_get_transcription_no_speaker_filter(self, dws):
        """get transcription 不应接受 --speaker 过滤参数。

        正确做法：拉取全量转写后由 AI 在本地按 speakerNick 过滤。
        反例：模型臆测 `--speaker 张三` 试图在 CLI 层面过滤特定发言人。
        """
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", _FAKE_TASK_UUID,
            "--speaker", "张三",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "get transcription 不存在 --speaker 参数，必须报错"

    def test_get_transcription_no_person_flag(self, dws):
        """get transcription 不应接受 --person 参数。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", _FAKE_TASK_UUID,
            "--person", "李总",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "get transcription 不存在 --person 参数，必须报错"

    def test_speaker_replace_no_confidence_flag(self, dws):
        """AI 不应把置信度传给 speaker replace。

        置信度判断是 Step 5 的 Agent 端逻辑，CLI 只做确定性替换。
        """
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", _FAKE_TASK_UUID,
            "--from", "发言人1", "--to", "张三",
            "--confidence", "0.85",
        )
        assert (
            result.returncode != 0
            or "unknown flag" in (result.stdout + result.stderr).lower()
            or "error" in (result.stdout + result.stderr).lower()
        ), "speaker replace 不存在 --confidence 参数，必须报错"
