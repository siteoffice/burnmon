# Starts controlcentre.exe with its own dev-only eval channel open
# (CONTROLCENTRE_UICHECK=1, never set by release builds or the README) on a
# fixture root of five real git repos, runs the tools\uicheck gitpage group
# (or the named cases) against the real running window, then stops the exe.
# Mirrors uicheck-dev.ps1 one exe, one port (9335) and one window title
# ("Control Centre") over. Called by uicheck.ps1 for any gitpage check, or run
# directly from the repo root in PowerShell:
#   .\scripts\uicheck-centre.ps1                  # the gitpage group
#   .\scripts\uicheck-centre.ps1 gitpage/strip    # one case
# Needs git on PATH. The fixture lives in a fresh temp folder and so do the
# exe's own settings (LOCALAPPDATA is pointed there for the exe only), so a
# run never touches the real controlcentre.json; the real file is hashed
# before and after as the guard.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot\..

$exe = Join-Path (Get-Location) "controlcentre.exe"
if (-not (Test-Path $exe)) {
    Write-Host "controlcentre.exe not found; run .\build.ps1 first."
    exit 1
}
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Host "git is not on PATH; the fixture repos need it."
    exit 1
}
$targetChecks = @($args)
if ($targetChecks.Count -eq 0) { $targetChecks = @("gitpage") }

$realSettings = Join-Path $env:LOCALAPPDATA "burnmon\controlcentre.json"
function Get-RealHash {
    if (Test-Path $realSettings) { return (Get-FileHash -Algorithm SHA256 -Path $realSettings).Hash }
    return "missing"
}
$hashBefore = Get-RealHash

$work = Join-Path $env:TEMP ("burnmon-uicheck-centre\" + [guid]::NewGuid().ToString())
$root = Join-Path $work "root"
$up = Join-Path $work "up"
$appdata = Join-Path $work "appdata"
New-Item -ItemType Directory -Force -Path $root, $up, $appdata | Out-Null
Write-Host "uicheck: fixture folder $work"

# Real git, no global config, a fixed author: the same five repos on every run.
$emptyConfig = Join-Path $work "empty.gitconfig"
Set-Content -Path $emptyConfig -Value "" -Encoding ascii
$env:GIT_CONFIG_GLOBAL = $emptyConfig
$env:GIT_CONFIG_NOSYSTEM = "1"
$env:GIT_AUTHOR_NAME = "Fixture"
$env:GIT_AUTHOR_EMAIL = "fixture@example.invalid"
$env:GIT_COMMITTER_NAME = "Fixture"
$env:GIT_COMMITTER_EMAIL = "fixture@example.invalid"

function Invoke-Git([string]$dir, [string[]]$gitArgs) {
    Push-Location $dir
    try {
        & git @gitArgs 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "git $($gitArgs -join ' ') failed in $dir" }
    } finally {
        Pop-Location
    }
}
function Add-File([string]$repo, [string]$name, [string]$text) {
    Set-Content -Path (Join-Path $repo $name) -Value $text -Encoding ascii
}
function New-FixtureRepo([string]$name) {
    $bare = Join-Path $up "$name.git"
    Invoke-Git $work @("init", "-q", "--bare", "-b", "main", $bare)
    Invoke-Git $root @("clone", "-q", $bare, $name)
    $repo = Join-Path $root $name
    Add-File $repo "a.txt" "one"
    Invoke-Git $repo @("add", "a.txt")
    Invoke-Git $repo @("commit", "-q", "-m", "first")
    Invoke-Git $repo @("push", "-q", "origin", "main")
    return $repo
}

# pub: clean, shared, nothing open.
[void](New-FixtureRepo "pub")
# dirty: one changed file.
$dirty = New-FixtureRepo "dirty"
Add-File $dirty "a.txt" "changed"
# ahead: one commit not pushed.
$ahead = New-FixtureRepo "ahead"
Add-File $ahead "b.txt" "two"
Invoke-Git $ahead @("add", "b.txt")
Invoke-Git $ahead @("commit", "-q", "-m", "local work")
# behind: the upstream moved on and the clone fetched.
$behind = New-FixtureRepo "behind"
$other = Join-Path $work "other"
Invoke-Git $work @("clone", "-q", (Join-Path $up "behind.git"), $other)
Add-File $other "c.txt" "three"
Invoke-Git $other @("add", "c.txt")
Invoke-Git $other @("commit", "-q", "-m", "upstream moved")
Invoke-Git $other @("push", "-q", "origin", "main")
Invoke-Git $behind @("fetch", "-q")
# feat: a feature branch with one commit, never pushed.
$feat = New-FixtureRepo "feat"
Invoke-Git $feat @("switch", "-q", "-c", "feat")
Add-File $feat "d.txt" "four"
Invoke-Git $feat @("add", "d.txt")
Invoke-Git $feat @("commit", "-q", "-m", "feature work")

# From here the exe reads the real git on this machine, not the fixture identity.
Remove-Item Env:\GIT_CONFIG_GLOBAL, Env:\GIT_CONFIG_NOSYSTEM -ErrorAction SilentlyContinue

$realLocalAppData = $env:LOCALAPPDATA
$env:LOCALAPPDATA = $appdata
$env:CONTROLCENTRE_UICHECK = "1"
$proc = $null
try {
    $proc = Start-Process -FilePath $exe -ArgumentList @("--root", $root) -PassThru
} finally {
    # Only the exe gets the temp settings folder; go run below needs the real cache.
    $env:LOCALAPPDATA = $realLocalAppData
    Remove-Item Env:\CONTROLCENTRE_UICHECK -ErrorAction SilentlyContinue
}
$exitCode = 0
try {
    $deadline = (Get-Date).AddSeconds(60)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        try {
            $client = New-Object System.Net.Sockets.TcpClient
            $client.Connect("127.0.0.1", 9335)
            $client.Close()
            $ready = $true
            break
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    if (-not $ready) {
        Write-Host "Dev eval port 9335 never came up. Either CONTROLCENTRE_UICHECK is not being read, or the window failed to start (it needs the WebView2 runtime)."
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
    $hashAfter = Get-RealHash
    Write-Host "uicheck: real controlcentre.json before $hashBefore"
    Write-Host "uicheck: real controlcentre.json after  $hashAfter"
    if ($hashBefore -ne $hashAfter) {
        Write-Host "uicheck: the real controlcentre.json changed during the run."
        $exitCode = 1
    }
    # WebView2's helper processes keep their files open for a moment after the exe stops.
    $removed = $false
    for ($try = 1; $try -le 8 -and -not $removed; $try++) {
        try {
            Remove-Item -Recurse -Force -Path $work -ErrorAction Stop
            $removed = $true
        } catch {
            Start-Sleep -Seconds 2
        }
    }
    if (-not $removed) {
        Write-Host "uicheck: could not remove the fixture folder $work (WebView2 still holds a file); delete it later."
    }
}
exit $exitCode
