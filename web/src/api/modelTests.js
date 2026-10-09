import { apiRequest } from './client.js'

const path = (uuid) => `/api/v1/model-tests/${encodeURIComponent(uuid)}`
const json = (method, body) => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })

export const modelTestKeys = {
  all: ['model-tests'],
  lists: ['model-tests', 'list'],
  list: (page) => ['model-tests', 'list', page],
  detail: (uuid) => ['model-tests', 'detail', uuid],
}

export const listModelTests = (page = 1) => apiRequest(`/api/v1/model-tests?page=${page}&per_page=20`)
export const getModelTest = (uuid) => apiRequest(path(uuid))
export const createModelTest = (selection) => apiRequest('/api/v1/model-tests', json('POST', selection))
export const executeModelTest = (uuid, selection) => apiRequest(`${path(uuid)}/execution`, json('PUT', selection))
export const stopModelTest = (uuid) => apiRequest(`${path(uuid)}/execution`, { method: 'DELETE' })
