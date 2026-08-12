export const ACTIVE_PRODUCTION_STATUSES = new Set(['queued', 'running'])

export function activeTaskFor(tasks, kind, resourceUuid) {
  return (tasks || []).find((task) => task.kind === kind && task.resource_uuid === resourceUuid && ACTIVE_PRODUCTION_STATUSES.has(task.status)) || null
}

export function moveSection(sectionUuids, index, direction) {
  const target = index + direction
  if (index < 0 || target < 0 || index >= sectionUuids.length || target >= sectionUuids.length) return sectionUuids
  const next = [...sectionUuids]
  ;[next[index], next[target]] = [next[target], next[index]]
  return next
}

export function normalizedTags(value) {
  return [...new Set(String(value || '').split(',').map((tag) => tag.trim().toLowerCase()).filter(Boolean))]
}

export function premiseAssetTitleFromFile(fileOrName, fallback = '') {
  const name = typeof fileOrName === 'string' ? fileOrName : fileOrName?.name || ''
  return name.replace(/\.[^.]+$/, '').replace(/^\.+/, '').replace(/[_-]+/g, ' ').trim() || fallback
}

export function collectPremiseTags(assets, locale) {
  return [...new Set((assets || []).flatMap((asset) => asset.tags || []).map((tag) => String(tag).trim().toLowerCase()).filter(Boolean))]
    .sort((left, right) => left.localeCompare(right, locale))
}

export function premiseSourceState(sourceOrUuid, settings, tasks) {
  const sourceUuid = typeof sourceOrUuid === 'string' ? sourceOrUuid : sourceOrUuid?.uuid
  if (typeof sourceOrUuid === 'object' && sourceOrUuid?.ignored_at) return 'ignored'
  const sourceSettings = (settings || []).filter((setting) => setting.source_uuid === sourceUuid)
  const generation = (tasks || []).find((task) => task.kind === 'premise_setting_generation' && task.resource_uuid === sourceUuid)
  const breakdowns = (tasks || []).filter((task) => task.kind === 'premise_asset_breakdown' && sourceSettings.some((setting) => setting.uuid === task.resource_uuid))
  if (generation && ['queued', 'running'].includes(generation.status)) return 'generating'
  if (generation && ['failed', 'interrupted'].includes(generation.status) && sourceSettings.length === 0) return 'failed'
  if (breakdowns.some((task) => ['queued', 'running'].includes(task.status))) return 'splitting'
  if (breakdowns.some((task) => task.status === 'completed')) return 'completed'
  if (sourceSettings.length > 0) return 'ready'
  return 'draft'
}
