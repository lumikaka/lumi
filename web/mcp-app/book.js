import { reviewContext } from './contracts.js'

// Rebuild from the first cursor on every calibration. Never combine an old
// prefix with a new suffix, or call an arbitrary seed page the whole book.
export async function readWholeBook(call, seed, signal) {
  const args = seed.request.arguments
  let limit = args.limit || 20
  let after = ''
  let first
  const items = []
  const seen = new Set()
  do {
    signal?.throwIfAborted()
    let next
    try {
      next = reviewContext(await call('review_book', { ...(args.chapter_uuid ? { chapter_uuid: args.chapter_uuid } : {}), limit, ...(after ? { after } : {}) }, { signal }))
    } catch (error) {
      if (error.code === 'result_too_large' && limit > 1) { limit = Math.max(1, Math.floor(limit / 2)); continue }
      throw error
    }
    signal?.throwIfAborted()
    if (next.view !== 'book' || next.project_uuid !== seed.project_uuid || (next.request.arguments.chapter_uuid || '') !== (args.chapter_uuid || '')) throw new Error('预览范围发生变化，请刷新')
    first ??= next
    if (next.total !== first.total) throw new Error('页面数量已变化，请刷新')
    for (const item of next.items) {
      if (seen.has(item.uuid)) throw new Error('页面顺序已变化，请刷新')
      seen.add(item.uuid); items.push(item)
    }
    const pagination = next.cursor_pagination
    if (!pagination) throw new Error('缺少分页信息，无法确认完整范围')
    if (!pagination.has_more) break
    if (!next.items.length || !/^\d+$/.test(pagination.next_cursor) || Number(pagination.next_cursor) <= Number(after || 0)) throw new Error('分页未前进，请刷新')
    after = pagination.next_cursor
  } while (true)
  if (first.total !== undefined && first.total !== items.length) throw new Error('页面不完整，请刷新')
  return { ...first, items, complete: true }
}

export function bookGroups(data) {
  const groups = []
  for (const item of data?.items ?? []) {
    let group = groups.at(-1)
    if (!group || group.uuid !== item.chapter_uuid) {
      group = { uuid: item.chapter_uuid, title: item.chapter_title || `章节 ${groups.length + 1}`, pages: [], bodyCount: 0 }
      groups.push(group)
    }
    const body = !['front_cover', 'back_cover'].includes(item.kind)
    if (body) group.bodyCount++
    const label = item.kind === 'front_cover' ? '封面' : item.kind === 'back_cover' ? '封底' : `第 ${item.page_number || group.bodyCount} 页`
    group.pages.push({ ...item, title: item.title || label, label, image_key: item.asset_uuid })
  }
  return groups
}
