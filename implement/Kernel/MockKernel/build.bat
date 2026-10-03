@echo off
REM build.bat —— MockKernel 一键构建（Windows / MSVC）
REM 产物：build\Release\export.dll
REM
REM 前置条件：已安装 Visual Studio 并配置好 MSVC 环境，或从
REM "x64 Native Tools Command Prompt for VS" 中运行本脚本。

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
echo 打包方式：将 export.dll 放入 binary\ 目录，与 META-INF.json 一起压缩为
echo MockKernel.zip，放入 CuckooInterface 的 user\Kernel\ 目录即可加载。

endlocal
