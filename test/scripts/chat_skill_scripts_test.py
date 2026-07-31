#!/usr/bin/env python3
"""Regression tests for the Chat Skill's bundled Python scripts."""

import contextlib
import importlib.util
import io
import json
import re
import types
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
MULTI_ROOT = ROOT / 'skills' / 'multi' / 'dingtalk-chat'
MONO_ROOT = ROOT / 'skills' / 'mono'
SCRIPT_NAMES = (
    'chat_export_messages.py',
    'chat_history_with_user.py',
    'bot_broadcast.py',
)


def load_script(name: str):
    path = MULTI_ROOT / 'scripts' / f'{name}.py'
    spec = importlib.util.spec_from_file_location(f'chat_skill_{name}', path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f'cannot load {path}')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ChatSkillScriptPathTest(unittest.TestCase):
    def test_shortcuts_are_self_contained_in_multi_skill(self):
        text = (MULTI_ROOT / 'SKILL.md').read_text(encoding='utf-8')
        block = text.split('<!-- VISIBLE_SHORTCUTS_START -->', 1)[1].split(
            '<!-- VISIBLE_SHORTCUTS_END -->', 1
        )[0]
        catalog = json.loads(
            (ROOT / 'docs' / 'shortcut-public-catalog.json').read_text(
                encoding='utf-8'
            )
        )
        chat_rows = [
            row for row in catalog['results'] if row.get('service') == 'chat'
        ]
        table_rows = re.findall(
            r'\|\s*`(\+[a-z0-9-]+)`\s*\|\s*([^|\n]+)\s*\|',
            block,
        )
        self.assertEqual(
            {row['command'] for row in chat_rows},
            {command for command, _ in table_rows},
        )
        self.assertTrue(all(description.strip() for _, description in table_rows))
        self.assertTrue(
            all(
                re.search(r'[\u4e00-\u9fff]', description)
                for _, description in table_rows
            ),
            'every shortcut description must retain a Chinese intent cue',
        )
        self.assertIn('| Shortcut | 适用场景 |', block)
        self.assertNotIn('| Shortcut | 风险 |', block)
        self.assertNotIn('dws chat +', block)
        for required in (
            '+messages-send',
            '+chat-messages',
            '+search-msg',
            '+conversation-clear-messages',
        ):
            self.assertIn(required, block)
        high_risk = {
            row['command']
            for row in chat_rows
            if row.get('risk') == 'high-risk-write'
        }
        high_risk_line = next(
            line for line in block.splitlines()
            if line.startswith('`risk=high-risk-write`')
        )
        self.assertEqual(
            high_risk,
            set(re.findall(r'`(\+[a-z0-9-]+)`', high_risk_line)),
        )
        for required in (
            'This is the only Shortcut entry point for multi/chat',
            'exact script/recipe > matching public Shortcut > atomic command',
            'dws shortcut list --service chat --format json',
            'never guess names',
            'confirmation=user_required',
            '--idempotency-key',
            '--download-resources',
            '--receiver-open-dingtalk-id',
        ):
            self.assertIn(required, text)

    def test_chat_references_are_atomic_only(self):
        references = list((MULTI_ROOT / 'references').rglob('*.md'))
        self.assertTrue(references)
        for path in references:
            text = path.read_text(encoding='utf-8')
            with self.subTest(path=path.relative_to(MULTI_ROOT)):
                self.assertNotRegex(text, r'(?i)\bshortcut\b')
                self.assertNotRegex(text, r'\+[a-z][a-z0-9-]+')

        chat_ref = (
            MULTI_ROOT / 'references' / 'chat.md'
        ).read_text(encoding='utf-8')
        message_ref = (
            MULTI_ROOT / 'references' / 'chat' / 'chat-message.md'
        ).read_text(encoding='utf-8')
        conversation_ref = (
            MULTI_ROOT / 'references' / 'chat' / 'chat-conversation.md'
        ).read_text(encoding='utf-8')

        for required in (
            'message send --group',
            'message search-advanced',
            'message send-by-bot',
            'message send-by-webhook',
        ):
            self.assertIn(required, chat_ref)
        for required in (
            'message send-card',
            'message update-card',
            '--flow-status',
            'message download-media',
            '--file-path',
        ):
            self.assertIn(required, message_ref)
        for required in (
            'clear-messages',
            'category delete',
            '按 leaf Schema 的确认语义执行',
        ):
            self.assertIn(required, conversation_ref)

    def test_documented_chat_commands_do_not_pass_literal_newline_escapes(self):
        docs = [MULTI_ROOT / 'SKILL.md']
        docs.extend((MULTI_ROOT / 'references').rglob('*.md'))
        offenders = []
        for path in docs:
            for line_number, line in enumerate(
                path.read_text(encoding='utf-8').splitlines(),
                start=1,
            ):
                if 'dws chat ' in line and r'\n' in line:
                    offenders.append(
                        f'{path.relative_to(ROOT)}:{line_number}: {line}'
                    )
        self.assertEqual([], offenders)

    def test_markdown_python_links_resolve(self):
        link_re = re.compile(r'\[[^\]]+\.py\]\(([^)]+\.py)\)')
        docs = list(MULTI_ROOT.rglob('*.md'))
        docs.extend((MONO_ROOT / 'references').rglob('*.md'))
        missing = []
        for doc in docs:
            text = doc.read_text(encoding='utf-8')
            for target in link_re.findall(text):
                resolved = (doc.parent / target).resolve()
                if not resolved.is_file():
                    missing.append(f'{doc.relative_to(ROOT)} -> {target}')
        self.assertEqual([], missing)

    def test_documented_invocations_use_python3_and_skill_relative_paths(self):
        docs = {
            MULTI_ROOT: (
                MULTI_ROOT / 'SKILL.md',
                MULTI_ROOT / 'references' / '01-messaging.md',
                MULTI_ROOT / 'references' / 'chat.md',
                MULTI_ROOT / 'references' / 'chat' / 'chat-workflows.md',
            ),
            MONO_ROOT: (
                MONO_ROOT / 'references' / 'best_practices' / '01-messaging.md',
                MONO_ROOT / 'references' / 'products' / 'chat.md',
            ),
        }
        bad_python = re.compile(r'(?<![A-Za-z0-9_])python\s+')
        invocation = re.compile(r'python3\s+(scripts/[A-Za-z0-9_.-]+\.py)')
        errors = []
        for skill_root, paths in docs.items():
            for doc in paths:
                text = doc.read_text(encoding='utf-8')
                if bad_python.search(text):
                    errors.append(f'{doc.relative_to(ROOT)} uses bare python')
                for target in invocation.findall(text):
                    if not (skill_root / target).is_file():
                        errors.append(
                            f'{doc.relative_to(ROOT)} -> {target} is missing'
                        )
        self.assertEqual([], errors)

    def test_mono_and_multi_script_copies_match(self):
        bad_python = re.compile(r'(?<![A-Za-z0-9_])python\s+')
        for name in SCRIPT_NAMES:
            multi_script = MULTI_ROOT / 'scripts' / name
            self.assertEqual(
                multi_script.read_bytes(),
                (MONO_ROOT / 'scripts' / name).read_bytes(),
                name,
            )
            self.assertIsNone(
                bad_python.search(multi_script.read_text(encoding='utf-8')),
                name,
            )


class ChatExportMessagesTest(unittest.TestCase):
    def setUp(self):
        self.module = load_script('chat_export_messages')

    def run_script(self, argv, responses):
        calls = []

        def fake_run_dws(args, dry_run=False):
            self.assertFalse(dry_run)
            calls.append(args)
            response = responses.pop(0)
            if isinstance(response, Exception):
                raise response
            return response

        stdout = io.StringIO()
        stderr = io.StringIO()
        with mock.patch.object(self.module, 'run_dws', fake_run_dws):
            with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(
                stderr
            ):
                code = self.module.run(argv)
        return code, calls, stdout.getvalue(), stderr.getvalue()

    def test_paginates_with_last_message_time_and_deduplicates(self):
        responses = [
            {
                'result': {
                    'value': [
                        {
                            'title': '项目群',
                            'openConversationId': 'cid-project',
                        }
                    ]
                }
            },
            {
                'result': {
                    'messages': [
                        {
                            'openMessageId': 'm1',
                            'createTime': '2026-07-01 10:00:00',
                            'content': 'one',
                        },
                        {
                            'openMessageId': 'm2',
                            'createTime': '2026-07-01 11:00:00',
                            'content': 'two',
                        },
                    ],
                    'hasMore': True,
                    'nextCursor': 'opaque-not-a-time',
                }
            },
            {
                'result': {
                    'messages': [
                        {
                            'openMessageId': 'm2',
                            'createTime': '2026-07-01 11:00:00',
                            'content': 'two',
                        },
                        {
                            'openMessageId': 'm3',
                            'createTime': '2026-07-01 12:00:00',
                            'content': 'three',
                        },
                    ],
                    'hasMore': False,
                }
            },
        ]
        code, calls, stdout, stderr = self.run_script(
            ['--query', '项目群', '--time', '2026-07-01 09:00:00'],
            responses,
        )
        self.assertEqual(0, code, stderr)
        second_page = calls[2]
        self.assertEqual(
            '2026-07-01 11:00:00',
            second_page[second_page.index('--time') + 1],
        )
        self.assertEqual(
            'newer', second_page[second_page.index('--direction') + 1]
        )
        self.assertNotIn('opaque-not-a-time', second_page)
        self.assertIn('合计: 3 条消息', stdout)

    def test_ambiguous_group_requires_explicit_id(self):
        responses = [
            {
                'result': {
                    'value': [
                        {'title': '项目一群', 'openConversationId': 'cid-1'},
                        {'title': '项目二群', 'openConversationId': 'cid-2'},
                    ]
                }
            }
        ]
        code, calls, _, stderr = self.run_script(
            ['--query', '项目', '--time', '2026-07-01 09:00:00'],
            responses,
        )
        self.assertEqual(2, code)
        self.assertEqual(1, len(calls))
        self.assertIn('请指定 --group', stderr)

    def test_command_failure_is_not_reported_as_empty_result(self):
        error = self.module.ScriptError('upstream failed')
        code, _, stdout, stderr = self.run_script(
            ['--group', 'cid-project', '--time', '2026-07-01 09:00:00'],
            [error],
        )
        self.assertEqual(1, code)
        self.assertNotIn('未拉取到消息', stdout)
        self.assertIn('upstream failed', stderr)

    def test_successful_empty_page_returns_zero(self):
        code, _, stdout, stderr = self.run_script(
            ['--group', 'cid-project', '--time', '2026-07-01 09:00:00'],
            [{'result': {'messages': [], 'hasMore': False}}],
        )
        self.assertEqual(0, code, stderr)
        self.assertIn('未拉取到消息', stdout)


class ChatHistoryWithUserTest(unittest.TestCase):
    def setUp(self):
        self.module = load_script('chat_history_with_user')

    def test_ambiguous_user_requires_explicit_id(self):
        calls = []

        def fake_run_dws(args, dry_run=False):
            calls.append(args)
            return {
                'result': {
                    'users': [
                        {'name': '张三（研发）', 'userId': 'u-1'},
                        {'name': '张三（销售）', 'userId': 'u-2'},
                    ]
                }
            }

        stdout = io.StringIO()
        stderr = io.StringIO()
        with mock.patch.object(self.module, 'run_dws', fake_run_dws):
            with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(
                stderr
            ):
                code = self.module.run(
                    ['--name', '张三', '--time', '2026-07-01 09:00:00']
                )
        self.assertEqual(2, code)
        self.assertEqual(1, len(calls))
        self.assertIn('请指定 --user', stderr.getvalue())

    def test_no_forward_uses_public_older_direction(self):
        calls = []
        responses = [
            {
                'result': {
                    'messages': [
                        {
                            'openMessageId': 'm1',
                            'createTime': '2026-07-01 08:00:00',
                            'content': 'hello',
                        }
                    ],
                    'hasMore': False,
                }
            }
        ]

        def fake_run_dws(args, dry_run=False):
            calls.append(args)
            return responses.pop(0)

        stdout = io.StringIO()
        with mock.patch.object(self.module, 'run_dws', fake_run_dws):
            with contextlib.redirect_stdout(stdout):
                code = self.module.run(
                    [
                        '--user',
                        'u-1',
                        '--time',
                        '2026-07-01 09:00:00',
                        '--no-forward',
                    ]
                )
        self.assertEqual(0, code)
        self.assertEqual(
            'older', calls[0][calls[0].index('--direction') + 1]
        )
        self.assertNotIn('--forward', calls[0])


class BotBroadcastTest(unittest.TestCase):
    def test_business_failure_counts_as_failure(self):
        module = load_script('bot_broadcast')
        completed = types.SimpleNamespace(
            returncode=0,
            stdout='{"success":false,"errorMsg":"robot rejected"}',
            stderr='',
        )
        stdout = io.StringIO()
        with mock.patch.object(module.subprocess, 'run', return_value=completed):
            with contextlib.redirect_stdout(stdout):
                code = module.run(
                    [
                        '--robot-code',
                        'robot',
                        '--chats',
                        'cid-1',
                        '--title',
                        'title',
                        '--text',
                        'text',
                    ]
                )
        self.assertEqual(1, code)
        self.assertIn('业务调用失败', stdout.getvalue())


if __name__ == '__main__':
    unittest.main()
