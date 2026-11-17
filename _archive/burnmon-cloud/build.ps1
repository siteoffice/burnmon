# Builds the BurnMon cloud plugin.
#
# Runs in: PowerShell (Windows PowerShell 5.1 or PowerShell 7).
# -Target is where cloud sessions copy their usage extract to. The default,
# ~/.burnmon/cloud/latest.jsonl, is the same on every computer: the copy tool
# expands "~" to the user's home folder (measured 2026-10-13), and BurnMon
# reads that folder. That build is the universal one, published in the ZND
# Skills Public marketplace. Give your own path(s) only for a build for one
# computer; a cloud session can only write into a folder connected to it.
# -Version stamps the manifest version of the outputs (the shared ZND version
# clock); the default is plugin\.claude-plugin\plugin.json.
# -OutDir also writes the unpacked plugin to a folder, for the marketplace
# repo (znd-skills-public\plugins\burnmon-cloud). An existing plugin.json
# there keeps its version unless -Version is given, because publish-skills.ps1
# owns that number.
#
# Examples:
#   cd C:\ZND\50_projects\burnmon\tools\burnmon-cloud
#   .\build.ps1
#   .\build.ps1 -OutDir C:\ZND\50_projects\znd-skills-public\plugins\burnmon-cloud
#   .\build.ps1 -Target 'C:\path\to\burnmon\.burnmon\cloud\latest.jsonl'
#
# Output: dist\burnmon-cloud-<version>.plugin (install it under Customize >
# Plugins > Add) and the same archive as .zip.
param(
    [string[]]$Target = @('~/.burnmon/cloud/latest.jsonl'),
    [string]$Version = '',
    [string]$OutDir = ''
)
$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$src = Join-Path $here 'plugin'
$manifest = Get-Content (Join-Path $src '.claude-plugin\plugin.json') -Raw | ConvertFrom-Json
if ($OutDir -and -not $Version) {
    $existing = Join-Path $OutDir '.claude-plugin\plugin.json'
    if (Test-Path -LiteralPath $existing) {
        $Version = (Get-Content -LiteralPath $existing -Raw | ConvertFrom-Json).version
    }
}
if ($Version) { $manifest.version = $Version }
$version = $manifest.version

foreach ($t in $Target) {
    if (-not ($t -replace '\\', '/').EndsWith('/.burnmon/cloud/latest.jsonl')) {
        throw "Target '$t' must end in .burnmon\cloud\latest.jsonl"
    }
}

# Hooks (1.1.0 and up). The extract rewrites latest.jsonl in place (1.1.2: a rename
# made the watcher go deaf after one event).
# SessionStart registers the drop folder with the watcher (--watch), so every write
# of latest.jsonl fires FileChanged, and FileChanged copies it to the computer:
# the main path, no model tokens. Stop, UserPromptSubmit and SessionEnd copy too,
# as the fallback for a watcher that does not fire. 3.4.0 dropped them and lost
# data: measured 2026-10-13 in one cloud session, FileChanged fired 6 times, the
# last one 53 s after SessionStart, while latest.jsonl was written 39 more times.
# 3.4.1 restores them. Their cost: Claude Code on the computer has no
# remote-devices server, so a plugin synced from claude.ai shows "MCP server
# 'remote-devices' not connected" there (see the README). The fallbacks can go
# again once coverage.py shows the watcher fires on every write of a long session.
# Hooks of one event run in parallel, so the Stop copy carries the file of the
# turn before.
# 1.1.1 diagnostic: every extract run logs to diag.log, and FileChanged also
# runs extract.py --fired next to the copy, which logs the moment the copy
# started. UserPromptSubmit copies diag.log next to latest.jsonl (BurnMon
# reads only .jsonl there). Each copy is its own hook entry, so a missing
# diag.log can never stop the latest.jsonl copy.
$script = '${CLAUDE_PLUGIN_ROOT}/scripts/extract.py'
$watch = [ordered]@{ type = 'command'; command = 'python3'; args = @($script, '--watch'); timeout = 10 }
$fired = [ordered]@{ type = 'command'; command = 'python3'; args = @($script, '--fired'); timeout = 10 }
$extract = [ordered]@{ type = 'command'; command = 'python3'; args = @($script); timeout = 30 }
$extractThrottled = [ordered]@{ type = 'command'; command = 'python3'; args = @($script, '--throttle'); async = $true; timeout = 30 }
$copies = @(foreach ($t in $Target) {
    [ordered]@{
        type = 'mcp_tool'; server = 'remote-devices'; tool = 'device_commit_files'; timeout = 30
        input = [ordered]@{ files = @([ordered]@{ devicePath = $t; stagedPath = '/mnt/user-data/outputs/.burnmon/cloud/latest.jsonl' }) }
    }
})
$diagCopies = @(foreach ($t in $Target) {
    [ordered]@{
        type = 'mcp_tool'; server = 'remote-devices'; tool = 'device_commit_files'; timeout = 30
        input = [ordered]@{ files = @([ordered]@{ devicePath = ($t.Substring(0, $t.Length - 'latest.jsonl'.Length) + 'diag.log'); stagedPath = '/mnt/user-data/outputs/.burnmon/cloud/diag.log' }) }
    }
})
$hooks = [ordered]@{
    description = 'BurnMon cloud: writes a usage-only extract of each cloud Cowork session and copies it to BurnMon''s drop folder after each write. No prompts or content.'
    hooks = [ordered]@{
        SessionStart     = @([ordered]@{ hooks = @($watch) })
        PostToolUse      = @([ordered]@{ hooks = @($extractThrottled) })
        SubagentStop     = @([ordered]@{ hooks = @($extract) })
        Stop             = @([ordered]@{ hooks = @($extract) + $copies })
        FileChanged      = @([ordered]@{ matcher = 'latest.jsonl'; hooks = @($fired) + $copies })
        UserPromptSubmit = @([ordered]@{ hooks = $copies + $diagCopies })
        SessionEnd       = @([ordered]@{ hooks = $copies })
    }
}

$stage = Join-Path ([IO.Path]::GetTempPath()) ("burnmon-cloud-" + [guid]::NewGuid())
Copy-Item $src $stage -Recurse
# Running the tests leaves Python bytecode next to extract.py; it does not ship.
Get-ChildItem $stage -Recurse -Force -Directory -Filter '__pycache__' | Remove-Item -Recurse -Force
New-Item -ItemType Directory -Path (Join-Path $stage 'hooks') -Force | Out-Null
$utf8 = New-Object Text.UTF8Encoding $false
$json = $hooks | ConvertTo-Json -Depth 12
[IO.File]::WriteAllText((Join-Path $stage 'hooks\hooks.json'), $json, $utf8)
[IO.File]::WriteAllText((Join-Path $stage '.claude-plugin\plugin.json'), (($manifest | ConvertTo-Json -Depth 12) + "`n"), $utf8)

if ($OutDir) {
    # Replace the unpacked plugin wholesale, so a file removed here is removed there too.
    if (Test-Path -LiteralPath $OutDir) { Remove-Item -LiteralPath $OutDir -Recurse -Force }
    New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
    Copy-Item -Path (Get-ChildItem $stage -Force).FullName -Destination $OutDir -Recurse -Force
    Write-Host "Wrote the unpacked plugin $version to $OutDir"
}

$dist = Join-Path $here 'dist'
New-Item -ItemType Directory -Path $dist -Force | Out-Null
$zip = Join-Path $dist "burnmon-cloud-$version.zip"
Remove-Item $zip -ErrorAction SilentlyContinue
# -Force lists .claude-plugin too: a '*' wildcard skips dot folders outside Windows.
Compress-Archive -Path (Get-ChildItem $stage -Force).FullName -DestinationPath $zip -Force
Copy-Item $zip (Join-Path $dist "burnmon-cloud-$version.plugin") -Force
Remove-Item $stage -Recurse -Force
Write-Host "Built $zip and burnmon-cloud-$version.plugin for: $($Target -join ', ')"
