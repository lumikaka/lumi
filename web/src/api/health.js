import { apiRequest } from './client.js'

export function getHealth() {
  return apiRequest('/api/v1/health')
}
