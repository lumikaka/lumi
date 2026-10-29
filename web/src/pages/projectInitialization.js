const recoverableStatuses = new Set(['failed', 'cancelled', 'interrupted'])

export function projectInitializationControls(initialization) {
  const locked = initialization?.locked === true
  const recoverable = locked && recoverableStatuses.has(initialization.status)
  return {
    locked,
    canCancel: locked && ['queued', 'running'].includes(initialization.status),
    canRetry: recoverable && Boolean(initialization.workflow_uuid),
    canTakeOver: recoverable && initialization.can_take_over === true,
  }
}

export function initializationChatSearch(search, initialization) {
  const next = new URLSearchParams(search)
  if (!initialization?.locked) return next
  for (const key of ['chat_new', 'chat_reference_type', 'chat_reference_uuid', 'chat_reference_title', 'chat_thread_uuid', 'workflow_uuid']) next.delete(key)
  if (initialization.thread_uuid) next.set('chat_thread_uuid', initialization.thread_uuid)
  if (initialization.workflow_uuid) next.set('workflow_uuid', initialization.workflow_uuid)
  return next
}
