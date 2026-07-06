#!/usr/bin/env python3
"""检查仓库文本文件是否存在 UTF-8 解码失败或常见乱码痕迹。"""
from __future__ import annotations

import argparse
from pathlib import Path

TEXT_SUFFIXES = {
    '.go', '.md', '.json', '.yml', '.yaml', '.toml', '.ini', '.conf', '.ps1', '.py', '.txt', '.mod', '.sum', '.gitignore', '.gitattributes', '.editorconfig'
}
SKIP_DIRS = {'.git', '.scratch', 'vendor', 'node_modules', 'bin', 'dist', 'build'}
SUSPICIOUS_MARKERS = [
    chr(63) * 3,
    chr(0xFFFD) * 3,
    chr(0x9352),
    chr(0x93C8) + chr(0x20AC),
    chr(0x7ECB) + chr(0x5B2A),
    chr(0x9286),
    chr(0x951B),
    chr(0x6FB6) + chr(63),
]


def is_text_candidate(path: Path) -> bool:
    if path.name in {'.gitignore', '.gitattributes', '.editorconfig'}:
        return True
    return path.suffix.lower() in TEXT_SUFFIXES


def should_skip(path: Path) -> bool:
    return any(part in SKIP_DIRS for part in path.parts)


def main() -> int:
    parser = argparse.ArgumentParser(description='检查 UTF-8 与常见中文乱码。')
    parser.add_argument('root', nargs='?', default='.', help='要检查的目录')
    args = parser.parse_args()

    root = Path(args.root).resolve()
    failures: list[str] = []
    checked = 0

    for path in sorted(root.rglob('*')):
        if not path.is_file() or should_skip(path.relative_to(root)) or not is_text_candidate(path):
            continue
        checked += 1
        rel = path.relative_to(root)
        try:
            text = path.read_text(encoding='utf-8')
        except UnicodeDecodeError as exc:
            failures.append(f'{rel}: UTF-8 解码失败：{exc}')
            continue
        for line_no, line in enumerate(text.splitlines(), 1):
            for marker in SUSPICIOUS_MARKERS:
                if marker in line:
                    failures.append(f'{rel}:{line_no}: 疑似乱码标记 {marker!r}: {line}')
                    break

    print(f'已检查文本文件：{checked}')
    if failures:
        print('发现编码问题：')
        for item in failures:
            print('  - ' + item)
        return 1
    print('未发现 UTF-8 解码错误或常见乱码标记。')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
