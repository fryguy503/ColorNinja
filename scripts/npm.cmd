@echo off
rem Let portable Wails commands use the project-pinned npm on Windows.
node "%~dp0..\.tools\npm\package\bin\npm-cli.js" %*
exit /b %errorlevel%
