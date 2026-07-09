from test_utils import combined_output


def assert_ok(result):
    output = combined_output(result)
    assert result.returncode == 0, output
    return output


def test_doc_import_help_and_validation(dws, tmp_path):
    output = assert_ok(dws.run_raw("doc", "import", "--help"))
    assert "dws doc import" in output
    assert "--file string" in output
    assert "--workspace string" in output
    assert "--name string" in output

    result = dws.run_raw("doc", "import", "--file", str(tmp_path / "missing.md"), "--dry-run")
    output = combined_output(result)
    assert result.returncode != 0
    assert "cannot read file" in output

    bad = tmp_path / "bad.exe"
    bad.write_text("bad", encoding="utf-8")
    result = dws.run_raw("doc", "import", "--file", str(bad), "--dry-run")
    output = combined_output(result)
    assert result.returncode != 0
    assert "unsupported file format" in output


def test_doc_import_dry_run(dws, tmp_path):
    source = tmp_path / "sample.md"
    source.write_text("# Sample\n\nhello\n", encoding="utf-8")

    output = assert_ok(
        dws.run_raw(
            "doc",
            "import",
            "--file",
            str(source),
            "--name",
            "Imported Sample",
            "--workspace",
            "workspace123",
            "--dry-run",
        )
    )
    assert "Imported Sample" in output
    assert "sample.md" in output
    assert "md" in output


def test_doc_import_get_dry_run(dws):
    output = assert_ok(dws.run_raw("doc", "import", "get", "--task-id", "task123", "--dry-run"))
    assert "task123" in output
