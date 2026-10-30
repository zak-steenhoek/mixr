@ECHO OFF

REM set repo root in relation to script path to avoid cwd dependency
SET "DEEJ_ROOT=%~dp0..\..\..\.."

IF "%GOPATH%"=="" GOTO NOGO
SET "RSRC=%GOPATH%\bin\rsrc.exe"
IF NOT EXIST "%RSRC%" GOTO INSTALL

:GENERATE
ECHO Creating architecture-specific Windows resources...
"%RSRC%" -arch 386 -manifest "%DEEJ_ROOT%\pkg\deej\assets\deej.manifest" -ico "%DEEJ_ROOT%\pkg\deej\assets\logo.ico" -o "%DEEJ_ROOT%\pkg\deej\cmd\rsrc_windows_386.syso"
IF ERRORLEVEL 1 GOTO GENERATEFAIL
"%RSRC%" -arch amd64 -manifest "%DEEJ_ROOT%\pkg\deej\assets\deej.manifest" -ico "%DEEJ_ROOT%\pkg\deej\assets\logo.ico" -o "%DEEJ_ROOT%\pkg\deej\cmd\rsrc_windows_amd64.syso"
IF ERRORLEVEL 1 GOTO GENERATEFAIL
GOTO DONE

:INSTALL
ECHO Installing rsrc v0.10.2...
go install github.com/akavel/rsrc@v0.10.2
IF ERRORLEVEL 1 GOTO GETFAIL
GOTO GENERATE

:GETFAIL
ECHO Failure installing github.com/akavel/rsrc@v0.10.2. Ensure that go and git are in PATH.
EXIT /B 1

:GENERATEFAIL
ECHO Failure generating architecture-specific Windows resources.
EXIT /B 1

:NOGO
ECHO GOPATH environment variable not set
EXIT /B 1

:DONE
