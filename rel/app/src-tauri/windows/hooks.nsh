!macro StopLumiProcesses id
  ; Tauri only checks the main executable. Stop it first so it cannot spawn a
  ; replacement backend while the installer removes the old backend process.
  !insertmacro CheckIfAppIsRunning "${MAINBINARYNAME}.exe" "${PRODUCTNAME}"

  !if "${INSTALLMODE}" == "currentUser"
    nsis_tauri_utils::FindProcessCurrentUser "lumi_web.exe"
  !else
    nsis_tauri_utils::FindProcess "lumi_web.exe"
  !endif
  Pop $R0
  ${If} $R0 = 0
    !if "${INSTALLMODE}" == "currentUser"
      nsis_tauri_utils::KillProcessCurrentUser "lumi_web.exe"
    !else
      nsis_tauri_utils::KillProcess "lumi_web.exe"
    !endif
    Pop $R0

    StrCpy $R1 0
    lumi_backend_wait_${id}:
      Sleep 200
      !if "${INSTALLMODE}" == "currentUser"
        nsis_tauri_utils::FindProcessCurrentUser "lumi_web.exe"
      !else
        nsis_tauri_utils::FindProcess "lumi_web.exe"
      !endif
      Pop $R0
      ${If} $R0 != 0
        Goto lumi_backend_stopped_${id}
      ${EndIf}
      IntOp $R1 $R1 + 1
      ${If} $R1 < 25
        Goto lumi_backend_wait_${id}
      ${EndIf}
      Abort "Lumi backend is still running. Close lumi_web.exe and try again."
  ${EndIf}

  lumi_backend_stopped_${id}:
!macroend

!macro RemoveLumiAppData directory id
  IfFileExists "${directory}\*.*" 0 lumi_app_data_removed_${id}
  ClearErrors
  RmDir /r "${directory}"
  IfErrors lumi_app_data_cleanup_failed_${id} lumi_app_data_removed_${id}

  lumi_app_data_cleanup_failed_${id}:
    Abort "Lumi user data could not be removed from ${directory}. Close any process using it and try again."
  lumi_app_data_removed_${id}:
!macroend

!macro NSIS_HOOK_PREINSTALL
  ; Also removes orphaned backends left by installers older than this fix.
  !insertmacro StopLumiProcesses install
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  !insertmacro StopLumiProcesses uninstall

  ; The stock Tauri checkbox is not shown during silent uninstall. Keep a
  ; matching explicit flag for CI and managed uninstall workflows.
  ${GetOptions} $CMDLINE "/DELETEAPPDATA" $R0
  ${IfNot} ${Errors}
    StrCpy $DeleteAppDataCheckboxState 1
  ${EndIf}
  ClearErrors
!macroend

!macro NSIS_HOOK_POSTUNINSTALL
  ; Tauri's target check can leave a stale shortcut behind, so finish with
  ; idempotent path-based cleanup after the normal uninstall work.
  ${If} $UpdateMode <> 1
    SetShellVarContext current
    Delete "$SMPROGRAMS\${PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${PRODUCTNAME}.lnk"
  ${EndIf}

  ; Tauri's recursive removals are unchecked. If the user requested deletion,
  ; retry after process cleanup and fail truthfully if Windows still holds a
  ; file lock instead of reporting a successful uninstall with data left over.
  ${If} $DeleteAppDataCheckboxState = 1
  ${AndIf} $UpdateMode <> 1
    SetShellVarContext current
    !insertmacro RemoveLumiAppData "$APPDATA\${BUNDLEID}" roaming
    !insertmacro RemoveLumiAppData "$LOCALAPPDATA\${BUNDLEID}" local
  ${EndIf}
!macroend
