@echo off
REM Copyright (C) 2026 LobbyChen
REM SPDX-License-Identifier: AGPL-3.0-only
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
rem Prefer MSVC when available; otherwise fall back to MinGW GCC.
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
  where gcc >nul 2>&1
  if errorlevel 1 (
    echo.
    echo [LuaKernel] ERROR: No usable C/C++ compiler was found.
    echo.
    echo Install Visual Studio Build Tools with "Desktop development with C++"
    echo or MinGW-w64 with gcc/g++ in PATH.
    exit /b 1
  )

  echo [LuaKernel] MSVC not found; using MinGW GCC.
  if exist "%BUILD%\CMakeCache.txt" rmdir /s /q "%BUILD%" >nul 2>&1
  pushd "%ROOT%"
  where ninja >nul 2>&1
  if not errorlevel 1 (
    cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Release -DCMAKE_C_COMPILER=gcc -DCMAKE_CXX_COMPILER=g++
  ) else (
    cmake -S . -B build -G "MinGW Makefiles" -DCMAKE_BUILD_TYPE=Release
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
  goto :done
)

echo [LuaKernel] Using MSVC:
where cl

rem ------------------------------------------------------------
rem Clear a previous generator cache so an earlier failed
rem configure cannot poison this build.
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

:done
echo [LuaKernel] Build complete.
if exist "%BUILD%\Release\binary\export.dll" (
  echo [LuaKernel] Output: %BUILD%\Release\binary\export.dll
) else if exist "%BUILD%\binary\export.dll" (
  echo [LuaKernel] Output: %BUILD%\binary\export.dll
)
endlocal
