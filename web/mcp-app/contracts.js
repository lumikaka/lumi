export const uuidV7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const views = new Set(['book', 'page', 'candidates', 'settings'])
const revision = (n) => Number.isSafeInteger(n) && n >= 0
export function envelope(result) {
  if (result?.structuredContent) return result.structuredContent
  // Only our two observed JSON text shapes; one parse, no recursive guessing.
  const blocks = result?.content
  if (blocks?.length !== 1 || blocks[0].type !== 'text' || blocks[0].text.length > 262144) throw new Error('无法读取工具结果')
  const parsed = JSON.parse(blocks[0].text)
  return parsed?.structuredContent ?? parsed
}
export function success(result) {
  const e = envelope(result)
  if (e?.success === false && e.data === null && typeof e.error?.code === 'string') {
    throw Object.assign(new Error(e.error.message), { kind: 'resource_error', details: e.error.details, code: e.error.code })
  }
  if (result?.isError || e?.success !== true || !e.data || e.error) throw new Error('工具信封无效')
  return e.data
}
export function reviewContext(result) {
  const d = success(result)
  if (!views.has(d.view) || !uuidV7.test(d.project_uuid) || !uuidV7.test(d.target_uuid) || !revision(d.revision) ||
      d.request?.name !== `review_${d.view}` || !d.request.arguments || !Array.isArray(d.items) ||
      typeof d.title !== 'string' || !safeOpenURL(d.open_url, d.project_uuid)) throw new Error('审阅上下文无效，请在 Lumi 中打开')
  for (const item of d.items) {
    if (!uuidV7.test(item.uuid) || !revision(item.revision) || !safeOpenURL(item.open_url, d.project_uuid) ||
        !['title', 'text', 'scene', 'kind'].every((k) => typeof item[k] === 'string') ||
        (item.asset_uuid && !uuidV7.test(item.asset_uuid)) || (item.chapter_uuid && !uuidV7.test(item.chapter_uuid))) throw new Error('审阅项目无效')
    const numbers = new Set()
    for (const v of item.variants ?? []) {
      if (!Number.isSafeInteger(v.number) || v.number < 1 || numbers.has(v.number) || typeof v.title !== 'string' || typeof v.text !== 'string' ||
          (v.asset_uuid && (!uuidV7.test(v.asset_uuid) || !uuidV7.test(v.uuid)))) throw new Error('候选无效')
      numbers.add(v.number)
    }
  }
  const a = d.request.arguments
  if (Array.isArray(a) || typeof a !== 'object') throw new Error('审阅请求无效')
  if (d.view === 'book' && d.target_uuid !== d.project_uuid) throw new Error('整本目标无效')
  if (d.view === 'page' || d.view === 'candidates') {
    const item = d.items[0]
    if (d.items.length !== 1 || item.uuid !== d.target_uuid || item.revision !== d.revision) throw new Error('目标修订不匹配')
    if (d.view === 'page' || a.kind === 'page') {
      if (a.page_uuid !== d.target_uuid || !uuidV7.test(a.chapter_uuid) || item.chapter_uuid !== a.chapter_uuid) throw new Error('页面目标不匹配')
    } else if (a.kind === 'setting') {
      if (a.setting_uuid !== d.target_uuid) throw new Error('设定目标不匹配')
    } else if (a.kind === 'text') {
      if (d.target_uuid !== d.project_uuid || !Array.isArray(a.candidates) || a.candidates.length < 2 || a.candidates.length > 12 || item.variants?.length !== a.candidates.length) throw new Error('文本候选不匹配')
    } else throw new Error('候选类型无效')
  }
  if (d.view === 'settings' && (d.target_uuid !== d.project_uuid || (a.setting_uuid && (!uuidV7.test(a.setting_uuid) || d.items.length !== 1 || d.items[0].uuid !== a.setting_uuid)))) throw new Error('设定集合不匹配')
  return d
}
export function safeOpenURL(value, project) {
  try { const u = new URL(value); return u.protocol === 'http:' && u.hostname === '127.0.0.1' && /^\d+$/.test(u.port) && !u.username && !u.password && u.pathname.startsWith(`/projects/${project}/`) } catch { return false }
}
export function imageData(result, asset, edge) {
  const d = success(result)
  if (d.asset_uuid !== asset || d.mime_type !== 'image/jpeg' || !Number.isInteger(d.width) || !Number.isInteger(d.height) ||
      Math.min(d.width, d.height) < 1 || Math.max(d.width, d.height) > edge || typeof d.data_uri !== 'string' ||
      !/^data:image\/jpeg;base64,[A-Za-z0-9+/]+={0,2}$/.test(d.data_uri) || d.data_uri.length > 23 + 163840) throw Object.assign(new Error('图片预览格式无效'), { kind: 'resource_error' })
  return d
}
export function failureKind(error) {
  if (error?.kind) return error.kind
  if (['permission_denied', 'user_rejected', 'host_denied'].includes(error?.code) || /\b(denied|rejected)\b|拒绝/.test(error?.message ?? '')) return 'host_denied'
  return 'host_error'
}
export const failureLabels = { resource_error: '资源读取失败', host_denied: '宿主拒绝了请求', host_error: '宿主连接失败', timeout: '请求超时', decode_error: '图片解码失败' }

// Host notifications may repeat the same review after the business data has
// changed. initialData alone cannot replace a previously cached query.
export function acceptReviewResult(queryClient, result) {
  const next = reviewContext(result)
  queryClient.setQueryData(['app-review', next.project_uuid, next.request], next)
  return next
}
