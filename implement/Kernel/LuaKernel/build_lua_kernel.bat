@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "ROOT=%~dp0"
set "BUILD=%ROOT%build"
set "VENDOR=%ROOT%vendor\lua-5.5.1"

if not exist "%VENDOR%\src\lapi.c" (
  echo [LuaKernel] Lua 5.5.1 source not found. Vendoring...
  call "%ROOT%vendor_lua.bat"
  if errorlevel 1 exit /b 1
)

where cmake >nul 2>&1
if errorlevel 1 (
  echo [LuaKernel] ERROR: cmake was not found in PATH.
  exit /b 1
)

rem ------------------------------------------------------------
rem Prepare a C/C++ compiler environment.
rem Ninja itself is only a build generator; it is NOT a compiler.
rem ------------------------------------------------------------
where cl >nul 2>&1
if errorlevel 1 (
  set "VSWHERE=%ProgramFiles(x86)%\Microsoft Visual Studio\Installer\vswhere.exe"
  if not exist "!VSWHERE!" set "VSWHERE=%ProgramFiles%\Microsoft Visual Studio\Installer\vswhere.exe"

  if exist "!VSWHERE!" (
    for /f "usebackq delims=" %%V in (`"!VSWHERE!" -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath`) do set "VSINSTALL=%%V"
  )

  if defined VSINSTALL (
    echo [LuaKernel] Found Visual Studio: !VSINSTALL!
    call "!VSINSTALL!\Common7\Tools\VsDevCmd.bat" -arch=x64 -host_arch=x64 >nul
  )
)

where cl >nul 2>&1
if errorlevel 1 (
  echo.
  echo [LuaKernel] ERROR: No usable C/C++ compiler was found.
  echo.
  echo Install Visual Studio Build Tools with:
  echo   Desktop development with C++
  echo   MSVC C++ build tools
  echo   Windows SDK
  echo.
  echo Then run this script from a normal PowerShell again.
  echo A Developer PowerShell for VS also works.
  exit /b 1
)

echo [LuaKernel] C compiler:
where cl

rem ------------------------------------------------------------
rem Do not pass -A to Ninja. Also clear a previous generator cache
rem so an earlier failed Ninja/VS configure cannot poison this build.
rem ------------------------------------------------------------
if exist "%BUILD%\CMakeCache.txt" (
  echo [LuaKernel] Removing previous CMake cache...
  rmdir /s /q "%BUILD%" >nul 2>&1
)

pushd "%ROOT%"

where ninja >nul 2>&1
if not errorlevel 1 (
  echo [LuaKernel] Configuring with Ninja + MSVC...
  cmake -S . -B build -G Ninja
) else (
  echo [LuaKernel] Ninja not found; using CMake default generator...
  cmake -S . -B build
)

if errorlevel 1 (
  echo [LuaKernel] CMake configure failed.
  popd
  exit /b 1
)

cmake --build build --config Release --parallel
if errorlevel 1 (
  echo [LuaKernel] Build failed.
  popd
  exit /b 1
)
popd

echo [LuaKernel] Build complete.
if exist "%BUILD%\Release\binary\export.dll" (
  echo [LuaKernel] Output: %BUILD%\Release\binary\export.dll
) else if exist "%BUILD%\binary\export.dll" (
  echo [LuaKernel] Output: %BUILD%\binary\export.dll
)
endlocal
