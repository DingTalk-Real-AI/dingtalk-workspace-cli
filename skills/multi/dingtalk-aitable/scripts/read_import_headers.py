#!/usr/bin/env python3
"""Read the first header row of a CSV or XLSX without parsing worksheet data rows."""
from __future__ import annotations

import argparse
import csv
import json
import posixpath
import re
import sys
import zipfile
import zlib
from pathlib import Path
from xml.etree import ElementTree as ET
from xml.parsers import expat

MAIN = 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'
OFFICE_REL = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships'
PACKAGE_REL = 'http://schemas.openxmlformats.org/package/2006/relationships'
MAX_HEADER_BYTES = 8 * 1024 * 1024


class HeaderComplete(Exception):
    pass


def reject_dtd(*_args) -> None:
    raise ValueError('不支持包含 DTD 的工作簿')


def read_metadata(archive: zipfile.ZipFile, name: str) -> bytes:
    if archive.getinfo(name).file_size > MAX_HEADER_BYTES:
        raise ValueError('工作簿元数据过大')
    data = archive.read(name)
    if b'<!DOCTYPE' in data or b'<!ENTITY' in data:
        raise ValueError('不支持包含 DTD 的工作簿')
    return data


class NoDTDStream:
    def __init__(self, stream):
        self.stream = stream
        self.tail = b''

    def read(self, size=-1):
        data = self.stream.read(size)
        combined = self.tail + data
        if b'<!DOCTYPE' in combined or b'<!ENTITY' in combined:
            raise ValueError('不支持包含 DTD 的工作簿')
        self.tail = combined[-16:]
        return data


def xlsx_headers(path: Path, sheet_name: str | None) -> tuple[str, list[str]]:
    with zipfile.ZipFile(path) as archive:
        workbook = ET.fromstring(read_metadata(archive, 'xl/workbook.xml'))
        sheets = workbook.findall(f'{{{MAIN}}}sheets/{{{MAIN}}}sheet')
        if sheet_name is None and len(sheets) != 1:
            raise ValueError('多个工作表时必须指定工作表')
        visible = [s for s in sheets if s.get('state', 'visible') == 'visible']
        matches = [s for s in sheets if s.get('name') == sheet_name] if sheet_name else visible[:1]
        if len(matches) != 1:
            raise ValueError('找不到指定工作表或没有可见工作表')
        sheet = matches[0]
        relations = ET.fromstring(read_metadata(archive, 'xl/_rels/workbook.xml.rels'))
        rel_id = sheet.get(f'{{{OFFICE_REL}}}id')
        matches = [r for r in relations.findall(f'{{{PACKAGE_REL}}}Relationship') if r.get('Id') == rel_id]
        if len(matches) != 1 or matches[0].get('TargetMode') == 'External':
            raise ValueError('工作表引用无效')
        target = matches[0].get('Target', '')
        member = posixpath.normpath(target.lstrip('/') if target.startswith('/') else posixpath.join('xl', target))
        if not member.startswith('xl/worksheets/'):
            raise ValueError('工作表路径无效')
        parser = expat.ParserCreate(namespace_separator='}')
        parser.StartDoctypeDeclHandler = reject_dtd
        cells: dict[int, tuple[str, str]] = {}
        active = False
        cell: dict[str, str] | None = None
        capture = False
        phonetic = 0
        sequential = 0
        found = False
        complete = False

        def start(name: str, attrs: dict[str, str]) -> None:
            nonlocal active, cell, capture, phonetic, sequential, found
            local = name.rsplit('}', 1)[-1]
            if local == 'row' and not found:
                if attrs.get('r', '1') != '1':
                    raise ValueError('首行不存在，无法确定表头')
                active = found = True
            elif active and local == 'c':
                ref = attrs.get('r', '')
                match = re.fullmatch(r'([A-Z]+)1', ref) if ref else None
                if ref and not match:
                    raise ValueError('表头单元格引用无效')
                if match:
                    index = 0
                    for ch in match.group(1):
                        index = index * 26 + ord(ch) - ord('A') + 1
                else:
                    index = sequential + 1
                if index > 16384 or index in cells:
                    raise ValueError('表头列无效')
                sequential = index
                cell = {'index': str(index), 'type': attrs.get('t', ''), 'value': ''}
            elif active and cell is not None and local == 'rPh':
                phonetic += 1
            elif active and cell is not None and local == 'f':
                raise ValueError('公式表头无法可靠读取')
            elif active and cell is not None and local in ('v', 't') and not phonetic:
                capture = True

        def end(name: str) -> None:
            nonlocal cell, capture, phonetic, complete
            local = name.rsplit('}', 1)[-1]
            if active and local == 'rPh' and cell is not None:
                phonetic -= 1
            elif active and local in ('v', 't'):
                capture = False
            elif active and local == 'c' and cell is not None:
                cells[int(cell['index'])] = (cell['type'], cell['value'])
                cell = None
            elif active and local == 'row':
                complete = True
                raise HeaderComplete()

        def data(text: str) -> None:
            if capture and cell is not None:
                cell['value'] += text

        parser.StartElementHandler = start
        parser.EndElementHandler = end
        parser.CharacterDataHandler = data
        try:
            with archive.open(member) as stream:
                # Stop feeding XML at the closing header row, before any data-row bytes reach the parser.
                for _ in range(MAX_HEADER_BYTES):
                    byte = stream.read(1)
                    if not byte:
                        break
                    parser.Parse(byte, False)
        except HeaderComplete:
            pass
        if not found or not complete or not cells:
            raise ValueError('首行没有表头')
        shared_indexes = {int(value) for kind, value in cells.values() if kind == 's'}
        if any(index < 0 for index in shared_indexes):
            raise ValueError('共享字符串引用无效')
        shared: dict[int, str] = {}
        if shared_indexes:
            with archive.open('xl/sharedStrings.xml') as stream:
                index = 0
                for _, element in ET.iterparse(NoDTDStream(stream), events=('end',)):
                    if element.tag == f'{{{MAIN}}}si':
                        if index in shared_indexes:
                            parts = []
                            for child in element:
                                if child.tag == f'{{{MAIN}}}t':
                                    parts.append(child.text or '')
                                elif child.tag == f'{{{MAIN}}}r':
                                    parts.extend(t.text or '' for t in child.findall(f'{{{MAIN}}}t'))
                            shared[index] = ''.join(parts)
                        index += 1
                        element.clear()
                        if len(shared) == len(shared_indexes):
                            break
            if len(shared) != len(shared_indexes):
                raise ValueError('共享字符串引用无效')
        headers = [''] * max(cells)
        for index, (kind, value) in cells.items():
            if kind == 'e':
                raise ValueError('表头包含错误单元格')
            headers[index - 1] = shared[int(value)] if kind == 's' else value
        return sheet.get('name', ''), headers


def main() -> int:
    parser = argparse.ArgumentParser(description='只读取 CSV/XLSX 的首行表头，不解析数据行')
    parser.add_argument('file', type=Path)
    parser.add_argument('--sheet', help='XLSX 工作表名称；多个工作表时必须指定')
    args = parser.parse_args()
    try:
        suffix = args.file.suffix.lower()
        if suffix == '.csv':
            if args.sheet:
                raise ValueError('CSV 不支持指定工作表')
            with args.file.open(encoding='utf-8-sig', newline='') as stream:
                headers = next(csv.reader(stream), None)
            if not headers:
                raise ValueError('首行没有表头')
            sheet = None
        elif suffix == '.xlsx':
            sheet, headers = xlsx_headers(args.file, args.sheet)
        else:
            raise ValueError('仅支持 CSV 和 XLSX；其他格式无法安全读取表头')
        print(json.dumps({'sheet': sheet, 'header_row': 1, 'headers': headers}, ensure_ascii=False))
        return 0
    except (OSError, ValueError, KeyError, RuntimeError, EOFError, ET.ParseError, expat.ExpatError, zipfile.BadZipFile, zlib.error, UnicodeError, csv.Error):
        print('无法安全读取表头：请检查文件、工作表和格式', file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
