@echo off
setlocal

set ROOT=%~dp0
set VENDOR=%ROOT%vendor
set ARCHIVE=%TEMP%\lua-5.5.1.tar.gz
set URL=https://www.lua.org/ftp/lua-5.5.1.tar.gz
set EXPECTED=1c4b4068d67061f2a2231ad2b5422e77acea1487ea9890f6320af614f4373dce

if exist "%VENDOR%\lua-5.5.1\src\lapi.c" (
  echo [LuaKernel] Lua 5.5.1 already vendored.
  exit /b 0
)

if not exist "%VENDOR%" mkdir "%VENDOR%"

powershell -NoProfile -ExecutionPolicy Bypass -Command "Invoke-WebRequest -Uri '%URL%' -OutFile '%ARCHIVE%'"
if errorlevel 1 exit /b 1

for /f %%H in ('powershell -NoProfile -Command "(Get-FileHash -Algorithm SHA256 '%ARCHIVE%').Hash.ToLower()"') do set ACTUAL=%%H

echo Expected: %EXPECTED%
echo Actual:   %ACTUAL%
if /I not "%ACTUAL%"=="%EXPECTED%" (
  echo [LuaKernel] SHA256 mismatch.
  del /q "%ARCHIVE%" >nul 2>nul
  exit /b 1
)

powershell -NoProfile -ExecutionPolicy Bypass -Command "tar -xzf '%ARCHIVE%' -C '%VENDOR%'"
if errorlevel 1 exit /b 1

del /q "%ARCHIVE%" >nul 2>nul

echo [LuaKernel] Lua 5.5.1 vendored to %VENDOR%\lua-5.5.1
endlocal
