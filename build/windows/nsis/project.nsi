Unicode true

####
## PRism installer (release skill §Windows): per-user install into
## %LOCALAPPDATA%\Programs\PRism without UAC, WebView2 check, Start-menu
## shortcut, optional "Start PRism when I sign in", clean uninstall.
##
## Built by `wails3 task windows:package` (makensis). For manual runs:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\..\bin\prism-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\..\bin\prism-arm64.exe -DINFO_PRODUCTVERSION=0.1.0 project.nsi
####
!define INFO_PROJECTNAME "prism"
!define INFO_COMPANYNAME "PRism"
!define INFO_PRODUCTNAME "PRism"
!ifndef INFO_PRODUCTVERSION
    !define INFO_PRODUCTVERSION "0.0.0"
!endif
!define INFO_COPYRIGHT "(c) 2026 PRism contributors"
!define PRODUCT_EXECUTABLE "prism.exe"
!define UNINST_KEY_NAME "PRism"
!define WAILS_INSTALL_SCOPE "user"
!define REQUEST_EXECUTION_LEVEL "user"

!include "wails_tools.nsh"
!include "LogicLib.nsh"

# Windows needs four numeric parts here.
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Start PRism now"
!define MUI_COMPONENTSPAGE_NODESC

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

## Signing hooks (release skill §Code signing); the release workflow signs the
## binaries and the installer with signtool when a certificate is configured.
#!uninstfinalize 'signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 "%1"'
#!finalize 'signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\PRism-${INFO_PRODUCTVERSION}-${ARCH}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
ShowInstDetails show

!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define AUMID_KEY "Software\Classes\AppUserModelId\PRism"

Function .onInit
    !insertmacro wails.checkArchitecture
FunctionEnd

# A running PRism keeps prism.exe locked; stop it before replacing files.
!macro StopPRism
    nsExec::Exec 'taskkill /IM ${PRODUCT_EXECUTABLE} /F'
    Pop $0
    Sleep 500
!macroend

Section "PRism (required)" SecMain
    SectionIn RO
    !insertmacro wails.setShellContext
    !insertmacro StopPRism

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    !insertmacro wails.files

    # Start-menu shortcut only: PRism lives in the tray. Toasts get their
    # AppUserModelID from the app itself (ADR 0002).
    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.writeUninstaller
SectionEnd

Section "Start PRism when I sign in" SecAutostart
    WriteRegStr HKCU "${RUN_KEY}" "PRism" '"$INSTDIR\${PRODUCT_EXECUTABLE}" --background'
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext
    !insertmacro StopPRism

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # WebView2 data
    RMDir /r $INSTDIR
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"

    DeleteRegValue HKCU "${RUN_KEY}" "PRism"

    # Toast registration made by the app: COM activator CLSID, AUMID, categories.
    ReadRegStr $0 HKCU "${AUMID_KEY}" "CustomActivator"
    ${If} $0 != ""
        DeleteRegKey HKCU "Software\Classes\CLSID\$0"
    ${EndIf}
    DeleteRegKey HKCU "${AUMID_KEY}"
    DeleteRegKey HKCU "Software\PRism"

    # Local data (%APPDATA%\PRism) and the keyring token are kept on purpose:
    # reinstalling keeps the sign-in, settings and unread items.

    !insertmacro wails.deleteUninstaller
SectionEnd
