; ==============================================================================
; ESTIRAHA NEXT — NSIS WINDOWS INSTALLER (Phase 1A Desktop PoC)
; Target: Windows 10/11 x64 (.exe installer)
; Principles:
;   - Supports both interactive GUI install and silent CI install (/S /D=...)
;   - Never touches user media directories during install or uninstall
;   - Leaves zero orphan background processes on uninstall
; ==============================================================================

!define APP_NAME "Estiraha Next"
!define APP_PUBLISHER "Estiraha Engineering"
!define APP_VERSION "0.2.0-phase-b"
!define APP_EXE "EstirahaNext.exe"

Name "${APP_NAME} ${APP_VERSION}"
OutFile "..\..\..\dist\EstirahaNext-Setup-${APP_VERSION}-x64.exe"
InstallDir "$LOCALAPPDATA\Programs\EstirahaNext"
RequestExecutionLevel user
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
