from __future__ import annotations

import os
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "enter-utf8.ps1"
POWERSHELL = os.environ.get("POWERSHELL_EXE", "powershell")
EXPECTED_CONFIG = {
    "core.quotepath": "false",
    "i18n.commitEncoding": "utf-8",
    "i18n.logOutputEncoding": "utf-8",
    "core.autocrlf": "false",
    "core.eol": "lf",
}
NOISE_MARKERS = (
    "could not lock config file",
    "permission denied",
    "fatal:",
    "unable to access '.git/config'",
)


def run(args: list[str], *, check: bool = False) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        args,
        cwd=ROOT,
        text=True,
        encoding="utf-8",
        errors="replace",
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=check,
    )


def git_config(name: str, value: str | None = None) -> subprocess.CompletedProcess[str]:
    if value is None:
        return run(["git", "config", "--get", name])
    return run(["git", "config", name, value], check=True)


def perturb_git_config() -> None:
    wrong_values = {
        "core.quotepath": "true",
        "i18n.commitEncoding": "gbk",
        "i18n.logOutputEncoding": "gbk",
        "core.autocrlf": "true",
        "core.eol": "crlf",
    }
    for name, value in wrong_values.items():
        git_config(name, value)


def run_enter_once(index: int) -> tuple[int, subprocess.CompletedProcess[str]]:
    command = f". '{SCRIPT}'; git config --get core.quotepath"
    completed = run([
        POWERSHELL,
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-Command",
        command,
    ])
    return index, completed


def main() -> int:
    if not SCRIPT.exists():
        print(f"找不到 UTF-8 入口脚本：{SCRIPT}", file=sys.stderr)
        return 1

    perturb_git_config()
    failures: list[str] = []
    worker_count = int(os.environ.get("ENTER_UTF8_VERIFY_WORKERS", "32"))
    with ThreadPoolExecutor(max_workers=worker_count) as executor:
        results = list(executor.map(run_enter_once, range(worker_count)))

    for index, completed in results:
        combined = f"{completed.stdout}\n{completed.stderr}"
        if completed.returncode != 0:
            failures.append(f"第 {index} 个并发入口返回码为 {completed.returncode}：{combined.strip()}")
        lowered = combined.lower()
        for marker in NOISE_MARKERS:
            if marker in lowered:
                failures.append(f"第 {index} 个并发入口出现 Git 配置噪声 `{marker}`：{combined.strip()}")

    for name, expected in EXPECTED_CONFIG.items():
        completed = git_config(name)
        actual = completed.stdout.strip()
        if completed.returncode != 0 or actual != expected:
            failures.append(f"Git 配置 {name} = {actual!r}，期望 {expected!r}")

    if failures:
        print("UTF-8 入口并发验证失败：", file=sys.stderr)
        for failure in failures[:12]:
            print(f"- {failure}", file=sys.stderr)
        if len(failures) > 12:
            print(f"- 其余失败 {len(failures) - 12} 条已省略", file=sys.stderr)
        return 1

    print(f"UTF-8 入口并发验证通过：{worker_count} 个并发入口无 Git 配置噪声，配置值符合预期。")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
