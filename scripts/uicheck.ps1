# Starts burnmon.exe with its dev-only eval channel open (BURNMON_UICHECK=1,
# never set by release builds or the README), runs one or more
# tools\uicheck checks against the real running window, then stops the exe.
# Screenshots and clicks/keys go straight from tools\uicheck to the window
# via Win32 (PrintWindow, SendInput); only DOM reads/evals go through the
# BURNMON_UICHECK channel, since only burnmon.exe itself can run JS in its
# own page. Run from the repo root in PowerShell:
#   .\scripts\uicheck.ps1                 # w0 to w8, then the seven d groups
#   .\scripts\uicheck.ps1 dev             # the seven d groups only
#   .\scripts\uicheck.ps1 reverse         # as no args, d groups and their cases backwards
#   .\scripts\uicheck.ps1 w0 w1           # runs only the named checks
#   .\scripts\uicheck.ps1 layout d0       # a d group, an old d name or group/case
#
# A d check (one of the seven groups in tools\uicheck\groups.go, a
# group/case such as layout/d34, or an old name such as d34) targets
# burnmon-dev.exe instead: its own dev eval channel (BURNMON_DEV_UICHECK=1,
# cmd\burnmon-dev\uicheck_devserver.go), port 9334 (not burnmon.exe's 9333)
# and window title "BurnMon Dev" (not "BurnMon"), so either exe can be
# driven without the other's channel or window getting in the way (the
# design doc's own normal case: both running at once). Named checks run one
# exe per invocation; do not mix a w and a d check in the same call. With no
# names the two exes run one after the other, as two separate launches.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot\..

# Same order as devGroups in tools\uicheck\groups.go. Matched by name, not by
# prefix: "waits" starts with "w".
$devGroups = @("fit", "charts", "station", "layout", "map", "waits", "shell")
function Test-DevCheck([string]$name) {
    return ($devGroups -contains $name) -or ($name -match '^[a-z]+/') -or ($name -match '^d\d+$')
}

$reverse = $args -contains "reverse"
$devOnly = $args -contains "dev"
$targetChecks = @($args | Where-Object { $_ -ne "reverse" -and $_ -ne "dev" })
$devPass = @()
if ($reverse) { $devPass = @("reverse") }

if ($targetChecks.Count -eq 0) {
    if ($devOnly) {
        & "$PSScriptRoot\uicheck-dev.ps1" @devGroups @devPass
        exit $LASTEXITCODE
    }
    # Two separate launches; the d groups run even when a w check failed, and
    # the run fails when either did.
    & $PSCommandPath w0 w1 w2 w3 w4 w5 w6 w7 w8
    $wExit = $LASTEXITCODE
    & "$PSScriptRoot\uicheck-dev.ps1" @devGroups @devPass
    $dExit = $LASTEXITCODE
    Write-Host "uicheck: burnmon.exe checks exit $wExit, burnmon-dev.exe groups exit $dExit"
    if ($wExit -ne 0) { exit $wExit }
    exit $dExit
}

$devChecks = @($targetChecks | Where-Object { Test-DevCheck $_ })
$otherChecks = @($targetChecks | Where-Object { -not (Test-DevCheck $_) })
if ($devChecks.Count -gt 0 -and $otherChecks.Count -gt 0) {
    Write-Host "uicheck: cannot mix a burnmon-dev.exe check ($($devChecks -join ', ')) with a burnmon.exe check ($($otherChecks -join ', ')) in one call; run them separately."
    exit 1
}
if ($devChecks.Count -gt 0) {
    & "$PSScriptRoot\uicheck-dev.ps1" @targetChecks @devPass
    exit $LASTEXITCODE
}
if ($reverse) {
    Write-Host "uicheck: reverse applies to the d groups only; running the w checks in order."
}

$exe = Join-Path (Get-Location) "burnmon.exe"
if (-not (Test-Path $exe)) {
    Write-Host "burnmon.exe not found; run .\build.ps1 first."
    exit 1
}
# Self-managed checks (tools\uicheck\main.go's own selfManaged map: w1 tests
# startup itself) start and stop their own burnmon.exe instance, so each must
# run before anything else here starts one, outside this script's own
# Start-Process/wait-for-port flow.
$selfManagedChecks = @("w1")
foreach ($sm in $selfManagedChecks) {
    if ($targetChecks -contains $sm) {
        Push-Location tools\uicheck
        try {
            Write-Host "--- uicheck $sm (self-managed) ---"
            go run . $sm
            if ($LASTEXITCODE -ne 0) {
                Write-Host "uicheck $sm FAILED"
                exit $LASTEXITCODE
            }
        } finally {
            Pop-Location
        }
        $targetChecks = $targetChecks | Where-Object { $_ -ne $sm }
    }
}
if ($targetChecks.Count -eq 0) {
    Write-Host "uicheck: all checks passed"
    exit 0
}

$logPath = "$env:LOCALAPPDATA\burnmon\burnmon-app.log"
$logLinesBefore = 0
if (Test-Path $logPath) { $logLinesBefore = (Get-Content $logPath | Measure-Object -Line).Lines }

$env:BURNMON_UICHECK = "1"
$proc = Start-Process -FilePath $exe -PassThru
try {
    # Poll the eval channel instead of a fixed sleep: it appears only once
    # the window and its bmUICheckResult binding are actually up.
    $deadline = (Get-Date).AddSeconds(20)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        try {
            $client = New-Object System.Net.Sockets.TcpClient
            $client.Connect("127.0.0.1", 9333)
            $client.Close()
            $ready = $true
            break
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    if (-not $ready) {
        Write-Host "Dev eval port 9333 never came up. Either BURNMON_UICHECK is not being read, or the window failed to start; check %LOCALAPPDATA%\burnmon\burnmon-app.log."
        exit 1
    }
    # The port opens as soon as bmUICheckResult is bound, well before the
    # startup goroutine's first Collect (W1's own fix) finishes; an eval
    # sent before that can time out waiting for the UI thread. Wait for
    # app.log's own "stage Collect (total)" line instead of a fixed sleep,
    # since Collect's real duration varies a lot with machine load (seen
    # anywhere from ~2s to 60s+ on this laptop during this session). The
    # log is append-only across runs (setupLog rotates at 512KB, never
    # truncates), so this only counts a match on a line added since this
    # process started, not a stale one from an earlier run.
    $collectDeadline = (Get-Date).AddSeconds(90)
    $collectDone = $false
    while ((Get-Date) -lt $collectDeadline) {
        if (Test-Path $logPath) {
            $newLines = Get-Content $logPath | Select-Object -Skip $logLinesBefore
            if ($newLines -match "stage Collect \(total\)") {
                $collectDone = $true
                break
            }
        }
        Start-Sleep -Seconds 2
    }
    if (-not $collectDone) {
        Write-Host "First Collect never finished within 90s; checks would likely time out anyway. Check %LOCALAPPDATA%\burnmon\burnmon-app.log."
        exit 1
    }

    Push-Location tools\uicheck
    try {
        foreach ($check in $targetChecks) {
            Write-Host "--- uicheck $check ---"
            go run . $check
            if ($LASTEXITCODE -ne 0) {
                Write-Host "uicheck $check FAILED"
                exit $LASTEXITCODE
            }
        }
    } finally {
        Pop-Location
    }
    Write-Host "uicheck: all checks passed"
} finally {
    Remove-Item Env:\BURNMON_UICHECK -ErrorAction SilentlyContinue
    if (-not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force
    }
}
