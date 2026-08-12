export function isDialogOverlayClick(event) {
  const dialog = event.currentTarget
  if (event.target !== dialog) return false

  const bounds = dialog.getBoundingClientRect()
  return event.clientX < bounds.left
    || event.clientX > bounds.right
    || event.clientY < bounds.top
    || event.clientY > bounds.bottom
}

export function requestDialogDismiss(onClose, dismissDisabled = false) {
  if (!dismissDisabled) onClose?.()
}
