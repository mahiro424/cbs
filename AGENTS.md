# cbs_rebuild Agent 入口

## Agent skills

### Issue 跟踪器

本项目的 issue 与 PRD 发布到 GitHub 仓库 `mahiro424/cbs`。父级配置见 `F:\yanjiu\docs\agents\issue-tracker.md`。

### Triage 标签

使用默认五状态标签：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。父级配置见 `F:\yanjiu\docs\agents\triage-labels.md`。

### Domain 领域文档

采用 single-context 布局：父级根目录 `F:\yanjiu\CONTEXT.md` 与 `F:\yanjiu\docs\adr\`，当前仓库优先阅读 `README.md`、`contracts/swagger.json` 和相关 issue/PR。

## 编码与终端规范

本项目在 Windows/PowerShell 环境中必须优先防止 GBK、ANSI、ASCII 与 UTF-8 混用导致的中文乱码。每次开始执行命令前，先完成以下动作：

1. 如果当前目录是 `F:\yanjiu\cbs_rebuild` 或其子目录，先执行：

```powershell
. .\scripts\enter-utf8.ps1
```

2. 如果无法执行脚本，则至少在当前 PowerShell 会话中设置：

```powershell
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)
[Console]::InputEncoding = $utf8NoBom
[Console]::OutputEncoding = $utf8NoBom
$OutputEncoding = $utf8NoBom
chcp.com 65001 > $null
```

3. 写文件时优先使用 Python 或 .NET 明确指定 UTF-8：

```python
from pathlib import Path
Path("文件.md").write_text(text, encoding="utf-8")
```

或：

```powershell
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)
[System.IO.File]::WriteAllText($path, $text, $utf8NoBom)
```

4. 避免在 Windows PowerShell 5.1 中直接使用不带 `-Encoding UTF8` 的 `Set-Content`、`Add-Content`、`Out-File`、`Get-Content` 处理中文文件。

5. 读取 JSON、Markdown、Go 源码、PowerShell 脚本时显式使用 UTF-8。例如：

```powershell
Get-Content -LiteralPath $path -Raw -Encoding UTF8
```

6. 通过 PowerShell 发送包含中文的 HTTP 请求体时，不要直接传普通字符串；应传 UTF-8 字节：

```powershell
$body = '{"DeviceName":"测试设备"}'
Invoke-RestMethod -Body ([System.Text.Encoding]::UTF8.GetBytes($body)) -ContentType 'application/json; charset=utf-8'
```

7. 所有对外回复、文档、注释和日志文本都使用中文；代码标识符、命令和协议字段保持原文。

8. 完成前必须运行编码检查：

```powershell
python .\scripts\check_encoding.py .
```

若检查发现连续问号、替换字符或典型 mojibake 片段等疑似乱码标记，必须先修复再提交。
