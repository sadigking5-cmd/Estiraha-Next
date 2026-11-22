; ==============================================================================
; ESTIRAHA NEXT — NSIS WINDOWS INSTALLER TEMPLATE (Phase 0 Foundation)
; Target: Windows 10/11 x64 (.exe installer)
; Principles:
;   - Never touches user media directories during install/uninstall
;   - Stores application binaries in $PROGRAMFILES64\Estiraha Next
;   - Stores local database & cache in $LOCALAPPDATA\EstirahaNext
; ==============================================================================

!define APP_NAME "Estiraha Next"
!define APP_PUBLISHER "Estiraha Engineering"
!define APP_VERSION "0.1.0"
!define APP_EXE "EstirahaNext.exe"

Name "${APP_NAME} ${APP_VERSION}"
OutFile "..\..\..\dist\EstirahaNext-Setup-${APP_VERSION}-x64.exe"
InstallDir "$PROGRAMFILES64\${APP_NAME}"
RequestExecutionLevel admin
Unicode True

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  SetOutPath "$INSTDIR"
File "..\..\..\dist\windows-amd64\${APP_EXE}"
  WriteUninstaller "$INSTDIR\Uninstall.exe"

  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
  CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
SectionEnd

Section "Uninstall"
  ; SAFETY RULE: Uninstall removes ONLY program binaries and shortcuts.
  ; User SQLite database, backups, and media files are NEVER auto-deleted.
  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"
  Delete "$DESKTOP\${APP_NAME}.lnk"
SectionEnd
