const stages = new Set(['selecting_references', 'preparing_references', 'generating', 'saving'])

export function imageTaskStage(task) {
  if (!task) return ''
  if (!['queued', 'running'].includes(task.status)) return task.status
  if (task.cancel_requested_at) return 'cancelling'
  if (task.status === 'queued') return 'queued'
  return stages.has(task.stage) ? task.stage : 'running'
}

export function imageStageElapsed(task, now) {
  if (task?.status !== 'running' || task.cancel_requested_at) return null
  const started = Date.parse(task.stage_started_at || task.started_at || '')
  return Number.isFinite(started) ? Math.max(0, Math.floor((now - started) / 1000)) : null
}

export function imageBatchCounts(workflow) {
  const counts = { total: 0, completed: 0, running: 0, queued: 0, failed: 0, cancelled: 0 }
  for (const step of workflow?.steps || []) {
    counts.total += 1
    if (step.status === 'interrupted') counts.failed += 1
    else if (Object.hasOwn(counts, step.status) && step.status !== 'total') counts[step.status] += 1
  }
  return counts
}
