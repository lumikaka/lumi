export const WORKSPACE_SECTIONS = Object.freeze([
  { key: 'overview', labelKey: 'projects.section.overview', route: 'overview/summary' },
  { key: 'premise', labelKey: 'projects.section.premise', route: 'premise' },
  { key: 'chapters', labelKey: 'projects.section.chapters', route: 'chapters' },
])

export const WORKSPACE_GROUP_ITEMS = Object.freeze({
  overview: [
    { key: 'summary', labelKey: 'projects.tab.summary', route: 'overview/summary', end: true },
    { key: 'profile', labelKey: 'projects.tab.profile', route: 'overview/profile', end: true },
    { key: 'prompts', labelKey: 'projects.tab.prompts', route: 'overview/prompts', end: true },
    { key: 'llm-logs', labelKey: 'projects.tab.llm_logs', route: 'overview/llm-logs', end: true },
    { key: 'exports', labelKey: 'projects.tab.exports', route: 'overview/exports', end: true },
  ],
  premise: [
    { key: 'premise', labelKey: 'projects.tab.premise', route: 'premise', end: true },
    { key: 'assets', labelKey: 'projects.tab.assets', route: 'assets', end: true },
  ],
  chapters: [
    { key: 'chapters', labelKey: 'projects.section.chapters', route: 'chapters' },
    { key: 'comic', labelKey: 'projects.tab.comic', route: 'comic' },
    { key: 'trash', labelKey: 'projects.tab.trash', route: 'trash' },
  ],
})

export function workspaceSectionForPath(pathname = '') {
  const match = pathname.match(/^\/projects\/[^/]+(?:\/([^/?#]+))?/)
  const route = match?.[1] || ''
  if (route === 'premise' || route === 'assets') return 'premise'
  if (route === 'chapters' || route === 'comic' || route === 'trash') return 'chapters'
  return 'overview'
}

export function canvasModeForPath(pathname = '') {
  const match = pathname.match(/^\/projects\/[^/]+(?:\/([^/?#]+))?/)
  const route = match?.[1] || ''
  if (route === 'premise' || route === 'assets') return 'premise'
  if (route === 'chapters' || route === 'trash') return 'chapters'
  if (route === 'comic' || pathname.includes('/overview/exports')) return 'works'
  return ''
}

export function workspaceRoute(projectUuid, route = '', search = '') {
  const base = `/projects/${encodeURIComponent(projectUuid || '')}`
  const pathname = route ? `${base}/${route}` : base
  return { pathname, search: search || '' }
}
