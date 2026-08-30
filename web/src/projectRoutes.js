const PROJECT_ROOT = 'projects'
const MODE_QUERY_KEY = 'workspace_mode'

export function projectRoute(projectUuid, route = '', search = '') {
  const suffix = String(route || '').replace(/^\/+|\/+$/g, '')
  const pathname = suffix
    ? `${projectBase(projectUuid)}/${suffix}`
    : projectBase(projectUuid)
  return { pathname, search: normalizedSearch(search) }
}

export function projectPremiseAssetRoute(projectUuid, assetUuid, search = '') {
  return projectRoute(projectUuid, `premise/assets/${encodeURIComponent(assetUuid || '')}`, search)
}

export function projectChapterRoute(projectUuid, chapterUuid, search = '') {
  return projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid || '')}`, search)
}

export function projectSectionRoute(projectUuid, chapterUuid, sectionUuid, search = '') {
  return projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid || '')}/sections/${encodeURIComponent(sectionUuid || '')}`, search)
}

export function projectPreviewRoute(projectUuid, chapterUuid, search = '') {
  return projectRoute(projectUuid, `chapters/${encodeURIComponent(chapterUuid || '')}/preview`, search)
}

export function projectRouteRequiresExpert(pathname = '', projectUuid = '') {
  const route = projectRelativePath(pathname, projectUuid)
  return /^(?:prompts|llm-logs|exports|assets|threads)(?:\/|$)/.test(route)
}

export function projectModeOverride(search = '') {
  const value = new URLSearchParams(String(search || '').replace(/^\?/, '')).get(MODE_QUERY_KEY)
  return value === 'simple' || value === 'expert' ? value : ''
}

export function withoutProjectModeOverride(search = '') {
  const params = searchParams(search)
  params.delete(MODE_QUERY_KEY)
  return normalizedSearch(params)
}

export function canonicalProjectLocation({ projectUuid, pathname = '', search = '', hash = '' }) {
  const route = projectRelativePath(pathname, projectUuid)
  if (route === null) return null

  const params = searchParams(search)
  let canonicalRoute = route
  let changed = false

  if (route === 'simple' || route === 'simple/home' || route === 'overview' || route === 'overview/summary') {
    canonicalRoute = ''
    changed = true
  } else if (route === 'simple/story' || route === 'overview/profile') {
    canonicalRoute = 'story'
    changed = true
  } else if (route === 'simple/settings') {
    canonicalRoute = 'premise'
    changed = true
  } else if (route === 'overview/prompts') {
    canonicalRoute = 'prompts'
    changed = true
  } else if (route === 'overview/llm-logs') {
    canonicalRoute = 'llm-logs'
    changed = true
  } else if (route === 'overview/exports') {
    canonicalRoute = 'exports'
    changed = true
  } else if (route === 'simple/books') {
    canonicalRoute = 'chapters'
    changed = true
  } else if (route === 'comic' || route === 'trash') {
    canonicalRoute = 'chapters'
    if (route === 'trash') params.set('state', 'trashed')
    changed = true
  } else {
    const simpleAsset = route.match(/^simple\/settings\/([^/]+)$/)
    const simpleSection = route.match(/^simple\/books\/([^/]+)\/pages\/([^/]+)$/)
    const simplePreview = route.match(/^simple\/books\/([^/]+)\/book$/)
    const simpleChapter = route.match(/^simple\/books\/([^/]+)(?:\/pages)?$/)
    const comicChapter = route.match(/^comic\/([^/]+)$/)

    if (simpleAsset) {
      canonicalRoute = `premise/assets/${encodeSegment(simpleAsset[1])}`
      changed = true
    } else if (simpleSection) {
      canonicalRoute = `chapters/${encodeSegment(simpleSection[1])}/sections/${encodeSegment(simpleSection[2])}`
      changed = true
    } else if (simplePreview) {
      canonicalRoute = `chapters/${encodeSegment(simplePreview[1])}/preview`
      changed = true
    } else if (simpleChapter) {
      canonicalRoute = `chapters/${encodeSegment(simpleChapter[1])}`
      changed = true
    } else if (comicChapter) {
      canonicalRoute = `chapters/${encodeSegment(comicChapter[1])}`
      changed = true
    }
  }

  if (canonicalRoute === 'premise' && params.get('premise_asset_uuid')) {
    canonicalRoute = `premise/assets/${encodeURIComponent(params.get('premise_asset_uuid'))}`
    params.delete('premise_asset_uuid')
    changed = true
  }

  const chapter = canonicalRoute.match(/^chapters\/([^/]+)$/)
  if (chapter && params.get('section_uuid')) {
    canonicalRoute = `chapters/${encodeSegment(chapter[1])}/sections/${encodeURIComponent(params.get('section_uuid'))}`
    params.delete('section_uuid')
    changed = true
  }

  if (!changed) return null
  const destination = projectRoute(projectUuid, canonicalRoute, params)
  return { ...destination, hash: hash || '' }
}

export function projectRelativePath(pathname = '', projectUuid = '') {
  const base = projectBase(projectUuid)
  if (pathname !== base && pathname !== `${base}/` && !pathname.startsWith(`${base}/`)) return null
  return pathname.slice(base.length).replace(/^\/+|\/+$/g, '')
}

function projectBase(projectUuid) {
  return `/${PROJECT_ROOT}/${encodeURIComponent(projectUuid || '')}`
}

function searchParams(search) {
  return search instanceof URLSearchParams
    ? new URLSearchParams(search)
    : new URLSearchParams(String(search || '').replace(/^\?/, ''))
}

function normalizedSearch(search) {
  const value = search instanceof URLSearchParams
    ? search.toString()
    : String(search || '').replace(/^\?/, '')
  return value ? `?${value}` : ''
}

function encodeSegment(value) {
  try { return encodeURIComponent(decodeURIComponent(value)) } catch { return encodeURIComponent(value) }
}
