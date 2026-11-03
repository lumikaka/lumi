export const uuidV7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const views = new Set(['book', 'chapters', 'chapter', 'reader', 'page', 'candidates', 'settings'])
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
      d.request?.name !== (d.view === 'reader' ? 'review_book' : `review_${d.view}`) || !d.request.arguments || !Array.isArray(d.items) ||
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
  for (const key of ['chapter_uuid', 'page_uuid', 'setting_uuid']) if (a[key] !== undefined && !uuidV7.test(a[key])) throw new Error('请求 UUID 无效')
  if (Array.isArray(a) || typeof a !== 'object') throw new Error('审阅请求无效')
  if (['book', 'chapters', 'reader'].includes(d.view) && d.target_uuid !== d.project_uuid) throw new Error('整本目标无效')
  if (isReader(d)) {
    if ((a.chapter_uuid && !uuidV7.test(a.chapter_uuid)) ||
        (d.scope !== undefined && d.scope !== (a.chapter_uuid ? 'chapter' : 'book')) ||
        (d.total !== undefined && (!Number.isSafeInteger(d.total) || d.total < d.items.length))) throw new Error('预览范围无效')
    for (const item of d.items) {
      if (!uuidV7.test(item.chapter_uuid) || (a.chapter_uuid && item.chapter_uuid !== a.chapter_uuid) ||
          (item.chapter_title !== undefined && typeof item.chapter_title !== 'string') ||
          (item.page_number !== undefined && (!Number.isSafeInteger(item.page_number) || item.page_number < 1))) throw new Error('预览章节无效')
    }
  }
  if (d.contract_version !== undefined && d.contract_version !== 2) throw new Error('未知审阅契约')
  if (d.contract_version === 2) {
    const base = `/projects/${d.project_uuid}/`
    const path = new URL(d.open_url).pathname
    if (d.view === 'book') {
      const b = d.book
      if (!b || a.chapter_uuid || a.mode === 'preview' || path !== base || d.items.length || typeof b.description !== 'string' || typeof b.generation_language !== 'string' || typeof b.setup_status !== 'string' || !['chapter_count', 'page_count', 'image_count'].every(k => revision(b[k])) || b.image_count > b.page_count) throw new Error('作品总览无效')
    }
    const chapter = (ch) => {
      if (!ch || !uuidV7.test(ch.uuid) || !revision(ch.revision) || !['title', 'summary', 'chapter_code'].every(k => typeof ch[k] === 'string') || (ch.story !== undefined && typeof ch.story !== 'string') || !revision(ch.page_count) || !revision(ch.image_count) || ch.image_count > ch.page_count || !safeOpenURL(ch.open_url, d.project_uuid) || new URL(ch.open_url).pathname !== `${base}chapters/${ch.uuid}`) throw new Error('章节信息无效')
    }
    if (d.view === 'chapters') {
      if (!Array.isArray(d.chapters) || path !== `${base}chapters` || d.items.length) throw new Error('章节列表无效')
      d.chapters.forEach(chapter)
    }
    if (d.view === 'chapter' || d.view === 'page' || (d.view === 'candidates' && a.kind === 'page')) {
      chapter(d.chapter)
      if (d.chapter.uuid !== a.chapter_uuid) throw new Error('章节目标不匹配')
      if (d.view === 'chapter' && (d.target_uuid !== a.chapter_uuid || d.revision !== d.chapter.revision || path !== `${base}chapters/${a.chapter_uuid}`)) throw new Error('章节修订不匹配')
      for (const item of d.items) if (item.chapter_uuid !== a.chapter_uuid) throw new Error('页面超出章节范围')
    }
    if (d.view === 'page') {
      if (path !== `${base}chapters/${a.chapter_uuid}/sections/${a.page_uuid}` || !Number.isSafeInteger(d.position) || d.position < 1 || d.position > d.total || d.total !== d.chapter.page_count) throw new Error('页码范围无效')
      for (const key of ['previous_page_uuid', 'next_page_uuid']) if (d[key] !== undefined && (!uuidV7.test(d[key]) || d[key] === d.target_uuid)) throw new Error('相邻页面无效')
      if (Boolean(d.previous_page_uuid) !== (d.position > 1) || Boolean(d.next_page_uuid) !== (d.position < d.total)) throw new Error('相邻页范围不匹配')
    }
    if (d.view === 'reader' && (a.mode === 'overview' || (!a.chapter_uuid && a.mode !== 'preview') || path !== (a.chapter_uuid ? `${base}chapters/${a.chapter_uuid}/preview` : `${base}preview`))) throw new Error('阅读范围无效')
    if (['reader', 'chapter', 'chapters', 'settings'].includes(d.view)) {
      const list = d.view === 'chapters' ? d.chapters : d.items
      const p = d.cursor_pagination
      const start = Number(p?.prev_cursor), end = Number(p?.next_cursor)
      if (!p || !revision(d.total) || !Number.isInteger(p.per_page) || p.per_page < 1 || p.per_page > 100 || !/^(0|[1-9][0-9]*)$/.test(p.prev_cursor) || !/^(0|[1-9][0-9]*)$/.test(p.next_cursor) || !revision(start) || !revision(end) || end - start !== list.length || end > d.total || list.length > p.per_page || p.has_more !== (end < d.total) || !/^[a-f0-9]{64}$/.test(d.snapshot)) throw new Error('分页范围无效')
      if (a.after && start !== Number(a.after) || a.before && start !== Math.max(0, Number(a.before) - p.per_page) || a.limit && a.limit !== p.per_page) throw new Error('分页请求不匹配')
    }
  }
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
  const payload = result?._meta?.['lumi/app_response']
  const d = payload ? success({structuredContent:payload,isError:result.isError}) : success(result)
  if (d.asset_uuid !== asset || d.mime_type !== 'image/jpeg' || !Number.isInteger(d.width) || !Number.isInteger(d.height) ||
      Math.min(d.width, d.height) < 1 || Math.max(d.width, d.height) > edge || typeof d.data_uri !== 'string' ||
      !/^data:image\/jpeg;base64,[A-Za-z0-9+/]+={0,2}$/.test(d.data_uri) || d.data_uri.length > 23 + 163840) throw Object.assign(new Error('图片预览格式无效'), { kind: 'resource_error' })
  return d
}
export function failureKind(error) {
  if (error?.kind) return error.kind
  if (['permission_denied', 'user_rejected', 'host_denied'].includes(error?.code) || /\b(denied|rejected)\b|拒绝/i.test(error?.message ?? '')) return 'host_denied'
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

// Only used to validate historical DTOs; workspaceLaunch routes their resource
// scope to normal pages without rendering a reader.
export function isReader(data) { return data.view === 'reader' || (data.view === 'book' && !data.book) }
