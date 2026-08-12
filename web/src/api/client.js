import { requireDesktopAuthentication } from '../desktopSession.js'

export class ApiError extends Error {
  constructor(error, status) {
    super(error?.message || '请求失败')
    this.name = 'ApiError'
    this.code = error?.code || 'unknown_error'
    this.details = error?.details || ''
    this.status = status
  }
}

const projectOpenRequests = new Map()

export async function apiRequest(path, options = {}) {
  return apiRequestWithRecovery(path, options, true)
}

async function apiRequestWithRecovery(path, options, allowRecovery) {
	const response = await fetch(path, {
    ...options,
    headers: {
      Accept: 'application/json',
      ...options.headers,
    },
  })

  let payload
  try {
    payload = await response.json()
  } catch {
    throw new ApiError({
      code: 'invalid_response',
      message: '服务端返回了无效响应',
      details: `HTTP ${response.status}`,
    }, response.status)
  }

  if (!response.ok || payload?.success !== true) {
    if (response.status === 401 && payload?.error?.code === 'desktop_authentication_required') {
      requireDesktopAuthentication()
    }
    const error = new ApiError(payload?.error, response.status)
    const projectUuid = allowRecovery && error.code === 'project_not_open' ? projectUuidFromPath(path) : ''
    if (projectUuid) {
      await ensureProjectOpenForRetry(projectUuid)
      return apiRequestWithRecovery(path, options, false)
    }
    throw error
  }
  return payload.data
}

function projectUuidFromPath(path) {
  if (typeof path !== 'string') return ''
  const match = path.match(/^\/api\/v1\/projects\/([^/?#]+)/)
  if (!match) return ''
  try { return decodeURIComponent(match[1]) } catch { return '' }
}

export function ensureProjectOpenForRetry(projectUuid) {
  let pending = projectOpenRequests.get(projectUuid)
  if (pending) return pending
  pending = apiRequestWithRecovery(`/api/v1/open-projects/${encodeURIComponent(projectUuid)}`, { method: 'PUT' }, false)
    .finally(() => projectOpenRequests.delete(projectUuid))
  projectOpenRequests.set(projectUuid, pending)
  return pending
}
