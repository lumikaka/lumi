// Re-read the whole loaded window after invalidation. A merge may remove an
// earlier segment, so appending pages from different revisions is incorrect.
export async function loadMCPActivityHistory(fetchPage, pageCount) {
  for (let attempt = 0; attempt < 3; attempt += 1) {
    try {
      let after = ''
      let result
      const items = []
      for (let page = 0; page < pageCount; page += 1) {
        result = await fetchPage(after)
        items.push(...result.items)
        after = result.cursor_pagination?.next_cursor || ''
        if (!after) break
      }
      return { ...result, items }
    } catch (error) {
      if (error.code !== 'mcp_activity_changed' || attempt === 2) throw error
    }
  }
}
