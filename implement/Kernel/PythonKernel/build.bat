@echo off
REM Copyright (C) 2026 LobbyChen
REM SPDX-License-Identifier: AGPL-3.0-only
REM build.bat —— PythonKernel 一键构建（Windows）
REM 产物：build\Release\export.dll 或 build\export.dll
REM
REM 前置条件：
REM   1. 已安装 Visual Studio（MSVC）或 MinGW g++ 之一
REM   2. 已安装 CMake
REM
REM PythonKernel 不再链接 Python 导入库，构建机无需安装 Python。
REM 运行时会发现并使用目标机器上安装的 CPython（64 位 3.8+）。

setlocal

cd /d "%~dp0"

echo [1/3] Configuring CMake...
cmake -B build -S . -A x64
if errorlevel 1 (
    echo Default generator failed, trying Ninja/MinGW...
    cmake -B build -S . -G Ninja -DCMAKE_BUILD_TYPE=Release
    if errorlevel 1 (
        echo CMake configure failed.
        exit /b 1
    )
)

echo [2/3] Building Release...
cmake --build build --config Release
if errorlevel 1 (
    echo Build failed.
    exit /b 1
)

echo [3/3] Done.
if exist "build\Release\export.dll" (
    echo Output: build\Release\export.dll
) else if exist "build\export.dll" (
    echo Output: build\export.dll
) else if exist "build\Release\binary\export.dll" (
    echo Output: build\Release\binary\export.dll
)

echo.
echo 打包方式：
echo   1. dist\META-INF.json
echo      dist\binary\export.dll
echo      dist\sdk\install.bat
echo      dist\sdk\cuckoo_sdk\__init__.py
echo      dist\sdk\cuckoo_sdk\sdk.md
echo   2. 将 dist 内容压缩为 PythonKernel.zip
echo   3. 放入 CuckooInterface 的 user\Kernel\ 目录
echo.
echo 注意：目标机器需安装 64 位 CPython 3.8+，或在运行时页面选择 Python。

endlocal
