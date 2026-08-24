export const DEFAULT_TRAJECTORY_OVERSCAN = 480

export function estimateTrajectoryRowHeight(row) {
  if (row?.rowType === 'summary') return 28
  return 30
}

export function measureTrajectoryRows(rows = [], measuredHeights = new Map(), estimate = estimateTrajectoryRowHeight) {
  let start = 0
  const entries = rows.map((row, index) => {
    const measured = Number(measuredHeights.get(row.key))
    const size = Number.isFinite(measured) && measured > 0 ? measured : estimate(row)
    const entry = { key: row.key, row, index, start, size, end: start + size }
    start += size
    return entry
  })
  return { entries, totalSize: start }
}

function firstEntryEndingAfter(entries, offset) {
  let low = 0
  let high = entries.length
  while (low < high) {
    const middle = Math.floor((low + high) / 2)
    if (entries[middle].end < offset) low = middle + 1
    else high = middle
  }
  return low
}

export function trajectoryVirtualWindow(measurement, scrollTop, viewportHeight, overscan = DEFAULT_TRAJECTORY_OVERSCAN) {
  const entries = measurement?.entries || []
  if (!entries.length) return []
  const startOffset = Math.max(0, Number(scrollTop) - overscan)
  const endOffset = Math.max(startOffset, Number(scrollTop) + Math.max(1, Number(viewportHeight) || 1) + overscan)
  const startIndex = firstEntryEndingAfter(entries, startOffset)
  const visible = []
  for (let index = startIndex; index < entries.length; index += 1) {
    const entry = entries[index]
    if (entry.start > endOffset) break
    visible.push(entry)
  }
  return visible
}

export function captureTrajectoryVirtualAnchor(measurement, scrollTop) {
  const entries = measurement?.entries || []
  if (!entries.length) return null
  const index = firstEntryEndingAfter(entries, Number(scrollTop) || 0)
  const entry = entries[Math.min(index, entries.length - 1)]
  return entry ? { key: entry.key, offset: entry.start - (Number(scrollTop) || 0) } : null
}

export function restoreTrajectoryVirtualAnchor(measurement, anchor) {
  if (!anchor?.key) return null
  const entry = measurement?.entries?.find((candidate) => candidate.key === anchor.key)
  return entry ? Math.max(0, entry.start - anchor.offset) : null
}

export function isTrajectoryAtTail({ scrollTop = 0, scrollHeight = 0, clientHeight = 0 } = {}, threshold = 72) {
  return Number(scrollHeight) - Number(scrollTop) - Number(clientHeight) <= threshold
}

export function shouldLoadEarlierTrajectory({ scrollTop = 0, hasPreviousPage = false, fetchingPreviousPage = false } = {}, threshold = 220) {
  return Boolean(hasPreviousPage && !fetchingPreviousPage && Number(scrollTop) <= threshold)
}

export function shouldFollowTrajectoryTail({ wasAtTail = false, previousLastKey = '', nextLastKey = '', prepending = false } = {}) {
  return Boolean(wasAtTail && !prepending && nextLastKey && nextLastKey !== previousLastKey)
}
