import { apiRequest } from './client.js'

function jsonRequest(method, body = {}) {
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

function projectPath(projectUuid, suffix = '') {
  return `/api/v1/projects/${encodeURIComponent(projectUuid)}${suffix}`
}

function threadPath(projectUuid, threadUuid = '', suffix = '') {
  const resource = threadUuid ? `/${encodeURIComponent(threadUuid)}` : ''
  return projectPath(projectUuid, `/chat_threads${resource}${suffix}`)
}

export function listChatThreads(projectUuid, { page = 1, perPage = 30 } = {}) {
  const search = new URLSearchParams()
  search.set('page', String(page))
  search.set('per_page', String(perPage))
  const query = search.size ? `?${search}` : ''
  return apiRequest(`${threadPath(projectUuid)}${query}`)
}

export function createChatThread(projectUuid, payload) {
  return apiRequest(threadPath(projectUuid), jsonRequest('POST', payload))
}

export function getChatThread(projectUuid, threadUuid) {
  return apiRequest(threadPath(projectUuid, threadUuid))
}

export function listChatTurns(projectUuid, threadUuid) {
  return apiRequest(threadPath(projectUuid, threadUuid, '/turns'))
}

export function createChatTurn(projectUuid, threadUuid, payload) {
  return apiRequest(threadPath(projectUuid, threadUuid, '/turns'), jsonRequest('POST', payload))
}

export function listChatItems(projectUuid, threadUuid, { before = '', after = '', limit = 100 } = {}) {
  const search = new URLSearchParams({ limit: String(limit) })
  if (before) search.set('before', before)
  if (after) search.set('after', after)
  return apiRequest(threadPath(projectUuid, threadUuid, `/items?${search}`))
}

export function listChatEvents(projectUuid, threadUuid, { after = '', limit = 100 } = {}) {
  const search = new URLSearchParams({ limit: String(limit) })
  if (after) search.set('after', after)
  return apiRequest(threadPath(projectUuid, threadUuid, `/events?${search}`))
}

export function getChatTrajectory(projectUuid, threadUuid, { before = '', after = '', limit = 80, itemUuid = '' } = {}) {
  const search = new URLSearchParams({ limit: String(limit) })
  if (before) search.set('before', before)
  if (after) search.set('after', after)
  if (itemUuid) search.set('item_uuid', itemUuid)
  return apiRequest(threadPath(projectUuid, threadUuid, `/trajectory?${search}`))
}

export function listFollowUps(projectUuid, threadUuid) {
  return apiRequest(threadPath(projectUuid, threadUuid, '/follow_ups'))
}

export function createFollowUp(projectUuid, threadUuid, inputText, uploadUuids = []) {
  return apiRequest(threadPath(projectUuid, threadUuid, '/follow_ups'), jsonRequest('POST', { input_text: inputText, upload_uuids: uploadUuids }))
}

export function updateFollowUp(projectUuid, threadUuid, followUpUuid, inputText) {
  return apiRequest(threadPath(projectUuid, threadUuid, `/follow_ups/${encodeURIComponent(followUpUuid)}`), jsonRequest('PATCH', { input_text: inputText }))
}

export function moveFollowUp(projectUuid, threadUuid, followUpUuid, position) {
  return apiRequest(threadPath(projectUuid, threadUuid, `/follow_ups/${encodeURIComponent(followUpUuid)}/position`), jsonRequest('PATCH', { position }))
}

export function deleteFollowUp(projectUuid, threadUuid, followUpUuid) {
  return apiRequest(threadPath(projectUuid, threadUuid, `/follow_ups/${encodeURIComponent(followUpUuid)}`), { method: 'DELETE' })
}

export function steerFollowUp(projectUuid, threadUuid, followUpUuid) {
  return apiRequest(threadPath(projectUuid, threadUuid, `/follow_ups/${encodeURIComponent(followUpUuid)}/steerings`), jsonRequest('POST'))
}

export function steerChatRun(projectUuid, threadUuid, inputText, uploadUuids = []) {
  return apiRequest(threadPath(projectUuid, threadUuid, '/steerings'), jsonRequest('POST', { input_text: inputText, upload_uuids: uploadUuids }))
}

export function abortChatTurn(projectUuid, threadUuid) {
  return apiRequest(threadPath(projectUuid, threadUuid, '/cancellations'), jsonRequest('POST'))
}

export function listUserInputRequests(projectUuid, threadUuid) {
  return apiRequest(threadPath(projectUuid, threadUuid, '/user_input_requests'))
}

export function respondUserInput(projectUuid, threadUuid, requestUuid, payload) {
  return apiRequest(threadPath(projectUuid, threadUuid, `/user_input_requests/${encodeURIComponent(requestUuid)}/responses`), jsonRequest('POST', payload))
}

export function cancelUserInput(projectUuid, threadUuid, requestUuid) {
  return apiRequest(threadPath(projectUuid, threadUuid, `/user_input_requests/${encodeURIComponent(requestUuid)}/cancellations`), jsonRequest('POST'))
}

export function listWorkflows(projectUuid) {
  return apiRequest(projectPath(projectUuid, '/workflows'))
}

export function createYoloWorkflow(projectUuid, payload) {
  return apiRequest(projectPath(projectUuid, '/workflows'), jsonRequest('POST', payload))
}

export function getWorkflow(projectUuid, workflowUuid) {
  return apiRequest(projectPath(projectUuid, `/workflows/${encodeURIComponent(workflowUuid)}`))
}

export function listWorkflowRuns(projectUuid, workflowUuid, { before = '', limit = 50 } = {}) {
  const search = new URLSearchParams({ limit: String(limit) })
  if (before) search.set('before', before)
  return apiRequest(projectPath(projectUuid, `/workflows/${encodeURIComponent(workflowUuid)}/runs?${search}`))
}

export function listWorkflowEvents(projectUuid, workflowUuid, { before = '', after = '', limit = 100 } = {}) {
  const search = new URLSearchParams({ limit: String(limit) })
  if (before) search.set('before', before)
  if (after) search.set('after', after)
  return apiRequest(projectPath(projectUuid, `/workflows/${encodeURIComponent(workflowUuid)}/events?${search}`))
}

export function listWorkflowLLMLogs(projectUuid, workflowUuid, { page = 1, perPage = 20, stepUuid = '' } = {}) {
  const search = new URLSearchParams({ page: String(page), per_page: String(perPage) })
  if (stepUuid) search.set('workflow_step_uuid', stepUuid)
  return apiRequest(projectPath(projectUuid, `/workflows/${encodeURIComponent(workflowUuid)}/llm-logs?${search}`))
}

export function cancelWorkflow(projectUuid, workflowUuid) {
  return apiRequest(projectPath(projectUuid, `/workflows/${encodeURIComponent(workflowUuid)}/cancellations`), jsonRequest('POST'))
}

export function retryWorkflow(projectUuid, workflowUuid) {
  return apiRequest(projectPath(projectUuid, `/workflows/${encodeURIComponent(workflowUuid)}/retries`), jsonRequest('POST'))
}

export function resolveWorkflowConflict(projectUuid, workflowUuid, payload) {
  return apiRequest(projectPath(projectUuid, `/workflows/${encodeURIComponent(workflowUuid)}/conflict-resolutions`), jsonRequest('POST', payload))
}
