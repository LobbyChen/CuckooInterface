@echo off
REM ============================================================
REM  CuckooInterface Auto-Build Script
REM  Builds: Go core, WPF UI, PythonKernel, LuaKernel
REM  Output: bin/ directory with all binaries + Kernel packages
REM ============================================================
setlocal EnableDelayedExpansion

cd /d "%~dp0"
set "ROOT=%cd%"
set "BIN=%ROOT%\bin"

echo ============================================================
echo  CuckooInterface Build Script
echo  Root: %ROOT%
echo  Output: %BIN%
echo ============================================================
echo.

REM ------------------------------------------------------------
REM  Step 0: Check build tools
REM ------------------------------------------------------------
echo [0/7] Checking build tools...

where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go compiler not found in PATH.
    exit /b 1
)

where dotnet >nul 2>&1
if errorlevel 1 (
    echo [ERROR] .NET SDK not found in PATH.
    exit /b 1
)

where cmake >nul 2>&1
if errorlevel 1 (
    echo [ERROR] CMake not found in PATH.
    exit /b 1
)

where python >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Python not found in PATH.
    exit /b 1
)

echo       Go:       OK
echo       .NET:     OK
echo       CMake:    OK
echo       Python:   OK
echo.

REM ------------------------------------------------------------
REM  Step 1: Clean old bin directory
REM ------------------------------------------------------------
echo [1/7] Cleaning old bin directory...
if exist "%BIN%" (
    rmdir /s /q "%BIN%"
)
mkdir "%BIN%"
mkdir "%BIN%\user"
mkdir "%BIN%\user\Kernel"
echo       bin/ cleaned and created.
echo.

REM ------------------------------------------------------------
REM  Step 2: Build Go main program
REM ------------------------------------------------------------
echo [2/7] Building Go main program (CuckooInterface.exe)...
go build -ldflags="-H windowsgui -s -w" -o "%BIN%\CuckooInterface.exe" .
if errorlevel 1 (
    echo [ERROR] Go build failed.
    exit /b 1
)
echo       CuckooInterface.exe built.
echo.

REM ------------------------------------------------------------
REM  Step 3: Build WPF UI
REM ------------------------------------------------------------
echo [3/7] Building WPF UI (CuckooInterfaceUI)...
dotnet publish "%ROOT%\CuckooInterfaceUI\CuckooInterfaceUI.csproj" -c Release -r win-x64 --self-contained false -o "%BIN%"
if errorlevel 1 (
    echo [ERROR] UI build failed.
    exit /b 1
)
echo       CuckooInterfaceUI published.
echo.

REM ------------------------------------------------------------
REM  Step 4: Build PythonKernel
REM ------------------------------------------------------------
echo [4/7] Building PythonKernel...
cd /d "%ROOT%\implement\Kernel\PythonKernel"

if exist "build" (
    rmdir /s /q "build"
)

cmake -B build -S . -G "MinGW Makefiles"
if errorlevel 1 (
    echo       MinGW generator failed, trying default...
    cmake -B build -S .
    if errorlevel 1 (
        echo [ERROR] CMake configure failed.
        cd /d "%ROOT%"
        exit /b 1
    )
)

cmake --build build --config Release
if errorlevel 1 (
    echo [ERROR] PythonKernel build failed.
    cd /d "%ROOT%"
    exit /b 1
)

if not exist "build\export.dll" (
    echo [ERROR] PythonKernel export.dll not found: "%ROOT%\implement\Kernel\PythonKernel\build\export.dll"
    cd /d "%ROOT%"
    exit /b 1
)

echo       PythonKernel (export.dll) built.
echo.

REM ------------------------------------------------------------
REM  Step 5: Package PythonKernel into zip
REM ------------------------------------------------------------
echo [5/7] Packaging PythonKernel.zip...

set "PKG=%TEMP%\CuckooPythonKernelPkg"
if exist "%PKG%" rmdir /s /q "%PKG%"
mkdir "%PKG%"
mkdir "%PKG%\binary"
mkdir "%PKG%\sdk"
mkdir "%PKG%\sdk\cuckoo_sdk"

copy /y "META-INF.json" "%PKG%\" >nul
if errorlevel 1 (
    echo [ERROR] Failed to copy PythonKernel META-INF.json.
    cd /d "%ROOT%"
    exit /b 1
)

copy /y "build\export.dll" "%PKG%\binary\" >nul
if errorlevel 1 (
    echo [ERROR] Failed to copy PythonKernel export.dll.
    cd /d "%ROOT%"
    exit /b 1
)

copy /y "sdk\cuckoo_sdk\__init__.py" "%PKG%\sdk\cuckoo_sdk\" >nul
if errorlevel 1 (
    echo [ERROR] Failed to copy PythonKernel SDK.
    cd /d "%ROOT%"
    exit /b 1
)

copy /y "sdk\cuckoo_sdk\sdk.md" "%PKG%\sdk\cuckoo_sdk\" >nul
if errorlevel 1 (
    echo [ERROR] Failed to copy PythonKernel SDK docs.
    cd /d "%ROOT%"
    exit /b 1
)

copy /y "sdk\install.bat" "%PKG%\sdk\" >nul
if errorlevel 1 (
    echo [ERROR] Failed to copy PythonKernel SDK installer.
    cd /d "%ROOT%"
    exit /b 1
)

powershell -NoProfile -Command "Compress-Archive -Path '%PKG%\*' -DestinationPath '%BIN%\user\Kernel\PythonKernel.zip' -Force"
if errorlevel 1 (
    echo [ERROR] Failed to create PythonKernel.zip.
    cd /d "%ROOT%"
    exit /b 1
)

rmdir /s /q "%PKG%"
echo       PythonKernel.zip created.
echo.

REM ------------------------------------------------------------
REM  Step 6: Build and package LuaKernel
REM ------------------------------------------------------------
echo [6/7] Building and packaging LuaKernel...
cd /d "%ROOT%\implement\Kernel\LuaKernel"

if not exist "build_lua_kernel.bat" (
    echo [ERROR] LuaKernel build script not found.
    echo         Expected: "%ROOT%\implement\Kernel\LuaKernel\build_lua_kernel.bat"
    cd /d "%ROOT%"
    exit /b 1
)

if not exist "META-INF.json" (
    echo [ERROR] LuaKernel META-INF.json not found.
    cd /d "%ROOT%"
    exit /b 1
)

call "build_lua_kernel.bat"
if errorlevel 1 (
    echo [ERROR] LuaKernel build failed.
    cd /d "%ROOT%"
    exit /b 1
)

REM LuaKernel CMake build outputs:
REM   build\Release\binary\export.dll
REM Be tolerant of a non-config subdirectory output as well.
set "LUA_DLL="
if exist "build\Release\binary\export.dll" (
    set "LUA_DLL=build\Release\binary\export.dll"
) else if exist "build\binary\export.dll" (
    set "LUA_DLL=build\binary\export.dll"
) else if exist "build\export.dll" (
    set "LUA_DLL=build\export.dll"
)

if not defined LUA_DLL (
    echo [ERROR] LuaKernel export.dll not found after build.
    echo         Checked:
    echo           build\Release\binary\export.dll
    echo           build\binary\export.dll
    echo           build\export.dll
    cd /d "%ROOT%"
    exit /b 1
)

set "LUAPKG=%TEMP%\CuckooLuaKernelPkg"
if exist "%LUAPKG%" rmdir /s /q "%LUAPKG%"
mkdir "%LUAPKG%"
mkdir "%LUAPKG%\binary"

copy /y "META-INF.json" "%LUAPKG%\" >nul
if errorlevel 1 (
    echo [ERROR] Failed to copy LuaKernel META-INF.json.
    rmdir /s /q "%LUAPKG%" >nul 2>&1
    cd /d "%ROOT%"
    exit /b 1
)

copy /y "%LUA_DLL%" "%LUAPKG%\binary\export.dll" >nul
if errorlevel 1 (
    echo [ERROR] Failed to copy LuaKernel export.dll.
    rmdir /s /q "%LUAPKG%" >nul 2>&1
    cd /d "%ROOT%"
    exit /b 1
)

powershell -NoProfile -Command "Compress-Archive -Path '%LUAPKG%\*' -DestinationPath '%BIN%\user\Kernel\LuaKernel.zip' -Force"
if errorlevel 1 (
    echo [ERROR] Failed to create LuaKernel.zip.
    rmdir /s /q "%LUAPKG%" >nul 2>&1
    cd /d "%ROOT%"
    exit /b 1
)

rmdir /s /q "%LUAPKG%"
echo       LuaKernel (export.dll) built.
echo       LuaKernel.zip created.
echo.

cd /d "%ROOT%"

REM ------------------------------------------------------------
REM  Step 7: Done
REM ------------------------------------------------------------
echo [7/7] Build complete!
echo.
echo ============================================================
echo  Output directory: %BIN%
echo.
echo  Contents:
dir /b "%BIN%"
echo.
echo  user\Kernel\:
dir /b "%BIN%\user\Kernel"
echo ============================================================
echo.
echo  Usage:
echo    1. Run bin\CuckooInterface.exe to start the daemon
echo    2. Right-click tray icon -^> Open UI
echo    3. Place plugin zips in bin\user\Base\ or bin\user\Active\
echo.

endlocal
