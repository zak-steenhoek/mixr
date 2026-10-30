@ECHO OFF
SETLOCAL

REM Resolve the repository root and run every generator from there.
SET "MIXR_ROOT=%~dp0..\..\..\.."
PUSHD "%MIXR_ROOT%"

ECHO Generating mixr ICO and Go tray icon bytes...
go run .\tools\icon-generator
IF ERRORLEVEL 1 GOTO GENERATEFAIL

IF "%GOPATH%"=="" GOTO NOGO
SET "RSRC=%GOPATH%\bin\rsrc.exe"
IF NOT EXIST "%RSRC%" GOTO INSTALL

:RESOURCES
ECHO Generating architecture-specific Windows resources...
"%RSRC%" -arch 386 -manifest ".\pkg\mixr\assets\mixr.manifest" -ico ".\pkg\mixr\assets\mixr.ico" -o ".\pkg\mixr\cmd\rsrc_windows_386.syso"
IF ERRORLEVEL 1 GOTO GENERATEFAIL
"%RSRC%" -arch amd64 -manifest ".\pkg\mixr\assets\mixr.manifest" -ico ".\pkg\mixr\assets\mixr.ico" -o ".\pkg\mixr\cmd\rsrc_windows_amd64.syso"
IF ERRORLEVEL 1 GOTO GENERATEFAIL
GOTO DONE

:INSTALL
ECHO Installing rsrc v0.10.2...
go install github.com/akavel/rsrc@v0.10.2
IF ERRORLEVEL 1 GOTO INSTALLFAIL
GOTO RESOURCES

:NOGO
ECHO GOPATH environment variable not set.
GOTO FAIL

:INSTALLFAIL
ECHO Failed to install rsrc v0.10.2. Ensure Go and Git are in PATH.
GOTO FAIL

:GENERATEFAIL
ECHO Failed to generate mixr branding resources.
GOTO FAIL

:DONE
POPD
ENDLOCAL
EXIT /B 0

:FAIL
POPD
ENDLOCAL
EXIT /B 1
