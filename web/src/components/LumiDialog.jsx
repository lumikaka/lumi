import { useEffect, useRef } from 'react'

import { isDialogOverlayClick, requestDialogDismiss } from './dialogOverlay.js'

export default function LumiDialog({ children, className = '', dismissDisabled = false, onClose, ...props }) {
  const dialogRef = useRef(null)

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return undefined
    dialog.showModal()
    return () => { if (dialog.open) dialog.close() }
  }, [])

  const dismiss = () => requestDialogDismiss(onClose, dismissDisabled)

  return (
    <dialog
      {...props}
      ref={dialogRef}
      className={`lumi-dialog${className ? ` ${className}` : ''}`}
      onCancel={(event) => { event.preventDefault(); dismiss() }}
      onClick={(event) => { if (isDialogOverlayClick(event)) dismiss() }}
    >
      {children}
    </dialog>
  )
}
