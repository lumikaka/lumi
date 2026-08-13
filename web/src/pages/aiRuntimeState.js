export const ACTIVE_TASK_STATUSES = new Set(['queued', 'running', 'waiting_for_input'])

export function latestTaskForResource(items, resourceUuid) {
  return (items || []).find((task) => task.resource_uuid === resourceUuid) || null
}

export function taskControls(task) {
  if (!task) return { canCancel: false, canRetry: false }
  return {
    canCancel: ACTIVE_TASK_STATUSES.has(task.status),
    canRetry: Boolean(task.retryable && ['failed', 'interrupted'].includes(task.status)),
  }
}
