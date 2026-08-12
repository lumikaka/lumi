import { apiRequest } from './client.js'

function jsonRequest(method, body) {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

function projectPath(projectUuid, suffix = '') {
  return `/api/v1/projects/${encodeURIComponent(projectUuid)}${suffix}`
}

export function getStoryProject(projectUuid) {
  return apiRequest(projectPath(projectUuid))
}

export function updateStoryProject(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid), jsonRequest('PATCH', payload))
}

export function listChapters(projectUuid, state = 'active') {
  return apiRequest(projectPath(projectUuid, `/chapters?state=${encodeURIComponent(state)}`))
}

export function getChapter(projectUuid, chapterUuid) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}`))
}

export function createChapter(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid, '/chapters'), jsonRequest('POST', payload))
}

export function updateChapter(projectUuid, chapterUuid, payload) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}`), jsonRequest('PATCH', payload))
}

export function updateChapterStory(projectUuid, chapterUuid, payload) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/current-story`), jsonRequest('PUT', payload))
}

export function listChapterStories(projectUuid, chapterUuid) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/stories`))
}

export function trashChapter(projectUuid, chapterUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}?expected_revision=${encodeURIComponent(expectedRevision)}`), { method: 'DELETE' })
}

export function restoreChapter(projectUuid, chapterUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/restorations`), jsonRequest('POST', { expected_revision: expectedRevision }))
}

export function permanentlyDeleteChapter(projectUuid, chapterUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/permanent?expected_revision=${encodeURIComponent(expectedRevision)}`), { method: 'DELETE' })
}

export function emptyChapterTrash(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/chapters/trash'), { method: 'DELETE' })
}

export function importChapters(projectUuid, { mode, files, chapterCode = '', title = '' }) {
  const form = new FormData()
  form.append('mode', mode)
  if (mode === 'single') {
    form.append('chapter_code', chapterCode)
    form.append('title', title)
    if (files?.[0]) form.append('file', files[0])
  } else {
    Array.from(files || []).forEach((file) => form.append('files[]', file))
  }
  return apiRequest(projectPath(projectUuid, '/chapter-imports'), { method: 'POST', body: form })
}

export function getStoryProfile(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/story-profile'))
}

export function updateStoryProfile(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid, '/story-profile'), jsonRequest('PUT', payload))
}

export function listStoryProfileVersions(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/story-profile/versions'))
}

export function importExternalStoryMD(projectUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, '/story-profile/imports'), jsonRequest('POST', { expected_revision: expectedRevision }))
}

export function regenerateStoryMD(projectUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, '/story-profile/projection'), jsonRequest('POST', { expected_revision: expectedRevision }))
}

export function listPromptCatalog(projectUuid, promptGroup = '') {
	const search = new URLSearchParams()
	if (promptGroup) search.set('prompt_group', promptGroup)
	const suffix = search.size ? `?${search}` : ''
	return apiRequest(projectPath(projectUuid, `/prompts${suffix}`))
}

export function listPromptVersions(projectUuid, { promptGroup, promptKey, page = 1, perPage = 20 }) {
  const search = new URLSearchParams({
    prompt_group: promptGroup,
    prompt_key: promptKey,
    page: String(page),
    per_page: String(perPage),
  })
  return apiRequest(projectPath(projectUuid, `/prompt-versions?${search}`))
}

export function createPromptVersion(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid, '/prompt-versions'), jsonRequest('POST', payload))
}

export function updatePromptGroup(projectUuid, promptGroup, payload) {
  return apiRequest(projectPath(projectUuid, `/prompt-groups/${encodeURIComponent(promptGroup)}`), jsonRequest('PATCH', payload))
}

export function restorePromptVersion(projectUuid, versionUuid, expectedCurrentVersion) {
  return apiRequest(projectPath(projectUuid, `/prompt-versions/${encodeURIComponent(versionUuid)}/restorations`), jsonRequest('POST', { expected_current_version: expectedCurrentVersion }))
}
