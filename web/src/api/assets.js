import { apiRequest as webRequest } from './client.js'

// One endpoint contract; callers inject only the transport.
export function createAssetsAPI(apiRequest = webRequest) {

function projectPath(projectUuid, suffix = '') {
  return `/api/v1/projects/${encodeURIComponent(projectUuid)}${suffix}`
}

function jsonRequest(method, body = {}) {
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

function createAssetUpload(projectUuid, { purpose, displayName = '', file }) {
  const form = new FormData()
  // The streaming endpoint deliberately requires policy fields before bytes.
  form.append('purpose', purpose)
  form.append('display_name', displayName)
  form.append('file', file)
  return apiRequest(projectPath(projectUuid, '/asset-uploads'), { method: 'POST', body: form })
}

function finalizeAssetUpload(projectUuid, uploadUuid, purpose) {
  return apiRequest(projectPath(projectUuid, `/asset-uploads/${encodeURIComponent(uploadUuid)}/completions`), jsonRequest('POST', { purpose }))
}

function getAssetUpload(projectUuid, uploadUuid) {
  return apiRequest(projectPath(projectUuid, `/asset-uploads/${encodeURIComponent(uploadUuid)}`))
}

function listAssets(projectUuid, { deleted = false, limit = 100 } = {}) {
  const search = new URLSearchParams({ deleted: String(deleted), limit: String(limit) })
  return apiRequest(projectPath(projectUuid, `/assets?${search}`))
}

function trashAsset(projectUuid, assetUuid) {
  return apiRequest(projectPath(projectUuid, `/assets/${encodeURIComponent(assetUuid)}`), { method: 'DELETE' })
}

function restoreAsset(projectUuid, assetUuid) {
  return apiRequest(projectPath(projectUuid, `/assets/${encodeURIComponent(assetUuid)}/restorations`), jsonRequest('POST'))
}

function listIntegrityScans(projectUuid, limit = 10) {
  return apiRequest(projectPath(projectUuid, `/integrity-scans?limit=${encodeURIComponent(limit)}`))
}

function createIntegrityScan(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/integrity-scans'), jsonRequest('POST'))
}

function reconcileAssets(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/asset-reconciliations'), jsonRequest('POST'))
}

function rebuildAssetThumbnail(projectUuid, assetUuid, profile = 'grid_256') {
  const search = new URLSearchParams({ profile })
  return apiRequest(projectPath(projectUuid, `/assets/${encodeURIComponent(assetUuid)}/thumbnails?${search}`), { method: 'POST' })
}

function listAssetMaintenanceTasks(projectUuid, limit = 20) {
  return apiRequest(projectPath(projectUuid, `/asset-maintenance-tasks?limit=${encodeURIComponent(limit)}`))
}

function cancelAssetMaintenanceTask(projectUuid, taskUuid) {
  return apiRequest(projectPath(projectUuid, `/asset-maintenance-tasks/${encodeURIComponent(taskUuid)}/cancellations`), jsonRequest('POST'))
}

  return { createAssetUpload, finalizeAssetUpload, getAssetUpload, listAssets, trashAsset, restoreAsset, listIntegrityScans, createIntegrityScan, reconcileAssets, rebuildAssetThumbnail, listAssetMaintenanceTasks, cancelAssetMaintenanceTask }
}

export const { createAssetUpload, finalizeAssetUpload, getAssetUpload, listAssets, trashAsset, restoreAsset, listIntegrityScans, createIntegrityScan, reconcileAssets, rebuildAssetThumbnail, listAssetMaintenanceTasks, cancelAssetMaintenanceTask } = createAssetsAPI()
