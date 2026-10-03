@echo off
REM ============================================================
REM  CuckooInterface Python SDK Installer
REM  Installs cuckoo_sdk package to Python's site-packages
REM  so it can be imported during local plugin development.
REM ============================================================
setlocal EnableDelayedExpansion

cd /d "%~dp0"
set "SDK_SRC=%cd%\cuckoo_sdk"

echo ============================================================
echo  CuckooInterface Python SDK Installer
echo  SDK source: %SDK_SRC%
echo ============================================================
echo.

REM ------------------------------------------------------------
REM  Check Python
REM ------------------------------------------------------------
where python >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Python not found in PATH.
    echo         Please install Python 3.x and add it to PATH,
    echo         or run this script from the desired Python environment.
    exit /b 1
)

REM ------------------------------------------------------------
REM  Show detected Python version and executable path
REM ------------------------------------------------------------
echo Detected Python:
for /f "delims=" %%v in ('python -c "import sys; print(sys.version.split()[0])"') do set "PY_VER=%%v"
for /f "delims=" %%p in ('python -c "import sys; print(sys.executable)"') do set "PY_EXE=%%p"
echo       Version: %PY_VER%
echo       Executable: %PY_EXE%
echo.
if "%PY_VER%"=="" (
    echo [ERROR] Failed to detect Python version.
    exit /b 1
)
echo NOTE: cuckoo_sdk is pure Python and works with any Python 3.x.
echo       The embedded runtime version is determined by how PythonKernel was built.
echo.

REM ------------------------------------------------------------
REM  Check SDK source exists
REM ------------------------------------------------------------
if not exist "%SDK_SRC%\__init__.py" (
    echo [ERROR] SDK source not found at: %SDK_SRC%\__init__.py
    exit /b 1
)

REM ------------------------------------------------------------
REM  Determine install target
REM  Usage: install.bat [user^|system]
REM    user   - install to user site-packages (default, no admin)
REM    system - install to system site-packages (may need admin)
REM ------------------------------------------------------------
set "INSTALL_SCOPE=user"
if /i "%~1"=="system" set "INSTALL_SCOPE=system"
if /i "%~1"=="user"   set "INSTALL_SCOPE=user"

echo Install scope: %INSTALL_SCOPE%
echo.

REM ------------------------------------------------------------
REM  Get site-packages path from Python
REM ------------------------------------------------------------
echo [1/4] Resolving Python site-packages path...

if /i "%INSTALL_SCOPE%"=="system" (
    for /f "delims=" %%i in ('python -c "import site; print(site.getsitepackages()[1] if len(site.getsitepackages())^>1 else site.getsitepackages()[0])"') do set "TARGET=%%i"
) else (
    for /f "delims=" %%i in ('python -c "import site; print(site.getusersitepackages())"') do set "TARGET=%%i"
)

if "%TARGET%"=="" (
    echo [ERROR] Failed to resolve site-packages path.
    exit /b 1
)

echo       Target: %TARGET%
echo.

REM ------------------------------------------------------------
REM  Create target directory if needed
REM ------------------------------------------------------------
echo [2/4] Preparing target directory...
if not exist "%TARGET%" (
    mkdir "%TARGET%"
)
echo       Ready.
echo.

REM ------------------------------------------------------------
REM  Copy SDK package
REM ------------------------------------------------------------
echo [3/4] Installing cuckoo_sdk...

set "DEST=%TARGET%\cuckoo_sdk"
if exist "%DEST%" (
    echo       Removing existing cuckoo_sdk...
    rmdir /s /q "%DEST%"
)

xcopy "%SDK_SRC%" "%DEST%\" /E /I /Q /Y >nul
if errorlevel 1 (
    echo [ERROR] Failed to copy SDK files.
    exit /b 1
)

echo       Installed to: %DEST%
echo.

REM ------------------------------------------------------------
REM  Verify installation (run from temp dir to avoid source dir shadowing)
REM ------------------------------------------------------------
echo [4/4] Verifying installation...
pushd "%TEMP%"
python -c "import cuckoo_sdk; print('cuckoo_sdk imported successfully from:', cuckoo_sdk.__file__)"
set "VERIFY_RC=%errorlevel%"
popd
if not "%VERIFY_RC%"=="0" (
    echo [ERROR] Verification failed - cannot import cuckoo_sdk.
    exit /b 1
)

echo.
echo ============================================================
echo  SDK installed successfully!
echo.
echo  You can now import it in your plugins:
echo.
echo    from cuckoo_sdk import on_event, emit_event, log_info
echo.
echo  To uninstall: remove "%DEST%"
echo ============================================================
echo.

endlocal