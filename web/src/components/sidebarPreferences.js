const PINNED_THREADS_KEY = 'lumi.sidebar.pinnedThreads'
const PINNED_PROJECTS_KEY = 'lumi.sidebar.pinnedProjects'
const READ_THREADS_KEY = 'lumi.sidebar.readThreads'

export function readPinnedProjects() {
  const value = readJSON(PINNED_PROJECTS_KEY)
  return Array.isArray(value) ? value.filter((uuid) => typeof uuid === 'string') : []
}

export function writePinnedProjects(uuids) {
  writeJSON(PINNED_PROJECTS_KEY, [...new Set(uuids)])
}

export function readPinnedThreads() {
  const value = readJSON(PINNED_THREADS_KEY)
  return Array.isArray(value) ? value.filter((uuid) => typeof uuid === 'string') : []
}

export function writePinnedThreads(uuids) {
  writeJSON(PINNED_THREADS_KEY, [...new Set(uuids)])
}

export function readThreadReadAt() {
  const value = readJSON(READ_THREADS_KEY)
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

export function markThreadRead(threadUuid, timestamp = new Date().toISOString()) {
  const next = { ...readThreadReadAt(), [threadUuid]: timestamp }
  writeJSON(READ_THREADS_KEY, next)
  return next
}

export function threadReadState(thread, readAt) {
  if (['busy', 'waiting_for_input', 'queued', 'in_progress'].includes(thread.status)) return 'in_progress'
  const seenAt = Date.parse(readAt?.[thread.uuid] || '')
  const updatedAt = Date.parse(thread.updated_at || thread.created_at || '')
  return Number.isFinite(seenAt) && (!Number.isFinite(updatedAt) || seenAt >= updatedAt) ? 'read' : 'unread'
}

function readJSON(key) {
  if (typeof window === 'undefined') return null
  try { return JSON.parse(window.localStorage.getItem(key) || 'null') } catch { return null }
}

function writeJSON(key, value) {
  if (typeof window === 'undefined') return
  try { window.localStorage.setItem(key, JSON.stringify(value)) } catch { /* restricted browser */ }
}
