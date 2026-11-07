#requires -Version 5.1
<#
.SYNOPSIS
Runs repeatable Windows checks without launching mixr or diagnostic tools.
.DESCRIPTION
Use -Plan to inspect commands without running Go, GCC, or builds.
Results go to a new validation-results directory per invocation. Full builds
are removed afterward unless -KeepArtifacts is supplied; logs always remain.
#>
[CmdletBinding()]
param(
    [ValidateSet('quick', 'full', 'race')]
    [string]$Mode = 'quick',
    [switch]$KeepArtifacts,
    [switch]$Plan
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../../..'))
$runId = (Get-Date -Format 'yyyyMMdd-HHmmss-fff') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
$runRoot = Join-Path $repoRoot "validation-results/$runId-$Mode"
$artifactRoot = Join-Path $runRoot 'artifacts'
$summary = [Collections.Generic.List[string]]::new()
$steps = 0
$status = 'FAIL'
$savedEnv = @{}
# Process-only changes; restore even when a prerequisite or command fails.
$envNames = @('GOOS', 'GOARCH', 'CGO_ENABLED', 'CC', 'MIXR_BUILD_OUTPUT_DIR',
    'MIXR_TEST_LIVE_FOREGROUND', 'GOTOOLCHAIN', 'GORACE')
foreach ($name in $envNames) {
    $savedEnv[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
New-Item -ItemType Directory -Path $runRoot | Out-Null
$summary.Add("Started: $([DateTimeOffset]::Now.ToString('o'))")
$summary.Add("Mode: $Mode; Plan: $Plan; KeepArtifacts: $KeepArtifacts")
$summary.Add("Repository: $repoRoot")
$summary.Add("Windows: $([Environment]::OSVersion.VersionString); PowerShell: $($PSVersionTable.PSVersion)")

function Invoke-Check {
    param([string]$Label, [string]$Program, [string[]]$Arguments)
    $script:steps++
    $display = "$Program $($Arguments -join ' ')"
    $summary.Add("[$env:GOARCH] $Label : $display")
    Write-Host $display
    if ($Plan) { return }
    $log = Join-Path $runRoot ('{0:D2}-{1}.txt' -f $script:steps, $Label)
    # Stream output to console and a separate tooling log. Native failures must
    # be checked explicitly in Windows PowerShell as well as PowerShell 7.
    $previousPreference = $ErrorActionPreference
    try {
        # Windows PowerShell represents native stderr as error records. Keep
        # those records in the log and decide success by the native exit code.
        $ErrorActionPreference = 'Continue'
        $PSNativeCommandUseErrorActionPreference = $false
        & $Program @Arguments 2>&1 | Tee-Object -FilePath $log -ErrorAction Stop | Out-Host
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousPreference
    }
    $summary.Add("  Exit: $code; Output: $([IO.Path]::GetFileName($log))")
    if ($code -ne 0) { throw "$Label failed (exit $code). See $log" }
}

Push-Location $repoRoot
try {
    if ($env:OS -ne 'Windows_NT') { throw 'Validation requires Windows.' }
    $env:GOOS = 'windows'
    # Keep optional desktop lookups out of ordinary automated validation.
    $env:MIXR_TEST_LIVE_FOREGROUND = '0'
    # Do not let Go download/install another toolchain automatically.
    $env:GOTOOLCHAIN = 'local'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    if (-not $Plan) {
        Get-Command go -CommandType Application -ErrorAction Stop | Out-Null
        $commit = & git rev-parse HEAD
        if ($LASTEXITCODE -ne 0) { throw 'Cannot determine Git commit.' }
        $version = & git describe --tags --always
        if ($LASTEXITCODE -ne 0) { throw 'Cannot determine Git version tag.' }
        $dirty = & git status --porcelain --untracked-files=no
        if ($LASTEXITCODE -ne 0) { throw 'Cannot determine Git working-tree state.' }
        $summary.Add("Commit: $commit; Version: $version; Tracked changes: $([bool]$dirty)")
        Invoke-Check 'go-version' 'go' @('version')
    }
    if ($Mode -eq 'race') {
        $env:CGO_ENABLED = '1'
        # Ignore caller GORACE overrides that could hide races or change exit codes.
        $env:GORACE = 'exitcode=66'
        if (-not $env:CC) { $env:CC = 'gcc' }
        $summary.Add("Race compiler: $env:CC")
        if (-not $Plan) {
            $guidance = 'See docs/WINDOWS_RACE_TESTING.md. Install MSYS2 UCRT64 GCC manually, then set session PATH and CC to its gcc.exe.'
            $compiler = Get-Command $env:CC -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
            if (-not $compiler) { throw "C compiler '$env:CC' not found. $guidance" }
            Invoke-Check 'gcc-version' $compiler.Source @('--version')
            $target = & $compiler.Source -dumpmachine
            if ($LASTEXITCODE -ne 0 -or "$target" -notmatch '^x86_64-w64-mingw32$') {
                throw "GCC must target x86_64-w64-mingw32; got '$target'. $guidance"
            }
            $library = & $compiler.Source --print-file-name libsynchronization.a
            if ($LASTEXITCODE -ne 0 -or -not [IO.Path]::IsPathRooted("$library") -or
                -not (Test-Path -LiteralPath "$library" -PathType Leaf)) {
                throw "GCC lacks a resolvable libsynchronization.a (MinGW-w64 runtime v8+ required). $guidance"
            }
            $summary.Add("Compiler target: $target; Synchronization library: $library")
        }
        Invoke-Check 'race-tests' 'go' @('test', '-race', '-count=1', '-buildvcs=false', './...')
    } else {
        $arches = @('amd64')
        if ($Mode -eq 'full') { $arches = @('386', 'amd64') }
        foreach ($arch in $arches) {
            $env:GOARCH = $arch
            Invoke-Check "tests-$arch" 'go' @('test', '-count=1', '-buildvcs=false', './...')
            if ($Mode -ne 'full') { continue }
            $outputDir = Join-Path $artifactRoot $arch
            New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
            $env:MIXR_BUILD_OUTPUT_DIR = $outputDir
            foreach ($kind in @('dev', 'release')) {
                Invoke-Check "build-$kind-$arch" 'cmd.exe' @('/d', '/c', "call pkg\mixr\scripts\windows\build-$kind.bat")
            }
            foreach ($tool in @('windows-routing-spike', 'windows-audio-control', 'windows-microphone-control')) {
                Invoke-Check "$tool-$arch" 'go' @('build', '-o', (Join-Path $outputDir "$tool-$arch.exe"), ".\cmd\$tool")
            }
        }
    }
    $status = 'PASS'
    if ($Plan) { $status = 'PLAN ONLY - no validation executed; prerequisites not checked' }
} catch {
    $summary.Add("Failure: $($_.Exception.Message)")
    throw
} finally {
    try {
        if (-not $KeepArtifacts -and (Test-Path -LiteralPath $artifactRoot)) {
            # Delete only the known build filenames in this unique run. Never
            # enumerate/delete releases, root executables, or runtime logs.
            foreach ($arch in @('386', 'amd64')) {
                $outputDir = Join-Path $artifactRoot $arch
                foreach ($file in @('mixr-dev.exe', 'mixr-release.exe', "windows-routing-spike-$arch.exe",
                    "windows-audio-control-$arch.exe", "windows-microphone-control-$arch.exe")) {
                    $target = Join-Path $outputDir $file
                    if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target }
                }
            }
        }
    } catch {
        $status = 'FAIL'
        $summary.Add("Cleanup failure: $($_.Exception.Message)")
        throw
    } finally {
        foreach ($name in $envNames) {
            [Environment]::SetEnvironmentVariable($name, $savedEnv[$name], 'Process')
        }
        Pop-Location
        $summary.Add("Result: $status")
        $summary.Add("Finished: $([DateTimeOffset]::Now.ToString('o'))")
        $summary | Set-Content -LiteralPath (Join-Path $runRoot 'summary.txt') -Encoding UTF8
        Write-Host "Result: $status; summary: $runRoot\summary.txt"
    }
}
