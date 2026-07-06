# 进入本仓库前执行：. .\scripts\enter-utf8.ps1
# 目标：在 Windows PowerShell 5.1 / PowerShell 7 / Codex shell 中统一使用 UTF-8，避免中文被 GBK/ANSI 或 ASCII 破坏。

$utf8NoBom = [System.Text.UTF8Encoding]::new($false)
[Console]::InputEncoding = $utf8NoBom
[Console]::OutputEncoding = $utf8NoBom
$global:OutputEncoding = $utf8NoBom

# PowerShell 5.1 的 Set-Content / Out-File / Get-Content 默认会使用 ANSI；这里显式改为 UTF-8。
$PSDefaultParameterValues['Get-Content:Encoding'] = 'UTF8'
$PSDefaultParameterValues['Set-Content:Encoding'] = 'UTF8'
$PSDefaultParameterValues['Add-Content:Encoding'] = 'UTF8'
$PSDefaultParameterValues['Out-File:Encoding'] = 'UTF8'
$PSDefaultParameterValues['Export-Csv:Encoding'] = 'UTF8'

# 让 Python / Go 子进程在控制台输出时优先使用 UTF-8。
$env:PYTHONUTF8 = '1'
$env:PYTHONIOENCODING = 'utf-8'
$env:LANG = 'zh_CN.UTF-8'
$env:LC_ALL = 'zh_CN.UTF-8'

# 当前控制台代码页切到 UTF-8；失败时不阻断后续命令。
try { chcp.com 65001 > $null } catch {}

# 仓库内 Git 输出使用 UTF-8，并避免中文路径被转义。
try {
  git config core.quotepath false | Out-Null
  git config i18n.commitEncoding utf-8 | Out-Null
  git config i18n.logOutputEncoding utf-8 | Out-Null
  git config core.autocrlf false | Out-Null
  git config core.eol lf | Out-Null
} catch {}

Write-Host '[utf8] session configured.'
