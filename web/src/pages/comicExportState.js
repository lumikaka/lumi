export function comicExportReadinessDecision(readiness) {
  if (!readiness?.can_export || Number(readiness?.image_section_count || 0) === 0) return 'blocked'
  if (Number(readiness?.missing_section_count || 0) > 0) return 'confirm'
  return 'ready'
}

export function comicExportEmptyError() {
  return { code: 'production_export_empty', status: 422 }
}

export const activeComicExportStatuses = new Set(['queued', 'running'])
export const retryableComicExportStatuses = new Set(['failed', 'interrupted', 'cancelled'])

export function comicExportOperationState(task, exportRecord) {
  if (exportRecord?.status === 'ready' && exportRecord?.output_asset) return 'ready'
  const status = task?.status || exportRecord?.status || 'queued'
  if (status === 'completed') return 'finalizing'
  return status
}

export function comicExportSnapshotMetrics(readiness, exportRecord) {
  const snapshot = exportRecord?.snapshot || {}
  return {
    version: Number(snapshot.version || 0),
    total: Number(snapshot.section_count ?? readiness?.active_section_count ?? 0),
    ready: Number(snapshot.exported_section_count ?? readiness?.image_section_count ?? 0),
    missing: Number(snapshot.missing_section_count ?? readiness?.missing_section_count ?? 0),
  }
}

export function comicExportDialogRequest(scope, chapterUuid = '', chapterLabel = '') {
  return {
    scope,
    chapterUuid: scope === 'chapter' ? chapterUuid : '',
    chapterLabel: scope === 'chapter' ? chapterLabel : '',
    idempotencyKey: `comic-export-dialog-${scope}-${Date.now()}-${Math.random().toString(36).slice(2)}`,
  }
}
