@echo off
REM build.bat —— PythonKernel 一键构建（Windows / MSVC）
REM 产物：build\Release\export.dll
REM
REM 前置条件：
REM   1. 已安装 Visual Studio 并配置好 MSVC 环境
REM   2. 已安装 Python 3.x（含开发头文件与库），且 python 在 PATH 中

setlocal

cd /d "%~dp0"

echo [1/3] Configuring CMake...
cmake -B build -S . -A x64
if errorlevel 1 (
    echo CMake configure failed.
    exit /b 1
)

echo [2/3] Building Release...
cmake --build build --config Release
if errorlevel 1 (
    echo Build failed.
    exit /b 1
)

echo [3/3] Done. Output: build\Release\export.dll
echo.
echo 打包方式：
echo   1. 创建打包目录结构：
echo      dist\META-INF.json
echo      dist\binary\export.dll
echo      dist\sdk\cuckoo_sdk\__init__.py
echo   2. 将 dist 内容压缩为 PythonKernel.zip
echo   3. 放入 CuckooInterface 的 user\Kernel\ 目录

endlocal
