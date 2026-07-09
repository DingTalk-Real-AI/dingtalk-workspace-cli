from test_utils import combined_output, dry_run_args


def assert_ok(result):
    output = combined_output(result)
    assert result.returncode == 0, output
    return output


def test_agoal_strategy_and_contract_cli_to_mcp(dws):
    output = assert_ok(
        dws.run_raw(
            "agoal",
            "strategy",
            "list",
            "--scope-type",
            "PERSONAL",
            "--scope-id",
            "user123",
            "--request-id",
            "req-1",
            "--dry-run",
        )
    )
    assert "list_strategy_decodings" in output
    assert dry_run_args(output) == {
        "scopeType": "PERSONAL",
        "openId": "user123",
        "requestId": "req-1",
    }

    output = assert_ok(
        dws.run_raw(
            "agoal",
            "strategy",
            "update",
            "--profile-id",
            "profile123",
            "--content",
            '[{"id":"e1","title":{"title":"new"}}]',
            "--dry-run",
        )
    )
    assert "update_strategy_decoding" in output
    assert dry_run_args(output) == {
        "profileId": "profile123",
        "content": [{"id": "e1", "title": {"title": "new"}}],
    }

    output = assert_ok(dws.run_raw("agoal", "contract", "fields", "--dry-run"))
    assert "list_op_contract_fields" in output
    assert dry_run_args(output) == {}

    output = assert_ok(
        dws.run_raw(
            "agoal",
            "contract",
            "update",
            "--contract-id",
            "contract123",
            "--dimensions",
            '[{"id":"dim1","title":"metric"}]',
            "--audit-config",
            '{"needAudit":true}',
            "--objective-template",
            '{"id":"tpl1"}',
            "--dry-run",
        )
    )
    assert "update_op_contract" in output
    assert dry_run_args(output) == {
        "contractId": "contract123",
        "dimensions": [{"id": "dim1", "title": "metric"}],
        "auditConfig": '{"needAudit":true}',
        "objectiveTemplate": '{"id":"tpl1"}',
    }


def test_agoal_scorecard_user_report_template_cli_to_mcp(dws):
    output = assert_ok(
        dws.run_raw(
            "agoal",
            "scorecard",
            "detail",
            "--selected-time",
            "2026-01-01T00:00:00+08:00",
            "--dept-id",
            "dept123",
            "--dry-run",
        )
    )
    assert "get_score_card_detail" in output
    args = dry_run_args(output)
    assert args["deptId"] == "dept123"
    assert args["selectedTime"] == 1767196800000

    output = assert_ok(
        dws.run_raw(
            "agoal",
            "scorecard",
            "update",
            "--dept-id",
            "dept123",
            "--selected-time",
            "2026-01-01",
            "--id",
            "sc123",
            "--tracking-period-type",
            "MONTHLY",
            "--content",
            '[{"id":"dim1","items":[]}]',
            "--dry-run",
        )
    )
    assert "update_score_card" in output
    args = dry_run_args(output)
    assert args["selectedTime"] == 1767196800000
    assert args["content"] == [{"id": "dim1", "items": []}]

    output = assert_ok(
        dws.run_raw(
            "agoal",
            "user",
            "objectives",
            "--user-id",
            "user123",
            "--rule-id",
            "rule123",
            "--period-ids",
            "p1,p2",
            "--dry-run",
        )
    )
    assert "list_user_objectives" in output
    assert dry_run_args(output) == {
        "dingUserId": "user123",
        "objectiveRuleId": "rule123",
        "periodIds": ["p1", "p2"],
    }

    output = assert_ok(
        dws.run_raw(
            "agoal",
            "report",
            "submit-detail",
            "--template-id",
            "tpl123",
            "--submit-state",
            "LATE",
            "--query-date",
            "2026-06-18T00:00:00+08:00",
            "--page",
            "1",
            "--page-size",
            "20",
            "--keyword",
            "alice",
            "--dry-run",
        )
    )
    assert "get_submit_detail" in output
    assert dry_run_args(output) == {
        "templateId": "tpl123",
        "submitState": "LATE",
        "queryDate": "2026-06-18",
        "page": 1,
        "pageSize": 20,
        "keyword": "alice",
    }

    output = assert_ok(
        dws.run_raw(
            "agoal",
            "obj-template",
            "create-or-update",
            "--title",
            "tpl",
            "--dimensions",
            '[{"title":"dim"}]',
            "--objective-weight",
            "--dimension-weight",
            "--compute-by-weight",
            "--dry-run",
        )
    )
    assert "create_or_update_obj_template" in output
    assert dry_run_args(output) == {
        "title": "tpl",
        "dimensions": '[{"title":"dim"}]',
        "objectiveWeight": True,
        "dimensionWeight": True,
        "computeByWeight": True,
    }
