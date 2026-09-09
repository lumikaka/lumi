import { apiRequest } from './client.js'
const path = (projectUuid, suffix) => `/api/v1/projects/${encodeURIComponent(projectUuid)}/${suffix}`
const json = (method, body) => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
export const listMCPGrants = (projectUuid) => apiRequest(path(projectUuid, 'mcp-grants'))
export const createMCPGrant = (projectUuid, body) => apiRequest(path(projectUuid, 'mcp-grants'), json('POST', body))
export const revokeMCPGrant = (projectUuid, uuid) => apiRequest(path(projectUuid, `mcp-grants/${encodeURIComponent(uuid)}`), { method: 'DELETE' })
export const listMCPCalls = (projectUuid) => apiRequest(path(projectUuid, 'mcp-calls'))
export const decideMCPCall = (projectUuid, call, decision) => apiRequest(path(projectUuid, `mcp-calls/${encodeURIComponent(call.uuid)}/decisions`), json('POST', { decision, fingerprint: call.fingerprint }))
export const getMCPConnection = () => apiRequest('/api/v1/mcp')
export const getMCPAuthorization = (uuid) => apiRequest(`/api/v1/mcp-authorization-requests/${encodeURIComponent(uuid)}`)
export const decideMCPAuthorization = (uuid, body) => apiRequest(`/api/v1/mcp-authorization-requests/${encodeURIComponent(uuid)}/decisions`, json('POST', body))
