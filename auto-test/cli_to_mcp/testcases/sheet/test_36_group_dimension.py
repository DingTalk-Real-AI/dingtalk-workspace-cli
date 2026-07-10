from test_utils import combined_output, dry_run_args


def assert_ok(result):
    output = combined_output(result)
    assert result.returncode == 0, output
    return output


def test_group_dimension_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("sheet", "group-dimension", "--help"))
    assert "dws sheet group-dimension" in output
    assert "--group-state string" in output

    output = assert_ok(
        dws.run_raw(
            "sheet",
            "group-dimension",
            "--node",
            "node123",
            "--sheet-id",
            "Sheet1",
            "--range",
            "3:7",
            "--group-state",
            "fold",
            "--dry-run",
        )
    )
    assert "group_dimension" in output
    assert dry_run_args(output) == {
        "nodeId": "node123",
        "sheetId": "Sheet1",
        "range": "3:7",
        "groupState": "fold",
    }


def test_ungroup_dimension_cli_to_mcp(dws):
    output = assert_ok(dws.run_raw("sheet", "ungroup-dimension", "--help"))
    assert "dws sheet ungroup-dimension" in output

    output = assert_ok(
        dws.run_raw(
            "sheet",
            "ungroup-dimension",
            "--node",
            "node123",
            "--sheet-id",
            "Sheet1",
            "--range",
            "C:F",
            "--dry-run",
        )
    )
    assert "ungroup_dimension" in output
    assert dry_run_args(output) == {
        "nodeId": "node123",
        "sheetId": "Sheet1",
        "range": "C:F",
    }


def test_group_dimension_rejects_invalid_state(dws):
    result = dws.run_raw(
        "sheet",
        "group-dimension",
        "--node",
        "node123",
        "--sheet-id",
        "Sheet1",
        "--range",
        "3:7",
        "--group-state",
        "invalid",
        "--dry-run",
    )
    output = combined_output(result)
    assert result.returncode != 0
    assert "group-state" in output
