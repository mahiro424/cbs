#!/usr/bin/env python3
"""将历史 GBK/GB18030 文本安全归一化为 UTF-8。"""
from __future__ import annotations

import argparse
import shutil
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

TEXT_SUFFIXES = {
    ".go",
    ".md",
    ".json",
    ".yml",
    ".yaml",
    ".toml",
    ".ini",
    ".conf",
    ".ps1",
    ".py",
    ".txt",
    ".mod",
    ".sum",
    ".gitignore",
    ".gitattributes",
    ".editorconfig",
}
SKIP_DIRS = {
    ".git",
    ".scratch",
    ".encoding-backups",
    ".venv",
    "vendor",
    "node_modules",
    "bin",
    "dist",
    "build",
    "__pycache__",
}
SOURCE_ENCODINGS = ("gb18030", "gbk", "cp936", "big5")


@dataclass(frozen=True)
class ConvertCandidate:
    path: Path
    relative_path: Path
    source_encoding: str
    byte_length: int


@dataclass(frozen=True)
class ConvertResult:
    candidate: ConvertCandidate
    backup_path: Path | None


def is_text_candidate(path: Path) -> bool:
    if path.name in {".gitignore", ".gitattributes", ".editorconfig"}:
        return True
    return path.suffix.lower() in TEXT_SUFFIXES


def should_skip(relative_path: Path) -> bool:
    return any(part in SKIP_DIRS for part in relative_path.parts)


def detect_non_utf8(path: Path, root: Path) -> ConvertCandidate | None:
    relative_path = path.relative_to(root)
    data = path.read_bytes()
    try:
        data.decode("utf-8")
        return None
    except UnicodeDecodeError:
        pass

    for encoding in SOURCE_ENCODINGS:
        try:
            data.decode(encoding)
            return ConvertCandidate(path, relative_path, encoding, len(data))
        except UnicodeDecodeError:
            continue

    raise UnicodeDecodeError("utf-8", data, 0, 1, f"无法用 UTF-8 或 {', '.join(SOURCE_ENCODINGS)} 解码")


def iter_candidates(root: Path) -> list[ConvertCandidate]:
    candidates: list[ConvertCandidate] = []
    for path in sorted(root.rglob("*")):
        if not path.is_file():
            continue
        relative_path = path.relative_to(root)
        if should_skip(relative_path) or not is_text_candidate(path):
            continue
        candidate = detect_non_utf8(path, root)
        if candidate is not None:
            candidates.append(candidate)
    return candidates


def unique_backup_dir(root: Path, requested: Path | None) -> Path:
    if requested is not None:
        return requested.resolve()
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    return (root / ".encoding-backups" / stamp).resolve()


def convert_candidate(candidate: ConvertCandidate, root: Path, backup_root: Path | None) -> ConvertResult:
    original = candidate.path.read_bytes()
    text = original.decode(candidate.source_encoding)

    backup_path: Path | None = None
    if backup_root is not None:
        backup_path = backup_root / candidate.relative_path
        backup_path.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(candidate.path, backup_path)

    candidate.path.write_text(text, encoding="utf-8", newline="")
    roundtrip = candidate.path.read_bytes().decode("utf-8")
    if roundtrip != text:
        raise RuntimeError(f"转换后 UTF-8 回读不一致：{candidate.relative_path}")
    return ConvertResult(candidate, backup_path)


def main() -> int:
    parser = argparse.ArgumentParser(description="扫描文本文件，将非 UTF-8 的 GBK/GB18030 文本归一化为 UTF-8。")
    parser.add_argument("root", nargs="?", default=".", help="要扫描的根目录")
    parser.add_argument("--write", action="store_true", help="实际写入转换；不指定时只做预演")
    parser.add_argument("--backup-dir", default=None, help="原始文件备份目录；默认写到 <root>/.encoding-backups/<时间戳>")
    parser.add_argument("--no-backup", action="store_true", help="不备份原始字节；只建议在临时目录中使用")
    parser.add_argument("--limit", type=int, default=0, help="最多打印多少条明细；0 表示全部打印")
    args = parser.parse_args()

    root = Path(args.root).resolve()
    if not root.exists():
        print(f"扫描根目录不存在：{root}")
        return 2

    try:
        candidates = iter_candidates(root)
    except UnicodeDecodeError as exc:
        print(f"发现无法识别编码的文本候选文件：{exc}")
        return 1

    print(f"扫描根目录：{root}")
    print(f"发现非 UTF-8 文本文件：{len(candidates)}")
    if not candidates:
        print("无需转换。")
        return 0

    detail_limit = len(candidates) if args.limit == 0 else min(args.limit, len(candidates))
    for candidate in candidates[:detail_limit]:
        print(f"  - {candidate.relative_path} | {candidate.source_encoding} | {candidate.byte_length} 字节")
    if detail_limit < len(candidates):
        print(f"  - 其余 {len(candidates) - detail_limit} 个文件已省略")

    if not args.write:
        print("预演完成；若确认无误，追加 --write 执行转换。")
        return 0

    backup_root: Path | None
    if args.no_backup:
        backup_root = None
    else:
        backup_root = unique_backup_dir(root, Path(args.backup_dir) if args.backup_dir else None)
        backup_root.mkdir(parents=True, exist_ok=True)
        print(f"原始文件备份目录：{backup_root}")

    results: list[ConvertResult] = []
    for candidate in candidates:
        results.append(convert_candidate(candidate, root, backup_root))

    print(f"已转换为 UTF-8：{len(results)} 个文件")
    if backup_root is not None:
        print(f"备份保留在：{backup_root}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
