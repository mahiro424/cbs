#!/usr/bin/env python3
"""验证编码治理工具的关键行为。"""
from __future__ import annotations

import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
WORK = ROOT / ".scratch" / "encoding-tools-test"
NORMALIZE = ROOT / "scripts" / "normalize_encoding.py"
CHECK = ROOT / "scripts" / "check_encoding.py"


def run(args: list[str], *, expect: int = 0) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        [sys.executable, *args],
        cwd=ROOT,
        text=True,
        encoding="utf-8",
        errors="replace",
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if completed.returncode != expect:
        print(completed.stdout)
        print(completed.stderr, file=sys.stderr)
        raise AssertionError(f"命令返回码 {completed.returncode}，期望 {expect}：{args}")
    return completed


def main() -> int:
    if WORK.exists():
        shutil.rmtree(WORK)
    WORK.mkdir(parents=True)

    gbk_file = WORK / "历史素材.txt"
    utf8_file = WORK / "说明.md"
    original_text = "登录成功：长白山测试设备\n订单状态：已兑换\n"
    gbk_file.write_bytes(original_text.encode("gb18030"))
    utf8_file.write_text("这是 UTF-8 文件。\n", encoding="utf-8")

    dry_run = run([str(NORMALIZE), str(WORK), "--limit", "10"])
    if "历史素材.txt" not in dry_run.stdout or "gb18030" not in dry_run.stdout:
        raise AssertionError("预演输出未包含 GB18030 候选文件。")

    backup_dir = WORK / ".encoding-backups" / "备份"
    write_run = run([str(NORMALIZE), str(WORK), "--write", "--backup-dir", str(backup_dir)])
    if "已转换为 UTF-8：1 个文件" not in write_run.stdout:
        raise AssertionError("转换输出数量不符合预期。")
    if gbk_file.read_text(encoding="utf-8") != original_text:
        raise AssertionError("转换后的 UTF-8 内容与原文不一致。")
    backup_file = backup_dir / "历史素材.txt"
    if not backup_file.exists() or backup_file.read_bytes() != original_text.encode("gb18030"):
        raise AssertionError("备份文件不存在或原始字节不一致。")

    check_run = run([str(CHECK), str(WORK)])
    if "未发现 UTF-8 解码错误" not in check_run.stdout:
        raise AssertionError("编码检查未通过。")

    print("编码治理工具验证通过：预演、备份、转换、UTF-8 回读与检查均正常。")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
