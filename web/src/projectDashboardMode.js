export const PROJECT_DASHBOARD_MODE_SIMPLE = 'simple'
export const PROJECT_DASHBOARD_MODE_EXPERT = 'expert'
export const PROJECT_DASHBOARD_MODE_STORAGE_PREFIX = 'lumi.projectDashboardMode'

export function normalizeProjectDashboardMode(value) {
  return value === PROJECT_DASHBOARD_MODE_EXPERT ? PROJECT_DASHBOARD_MODE_EXPERT : PROJECT_DASHBOARD_MODE_SIMPLE
}

export function projectDashboardModeStorageKey(projectUuid) {
  return `${PROJECT_DASHBOARD_MODE_STORAGE_PREFIX}.${projectUuid || ''}`
}

export function readProjectDashboardMode(storage, projectUuid) {
  if (!projectUuid) return PROJECT_DASHBOARD_MODE_SIMPLE
  try {
    return normalizeProjectDashboardMode(storage?.getItem(projectDashboardModeStorageKey(projectUuid)))
  } catch {
    return PROJECT_DASHBOARD_MODE_SIMPLE
  }
}

export function writeProjectDashboardMode(storage, projectUuid, mode) {
  const normalized = normalizeProjectDashboardMode(mode)
  if (!projectUuid) return normalized
  try {
    storage?.setItem(projectDashboardModeStorageKey(projectUuid), normalized)
  } catch {
    // Restricted browser storage must not make either dashboard unusable.
  }
  return normalized
}
