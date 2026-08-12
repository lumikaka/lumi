import { apiRequest, ensureProjectOpenForRetry } from './client.js'

function jsonRequest(method, body) {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export async function listRecentProjects() {
  return apiRequest('/api/v1/recent-projects')
}

export async function listOpenProjects() {
  return apiRequest('/api/v1/open-projects')
}

export async function getProjectDefaults() {
  return apiRequest('/api/v1/project-defaults')
}

export async function createProject({ name, parentPath, generationLanguage = 'zh-Hans', pictureBook }) {
  return apiRequest('/api/v1/projects', jsonRequest('POST', { name, parent_path: parentPath, generation_language: generationLanguage, picture_book: pictureBook }))
}

export async function preflightImageGeneration(pictureBook) {
  return apiRequest('/api/v1/image-generation-preflights', jsonRequest('POST', { picture_book: pictureBook }))
}

export async function preflightProjectImageGeneration(projectUuid) {
  return apiRequest(`/api/v1/projects/${encodeURIComponent(projectUuid)}/image-generation-preflights`, jsonRequest('POST', {}))
}

export async function ensureProjectOpen(uuid) {
  return ensureProjectOpenForRetry(uuid)
}

export async function openProjectPath(rootPath) {
  return apiRequest('/api/v1/open-projects', jsonRequest('POST', { root_path: rootPath }))
}

export async function selectProjectDirectory(initialPath = '') {
  return apiRequest('/api/v1/directory-selections', jsonRequest('POST', { initial_path: initialPath }))
}

export async function relocateRecentProject({ uuid, rootPath }) {
  return apiRequest(`/api/v1/recent-projects/${encodeURIComponent(uuid)}`, jsonRequest('PATCH', { root_path: rootPath }))
}

export async function forgetRecentProject(uuid) {
  return apiRequest(`/api/v1/recent-projects/${encodeURIComponent(uuid)}`, { method: 'DELETE' })
}
