import { apiRequest as webRequest } from './client.js'

// One endpoint contract; callers inject only the transport.
export function createAiAPI(apiRequest = webRequest) {

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

function listProviders() {
  return apiRequest(providerPath())
}

function getActiveProvider() {
  return apiRequest(providerPath('', '/active'))
}

function getModelSettings() {
  return apiRequest('/api/v1/model-settings')
}

function updateModelSettings(expectedRevision, overrides) {
  return apiRequest('/api/v1/model-settings', jsonRequest('PATCH', {
    expected_revision: expectedRevision,
    overrides,
  }))
}

function updateModelRateLimits(expectedRevision, rateLimitUpdates) {
  return apiRequest('/api/v1/model-settings', jsonRequest('PATCH', {
    expected_revision: expectedRevision,
    rate_limit_updates: rateLimitUpdates,
  }))
}

function getProjectModelSettings(projectUuid) {
  return apiRequest(taskPath(projectUuid, '/model-settings'))
}

function updateProjectModelSettings(projectUuid, expectedRevision, overrides) {
  return apiRequest(taskPath(projectUuid, '/model-settings'), jsonRequest('PATCH', {
    expected_revision: expectedRevision,
    overrides,
  }))
}

function getSiteSettings() {
  return apiRequest('/api/v1/site-settings')
}

function updateSiteSettings(settings) {
  return apiRequest('/api/v1/site-settings', jsonRequest('PATCH', { settings }))
}

function resetSiteSettings(keys) {
  return apiRequest('/api/v1/site-settings/resets', jsonRequest('POST', { keys }))
}

function checkProvider(providerUuid) {
  return apiRequest(providerPath(providerUuid, '/connection-checks'), jsonRequest('POST'))
}

function listProjectLLMLogs(projectUuid, {
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
  from = '',
  to = '',
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
  if (from) query.set('from', from)
  if (to) query.set('to', to)
  return apiRequest(`/api/v1/projects/${encodeURIComponent(projectUuid)}/llm-logs?${query}`)
}

function getProjectLLMLog(projectUuid, logUuid) {
  return apiRequest(`/api/v1/projects/${encodeURIComponent(projectUuid)}/llm-logs/${encodeURIComponent(logUuid)}`)
}

function createChapterGeneration(projectUuid, chapterUuid, payload) {
  return apiRequest(taskPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/generations`), jsonRequest('POST', payload))
}

function createStoryProfileGeneration(projectUuid, payload) {
  return apiRequest(taskPath(projectUuid, '/story-profile/generations'), jsonRequest('POST', payload))
}

function createStoryProfileReconstruction(projectUuid, payload) {
  return apiRequest(taskPath(projectUuid, '/story-profile/reconstructions'), jsonRequest('POST', payload))
}

function createChapterBatch(projectUuid, payload) {
  return apiRequest(taskPath(projectUuid, '/chapter-batches'), jsonRequest('POST', payload))
}

function createComicStoryboardGeneration(projectUuid, chapterUuid, payload) {
  return apiRequest(taskPath(projectUuid, `/chapters/${encodeURIComponent(chapterUuid)}/comic-storyboard-generations`), jsonRequest('POST', payload))
}

function listTasks(projectUuid, { status = '', limit = 50 } = {}) {
  const search = new URLSearchParams({ limit: String(limit) })
  if (status) search.set('status', status)
  return apiRequest(taskPath(projectUuid, `/tasks?${search}`))
}

function getTask(projectUuid, taskUuid) {
  return apiRequest(taskPath(projectUuid, `/tasks/${encodeURIComponent(taskUuid)}`))
}

function listTaskEvents(projectUuid, taskUuid, { before = 0, after = 0, limit = 50 } = {}) {
  const search = new URLSearchParams()
  if (before) search.set('before', String(before))
  else search.set('after', String(after))
  search.set('limit', String(limit))
  return apiRequest(taskPath(projectUuid, `/tasks/${encodeURIComponent(taskUuid)}/events?${search}`))
}

function cancelTask(projectUuid, taskUuid) {
  return apiRequest(taskPath(projectUuid, `/tasks/${encodeURIComponent(taskUuid)}/cancellations`), jsonRequest('POST'))
}

function retryTask(projectUuid, taskUuid) {
  return apiRequest(taskPath(projectUuid, `/tasks/${encodeURIComponent(taskUuid)}/retries`), jsonRequest('POST'))
}

  return { listProviders, getActiveProvider, getModelSettings, updateModelSettings, updateModelRateLimits, getProjectModelSettings, updateProjectModelSettings, getSiteSettings, updateSiteSettings, resetSiteSettings, checkProvider, listProjectLLMLogs, getProjectLLMLog, createChapterGeneration, createStoryProfileGeneration, createStoryProfileReconstruction, createChapterBatch, createComicStoryboardGeneration, listTasks, getTask, listTaskEvents, cancelTask, retryTask }
}

export const { listProviders, getActiveProvider, getModelSettings, updateModelSettings, updateModelRateLimits, getProjectModelSettings, updateProjectModelSettings, getSiteSettings, updateSiteSettings, resetSiteSettings, checkProvider, listProjectLLMLogs, getProjectLLMLog, createChapterGeneration, createStoryProfileGeneration, createStoryProfileReconstruction, createChapterBatch, createComicStoryboardGeneration, listTasks, getTask, listTaskEvents, cancelTask, retryTask } = createAiAPI()
