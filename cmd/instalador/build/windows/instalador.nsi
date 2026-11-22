Unicode true
!include "MUI2.nsh"

Name "Monitor de cámaras — Instalador"
OutFile "${SALIDA}"
InstallDir "$LOCALAPPDATA\Programs\MonitorCamaras"
RequestExecutionLevel user

!define MUI_FINISHPAGE_RUN "$INSTDIR\MonitorCamaras-Instalador.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Abrir el asistente de instalación"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Spanish"

Section "Asistente"
  SetOutPath "$INSTDIR"
  File "/oname=MonitorCamaras-Instalador.exe" "${PORTABLE}"
  WriteUninstaller "$INSTDIR\Desinstalar.exe"
  CreateShortcut "$SMPROGRAMS\Monitor de cámaras - Instalador.lnk" "$INSTDIR\MonitorCamaras-Instalador.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\MonitorCamaras" "DisplayName" "Monitor de cámaras — Instalador"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\MonitorCamaras" "UninstallString" '$\"$INSTDIR\Desinstalar.exe$\"'
SectionEnd

Section "Uninstall"
  Delete "$SMPROGRAMS\Monitor de cámaras - Instalador.lnk"
  Delete "$INSTDIR\MonitorCamaras-Instalador.exe"
  Delete "$INSTDIR\Desinstalar.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\MonitorCamaras"
SectionEnd
