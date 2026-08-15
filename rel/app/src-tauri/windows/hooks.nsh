!macro NSIS_HOOK_POSTUNINSTALL
  ; Tauri's target check can leave a stale shortcut behind, so finish with
  ; idempotent path-based cleanup after the normal uninstall work.
  ${If} $UpdateMode <> 1
    SetShellVarContext current
    Delete "$SMPROGRAMS\${PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${PRODUCTNAME}.lnk"
  ${EndIf}
!macroend
