function machineText(value) {
  if (value == null) return ''
  if (typeof value === 'string') return value
  try { return JSON.stringify(value) } catch { return String(value) }
}

function normalizeSearchText(value) {
  return String(value || '').normalize('NFKC').toLocaleLowerCase()
}

export function tokenizeTrajectoryQuery(query) {
  return [...new Set(normalizeSearchText(query).split(/\s+/u).filter(Boolean))]
}

export function buildTrajectorySearchDocument(row) {
  if (!row) return ''
  const source = row.source || row.turn || {}
  const values = [
    row.key,
    row.sourceUuid,
    row.turnUuid,
    row.rowType,
    row.kind,
    row.status,
    row.preview,
    row.requestUuid,
    row.requestOrdinal,
    row.callUuid,
    row.orderingAccuracy,
    row.derivedReason,
    row.isSteering ? 'steering' : '',
    row.turn?.queue_sequence,
    row.turn?.source_type,
    row.turn?.error_code,
    row.turn?.error_message,
    row.input,
    row.output,
    source.tool_name,
    source.arguments,
    source.result,
    source.error_code,
    source.error_message,
    source.summary,
    source.system_prompt_digest,
    source.tool_catalog_digest,
    source.options,
  ]
  return normalizeSearchText(values.map(machineText).join('\n'))
}

function detailSearchText(detail) {
  if (!detail) return ''
  return normalizeSearchText([
    detail.uuid,
    detail.provider_uuid,
    detail.provider_type,
    detail.model,
    detail.status,
    detail.finish_reason,
    detail.error_code,
    detail.error_message,
    detail.request_payload,
    detail.response,
  ].map(machineText).join('\n'))
}

export function reconcileTrajectorySearchIndex(current = new Map(), rows = []) {
  const next = new Map()
  for (const row of rows) {
    const base = buildTrajectorySearchDocument(row)
    const existing = current.get(row.key)
    const requestUuid = row.sourceKind === 'model_request' ? row.sourceUuid : row.requestUuid || ''
    if (existing?.base === base && existing.requestUuid === requestUuid) {
      next.set(row.key, existing)
      continue
    }
    const detail = existing?.requestUuid === requestUuid ? existing.detail || '' : ''
    next.set(row.key, { key: row.key, requestUuid, base, detail, text: `${base}\n${detail}` })
  }
  return next
}

export function updateTrajectoryRequestSearchDocument(index = new Map(), requestUuid, detail) {
  if (!requestUuid) return index
  const addition = detailSearchText(detail)
  let changed = false
  const next = new Map(index)
  for (const [key, entry] of index) {
    if (entry.requestUuid !== requestUuid || entry.detail === addition) continue
    next.set(key, { ...entry, detail: addition, text: `${entry.base}\n${addition}` })
    changed = true
  }
  return changed ? next : index
}

export function matchingTrajectoryKeys(index = new Map(), query = '') {
  const tokens = tokenizeTrajectoryQuery(query)
  if (!tokens.length) return null
  const matches = new Set()
  for (const [key, entry] of index) {
    if (tokens.every((token) => entry.text.includes(token))) matches.add(key)
  }
  return matches
}

function rowMatchesFilters(row, matchedKeys, kind, status, rangeKeys) {
  if (row.rowType === 'turn') return matchedKeys?.has(row.key) || false
  if (rangeKeys && !rangeKeys.has(row.key) && !rangeKeys.has(row.sourceUuid) && !rangeKeys.has(row.requestUuid) && !rangeKeys.has(row.callUuid)) return false
  if (matchedKeys && !matchedKeys.has(row.key)) return false
  const rowKind = row.rowType === 'request' ? 'request' : row.kind
  if (kind && rowKind !== kind) return false
  if (status && row.status !== status) return false
  return true
}

export function filterTrajectoryRows(rows = [], index = new Map(), { query = '', kind = '', status = '', rangeKeys = null } = {}) {
  const matchedKeys = matchingTrajectoryKeys(index, query)
  if (!matchedKeys && !kind && !status && !rangeKeys) return rows
  const included = new Set()
  const matchedTurns = new Set()
  for (const row of rows) {
    if (!rowMatchesFilters(row, matchedKeys, kind, status, rangeKeys)) continue
    included.add(row.key)
    if (row.turnUuid) matchedTurns.add(row.turnUuid)
  }
  for (const row of rows) {
    if (row.rowType === 'turn' && matchedTurns.has(row.turnUuid)) included.add(row.key)
  }
  return rows.filter((row) => included.has(row.key))
}
