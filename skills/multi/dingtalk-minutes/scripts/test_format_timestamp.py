import json
import subprocess
import sys
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name('format_timestamp.py')


class FormatTimestampTest(unittest.TestCase):
    def run_script(self, *args):
        return subprocess.run([sys.executable, str(SCRIPT), *args], capture_output=True, text=True)

    def test_seconds_milliseconds_and_observed_date(self):
        for raw, unit in [('1783906929', 's'), ('1783906929000', 'ms')]:
            result = self.run_script('--unit', unit, '--timezone', 'Asia/Shanghai', raw)
            self.assertEqual(result.returncode, 0, result.stderr)
            item = json.loads(result.stdout)['items'][0]
            self.assertEqual(item['datetime'], '2026-07-13T09:42:09.000000+08:00')
            self.assertEqual(item['weekday'], '星期一')

    def test_batch_order_and_weekday(self):
        result = self.run_script('--unit', 'ms', '--timezone', 'Asia/Shanghai', '1788487433000', '1788422619000')
        self.assertEqual(result.returncode, 0, result.stderr)
        items = json.loads(result.stdout)['items']
        self.assertEqual([x['index'] for x in items], [0, 1])
        self.assertEqual([x['weekday'] for x in items], ['星期五', '星期四'])
        self.assertEqual(items[0]['timestamp'], '1788487433000')

    def test_cross_day_and_negative_fraction(self):
        result = self.run_script('--unit', 's', '--timezone', 'America/New_York', '--', '-0.000001')
        self.assertEqual(result.returncode, 0, result.stderr)
        item = json.loads(result.stdout)['items'][0]
        self.assertEqual(item['datetime'], '1969-12-31T18:59:59.999999-05:00')
        self.assertEqual(item['weekday'], '星期三')

    def test_dst_boundary(self):
        result = self.run_script('--unit', 's', '--timezone', 'America/New_York', '1710053999', '1710054000')
        self.assertEqual(result.returncode, 0, result.stderr)
        items = json.loads(result.stdout)['items']
        self.assertEqual(items[0]['datetime'], '2024-03-10T01:59:59.000000-05:00')
        self.assertEqual(items[1]['datetime'], '2024-03-10T03:00:00.000000-04:00')

    def test_bad_timestamp_fails_entire_batch(self):
        for invalid in ['bad', '', 'NaN', 'Infinity', '1e1000', '253402300800', '0.0000001']:
            with self.subTest(invalid=invalid):
                result = self.run_script('--unit', 's', '--timezone', 'UTC', '0', invalid)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, '')
                self.assertFalse(json.loads(result.stderr)['ok'])

    def test_missing_unit_or_zone_and_invalid_zone(self):
        for args in [('0',), ('--unit', 's', '0'), ('--timezone', 'UTC', '0'), ('--unit', 'seconds', '--timezone', 'UTC', '0'), ('--unit', 's', '--timezone', 'No/Such_Zone', '0')]:
            result = self.run_script(*args)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, '')


if __name__ == '__main__':
    unittest.main()
