import { trajectorySummaryKey } from './trajectoryIdentity.js'

export function buildAssistantToolGroups(rows = []) {
  const groups = new Map()
  for (let index = 0; index < rows.length; index += 1) {
    const assistant = rows[index]
    if (assistant?.rowType !== 'item' || assistant.kind !== 'assistant' || !assistant.requestUuid) continue
    const tools = []
    for (let cursor = index + 1; cursor < rows.length; cursor += 1) {
      const candidate = rows[cursor]
      if (candidate.rowType === 'turn' || candidate.turnUuid !== assistant.turnUuid) break
      if (candidate.kind === 'assistant' || candidate.kind === 'user') break
      if (candidate.kind === 'tool' && candidate.requestUuid === assistant.requestUuid) tools.push(candidate)
    }
    if (!tools.length) continue
    groups.set(assistant.key, {
      key: trajectorySummaryKey('assistant-tools', assistant.sourceUuid),
      assistantKey: assistant.key,
      assistantUuid: assistant.sourceUuid,
      turnUuid: assistant.turnUuid,
      toolKeys: new Set(tools.map((tool) => tool.key)),
      count: tools.length,
    })
  }
  return groups
}

function turnSummary(row, hiddenCount) {
  return {
    rowType: 'summary',
    summaryKind: 'turn',
    key: trajectorySummaryKey('turn', row.turnUuid),
    sourceUuid: row.turnUuid,
    turnUuid: row.turnUuid,
    kind: 'context',
    status: row.turn?.status || 'completed',
    hiddenCount,
    ariaRowIndex: row.ariaRowIndex + 1,
  }
}

function isLedgerContentRow(row) {
  return row?.rowType !== 'turn' && row?.rowType !== 'request'
}

function isRequestAnchorCandidate(row) {
  return isLedgerContentRow(row) && row.rowType !== 'summary' && row.kind !== 'user' && row.kind !== 'context'
}

function requestBoundariesForRows(rows, fullRows) {
  const visibleKeys = new Set(rows.filter(isLedgerContentRow).map((row) => row.key))
  const boundaries = new Map()
  const requests = fullRows.filter((row) => row.rowType === 'request')

  for (const request of requests) {
    const anchor = fullRows.find((row) => visibleKeys.has(row.key)
      && isRequestAnchorCandidate(row)
      && row.turnUuid === request.turnUuid
      && row.requestUuid === request.sourceUuid)
    if (!anchor) continue
    const current = boundaries.get(anchor.key) || []
    current.push({ ...request, runIndex: current.length })
    boundaries.set(anchor.key, current)
  }
  return boundaries
}

function groupRequestMarkers(rows, fullRows, requestBoundaries) {
  const segments = new Map()
  const order = new Map()
  let segment = 0
  for (const [index, row] of fullRows.entries()) {
    segments.set(row.key, segment)
    order.set(row.key, index)
    if (row.rowType !== 'request') segment++
  }
  const result = []
  let pending = []
  const flush = () => {
    if (!pending.length) return
    pending.sort((left, right) => order.get(left.key) - order.get(right.key))
    result.push({ ...pending[0], requestBoundaries: pending.map((request, runIndex) => ({ ...request, runIndex })) })
    pending = []
  }
  for (const row of rows) {
    if (row.rowType === 'request') {
      if (pending.length && (pending[0].turnUuid !== row.turnUuid || segments.get(pending[0].key) !== segments.get(row.key))) flush()
      pending.push(row)
      continue
    }
    const boundaries = requestBoundaries.get(row.key) || []
    const shared = pending.length && pending[0].turnUuid === row.turnUuid && segments.get(pending[0].key) === segments.get(row.key)
      ? boundaries.filter((request) => segments.get(request.key) === segments.get(row.key))
      : []
    // Consecutive requests share a marker strip, not the following item's
    // execution identity. Each dot still selects its own original Request.
    pending.push(...shared)
    flush()
    result.push({ ...row, requestBoundaries: boundaries.filter((request) => !shared.includes(request)) })
  }
  flush()
  return result
}

/**
 * Convert the fact projection into the compact ledger presentation used by the
 * reference trajectory: Turn is a rail/label. A Request with linked visible
 * content is a boundary dot; otherwise it keeps its own dot position.
 */
export function buildTrajectoryLedgerRows(rows = [], fullRows = rows) {
  const turnRows = new Map(fullRows.filter((row) => row.rowType === 'turn').map((row) => [row.turnUuid, row]))
  const requestBoundaries = requestBoundariesForRows(rows, fullRows)
  const anchoredRequests = new Set([...requestBoundaries.values()].flat().map((request) => request.key))
  const contentRows = groupRequestMarkers(rows.filter((row) => isLedgerContentRow(row)
    || row.rowType === 'request' && !anchoredRequests.has(row.key)), fullRows, requestBoundaries)
  const turnIndexes = new Map()

  for (const [index, row] of contentRows.entries()) {
    if (!row.turnUuid) continue
    const current = turnIndexes.get(row.turnUuid)
    if (current) current.last = index
    else turnIndexes.set(row.turnUuid, { first: index, last: index })
  }

  return contentRows.map((row, index) => {
    const bounds = row.turnUuid ? turnIndexes.get(row.turnUuid) : null
    const turnRow = row.turnUuid ? turnRows.get(row.turnUuid) : null
    return {
      ...row,
      ariaRowIndex: index + 1,
      turn: turnRow?.turn,
      turnStart: Boolean(bounds && bounds.first === index),
      turnEnd: Boolean(bounds && bounds.last === index),
      requestBoundaries: row.requestBoundaries,
    }
  })
}

function toolSummary(row, group, hiddenCount) {
  return {
    rowType: 'summary',
    summaryKind: 'assistant-tools',
    key: group.key,
    sourceUuid: group.key,
    turnUuid: row.turnUuid,
    kind: 'tool',
    status: 'completed',
    hiddenCount,
    ownerKey: row.key,
    ariaRowIndex: row.ariaRowIndex + 1,
  }
}

export function applyTrajectoryCollapse(rows = [], fullRows = rows, {
  collapsedTurns = new Set(),
  collapsedToolGroups = new Set(),
  toolGroups = buildAssistantToolGroups(fullRows),
} = {}) {
  const visibleKeys = new Set(rows.map((row) => row.key))
  const toolGroupByToolKey = new Map()
  for (const group of toolGroups.values()) {
    for (const key of group.toolKeys) toolGroupByToolKey.set(key, group)
  }
  const result = []
  const fullLedgerRows = collapsedTurns.size ? buildTrajectoryLedgerRows(fullRows) : []
  let collapsedTurnUuid = ''
  for (const row of rows) {
    if (row.rowType === 'turn') {
      collapsedTurnUuid = collapsedTurns.has(row.turnUuid) ? row.turnUuid : ''
      result.push(row)
      if (collapsedTurnUuid) {
        const hiddenCount = fullLedgerRows.filter((candidate) => candidate.turnUuid === row.turnUuid).length
        result.push(turnSummary(row, hiddenCount))
      }
      continue
    }
    if (collapsedTurnUuid && row.turnUuid === collapsedTurnUuid) continue
    const owningGroup = toolGroupByToolKey.get(row.key)
    if (owningGroup && collapsedToolGroups.has(owningGroup.assistantKey)) continue
    result.push(row)
    const group = toolGroups.get(row.key)
    if (group && collapsedToolGroups.has(row.key)) {
      const hiddenCount = [...group.toolKeys].filter((key) => visibleKeys.has(key)).length
      if (hiddenCount) result.push(toolSummary(row, group, hiddenCount))
    }
  }
  return buildTrajectoryLedgerRows(result, fullRows)
}
