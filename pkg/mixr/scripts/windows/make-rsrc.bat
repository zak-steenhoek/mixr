@ECHO OFF

REM Compatibility wrapper for the former resource-only entry point.
CALL "%~dp0generate-branding.bat"
EXIT /B %ERRORLEVEL%
