// Cursor paging for compact card/navigation facts. No timers or background polling.
export async function readSummaryPages(request, path, query = {}, options = {}) {
  const items = [], seen = new Set()
  let after = '', snapshot, total
  do {
    options.signal?.throwIfAborted()
    const params = new URLSearchParams({...query,view:'summary',limit:'20',...(after?{after}:{})})
    const page = await request(`${path}?${params}`,options)
    const p = page.cursor_pagination
    if (!p || !Array.isArray(page.items) || !Number.isSafeInteger(page.total) || page.total < 0 || !/^[a-f0-9]{64}$/.test(page.snapshot)) throw new Error('摘要分页响应无效')
    if (snapshot !== undefined && (snapshot !== page.snapshot || total !== page.total)) throw new Error('列表已变化，请刷新后重试')
    snapshot = page.snapshot;total = page.total
    for (const item of page.items) {if(seen.has(item.uuid))throw new Error('摘要分页顺序已变化');seen.add(item.uuid);items.push(item)}
    if (!p.has_more) break
    if (!page.items.length || !/^\d+$/.test(p.next_cursor) || Number(p.next_cursor)<=Number(after||0)) throw new Error('摘要分页未前进')
    after = p.next_cursor
  } while (true)
  if (items.length !== total) throw new Error('摘要列表未读取完整')
  return {items,total,snapshot}
}
