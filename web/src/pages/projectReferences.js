const UUID_V7_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u
const PROJECT_REFERENCE_PREFIX = '@project/'
const PRESERVED_CHAT_PARAMS = ['chat_thread_uuid', 'workflow_uuid']
const CHAT_CONTROL_PARAMS = ['chat_thread_uuid', 'workflow_uuid', 'chat_new', 'chat_reference_type', 'chat_reference_uuid', 'chat_reference_title']

export function isCanonicalUUIDv7(value = '') {
  return UUID_V7_PATTERN.test(String(value))
}

export function parseProjectReference(value = '') {
  const candidate = String(value)
  if (!candidate.startsWith(PROJECT_REFERENCE_PREFIX)) return null
  if (/[%\\?#\u0000-\u001f\u007f]/u.test(candidate) || candidate.includes('..') || candidate.includes('//')) return null

  if (candidate === '@project/story-profile') return { kind: 'story-profile' }
  if (candidate === '@project/premise') return { kind: 'premise' }
  if (candidate === '@project/exports') return { kind: 'exports' }

  let match = candidate.match(/^@project\/premise\/assets\/([0-9a-f-]+)$/u)
  if (match) return isCanonicalUUIDv7(match[1]) ? { kind: 'premise-asset', assetUuid: match[1] } : null

  match = candidate.match(/^@project\/workflows\/([0-9a-f-]+)$/u)
  if (match) return isCanonicalUUIDv7(match[1]) ? { kind: 'workflow', workflowUuid: match[1] } : null

  match = candidate.match(/^@project\/chapters\/([0-9a-f-]+)$/u)
  if (match) return isCanonicalUUIDv7(match[1]) ? { kind: 'chapter', chapterUuid: match[1] } : null

  match = candidate.match(/^@project\/chapters\/([0-9a-f-]+)\/body$/u)
  if (match) return isCanonicalUUIDv7(match[1]) ? { kind: 'chapter-body', chapterUuid: match[1] } : null

  match = candidate.match(/^@project\/chapters\/([0-9a-f-]+)\/sections\/([0-9a-f-]+)$/u)
  if (!match || !isCanonicalUUIDv7(match[1]) || !isCanonicalUUIDv7(match[2])) return null
  return { kind: 'section', chapterUuid: match[1], sectionUuid: match[2] }
}

export function resolveProjectReference(value, { projectUuid, search = '' } = {}) {
  const reference = typeof value === 'string' ? parseProjectReference(value) : value
  if (!reference || !isCanonicalUUIDv7(projectUuid)) return null

  const projectRoot = `/projects/${encodeURIComponent(projectUuid)}`
  const params = preservedChatSearch(search)
  let pathname = projectRoot

  switch (reference.kind) {
    case 'story-profile':
      pathname += '/overview/profile'
      break
    case 'premise':
      pathname += '/premise'
      break
    case 'premise-asset':
      if (!isCanonicalUUIDv7(reference.assetUuid)) return null
      pathname += '/premise'
      params.set('premise_asset_uuid', reference.assetUuid)
      break
    case 'workflow':
      if (!isCanonicalUUIDv7(reference.workflowUuid)) return null
      params.set('workflow_uuid', reference.workflowUuid)
      break
    case 'chapter':
      if (!isCanonicalUUIDv7(reference.chapterUuid)) return null
      pathname += `/chapters/${encodeURIComponent(reference.chapterUuid)}`
      break
    case 'chapter-body':
      if (!isCanonicalUUIDv7(reference.chapterUuid)) return null
      pathname += `/chapters/${encodeURIComponent(reference.chapterUuid)}`
      params.set('workspace_tab', 'body')
      break
    case 'section':
      if (!isCanonicalUUIDv7(reference.chapterUuid) || !isCanonicalUUIDv7(reference.sectionUuid)) return null
      pathname += `/chapters/${encodeURIComponent(reference.chapterUuid)}`
      params.set('section_uuid', reference.sectionUuid)
      break
    case 'exports':
      pathname += '/overview/exports'
      break
    default:
      return null
  }

  const query = params.toString()
  return { pathname, search: query ? `?${query}` : '' }
}

export function projectChatControlKey(search = '') {
  const source = new URLSearchParams(search)
  const result = new URLSearchParams()
  CHAT_CONTROL_PARAMS.forEach((key) => {
    if (source.has(key)) result.set(key, source.get(key) || '')
  })
  return result.toString()
}

export function isPlainProjectReferenceClick(event = {}) {
  return (event.button ?? 0) === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey
}

function preservedChatSearch(search) {
  const source = new URLSearchParams(search)
  const result = new URLSearchParams()
  PRESERVED_CHAT_PARAMS.forEach((key) => {
    const value = source.get(key)
    if (value) result.set(key, value)
  })
  return result
}
