import { QueryClient } from '@tanstack/react-query'
import { createAppRequest, workspaceLaunch } from '../mcp-app/workspace.js'
import { createWorkspaceAPI } from '../src/workspace/services.js'
import { PreviewCache } from '../mcp-app/images.js'
import { ApiError } from '../src/api/client.js'

const allowed = new Set(['read_app_resource', 'write_app_resource', 'read_app_image', 'watch_app_changes'])
const unavailable = message => new ApiError({ code: 'workspace_capability_unavailable', message, details: '请在当前 Cindy 任务对话中继续；导出文件通过任务文件列表交付。' }, 501)

// transport is supplied by a verified Host connector (or an explicit isolated
// test harness). Neither a query string nor a BroadcastChannel creates one.
export function createCindySession(result, transport) {
  if (typeof transport?.call !== 'function') throw unavailable('当前 Cindy 尚无已核验的页面任务授权通道')
  const launch = workspaceLaunch(result)
  const controller = new AbortController()
  const bridge = {
    async call(name, args, options = {}) {
      if (!allowed.has(name)) throw unavailable('此操作不属于作品工作区')
      const project = options.meta?.['lumi/expected_project_uuid'] || args?.project_uuid || launch.project_uuid
      if (project !== launch.project_uuid) throw new ApiError({ code: 'app_project_mismatch', message: '请求不属于当前页面项目' }, 403)
      const response = await transport.call(name, args, { signal: AbortSignal.any([controller.signal, ...(options.signal ? [options.signal] : [])]), project })
      if (controller.signal.aborted) throw new ApiError({ code: name === 'write_app_resource' ? 'result_unknown' : 'workspace_revoked', message: '页面已关闭，请在当前任务重读事实' }, 409)
      return name === 'watch_app_changes' ? { structuredContent: response, isError: !response.success } : { _meta: { 'lumi/app_response': response }, isError: !response.success }
    },
    request(method, params, options) {
      if (method !== 'tools/call') throw unavailable('此宿主请求不可用')
      return bridge.call(params.name, params.arguments, options)
    },
  }
  const request = createAppRequest(bridge, launch.project_uuid, { canWrite: launch.permissions?.can_write === true })
  const api = createWorkspaceAPI(async (path, options = {}) => {
    if (options.body instanceof FormData) throw unavailable('此 Cindy 页面尚未接入授权文件选择，请通过任务附件导入')
    if ((options.method || 'GET') !== 'GET' && /\/(?:comic-exports|[^/]*generations|[^/]*generation-batches|[^/]*reconstructions)(?:[/?]|$)|\/retries$/.test(path)) throw unavailable('此 Cindy 页面尚未接入生成或导出操作')
    try { return await request(path, options) }
    catch (error) { if (error.code === 'workspace_capability_unavailable') throw unavailable(error.message); throw error }
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, refetchOnReconnect: false, staleTime: Infinity, gcTime: 300000 }, mutations: { retry: false } } })
  const previews = new PreviewCache((...args) => bridge.call(...args))
  let closing
  return { key: crypto.randomUUID(), launch, bridge, api, client, previews,
    close() {
      if (!closing) {
        controller.abort()
        closing = (async () => {
          await client.cancelQueries(); client.clear()
          previews.cache.clear(); previews.bytes = 0
          await transport.close?.()
        })()
      }
      return closing
    },
  }
}
