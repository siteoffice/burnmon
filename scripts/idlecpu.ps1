# Measures one process's CPU over a window, as a share of the whole machine:
# the CPU time it used, divided by the window times the logical cores. B8
# item 2's target is under 2 percent for BurnMon Dev left idle for 10 minutes.
# It reads TotalProcessorTime at the start and the end and nothing else, so
# it adds no load of its own while it waits. Run from the repo root in
# PowerShell (5.1 or 7):
#   .\scripts\idlecpu.ps1 -Name burnmon-dev -Minutes 10
param(
    [Parameter(Mandatory = $true)][string]$Name,
    [double]$Minutes = 10
)
$ErrorActionPreference = "Stop"

if ($Minutes -le 0) {
    Write-Host "idlecpu: -Minutes must be more than 0."
    exit 1
}

# Exactly one process: with two of the same name the numbers would mix, and
# the end sample goes by process id so a restart in the window is caught.
$procs = @(Get-Process -Name $Name -ErrorAction SilentlyContinue)
if ($procs.Count -ne 1) {
    Write-Host "idlecpu: want exactly one process named $Name, found $($procs.Count)."
    exit 1
}
$proc = $procs[0]
$cores = [Environment]::ProcessorCount
$start = $proc.TotalProcessorTime.TotalSeconds
Write-Host "idlecpu: $Name pid $($proc.Id), sampling for $Minutes minutes"

Start-Sleep -Milliseconds ([int]($Minutes * 60000))

$after = Get-Process -Id $proc.Id -ErrorAction SilentlyContinue
if ($null -eq $after -or $after.ProcessName -ne $proc.ProcessName) {
    Write-Host "idlecpu: $Name pid $($proc.Id) exited during the window; no result."
    exit 1
}
$delta = $after.TotalProcessorTime.TotalSeconds - $start
$pct = $delta / ($Minutes * 60 * $cores) * 100

# Invariant culture, so the decimal point is a point on a Dutch locale too.
$inv = [Globalization.CultureInfo]::InvariantCulture
Write-Host "idlecpu: cpu seconds $($delta.ToString('0.00', $inv)) over $Minutes minutes"
Write-Host "idlecpu: logical cores $cores"
Write-Host "idlecpu: cpu percent of the machine $($pct.ToString('0.00', $inv))"
