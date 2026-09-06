#!/usr/bin/env python3

import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('entry_baseline',
    Path(__file__).with_name('build-schema-entry-baseline.py'))
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


class BaselineSourceTests(unittest.TestCase):
    def test_source_bytes_and_executable_mode_are_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive = root / 'source.tar'
            with tarfile.open(archive, 'w') as output:
                member = tarfile.TarInfo('scripts/build.sh')
                member.mode, member.size = 0o755, 3
                output.addfile(member, io.BytesIO(b'old'))
            source = root / 'source'
            source.mkdir()
            build.extract_source(archive, source)
            self.assertEqual((source / 'scripts/build.sh').read_bytes(), b'old')
            self.assertEqual((source / 'scripts/build.sh').stat().st_mode & 0o777, 0o755)

    def test_archive_cannot_escape_or_import_linked_files(self):
        for name, kind in (('../escape', tarfile.REGTYPE), ('/absolute', tarfile.REGTYPE),
                           ('link', tarfile.SYMTYPE), ('hardlink', tarfile.LNKTYPE)):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                archive = root / 'source.tar'
                with tarfile.open(archive, 'w') as output:
                    output.addfile(tarfile.TarInfo('safe-first'))
                    member = tarfile.TarInfo(name)
                    member.type, member.linkname = kind, '../outside'
                    output.addfile(member)
                source = root / 'source'
                source.mkdir()
                with self.assertRaisesRegex(RuntimeError, 'non-regular or escaping'):
                    build.extract_source(archive, source)
                self.assertEqual(list(source.iterdir()), [])


if __name__ == '__main__':
    unittest.main()
