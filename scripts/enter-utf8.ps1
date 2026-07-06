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
# 注意：Agent 可能并发启动多个 PowerShell 进程。如果每个进程都无条件写 .git/config，
# Git 会短暂争抢 config.lock，并把 lock/permission 噪声写到 stderr。这里先读后写、
# 失败静默重试，确保入口脚本本身不污染后续命令输出。
function Set-RepoGitConfigValue {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][string]$Value
  )

  for ($attempt = 0; $attempt -lt 8; $attempt++) {
    $currentOutput = @(& git config --get $Name 2>$null)
    $current = $null
    if ($currentOutput.Count -gt 0) {
      $current = [string]$currentOutput[0]
    }
    if ($LASTEXITCODE -eq 0 -and $null -ne $current -and $current.Trim() -eq $Value) {
      return
    }

    & git config $Name $Value 2>$null
    if ($LASTEXITCODE -eq 0) {
      return
    }

    Start-Sleep -Milliseconds (50 * ($attempt + 1))
  }
}

function Set-RepoGitUtf8Config {
  $gitCommand = Get-Command git -ErrorAction SilentlyContinue
  if (-not $gitCommand) {
    return
  }

  $insideOutput = @(& git rev-parse --is-inside-work-tree 2>$null)
  $insideWorkTree = $null
  if ($insideOutput.Count -gt 0) {
    $insideWorkTree = [string]$insideOutput[0]
  }
  if ($LASTEXITCODE -ne 0 -or $null -eq $insideWorkTree -or $insideWorkTree.Trim() -ne 'true') {
    return
  }

  Set-RepoGitConfigValue -Name 'core.quotepath' -Value 'false'
  Set-RepoGitConfigValue -Name 'i18n.commitEncoding' -Value 'utf-8'
  Set-RepoGitConfigValue -Name 'i18n.logOutputEncoding' -Value 'utf-8'
  Set-RepoGitConfigValue -Name 'core.autocrlf' -Value 'false'
  Set-RepoGitConfigValue -Name 'core.eol' -Value 'lf'
}

try {
  Set-RepoGitUtf8Config
} catch {}

Write-Host '[utf8] session configured.'
