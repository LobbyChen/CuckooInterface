; Copyright (C) 2026 LobbyChen
; SPDX-License-Identifier: AGPL-3.0-only
;
; CuckooInterface NSIS installer script.
; Build with NSIS 3:
;   makensis installer.nsi
; Optional version injection:
;   makensis /DPRODUCT_VERSION=0.1.0 /DPRODUCT_FILENAME_TAG=B0.1.0 installer.nsi

Unicode true
SetCompressor /SOLID lzma
RequestExecutionLevel user

!define PRODUCT_NAME "CuckooInterface"
!define PRODUCT_EXE "CuckooInterface.exe"
!define PRODUCT_UI_EXE "CuckooInterfaceUI.exe"

; 版本号由 CI 通过 /DPRODUCT_VERSION 注入；本地直接 makensis 时使用兜底值
!ifndef PRODUCT_VERSION
  !define PRODUCT_VERSION "0.0.0-dev"
!endif
; 输出文件名中的版本片段（如 B0.7.1）；未注入时使用 "dev"
!ifndef PRODUCT_FILENAME_TAG
  !define PRODUCT_FILENAME_TAG "dev"
!endif

!define PRODUCT_PUBLISHER "LobbyChen"
!define PRODUCT_DIR_REGKEY "Software\Microsoft\Windows\CurrentVersion\App Paths\${PRODUCT_EXE}"
!define PRODUCT_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"
!define PRODUCT_UNINST_ROOT_KEY "HKCU"
!define PRODUCT_STARTMENU_REGVAL "NSIS:StartMenuDir"

; MUI 2 ------
!include "MUI2.nsh"

; MUI Settings
!define MUI_ABORTWARNING
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "${NSISDIR}\Contrib\Graphics\Icons\modern-uninstall.ico"

; Welcome page
!insertmacro MUI_PAGE_WELCOME
; License page
!define MUI_LICENSEPAGE_CHECKBOX
!insertmacro MUI_PAGE_LICENSE "..\LICENSE.txt"
; Directory page
!insertmacro MUI_PAGE_DIRECTORY
; Start menu page
var ICONS_GROUP
!define MUI_STARTMENUPAGE_NODISABLE
!define MUI_STARTMENUPAGE_DEFAULTFOLDER "CuckooInterface"
!define MUI_STARTMENUPAGE_REGISTRY_ROOT "${PRODUCT_UNINST_ROOT_KEY}"
!define MUI_STARTMENUPAGE_REGISTRY_KEY "${PRODUCT_UNINST_KEY}"
!define MUI_STARTMENUPAGE_REGISTRY_VALUENAME "${PRODUCT_STARTMENU_REGVAL}"
!insertmacro MUI_PAGE_STARTMENU Application $ICONS_GROUP
; Instfiles page
!insertmacro MUI_PAGE_INSTFILES
; Finish page
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "启动 CuckooInterface"
!insertmacro MUI_PAGE_FINISH

; Uninstaller pages
!insertmacro MUI_UNPAGE_INSTFILES

; Language files
!insertmacro MUI_LANGUAGE "SimpChinese"

; MUI end ------

Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile "CuckooInterface_${PRODUCT_FILENAME_TAG}_Setup_amd64.exe"
InstallDir "$LOCALAPPDATA\Programs\CuckooInterface"
InstallDirRegKey ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "InstallDir"
ShowInstDetails show
ShowUnInstDetails show

Section "MainSection" SEC01
  SetShellVarContext current

  SetOutPath "$INSTDIR"
  SetOverwrite ifnewer
  File "..\LICENSE.txt"
  SetOverwrite on

  ; Go Core / Daemon 与 WPF UI 主程序
  File "..\bin\${PRODUCT_EXE}"
  File "..\bin\${PRODUCT_UI_EXE}"

  ; UI 托管依赖（WPF-UI、WebView2 托管库等）
  File "..\bin\*.dll"
  File "..\bin\*.json"

  ; WebView2 原生加载器与其他 runtimes 资源
  SetOutPath "$INSTDIR\runtimes"
  File /r "..\bin\runtimes\*.*"

  ; 随包分发的 Kernel 插件（PythonKernel / LuaKernel 等）
  SetOutPath "$INSTDIR\user\Kernel"
  File "..\bin\user\Kernel\*.zip"

  ; 运行期目录
  CreateDirectory "$INSTDIR\user"
  CreateDirectory "$INSTDIR\user\Base"
  CreateDirectory "$INSTDIR\user\Active"
  CreateDirectory "$INSTDIR\user\Kernel"
  CreateDirectory "$INSTDIR\configs"
  CreateDirectory "$INSTDIR\logs"
  CreateDirectory "$INSTDIR\plugins"

  ; Shortcuts
  !insertmacro MUI_STARTMENU_WRITE_BEGIN Application
  CreateDirectory "$SMPROGRAMS\$ICONS_GROUP"
  CreateShortCut "$SMPROGRAMS\$ICONS_GROUP\CuckooInterface.lnk" "$INSTDIR\${PRODUCT_EXE}"
  CreateShortCut "$DESKTOP\CuckooInterface.lnk" "$INSTDIR\${PRODUCT_EXE}"
  !insertmacro MUI_STARTMENU_WRITE_END
SectionEnd

Section -AdditionalIcons
  !insertmacro MUI_STARTMENU_WRITE_BEGIN Application
  CreateShortCut "$SMPROGRAMS\$ICONS_GROUP\Uninstall.lnk" "$INSTDIR\uninst.exe"
  !insertmacro MUI_STARTMENU_WRITE_END
SectionEnd

Section -Post
  WriteUninstaller "$INSTDIR\uninst.exe"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_DIR_REGKEY}" "" "$INSTDIR\${PRODUCT_EXE}"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "UninstallString" '"$INSTDIR\uninst.exe"'
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninst.exe" /S'
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXE}"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "Publisher" "${PRODUCT_PUBLISHER}"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "InstallDir" "$INSTDIR"
  WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "NoModify" 1
  WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "NoRepair" 1
SectionEnd


Function un.onUninstSuccess
  HideWindow
  MessageBox MB_ICONINFORMATION|MB_OK "$(^Name) 已成功从您的计算机中移除。"
FunctionEnd

Function un.onInit
  MessageBox MB_ICONQUESTION|MB_YESNO|MB_DEFBUTTON2 "您确实要完全移除 $(^Name) 及其所有组件吗？" IDYES +2
  Abort
FunctionEnd

Section Uninstall
  SetShellVarContext current

  ; 先停止 UI 与 Core/Daemon，避免运行中的进程占用文件导致卸载失败。
  nsExec::Exec 'taskkill /F /IM ${PRODUCT_UI_EXE}'
  nsExec::Exec 'taskkill /F /IM ${PRODUCT_EXE}'

  !insertmacro MUI_STARTMENU_GETFOLDER "Application" $ICONS_GROUP

  Delete "$INSTDIR\uninst.exe"
  Delete "$INSTDIR\${PRODUCT_EXE}"
  Delete "$INSTDIR\${PRODUCT_UI_EXE}"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\*.dll"
  Delete "$INSTDIR\*.json"
  RMDir /r "$INSTDIR\runtimes"

  ; 仅移除安装包附带的 Kernel；保留用户自行添加的插件、配置与日志。
  Delete "$INSTDIR\user\Kernel\*.zip"

  Delete "$DESKTOP\CuckooInterface.lnk"
  Delete "$SMPROGRAMS\$ICONS_GROUP\CuckooInterface.lnk"
  Delete "$SMPROGRAMS\$ICONS_GROUP\Uninstall.lnk"

  RMDir "$SMPROGRAMS\$ICONS_GROUP"
  RMDir "$INSTDIR\user\Kernel"
  RMDir "$INSTDIR\user\Base"
  RMDir "$INSTDIR\user\Active"
  RMDir "$INSTDIR\user"
  RMDir "$INSTDIR\configs"
  RMDir "$INSTDIR\logs"
  RMDir "$INSTDIR\plugins"
  RMDir "$INSTDIR"

  DeleteRegKey ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}"
  DeleteRegKey ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_DIR_REGKEY}"
  SetAutoClose true
SectionEnd
