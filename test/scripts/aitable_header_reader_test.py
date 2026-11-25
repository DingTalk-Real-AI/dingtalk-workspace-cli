import json
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = [ROOT / 'skills/mono/scripts/read_import_headers.py', ROOT / 'skills/multi/dingtalk-aitable/scripts/read_import_headers.py']

class HeaderReaderTest(unittest.TestCase):
    def run_script(self, script, source, *args):
        return subprocess.run([sys.executable, str(script), str(source), *args], capture_output=True, text=True)

    def test_csv_returns_only_first_record(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / 'input.csv'
            p.write_text('"First, Name",Amount\nPRIVATE_SENTINEL,900\n', encoding='utf-8-sig')
            for script in SCRIPTS:
                r = self.run_script(script, p)
                self.assertEqual(r.returncode, 0, r.stderr)
                self.assertEqual(json.loads(r.stdout)['headers'], ['First, Name', 'Amount'])
                self.assertNotIn('PRIVATE_SENTINEL', r.stdout + r.stderr)

    def test_xlsx_shared_inline_sparse_and_no_data_rows(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / 'input.xlsx'
            with zipfile.ZipFile(p, 'w') as z:
                z.writestr('xl/workbook.xml', '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Orders" sheetId="1" r:id="rId1"/></sheets></workbook>')
                z.writestr('xl/_rels/workbook.xml.rels', '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"/></Relationships>')
                z.writestr('xl/sharedStrings.xml', '<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Amount</t></si><si><t>PRIVATE_SENTINEL</t></si></sst>')
                z.writestr('xl/worksheets/sheet1.xml', '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>TMS-ID</t></is></c><c r="C1" t="s"><v>0</v></c></row><row r="2"><c r="A2" t="s"><v>1</v></c><c r="C2"><v>SECRET</v></c></row></sheetData></worksheet>')
            for script in SCRIPTS:
                r = self.run_script(script, p, '--sheet', 'Orders')
                self.assertEqual(r.returncode, 0, r.stderr)
                self.assertEqual(json.loads(r.stdout)['headers'], ['TMS-ID', '', 'Amount'])
                self.assertNotIn('PRIVATE_SENTINEL', r.stdout + r.stderr)
                self.assertNotIn('SECRET', r.stdout + r.stderr)
                missing = self.run_script(script, p, '--sheet', 'missing')
                self.assertNotEqual(missing.returncode, 0)
                self.assertNotIn('SECRET', missing.stdout + missing.stderr)

    def test_xlsx_rich_text_excludes_phonetic_annotations(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / 'input.xlsx'
            with zipfile.ZipFile(p, 'w') as z:
                z.writestr('xl/workbook.xml', '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Orders" sheetId="1" r:id="rId1"/></sheets></workbook>')
                z.writestr('xl/_rels/workbook.xml.rels', '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>')
                z.writestr('xl/sharedStrings.xml', '<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><r><t>共</t></r><r><t>有</t></r><rPh sb="0" eb="2"><t>きょうゆう</t></rPh></si></sst>')
                z.writestr('xl/worksheets/sheet1.xml', '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><r><t>漢</t></r><r><t>字</t></r><rPh sb="0" eb="2"><t>かんじ</t></rPh></is></c><c r="B1" t="s"><v>0</v></c></row></sheetData></worksheet>')
            for script in SCRIPTS:
                with self.subTest(script=script):
                    r = self.run_script(script, p)
                    self.assertEqual(r.returncode, 0, r.stderr)
                    self.assertEqual(json.loads(r.stdout)['headers'], ['漢字', '共有'])

    def test_xlsx_stops_before_parsing_malformed_data_rows(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / 'input.xlsx'
            with zipfile.ZipFile(p, 'w') as z:
                z.writestr('xl/workbook.xml', '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Orders" sheetId="1" r:id="rId1"/></sheets></workbook>')
                z.writestr('xl/_rels/workbook.xml.rels', '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>')
                z.writestr('xl/worksheets/sheet1.xml', '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Header</t></is></c></row><row r="2"><INVALID')
            for script in SCRIPTS:
                r = self.run_script(script, p)
                self.assertEqual(r.returncode, 0, r.stderr)
                self.assertEqual(json.loads(r.stdout)['headers'], ['Header'])

    def test_unknown_format_fails_without_content(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / 'input.xls'
            p.write_bytes(b'PRIVATE_SENTINEL')
            for script in SCRIPTS:
                r = self.run_script(script, p)
                self.assertNotEqual(r.returncode, 0)
                self.assertNotIn('PRIVATE_SENTINEL', r.stdout + r.stderr)

    def test_incomplete_header_fails_closed(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / 'input.xlsx'
            with zipfile.ZipFile(p, 'w') as z:
                z.writestr('xl/workbook.xml', '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Orders" sheetId="1" r:id="rId1"/></sheets></workbook>')
                z.writestr('xl/_rels/workbook.xml.rels', '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>')
                z.writestr('xl/worksheets/sheet1.xml', '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Header</t></is></c>')
            for script in SCRIPTS:
                r = self.run_script(script, p)
                self.assertNotEqual(r.returncode, 0)
                self.assertEqual(r.stdout, '')

    def test_dtd_in_metadata_is_rejected(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / 'input.xlsx'
            with zipfile.ZipFile(p, 'w') as z:
                z.writestr('xl/workbook.xml', '<!DOCTYPE workbook [<!ENTITY secret "PRIVATE_SENTINEL">]><workbook>&secret;</workbook>')
            for script in SCRIPTS:
                r = self.run_script(script, p)
                self.assertNotEqual(r.returncode, 0)
                self.assertEqual(r.stdout, '')
                self.assertNotIn('PRIVATE_SENTINEL', r.stderr)

    def test_multiple_sheets_require_explicit_selection(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / 'input.xlsx'
            with zipfile.ZipFile(p, 'w') as z:
                z.writestr('xl/workbook.xml', '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="First" sheetId="1" r:id="rId1"/><sheet name="Second" sheetId="2" r:id="rId2"/></sheets></workbook>')
                z.writestr('xl/_rels/workbook.xml.rels', '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId2" Target="worksheets/sheet2.xml"/></Relationships>')
                z.writestr('xl/worksheets/sheet2.xml', '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Selected</t></is></c></row></sheetData></worksheet>')
            for script in SCRIPTS:
                self.assertNotEqual(self.run_script(script, p).returncode, 0)
                r = self.run_script(script, p, '--sheet', 'Second')
                self.assertEqual(r.returncode, 0, r.stderr)
                self.assertEqual(json.loads(r.stdout)['headers'], ['Selected'])

if __name__ == '__main__':
    unittest.main()
