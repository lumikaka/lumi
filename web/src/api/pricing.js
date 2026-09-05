import { apiRequest } from './client.js'

const json = (method, body = {}) => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
const projectPath = (uuid, suffix) => `/api/v1/projects/${encodeURIComponent(uuid)}${suffix}`
export const listModelPrices = () => apiRequest('/api/v1/model-prices')
export const createModelPrice = (rule, expectedUuid = '') => apiRequest('/api/v1/model-prices', json('POST', { rule, expected_uuid: expectedUuid }))
export const deleteModelPrice = (uuid) => apiRequest(`/api/v1/model-prices/${encodeURIComponent(uuid)}`, json('DELETE'))
export function costFilter(filters = {}) {
  const names = { providerUuid: 'provider_uuid', providerType: 'provider_type', requestType: 'request_type' }
  return Object.fromEntries(Object.entries(filters).filter(([, value]) => value !== '' && value != null).map(([key, value]) => [names[key] || key, value]))
}
export const getCostSummary = (uuid, filters) => apiRequest(projectPath(uuid, `/llm-cost-summary?${new URLSearchParams(costFilter(filters))}`))
export const listCostBackfills = (uuid) => apiRequest(projectPath(uuid, '/llm-cost-backfills'))
export const previewCostBackfill = (uuid, filters, priceUuids) => apiRequest(projectPath(uuid, '/llm-cost-backfills'), json('POST', { filter: costFilter(filters), price_uuids: priceUuids }))
export const applyCostBackfill = (uuid, backfillUuid) => apiRequest(projectPath(uuid, `/llm-cost-backfills/${encodeURIComponent(backfillUuid)}/applications`), json('POST'))
