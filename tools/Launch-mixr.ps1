# Run from the release directory. Preserve fatal runtime stderr and process exits.
$ErrorActionPreference = 'Stop'
$releaseDirectory = $PSScriptRoot
$executable = Join-Path $releaseDirectory 'mixr-release.exe'
if (Get-Process -Name 'mixr-release' -ErrorAction SilentlyContinue) {
    throw 'A mixr release process is already running. Quit it before starting this build.'
}
$logDirectory = Join-Path $releaseDirectory 'logs'
New-Item -ItemType Directory -Force -Path $logDirectory | Out-Null
$runStamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$processLog = Join-Path $logDirectory "mixr-process-$runStamp.log"
$stderrLog = Join-Path $logDirectory "mixr-stderr-$runStamp.log"
$stdoutLog = Join-Path $logDirectory "mixr-stdout-$runStamp.log"
# Own the Process handle from creation: Start-Process can lose the exit code
# when a GUI application exits before its Process object is returned.
$process = New-Object System.Diagnostics.Process
$process.StartInfo.FileName = $executable
$process.StartInfo.WorkingDirectory = $releaseDirectory
$process.StartInfo.UseShellExecute = $false
$process.StartInfo.CreateNoWindow = $true
$process.StartInfo.RedirectStandardError = $true
$process.StartInfo.RedirectStandardOutput = $true
$stderrStream = [System.IO.File]::Create($stderrLog)
$stdoutStream = [System.IO.File]::Create($stdoutLog)
if (-not $process.Start()) { throw 'Could not start mixr.' }
$stderrCopy = $process.StandardError.BaseStream.CopyToAsync($stderrStream)
$stdoutCopy = $process.StandardOutput.BaseStream.CopyToAsync($stdoutStream)
[pscustomobject]@{time=(Get-Date).ToString('o'); event='started'; pid=$process.Id; executable=$executable; sha256=(Get-FileHash -LiteralPath $executable).Hash; config=(Join-Path $releaseDirectory 'config.yaml')} | ConvertTo-Json -Compress | Add-Content -LiteralPath $processLog
Write-Host "mixr started (PID $($process.Id)). Quit using the tray; keep this window open for exit capture."
while (-not $process.WaitForExit(15000)) {
    try {
        $process.Refresh()
        [pscustomobject]@{time=(Get-Date).ToString('o'); event='alive'; pid=$process.Id; threads=$process.Threads.Count; handles=$process.HandleCount; privateBytes=$process.PrivateMemorySize64; cpuSeconds=$process.TotalProcessorTime.TotalSeconds} | ConvertTo-Json -Compress | Add-Content -LiteralPath $processLog
    } catch {
        [pscustomobject]@{time=(Get-Date).ToString('o'); event='sample-error'; message=$_.Exception.Message} | ConvertTo-Json -Compress | Add-Content -LiteralPath $processLog
    }
}
$process.WaitForExit()
$null = $stderrCopy.GetAwaiter().GetResult()
$null = $stdoutCopy.GetAwaiter().GetResult()
$stderrStream.Dispose()
$stdoutStream.Dispose()
[pscustomobject]@{time=(Get-Date).ToString('o'); event='exited'; pid=$process.Id; exitCode=$process.ExitCode} | ConvertTo-Json -Compress | Add-Content -LiteralPath $processLog
Write-Host "mixr exited with code $($process.ExitCode). Evidence: $logDirectory"
exit $process.ExitCode


