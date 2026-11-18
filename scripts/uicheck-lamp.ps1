# Starts the lamp (burnmon-dev.exe lamp --demo) with its dev-only eval channel
# open (BURNMON_LAMP_UICHECK=1, never set by release builds or the README),
# runs the tools\uicheck lamp group against the real running window, then
# stops the lamp. Mirrors uicheck-centre.ps1: one exe, one port (9336).
# Run it from the repo root in PowerShell, after .\build.ps1:
#   .\scripts\uicheck-lamp.ps1                 # the lamp group
#   .\scripts\uicheck-lamp.ps1 lamp/states     # one case
# Needs a logged-in, unlocked desktop (the lamp is a real window). The demo
# reads no snapshot and no setting and writes no BurnMon file; its browser
# profile is under the temp folder. The page's own poll is frozen by the check.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot\..

$exe = Join-Path (Get-Location) "burnmon-dev.exe"
if (-not (Test-Path $exe)) {
    Write-Host "burnmon-dev.exe not found; run .\build.ps1 first."
    exit 1
}
$targetChecks = @($args)
if ($targetChecks.Count -eq 0) { $targetChecks = @("lamp") }

$env:BURNMON_LAMP_UICHECK = "1"
$proc = $null
try {
    $proc = Start-Process -FilePath $exe -ArgumentList @("lamp", "--demo") -PassThru
} finally {
    Remove-Item Env:\BURNMON_LAMP_UICHECK -ErrorAction SilentlyContinue
}
$exitCode = 0
try {
    $deadline = (Get-Date).AddSeconds(60)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        try {
            $client = New-Object System.Net.Sockets.TcpClient
            $client.Connect("127.0.0.1", 9336)
            $client.Close()
            $ready = $true
            break
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    if (-not $ready) {
        Write-Host "Lamp eval port 9336 never came up. Either BURNMON_LAMP_UICHECK is not being read, another lamp demo already runs, or the window failed to start (it needs the WebView2 runtime)."
        $exitCode = 1
    } else {
        Push-Location tools\uicheck
        try {
            foreach ($check in $targetChecks) {
                Write-Host "--- uicheck $check ---"
                go run . $check
                if ($LASTEXITCODE -ne 0) {
                    Write-Host "uicheck $check FAILED"
                    $exitCode = $LASTEXITCODE
                    break
                }
            }
        } finally {
            Pop-Location
        }
        if ($exitCode -eq 0) { Write-Host "uicheck: all checks passed" }
    }
} finally {
    if ($proc -and -not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force
    }
    if ($proc) { [void]$proc.WaitForExit(10000) }
}
exit $exitCode
