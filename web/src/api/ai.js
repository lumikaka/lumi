import { apiRequest } from './client.js'

function jsonRequest(method, body = {}) {
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

function providerPath(providerUuid = '', suffix = '') {
  const resource = providerUuid ? `/${encodeURIComponent(providerUuid)}` : ''
  return `/api/v1/providers${resource}${suffix}`
}

function taskPath(projectUuid, suffix = '') {
  return `/api/v1/projects/${encodeURIComponent(projectUuid)}${suffix}`
}

export function listProviders() {
  return apiRequest(providerPath())
}

export function getActiveProvider() {
  return apiRequest(providerPath('', '/active'))
}

export function getProjectModelSettings(projectUuid) {
  return apiRequest(taskPath(projectUuid, '/model-settings'))
}

export function updateProjectModelSettings(projectUuid, expectedRevision, overrides) {
  return apiRequest(taskPath(projectUuid, '/model-settings'), jsonRequest('PATCH', {
    expected_revision: expectedRevision,
    overrides,
  }))
}

export function getSiteSettings() {
  return apiRequest('/api/v1/site-settings')
}

export function updateSiteSettings(settings) {
  return apiRequest('/api/v1/site-settings', jsonRequest('PATCH', { settings }))
}

export function resetSiteSettings(keys) {
  return apiRequest('/api/v1/site-settings/resets', jsonRequest('POST', { keys }))
}

export function checkProvider(providerUuid) {
  return apiRequest(providerPath(providerUuid, '/connection-checks'), jsonRequest('POST'))
}

export function listProjectLLMLogs(projectUuid, {
  page = 1,
  perPage = 12,
  scope = '',
  providerUuid = '',
  providerType = '',
  model = '',
  scenario = '',
  status = '',
  requestType = '',
  keyword = '',
} = {}) {
  const query = new URLSearchParams({ page: String(page), per_page: String(perPage) })
  if (scope) query.set('scope', scope)
  if (providerUuid) query.set('provider_uuid', providerUuid)
  if (providerType) query.set('provider_type', providerType)
  if (model) query.set('model', model)
  if (scenario) query.set('scenario', scenario)
  if (status) query.set('status', status)
  if (requestType) query.set('request_type', requestType)
  if (keyword) query.set('keyword', keyword)
  return apiRequest(`/api/v1/projects/${encodeURIComponent(projectUuid)}/llm-logs?${query}`)
}

export function getProjectLLMLog(projectUuid, logUuid) {
  return apiRequest(`/api/v1/projects/${encodeURIComponent(projectUuid)}/llm-logs/${encodeURIComponent(logUuid)}`)
}

export function createChapterGeneration(projectUuid, chapterUuid, payload) {
  return apiRequest(taskPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/generations`), jsonRequest('POST', payload))
}

export function createStoryProfileGeneration(projectUuid, payload) {
  return apiRequest(taskPath(projectUuid, '/story-profile/generations'), jsonRequest('POST', payload))
}

export function createStoryProfileReconstruction(projectUuid, payload) {
  return apiRequest(taskPath(projectUuid, '/story-profile/reconstructions'), jsonRequest('POST', payload))
}

export function createChapterBatch(projectUuid, payload) {
  return apiRequest(taskPath(projectUuid, '/chapter-batches'), jsonRequest('POST', payload))
}

export function createComicStoryboardGeneration(projectUuid, chapterUuid, payload) {
  return apiRequest(taskPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/comic-storyboard-generations`), jsonRequest('POST', payload))
}

export function listTasks(projectUuid, { status = '', limit = 50 } = {}) {
  const search = new URLSearchParams({ limit: String(limit) })
  if (status) search.set('status', status)
  return apiRequest(taskPath(projectUuid, `/tasks?${search}`))
}

export function getTask(projectUuid, taskUuid) {
  return apiRequest(taskPath(projectUuid, `/tasks/${encodeURIComponent(taskUuid)}`))
}

export function listTaskEvents(projectUuid, taskUuid, { before = 0, after = 0, limit = 50 } = {}) {
  const search = new URLSearchParams()
  if (before) search.set('before', String(before))
  else search.set('after', String(after))
  search.set('limit', String(limit))
  return apiRequest(taskPath(projectUuid, `/tasks/${encodeURIComponent(taskUuid)}/events?${search}`))
}

export function cancelTask(projectUuid, taskUuid) {
  return apiRequest(taskPath(projectUuid, `/tasks/${encodeURIComponent(taskUuid)}/cancellations`), jsonRequest('POST'))
}

export function retryTask(projectUuid, taskUuid) {
  return apiRequest(taskPath(projectUuid, `/tasks/${encodeURIComponent(taskUuid)}/retries`), jsonRequest('POST'))
}
