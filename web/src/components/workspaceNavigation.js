export const WORKSPACE_SECTIONS = Object.freeze([
  { key: 'overview', labelKey: 'projects.section.overview', route: '' },
  { key: 'premise', labelKey: 'projects.section.premise', route: 'premise' },
  { key: 'chapters', labelKey: 'projects.section.chapters', route: 'chapters' },
])

export const WORKSPACE_GROUP_ITEMS = Object.freeze({
  overview: [
    { key: 'summary', labelKey: 'projects.tab.summary', route: '', end: true },
    { key: 'profile', labelKey: 'projects.tab.profile', route: 'story', end: true },
    { key: 'prompts', labelKey: 'projects.tab.prompts', route: 'prompts', end: true },
    { key: 'llm-logs', labelKey: 'projects.tab.llm_logs', route: 'llm-logs', end: true },
    { key: 'exports', labelKey: 'projects.tab.exports', route: 'exports', end: true },
  ],
  premise: [
    { key: 'premise', labelKey: 'projects.tab.premise', route: 'premise', end: true },
    { key: 'assets', labelKey: 'projects.tab.assets', route: 'assets', end: true },
  ],
  chapters: [
    { key: 'chapters', labelKey: 'projects.section.chapters', route: 'chapters' },
    { key: 'trash', labelKey: 'projects.tab.trash', route: 'chapters?state=trashed' },
  ],
})

export function workspaceSectionForPath(pathname = '') {
  const match = pathname.match(/^\/projects\/[^/]+(?:\/([^/?#]+))?/)
  const route = match?.[1] || ''
  if (route === 'premise' || route === 'assets') return 'premise'
  if (route === 'chapters') return 'chapters'
  return 'overview'
}

export function workspaceRoute(projectUuid, route = '', search = '') {
  const base = `/projects/${encodeURIComponent(projectUuid || '')}`
  const [path, routeSearch = ''] = String(route || '').split('?', 2)
  const pathname = path ? `${base}/${path}` : base
  const params = new URLSearchParams(String(search || '').replace(/^\?/, ''))
  const routeParams = new URLSearchParams(routeSearch)
  if (!routeParams.has('state')) params.delete('state')
  routeParams.forEach((value, key) => params.set(key, value))
  const query = params.toString()
  return { pathname, search: query ? `?${query}` : '' }
}
