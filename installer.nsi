; 本文件必须存成 UTF-8 带 BOM：否则 makensis 按系统 ANSI 页解析，中文变乱码。

Unicode true
ManifestDPIAware true
XPStyle on

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

!define APP_NAME "快速扫描"
!define APP_ID   "quick-scan"
!define APP_EXE  "quick-scan.exe"
!define APP_PUBLISHER "xiyaowong"

; 由 justfile 从 app.rc 的 PRODUCTVERSION 读出后传入。
!ifndef VERSION
  !define VERSION "0.0.0"
!endif

Name "${APP_NAME}"
OutFile "dist\${APP_ID}-${VERSION}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\${APP_ID}"
RequestExecutionLevel user
SetCompressor /SOLID lzma
SetCompressorDictSize 32
CRCCheck on
BrandingText "${APP_NAME} ${VERSION}"

VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=2052 "ProductName"     "${APP_NAME}"
VIAddVersionKey /LANG=2052 "FileDescription" "${APP_NAME} 安装程序"
VIAddVersionKey /LANG=2052 "FileVersion"     "${VERSION}"
VIAddVersionKey /LANG=2052 "ProductVersion"  "${VERSION}"
VIAddVersionKey /LANG=2052 "OriginalFilename" "${APP_ID}-${VERSION}-setup.exe"
VIAddVersionKey /LANG=2052 "LegalCopyright"  ""

!define MUI_ICON   "app.ico"
!define MUI_UNICON "app.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "立即运行${APP_NAME}"
!define MUI_FINISHPAGE_NOREBOOTSUPPORT

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"

; 按窗口标题找，安装程序带「安装」「卸载」后缀不会误中。
; 自动更新走 /S：调用方已经把正在运行的 exe 让开，这里再弹框没人点，
; 安装会永远卡住。
!macro CheckAppRunning UN
Function ${UN}CheckAppRunning
  IfSilent done
  loop:
    FindWindow $0 "" "${APP_NAME}"
    StrCmp $0 0 done
    MessageBox MB_OKCANCEL|MB_ICONEXCLAMATION "${APP_NAME} 正在运行。$\r$\n$\r$\n请先关闭它，再点「确定」继续。" IDOK loop IDCANCEL cancel
  done:
    Return
  cancel:
    Abort
FunctionEnd
!macroend

!insertmacro CheckAppRunning ""
!insertmacro CheckAppRunning "un."

Function .onInit
  SetShellVarContext current
  Call CheckAppRunning
FunctionEnd

Function un.onInit
  SetShellVarContext current
  Call un.CheckAppRunning
FunctionEnd

Section "主程序" SecApp
  SectionIn RO
  SetOutPath "$INSTDIR"
  ; 静默安装时调用方已把旧 exe 改名让开；这里再删一次兜住其它残留。
  Delete "$INSTDIR\${APP_EXE}"
  File "quick-scan.exe"
  WriteUninstaller "$INSTDIR\uninst.exe"

  ; 升级不给没要过的人凭空加一个开始菜单项。
  IfSilent skipShortcut
  CreateDirectory "$SMPROGRAMS"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
  skipShortcut:

  ; 唯一的注册表写入，卸载时整键删除。
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "DisplayName"     "${APP_NAME}"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "DisplayVersion"  "${VERSION}"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "DisplayIcon"     "$INSTDIR\${APP_EXE}"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "Publisher"       "${APP_PUBLISHER}"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "InstallLocation" "$INSTDIR"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "UninstallString" '"$INSTDIR\uninst.exe"'
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "QuietUninstallString" '"$INSTDIR\uninst.exe" /S'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}" "EstimatedSize" "$0"
SectionEnd

Section "桌面快捷方式" SecDesktop
  ; 升级不给没要过的人凭空加一个桌面图标。
  IfSilent done
  CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
  done:
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecApp}     "安装${APP_NAME}（仅当前用户，不需要管理员权限）。"
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "在桌面放一个「${APP_NAME}」快捷方式。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
  Delete "$SMPROGRAMS\${APP_NAME}.lnk"
  Delete "$DESKTOP\${APP_NAME}.lnk"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_ID}"

  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\uninst.exe"
  ; 不用 RMDir /r：目录里有别的东西就留着，不替用户做决定。
  RMDir "$INSTDIR"

  ; 运行期残留：配置文件、OCR 模型解包目录、崩溃恢复会话。
  RMDir /r "$TEMP\${APP_ID}"
  RMDir /r "$TEMP\${APP_ID}-session"
  FindFirst $0 $1 "$TEMP\${APP_ID}_*"
  ${DoWhile} $1 != ""
    RMDir /r "$TEMP\$1"
    FindNext $0 $1
  ${Loop}
  FindClose $0
SectionEnd
