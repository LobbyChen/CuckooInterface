@echo off
setlocal
set ROOT=%~dp0
set BUILD=%ROOT%build\Release\binary
set STAGE=%ROOT%build\LuaKernelPackage
set ZIP=%ROOT%build\LuaKernel.zip

if not exist "%BUILD%\export.dll" (
  echo [LuaKernel] export.dll not found. Build first.
  exit /b 1
)

if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%\binary"
copy /y "%BUILD%\export.dll" "%STAGE%\binary\export.dll" >nul
copy /y "%ROOT%META-INF.json" "%STAGE%\META-INF.json" >nul

if exist "%ROOT%sdk" xcopy /e /i /y "%ROOT%sdk" "%STAGE%\sdk" >nul

if exist "%ZIP%" del /q "%ZIP%"
powershell -NoProfile -ExecutionPolicy Bypass -Command "Compress-Archive -Path '%STAGE%\*' -DestinationPath '%ZIP%' -Force"
if errorlevel 1 exit /b 1

echo [LuaKernel] Package: %ZIP%
endlocal
