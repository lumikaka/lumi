import { appResponse } from '../mcp-app/workspace.js'
import { modelContextParams } from '../mcp-app/model-context.js'

async function hostRequest(run) {
  try { return await run() }
  catch (error) {
    // An SDK timeout is uncertain: preserve ModelContextAction's rule that a
    // queued click cannot overtake a possibly still applying context update.
    if (error.code === -32001 || error.name === 'TimeoutError') error.kind = 'timeout'
    else if (error.name === 'AbortError') error.kind = 'cancelled'
    else if (error.code === -32601) error.kind = 'unsupported'
    throw error
  }
}
// The official SDK connects to the Host-owned tool-result card. No session,
// directory or permission supplied by this page becomes an authority source.
export function createHostTransport(app) {
  const controller = new AbortController()
  const options = signal => ({ signal: AbortSignal.any([controller.signal, ...(signal ? [signal] : [])]), timeout: 60000, maxTotalTimeout: 60000 })
  const initialization = () => ({ hostCapabilities: app.getHostCapabilities(), hostInfo: { name: 'Cindy' } })
  return {
    get initialization() { return initialization() },
    async call(name, args, { project, signal } = {}) {
      const response = await hostRequest(() => app.callServerTool({ name, arguments: { ...args, expected_project_uuid: project } }, options(signal)))
      if (name === 'watch_app_changes') {
        const result = response.structuredContent
        if (!result || typeof result.success !== 'boolean') throw new Error('宿主未转交实时提示结果')
        return result
      }
      // appResponse validates failures; retain the complete success envelope
      // so the existing session adapter can enforce its lifecycle as well.
      appResponse(response)
      return response._meta['lumi/app_response']
    },
    async updateModelContext(context, { signal } = {}) {
      return hostRequest(() => app.updateModelContext(modelContextParams(initialization(), context), { ...options(signal), timeout: 10000, maxTotalTimeout: 10000 }))
    },
    async request(method, params, { signal, onExchange } = {}) {
      if (method !== 'ui/open-link') throw new Error('此宿主请求不可用')
      onExchange?.({ stage: 'sending', request: { method, params } })
      const result = await hostRequest(() => app.openLink(params, options(signal)))
      onExchange?.({ stage: 'response', response: { result } })
      return result
    },
    close() { controller.abort() },
  }
}

export function applyHostTheme(context, document = globalThis.document) {
  if (!document) return
  if (context?.theme) document.documentElement.style.colorScheme = context.theme
  for (const [key, value] of Object.entries(context?.styles?.variables || {})) document.documentElement.style.setProperty(key, value)
  const variables = context?.styles?.variables || {}
  for (const [role, standard] of Object.entries({
    '--text-primary': '--color-text-primary', '--text-secondary': '--color-text-secondary',
    '--surface': '--color-background-primary', '--panel-bg': '--color-background-secondary',
    '--hover-bg': '--color-background-tertiary', '--active-bg': '--color-background-info',
    '--border-default': '--color-border-primary', '--accent': '--color-text-info',
    '--text-on-accent': '--color-text-inverse', '--danger': '--color-text-danger',
    '--warning': '--color-text-warning', '--success': '--color-text-success',
  })) if (!(role in variables) && standard in variables) document.documentElement.style.setProperty(role, `var(${standard})`)
}
