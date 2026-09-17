"""Format Unix timestamps locally; requires Python 3.9+ and an IANA timezone database."""

import argparse
import json
import sys
from datetime import datetime, timedelta, timezone
from decimal import Decimal, InvalidOperation, localcontext
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError


def format_timestamps(values, unit, zone):
    if unit not in ('s', 'ms'):
        raise ValueError('unit must be s or ms')
    tz = ZoneInfo(zone)
    items = []
    for index, raw in enumerate(values):
        if not isinstance(raw, str) or not raw.strip() or len(raw) > 100:
            raise ValueError('timestamp must be a nonempty number of at most 100 characters')
        with localcontext() as ctx:
            ctx.prec = 120
            number = Decimal(raw)
            if not number.is_finite():
                raise ValueError('timestamp must be finite')
            seconds = number / (1000 if unit == 'ms' else 1)
            if not Decimal('-62135596800') <= seconds < Decimal('253402300800'):
                raise ValueError('timestamp is outside supported datetime range')
            micros = seconds * 1000000
            if micros != micros.to_integral_value():
                raise ValueError('precision finer than one microsecond is unsupported')
            dt = (datetime(1970, 1, 1, tzinfo=timezone.utc)
                  + timedelta(microseconds=int(micros))).astimezone(tz)
        items.append({
            'index': index,
            'timestamp': raw,
            'unit': unit,
            'timezone': zone,
            'datetime': dt.isoformat(timespec='microseconds'),
            'weekday': '星期' + '一二三四五六日'[dt.weekday()],
        })
    return items


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('timestamps', nargs='+', help='One or more original Unix timestamp values')
    parser.add_argument('--unit', choices=('s', 'ms'), required=True)
    parser.add_argument('--timezone', required=True, help='Explicit IANA timezone, e.g. Asia/Shanghai')
    args = parser.parse_args(argv)
    try:
        items = format_timestamps(args.timestamps, args.unit, args.timezone)
    except (ValueError, InvalidOperation, OverflowError, ZoneInfoNotFoundError) as exc:
        print(json.dumps({'ok': False, 'error': str(exc)}, ensure_ascii=False), file=sys.stderr)
        return 2
    # Buffer the whole batch: invalid input never produces a partial success on stdout.
    print(json.dumps({'ok': True, 'items': items}, ensure_ascii=False, indent=2))
    return 0


if __name__ == '__main__':
    sys.exit(main())
