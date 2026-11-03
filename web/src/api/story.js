import { readSummaryPages } from './summary.js'
import { apiRequest as webRequest } from './client.js'

// One endpoint contract; callers inject only the transport.
export function createStoryAPI(apiRequest = webRequest) {

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

function getStoryProject(projectUuid) {
  return apiRequest(projectPath(projectUuid))
}

function updateStoryProject(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid), jsonRequest('PATCH', payload))
}

function listChapters(projectUuid, state = 'active') {
  return apiRequest(projectPath(projectUuid, `/chapters?state=${encodeURIComponent(state)}`))
}

function getChapter(projectUuid, chapterUuid) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}`))
}

function createChapter(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid, '/chapters'), jsonRequest('POST', payload))
}

function reorderChapters(projectUuid, chapterUuids) {
  return apiRequest(projectPath(projectUuid, '/chapter-order'), jsonRequest('PUT', { chapter_uuids: chapterUuids }))
}

function updateChapter(projectUuid, chapterUuid, payload) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}`), jsonRequest('PATCH', payload))
}

function updateChapterStory(projectUuid, chapterUuid, payload) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/current-story`), jsonRequest('PUT', payload))
}

function listChapterStories(projectUuid, chapterUuid) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/stories`))
}

function trashChapter(projectUuid, chapterUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}?expected_revision=${encodeURIComponent(expectedRevision)}`), { method: 'DELETE' })
}

function restoreChapter(projectUuid, chapterUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/restorations`), jsonRequest('POST', { expected_revision: expectedRevision }))
}

function permanentlyDeleteChapter(projectUuid, chapterUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/permanent?expected_revision=${encodeURIComponent(expectedRevision)}`), { method: 'DELETE' })
}

function emptyChapterTrash(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/chapters/trash'), { method: 'DELETE' })
}

function importChapters(projectUuid, { mode, files, chapterCode = '', title = '' }) {
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

function getStoryProfile(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/story-profile'))
}

function updateStoryProfile(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid, '/story-profile'), jsonRequest('PUT', payload))
}

function listStoryProfileVersions(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/story-profile/versions'))
}

function restoreStoryProfileVersion(projectUuid, versionUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, `/story-profile/versions/${encodeURIComponent(versionUuid)}/restorations`), jsonRequest('POST', { expected_revision: expectedRevision }))
}

function importExternalStoryMD(projectUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, '/story-profile/imports'), jsonRequest('POST', { expected_revision: expectedRevision }))
}

function regenerateStoryMD(projectUuid, expectedRevision) {
  return apiRequest(projectPath(projectUuid, '/story-profile/projection'), jsonRequest('POST', { expected_revision: expectedRevision }))
}

function listPromptCatalog(projectUuid, promptGroup = '') {
	const search = new URLSearchParams()
	if (promptGroup) search.set('prompt_group', promptGroup)
	const suffix = search.size ? `?${search}` : ''
	return apiRequest(projectPath(projectUuid, `/prompts${suffix}`))
}

function listPromptVersions(projectUuid, { promptGroup, promptKey, page = 1, perPage = 20 }) {
  const search = new URLSearchParams({
    prompt_group: promptGroup,
    prompt_key: promptKey,
    page: String(page),
    per_page: String(perPage),
  })
  return apiRequest(projectPath(projectUuid, `/prompt-versions?${search}`))
}

function createPromptVersion(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid, '/prompt-versions'), jsonRequest('POST', payload))
}

function updatePromptGroup(projectUuid, promptGroup, payload) {
  return apiRequest(projectPath(projectUuid, `/prompt-groups/${encodeURIComponent(promptGroup)}`), jsonRequest('PATCH', payload))
}

function restorePromptVersion(projectUuid, versionUuid, expectedCurrentVersion) {
  return apiRequest(projectPath(projectUuid, `/prompt-versions/${encodeURIComponent(versionUuid)}/restorations`), jsonRequest('POST', { expected_current_version: expectedCurrentVersion }))
}

function listChapterSummaries(projectUuid,state='active',options={}) { return readSummaryPages(apiRequest,projectPath(projectUuid,'/chapters'),{state},options) }
function getChapterSummary(projectUuid,chapterUuid,options={}) { return apiRequest(projectPath(projectUuid,`/chapters/${encodeURIComponent(chapterUuid)}?view=summary`),options) }
function getStoryProfileSummary(projectUuid,options={}) { return apiRequest(projectPath(projectUuid,'/story-profile?view=summary'),options) }

  return { listChapterSummaries, getChapterSummary, getStoryProfileSummary, getStoryProject, updateStoryProject, listChapters, getChapter, createChapter, reorderChapters, updateChapter, updateChapterStory, listChapterStories, trashChapter, restoreChapter, permanentlyDeleteChapter, emptyChapterTrash, importChapters, getStoryProfile, updateStoryProfile, listStoryProfileVersions, restoreStoryProfileVersion, importExternalStoryMD, regenerateStoryMD, listPromptCatalog, listPromptVersions, createPromptVersion, updatePromptGroup, restorePromptVersion }
}

export const { listChapterSummaries, getChapterSummary, getStoryProfileSummary, getStoryProject, updateStoryProject, listChapters, getChapter, createChapter, reorderChapters, updateChapter, updateChapterStory, listChapterStories, trashChapter, restoreChapter, permanentlyDeleteChapter, emptyChapterTrash, importChapters, getStoryProfile, updateStoryProfile, listStoryProfileVersions, restoreStoryProfileVersion, importExternalStoryMD, regenerateStoryMD, listPromptCatalog, listPromptVersions, createPromptVersion, updatePromptGroup, restorePromptVersion } = createStoryAPI()
