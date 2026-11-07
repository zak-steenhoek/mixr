@ECHO OFF

ECHO Building mixr (release)...

REM set repo root in relation to script path to avoid cwd dependency
SET "MIXR_ROOT=%~dp0..\..\..\.."
IF NOT DEFINED MIXR_BUILD_OUTPUT_DIR SET "MIXR_BUILD_OUTPUT_DIR=%MIXR_ROOT%"

REM shove git commit, version tag into env
for /f "delims=" %%a in ('git rev-list -1 --abbrev-commit HEAD') do @set GIT_COMMIT=%%a
for /f "delims=" %%a in ('git describe --tags --always') do @set VERSION_TAG=%%a
set BUILD_TYPE=release
ECHO Embedding build-time parameters:
ECHO - gitCommit %GIT_COMMIT%
ECHO - versionTag %VERSION_TAG%
ECHO - buildType %BUILD_TYPE%

go build -o "%MIXR_BUILD_OUTPUT_DIR%\mixr-release.exe" -ldflags "-H=windowsgui -s -w -X main.gitCommit=%GIT_COMMIT% -X main.versionTag=%VERSION_TAG% -X main.buildType=%BUILD_TYPE%" "%MIXR_ROOT%\pkg\mixr\cmd"
IF %ERRORLEVEL% NEQ 0 GOTO BUILDERROR
ECHO Done.
GOTO DONE

:BUILDERROR
ECHO Failed to build mixr in release mode! See above output for details.
EXIT /B 1

:DONE
