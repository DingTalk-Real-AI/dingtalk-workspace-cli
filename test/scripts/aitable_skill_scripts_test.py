import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "skills" / "multi" / "dingtalk-aitable" / "scripts"


def load_module(name: str, filename: str):
    spec = importlib.util.spec_from_file_location(name, SCRIPTS / filename)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


IMPORT_RECORDS = load_module("aitable_import_records", "import_records.py")
BULK_FIELDS = load_module("aitable_bulk_fields", "bulk_add_fields.py")
UPLOAD_ATTACHMENT = load_module("aitable_upload_attachment", "upload_attachment.py")
EXPORT_TASK = load_module("aitable_export_task", "aitable_export_via_task.py")
IMPORT_TASK = load_module("aitable_import_task", "aitable_import_via_task.py")


class AITableSkillScriptsTest(unittest.TestCase):
    def make_fake_dws(self, directory: Path, body: str) -> Path:
        script = directory / "fake-dws"
        script.write_text("#!/usr/bin/env python3\n" + body, encoding="utf-8")
        script.chmod(0o755)
        return script

    def run_script(self, script: str, args, cwd: Path):
        env = os.environ.copy()
        env["OPENCLAW_WORKSPACE"] = str(cwd)
        return subprocess.run(
            [sys.executable, str(SCRIPTS / script), *map(str, args)],
            cwd=cwd,
            env=env,
            text=True,
            capture_output=True,
        )

    def test_csv_values_remain_strings(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            source = root / "records.csv"
            source.write_text("fldPhone01,fldBool01\n00123,true\n", encoding="utf-8")
            previous = os.getcwd()
            os.chdir(root)
            try:
                records = IMPORT_RECORDS.load_records(str(source))
            finally:
                os.chdir(previous)
            self.assertEqual(records[0]["cells"]["fldPhone01"], "00123")
            self.assertEqual(records[0]["cells"]["fldBool01"], "true")

    def test_import_records_checks_ids_and_readback(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            source = root / "records.json"
            source.write_text('[{"cells":{"fldText01":"值"}}]', encoding="utf-8")
            fake = self.make_fake_dws(
                root,
                """import json, sys
args = sys.argv[1:]
if args[:3] == ['aitable', 'record', 'create']:
    print(json.dumps({'status':'success','data':{'newRecordIds':['rec12345678']}}))
elif args[:3] == ['aitable', 'record', 'query']:
    print(json.dumps({'status':'success','data':{'records':[{'recordId':'rec12345678'}]}}))
else:
    print(json.dumps({'status':'error','summary':'unexpected command'}))
""",
            )
            result = self.run_script(
                "import_records.py",
                ["base12345678", "table1234567", source, "--dws", fake],
                root,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            payload = json.loads(result.stdout)
            self.assertTrue(payload["complete"])
            self.assertEqual(payload["recordIds"], ["rec12345678"])
            self.assertEqual(payload["ledger"][0]["status"], "success")

    def test_import_records_treats_exit_zero_business_error_as_failure(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            source = root / "records.json"
            source.write_text('[{"cells":{"fldText01":"值"}}]', encoding="utf-8")
            fake = self.make_fake_dws(
                root,
                "import json; print(json.dumps({'status':'error','summary':'denied'}))\n",
            )
            result = self.run_script(
                "import_records.py",
                ["base12345678", "table1234567", source, "--dws", fake],
                root,
            )
            self.assertEqual(result.returncode, 2)
            payload = json.loads(result.stdout)
            self.assertFalse(payload["complete"])
            self.assertIn("业务失败", payload["ledger"][0]["error"])

    def test_bulk_fields_preserves_ai_config_and_verifies(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            source = root / "fields.json"
            source.write_text(
                json.dumps([
                    {
                        "fieldName": "AI 摘要",
                        "type": "text",
                        "aiConfig": {"outputType": "text", "prompt": [{"type": "text", "value": "摘要"}]},
                    }
                ], ensure_ascii=False),
                encoding="utf-8",
            )
            fake = self.make_fake_dws(
                root,
                """import json, sys
args = sys.argv[1:]
if args[:3] == ['aitable', 'field', 'create']:
    fields = json.loads(args[args.index('--fields') + 1])
    if 'aiConfig' not in fields[0]:
        print(json.dumps({'status':'error','summary':'aiConfig lost'}))
    else:
        print(json.dumps({'status':'success','data':{'results':[{'success':True,'fieldId':'field123456'}]}}))
elif args[:3] == ['aitable', 'field', 'get']:
    print(json.dumps({'status':'success','data':{'fields':[{'fieldId':'field123456'}]}}))
else:
    print(json.dumps({'status':'error','summary':'unexpected command'}))
""",
            )
            result = self.run_script(
                "bulk_add_fields.py",
                ["base12345678", "table1234567", source, "--dws", fake],
                root,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            payload = json.loads(result.stdout)
            self.assertTrue(payload["complete"])
            self.assertEqual(payload["fieldIds"], ["field123456"])

    def test_bulk_fields_reports_partial_result(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            source = root / "fields.json"
            source.write_text(
                '[{"fieldName":"A","type":"text"},{"fieldName":"B","type":"text"}]',
                encoding="utf-8",
            )
            fake = self.make_fake_dws(
                root,
                """import json, sys
args = sys.argv[1:]
if args[:3] == ['aitable', 'field', 'create']:
    print(json.dumps({'status':'success','data':{'results':[{'success':True,'fieldId':'field123456'},{'success':False,'reason':'duplicate'}]}}))
elif args[:3] == ['aitable', 'field', 'get']:
    print(json.dumps({'status':'success','data':{'fields':[{'fieldId':'field123456'}]}}))
""",
            )
            result = self.run_script(
                "bulk_add_fields.py",
                ["base12345678", "table1234567", source, "--dws", fake],
                root,
            )
            self.assertEqual(result.returncode, 2)
            payload = json.loads(result.stdout)
            self.assertFalse(payload["complete"])
            self.assertEqual(payload["verifiedCount"], 1)
            self.assertEqual(payload["ledger"][1]["error"], "duplicate")

    def test_upload_helpers_reject_plain_http_urls(self):
        with tempfile.TemporaryDirectory() as raw:
            file_path = Path(raw) / "file.bin"
            file_path.write_bytes(b"x")
            self.assertFalse(
                UPLOAD_ATTACHMENT.upload_to_oss(
                    "http://example.com/upload", file_path, "application/octet-stream"
                )
            )
            ok, error = IMPORT_TASK.put_file("http://example.com/upload", file_path)
            self.assertFalse(ok)
            self.assertIn("HTTPS", error)

    def test_export_helpers_reject_http_and_existing_output(self):
        with self.assertRaises(ValueError):
            EXPORT_TASK.normalize_download_url("http://example.com/file.xlsx")
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            output = root / "existing.xlsx"
            output.write_bytes(b"existing")
            previous = os.getcwd()
            os.chdir(root)
            try:
                with self.assertRaises(ValueError):
                    EXPORT_TASK.resolve_output_path("existing.xlsx", "ignored.xlsx", False)
            finally:
                os.chdir(previous)


if __name__ == "__main__":
    unittest.main()
