#requires -Version 5.1
# Lightweight orchestration checks only: fake Go/GCC, real Git metadata.
[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../../..'))
$testRoot = Join-Path $repoRoot ('validation-results/tooling-test-' + [guid]::NewGuid().ToString('N'))
$fakeBin = Join-Path $testRoot 'fake tools'
New-Item -ItemType Directory -Path $fakeBin -Force | Out-Null
$names = @('PATH', 'CC', 'GOOS', 'GOARCH', 'CGO_ENABLED', 'MIXR_BUILD_OUTPUT_DIR',
    'MIXR_TEST_LIVE_FOREGROUND', 'GOTOOLCHAIN', 'GORACE', 'MIXR_TOOLING_TEST_TRACE',
    'MIXR_TOOLING_TEST_FAIL', 'MIXR_TOOLING_TEST_TARGET', 'MIXR_TOOLING_TEST_LIBRARY')
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }

function Assert([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw "Assertion failed: $Message" }
}

function Run-Case([string]$Mode, [switch]$Keep, [switch]$Plan, [switch]$Fail) {
    $before = @(Get-ChildItem (Join-Path $repoRoot 'validation-results') -Directory).FullName
    $cwd = (Get-Location).Path
    $didFail = $false
    try { & "$PSScriptRoot/validate.ps1" -Mode $Mode -KeepArtifacts:$Keep -Plan:$Plan }
    catch { $didFail = $true; Write-Host "Expected/check failure: $($_.Exception.Message)" }
    Assert ($didFail -eq [bool]$Fail) "$Mode failure expectation"
    Assert ((Get-Location).Path -eq $cwd) 'working directory restored'
    Assert ($env:GOARCH -eq 'sentinel-arch' -and $env:CGO_ENABLED -eq 'sentinel-cgo' -and
        $env:MIXR_BUILD_OUTPUT_DIR -eq 'sentinel-output' -and $env:GORACE -eq 'sentinel-race') 'environment restored'
    $runs = @(Get-ChildItem (Join-Path $repoRoot 'validation-results') -Directory |
        Where-Object { $_.FullName -notin $before })
    Assert ($runs.Count -eq 1) 'one unique result directory'
    $result = Get-Content (Join-Path $runs[0].FullName 'summary.txt') -Raw
    if ($Fail) { Assert ($result -match 'Result: FAIL') 'failure summary' }
    elseif ($Plan) { Assert ($result -match 'Result: PLAN ONLY') 'plan is not a pass' }
    else { Assert ($result -match 'Result: PASS') 'pass summary' }
    return $runs[0].FullName
}

try {
    @'
@echo off
echo [%GOOS%/%GOARCH%/cgo=%CGO_ENABLED%] %*>>"%MIXR_TOOLING_TEST_TRACE%"
if "%~1"=="version" echo fake-go-for-tooling-test
if "%~1"=="test" if "%MIXR_TOOLING_TEST_FAIL%"=="1" exit /b 23
if "%~1"=="test" if "%MIXR_TOOLING_TEST_FAIL%"=="stderr" echo fake-stderr-with-success 1>&2
if "%~1"=="build" if "%~2"=="-o" echo fake-build-output>"%~3"
if "%~1"=="build" if "%~nx3"=="mixr-release.exe" if "%MIXR_TOOLING_TEST_FAIL%"=="build" exit /b 24
exit /b 0
'@ | Set-Content (Join-Path $fakeBin 'go.cmd') -Encoding ASCII
    @'
@echo off
if "%~1"=="--version" echo fake-gcc-for-tooling-test
if "%~1"=="-dumpmachine" echo %MIXR_TOOLING_TEST_TARGET%
if "%~1"=="--print-file-name" echo %MIXR_TOOLING_TEST_LIBRARY%
exit /b 0
'@ | Set-Content (Join-Path $fakeBin 'gcc.cmd') -Encoding ASCII
    $env:PATH = "$fakeBin;$env:PATH"
    Assert (@(Get-Command go -CommandType Application)[0].Source -eq (Join-Path $fakeBin 'go.cmd')) 'Go is replaced by test stub'
    $env:CC = Join-Path $fakeBin 'gcc.cmd'
    $env:MIXR_TOOLING_TEST_TRACE = Join-Path $testRoot 'trace.txt'
    $env:MIXR_TOOLING_TEST_TARGET = 'x86_64-w64-mingw32'
    $env:MIXR_TOOLING_TEST_LIBRARY = Join-Path $fakeBin 'libsynchronization.a'
    Set-Content $env:MIXR_TOOLING_TEST_LIBRARY 'fake library'
    $env:GOARCH = 'sentinel-arch'
    $env:CGO_ENABLED = 'sentinel-cgo'
    $env:MIXR_BUILD_OUTPUT_DIR = 'sentinel-output'
    $env:GORACE = 'sentinel-race'
    $env:MIXR_TOOLING_TEST_FAIL = '0'
    foreach ($mode in @('quick', 'full', 'race')) { $null = Run-Case $mode -Plan }
    Assert (-not (Test-Path $env:MIXR_TOOLING_TEST_TRACE)) 'plans never invoke Go/GCC'
    $null = Run-Case 'quick'
    $env:MIXR_TOOLING_TEST_FAIL = 'stderr'
    $stderrRun = Run-Case 'quick'
    Assert ((Get-Content "$stderrRun/02-tests-amd64.txt" -Raw) -match 'fake-stderr-with-success') 'native stderr retained without false failure'
    $env:MIXR_TOOLING_TEST_FAIL = '0'
    $kept = Run-Case 'full' -Keep
    Assert (@(Get-ChildItem "$kept/artifacts" -Recurse -Filter '*.exe').Count -eq 10) 'both architectures retained'
    $cleaned = Run-Case 'full'
    Assert (@(Get-ChildItem "$cleaned/artifacts" -Recurse -Filter '*.exe').Count -eq 0) 'own outputs cleaned'
    Assert (@(Get-ChildItem "$kept/artifacts" -Recurse -Filter '*.exe').Count -eq 10) 'earlier outputs preserved'
    $trace = Get-Content $env:MIXR_TOOLING_TEST_TRACE -Raw
    Assert ($trace -match '\[windows/386/cgo=0\] test -count=1 -buildvcs=false ./\.\.\.') '386 tests and flags'
    Assert ($trace -match '-H=windowsgui -s -w -X main.gitCommit=\w+ -X main.versionTag=\S+ -X main.buildType=release') 'release metadata and flags'
    $null = Run-Case 'race'
    $env:CC = Join-Path $fakeBin 'missing-gcc.exe'
    $null = Run-Case 'race' -Fail
    $env:CC = Join-Path $fakeBin 'gcc.cmd'
    $env:MIXR_TOOLING_TEST_TARGET = 'i686-w64-mingw32'
    $null = Run-Case 'race' -Fail
    $env:MIXR_TOOLING_TEST_TARGET = 'x86_64-w64-mingw32'
    $env:MIXR_TOOLING_TEST_LIBRARY = 'libsynchronization.a'
    $null = Run-Case 'race' -Fail
    $env:MIXR_TOOLING_TEST_LIBRARY = Join-Path $fakeBin 'libsynchronization.a'
    $env:MIXR_TOOLING_TEST_FAIL = 'build'
    $failedBuild = Run-Case 'full' -Fail
    Assert (@(Get-ChildItem "$failedBuild/artifacts" -Recurse -Filter '*.exe').Count -eq 0) 'failed build outputs cleaned'
    Assert ((Get-Content "$failedBuild/summary.txt" -Raw) -match 'exit 24') 'batch failure propagated'
    $env:MIXR_TOOLING_TEST_FAIL = '1'
    $failed = Run-Case 'full' -Fail
    Assert (-not (Test-Path "$failed/artifacts")) 'stop before builds on test failure'
    $null = Run-Case 'race' -Fail
    Write-Host "PASS: tooling orchestration only; no actual Go/GCC tests or builds. Evidence: $testRoot"
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
}
