import { apiRequest } from './client.js'

function projectPath(projectUuid, suffix = '') {
  return `/api/v1/projects/${encodeURIComponent(projectUuid)}${suffix}`
}

function jsonRequest(method, body = {}) {
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

export function createAssetUpload(projectUuid, { purpose, displayName = '', file }) {
  const form = new FormData()
  // The streaming endpoint deliberately requires policy fields before bytes.
  form.append('purpose', purpose)
  form.append('display_name', displayName)
  form.append('file', file)
  return apiRequest(projectPath(projectUuid, '/asset-uploads'), { method: 'POST', body: form })
}

export function finalizeAssetUpload(projectUuid, uploadUuid, purpose) {
  return apiRequest(projectPath(projectUuid, `/asset-uploads/${encodeURIComponent(uploadUuid)}/completions`), jsonRequest('POST', { purpose }))
}

export function getAssetUpload(projectUuid, uploadUuid) {
  return apiRequest(projectPath(projectUuid, `/asset-uploads/${encodeURIComponent(uploadUuid)}`))
}

export function listAssets(projectUuid, { deleted = false, limit = 100 } = {}) {
  const search = new URLSearchParams({ deleted: String(deleted), limit: String(limit) })
  return apiRequest(projectPath(projectUuid, `/assets?${search}`))
}

export function trashAsset(projectUuid, assetUuid) {
  return apiRequest(projectPath(projectUuid, `/assets/${encodeURIComponent(assetUuid)}`), { method: 'DELETE' })
}

export function restoreAsset(projectUuid, assetUuid) {
  return apiRequest(projectPath(projectUuid, `/assets/${encodeURIComponent(assetUuid)}/restorations`), jsonRequest('POST'))
}

export function listIntegrityScans(projectUuid, limit = 10) {
  return apiRequest(projectPath(projectUuid, `/integrity-scans?limit=${encodeURIComponent(limit)}`))
}

export function createIntegrityScan(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/integrity-scans'), jsonRequest('POST'))
}

export function reconcileAssets(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/asset-reconciliations'), jsonRequest('POST'))
}

export function rebuildAssetThumbnail(projectUuid, assetUuid, profile = 'grid_256') {
  const search = new URLSearchParams({ profile })
  return apiRequest(projectPath(projectUuid, `/assets/${encodeURIComponent(assetUuid)}/thumbnails?${search}`), { method: 'POST' })
}

export function listAssetMaintenanceTasks(projectUuid, limit = 20) {
  return apiRequest(projectPath(projectUuid, `/asset-maintenance-tasks?limit=${encodeURIComponent(limit)}`))
}

export function cancelAssetMaintenanceTask(projectUuid, taskUuid) {
  return apiRequest(projectPath(projectUuid, `/asset-maintenance-tasks/${encodeURIComponent(taskUuid)}/cancellations`), jsonRequest('POST'))
}
