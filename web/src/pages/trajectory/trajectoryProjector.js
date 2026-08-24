import { trajectoryKey } from './trajectoryIdentity.js'

const TERMINAL_TURN_STATUSES = new Set(['completed', 'failed', 'cancelled', 'interrupted'])
const ITEM_KINDS = new Set(['system', 'user', 'context', 'assistant', 'tool', 'compaction', 'error'])

export function createTrajectoryProjection() {
  return rebuildProjection({
    thread: null,
    overview: emptyOverview(),
    historyComplete: false,
    cursorPagination: emptyCursor(),
    raw: emptyRawProjection(),
  })
}

export function replaceTrajectoryProjection(snapshot) {
  return rebuildProjection({
    thread: snapshot?.thread || null,
    overview: snapshot?.overview || emptyOverview(),
    historyComplete: Boolean(snapshot?.history_complete),
    cursorPagination: snapshot?.cursor_pagination || emptyCursor(),
    raw: rawProjectionFromSnapshot(snapshot),
  })
}

export function prependTrajectoryPage(current, olderPage) {
  const base = current || createTrajectoryProjection()
  return rebuildProjection({
    thread: olderPage?.thread || base.thread,
    overview: olderPage?.overview || base.overview,
    historyComplete: Boolean(olderPage?.history_complete || base.historyComplete),
    cursorPagination: {
      ...(base.cursorPagination || emptyCursor()),
      ...(olderPage?.cursor_pagination || {}),
      next_cursor: base.cursorPagination?.next_cursor || olderPage?.cursor_pagination?.next_cursor || '',
    },
    raw: mergeRawProjection(base.raw, rawProjectionFromSnapshot(olderPage), false),
  })
}

export function applyTrajectoryUpserts(current, snapshot) {
  const base = current || createTrajectoryProjection()
  return rebuildProjection({
    thread: snapshot?.thread || base.thread,
    overview: snapshot?.overview || base.overview,
    historyComplete: Boolean(snapshot?.history_complete || base.historyComplete),
    cursorPagination: { ...(base.cursorPagination || emptyCursor()), ...(snapshot?.cursor_pagination || {}) },
    raw: mergeRawProjection(base.raw, rawProjectionFromSnapshot(snapshot), true),
  })
}

export function combineTrajectoryPages(pages = []) {
  if (!pages.length) return createTrajectoryProjection()
  let projection = replaceTrajectoryProjection(pages.at(-1))
  for (let index = pages.length - 2; index >= 0; index -= 1) {
    projection = prependTrajectoryPage(projection, pages[index])
  }
  return projection
}

function rebuildProjection(base) {
  const turns = sortTurns([...base.raw.turns.values()])
  const turnByUuid = new Map(turns.map((turn) => [turn.uuid, turn]))
  const requests = [...base.raw.requests.values()].map(projectRequestBoundary).sort(compareExecutionFacts)
  const requestByUuid = new Map(requests.map((request) => [request.sourceUuid, request]))
  const persistedAssistantRequests = new Set(
    [...base.raw.chatItems.values()]
      .filter((item) => item?.item_type === 'assistant_message' && item?.request_uuid)
      .map((item) => item.request_uuid),
  )

  const items = []
  for (const source of base.raw.chatItems.values()) {
    const item = projectChatItem(source)
    if (item) items.push(item)
  }
  for (const source of base.raw.tools.values()) {
    items.push(projectToolItem(source, turnByUuid.get(source.turn_uuid)))
  }
  for (const source of base.raw.compactions.values()) {
    items.push(projectCompactionItem(source))
  }
  for (const request of requests) {
    const source = base.raw.requests.get(request.sourceUuid)
    if (shouldProjectRequestAssistant(source, persistedAssistantRequests)) {
      items.push(projectRequestAssistant(source))
    }
  }
  items.push(...projectSystemChanges([...base.raw.requests.values()], base.historyComplete))

  const dedupedItems = uniqueByStableKey(items).sort(compareExecutionFacts)
  const itemByKey = new Map(dedupedItems.map((item) => [item.key, item]))
  const rows = buildTrajectoryRows(turns, requests, dedupedItems)

  return {
    thread: base.thread,
    overview: base.overview || emptyOverview(),
    historyComplete: Boolean(base.historyComplete),
    cursorPagination: base.cursorPagination || emptyCursor(),
    raw: base.raw,
    turns,
    turnByUuid,
    requests,
    requestByUuid,
    items: dedupedItems,
    itemByKey,
    rows,
  }
}

function rawProjectionFromSnapshot(snapshot = {}) {
  return {
    turns: mapByUuid(snapshot.turns),
    chatItems: mapByUuid(snapshot.items),
    tools: mapByUuid(snapshot.tools),
    requests: mapByUuid(snapshot.model_requests),
    compactions: mapByUuid(snapshot.compactions),
  }
}

function emptyRawProjection() {
  return { turns: new Map(), chatItems: new Map(), tools: new Map(), requests: new Map(), compactions: new Map() }
}

function mergeRawProjection(current = emptyRawProjection(), incoming = emptyRawProjection(), incomingWins = true) {
  return {
    turns: mergeMaps(current.turns, incoming.turns, incomingWins),
    chatItems: mergeMaps(current.chatItems, incoming.chatItems, incomingWins),
    tools: mergeMaps(current.tools, incoming.tools, incomingWins),
    requests: mergeMaps(current.requests, incoming.requests, incomingWins),
    compactions: mergeMaps(current.compactions, incoming.compactions, incomingWins),
  }
}

function mapByUuid(values = []) {
  const result = new Map()
  for (const value of values || []) {
    if (value?.uuid) result.set(value.uuid, value)
  }
  return result
}

function mergeMaps(current = new Map(), incoming = new Map(), incomingWins = true) {
  const result = new Map(current)
  for (const [uuid, value] of incoming) {
    if (incomingWins || !result.has(uuid)) result.set(uuid, value)
  }
  return result
}

function projectChatItem(source) {
  if (!source?.uuid) return null
  const kind = itemKind(source)
  if (!kind || source.item_type === 'tool_call' || source.item_type === 'tool_result' || source.item_type === 'user_input_request') return null
  const status = normalizeStatus(source.status, kind)
  return trajectoryItem({
    id: source.uuid,
    sourceUuid: source.uuid,
    sourceKind: 'chat_item',
    threadUuid: source.thread_uuid,
    turnUuid: source.turn_uuid || null,
    seq: numberOrNull(source.event_sequence ?? source.sequence),
    itemSequence: numberOrNull(source.sequence),
    eventSequence: numberOrNull(source.event_sequence),
    kind,
    requestUuid: source.request_uuid || undefined,
    requestOrdinal: numberOrUndefined(source.request_ordinal),
    callUuid: source.tool_call_uuid || undefined,
    status,
    startedAt: timestamp(source.created_at),
    completedAt: status === 'completed' || status === 'error' ? timestamp(source.created_at) : undefined,
    preview: oneLine(source.content || source.item_type || kind),
    input: kind === 'user' ? source.content : undefined,
    output: kind === 'assistant' || kind === 'error' || kind === 'context' ? source.content : undefined,
    source,
    isSteering: Boolean(source.metadata?.steering),
    orderingAccuracy: source.ordering_accuracy || 'approximate',
    sortAt: timestamp(source.created_at),
  })
}

function projectToolItem(source, turn) {
  const status = deriveToolStatus(source, turn)
  const argumentsPreview = compactMachineValue(source?.arguments)
  const resultPreview = compactMachineValue(source?.result)
  const previewParts = [source?.tool_name || 'tool']
  if (argumentsPreview && argumentsPreview !== '{}') previewParts.push(argumentsPreview)
  if (resultPreview && resultPreview !== '{}') previewParts.push(`→ ${resultPreview}`)
  return trajectoryItem({
    id: source.tool_call_uuid || source.uuid,
    sourceUuid: source.tool_call_uuid || source.uuid,
    sourceKind: 'tool',
    threadUuid: source.thread_uuid,
    turnUuid: source.turn_uuid || null,
    seq: numberOrNull(source.start_event_sequence ?? source.call_sequence),
    itemSequence: numberOrNull(source.call_sequence),
    eventSequence: numberOrNull(source.start_event_sequence),
    kind: 'tool',
    requestUuid: source.request_uuid || undefined,
    requestOrdinal: numberOrUndefined(source.request_ordinal),
    callUuid: source.tool_call_uuid || undefined,
    status,
    startedAt: timestamp(source.started_at || source.created_at),
    completedAt: timestamp(source.completed_at),
    durationMs: numberOrUndefined(source.duration_ms),
    preview: previewParts.join(' · '),
    input: source.arguments,
    output: source.result,
    source,
    derivedReason: status === 'interrupted' ? source.derived_reason || 'Turn ended before the Tool lifecycle completed.' : undefined,
    orderingAccuracy: source.ordering_accuracy || 'approximate',
    sortAt: timestamp(source.created_at || source.started_at),
  })
}

function projectCompactionItem(source) {
  return trajectoryItem({
    id: source.uuid,
    sourceUuid: source.uuid,
    sourceKind: 'compaction',
    threadUuid: source.thread_uuid,
    turnUuid: source.turn_uuid || null,
    seq: numberOrNull(source.event_sequence ?? source.through_item_sequence),
    itemSequence: numberOrNull(source.through_item_sequence),
    eventSequence: numberOrNull(source.event_sequence),
    kind: 'compaction',
    status: 'completed',
    startedAt: timestamp(source.created_at),
    completedAt: undefined,
    durationMs: undefined,
    preview: oneLine(source.summary || 'Context compaction'),
    output: source.summary,
    source,
    orderingAccuracy: source.ordering_accuracy || 'approximate',
    sortAt: timestamp(source.created_at),
  })
}

function projectRequestBoundary(source) {
  const createdAt = timestamp(source?.created_at)
  const completedAt = timestamp(source?.completed_at)
  return {
    rowType: 'request',
    id: source.uuid,
    key: trajectoryKey('model_request', source.uuid),
    sourceUuid: source.uuid,
    sourceKind: 'model_request',
    threadUuid: source.thread_uuid,
    turnUuid: source.turn_uuid || null,
    requestUuid: source.uuid,
    requestOrdinal: Number(source.request_ordinal) || 0,
    requestType: source.request_type || 'text',
    status: normalizeStatus(source.status, 'model_request'),
    seq: numberOrNull(source.start_event_sequence),
    eventSequence: numberOrNull(source.start_event_sequence),
    preview: `Request #${source.request_ordinal || '—'} · ${source.model || 'Unknown model'}`,
    startedAt: createdAt,
    completedAt,
    durationMs: numberOrUndefined(source.duration_ms),
    orderingAccuracy: source.ordering_accuracy || 'legacy_unlinked',
    sortAt: createdAt,
    source,
  }
}

function shouldProjectRequestAssistant(source, persistedAssistantRequests) {
  if (!source?.uuid || !source.has_response) return false
  if (source.has_tool_calls) return true
  if (persistedAssistantRequests.has(source.uuid)) return false
  return Boolean(String(source.assistant_preview || source.output_summary || '').trim())
}

function projectRequestAssistant(source) {
  const completedAt = timestamp(source.completed_at || source.created_at)
  return trajectoryItem({
    id: source.uuid,
    sourceUuid: source.uuid,
    sourceKind: 'model_request_assistant',
    threadUuid: source.thread_uuid,
    turnUuid: source.turn_uuid || null,
    seq: numberOrNull(source.end_event_sequence ?? source.start_event_sequence),
    eventSequence: numberOrNull(source.end_event_sequence ?? source.start_event_sequence),
    kind: 'assistant',
    requestUuid: source.uuid,
    requestOrdinal: numberOrUndefined(source.request_ordinal),
    status: source.status === 'completed' ? 'completed' : normalizeStatus(source.status, 'assistant'),
    startedAt: completedAt,
    completedAt,
    preview: oneLine(source.assistant_preview || source.output_summary || 'Tool calls requested'),
    output: source.assistant_preview || source.output_summary || undefined,
    source,
    orderingAccuracy: source.ordering_accuracy || 'legacy_unlinked',
    sortAt: completedAt,
    sortRank: 2,
  })
}

function projectSystemChanges(requests, historyComplete = false) {
  const ordered = [...requests]
    .filter((request) => request?.uuid && (request.system_prompt_digest || request.tool_catalog_digest))
    .sort(compareSourceRequests)
  const result = []
  let previous = null
  for (const request of ordered) {
    if (!previous) {
      result.push(trajectoryItem({
        id: request.uuid,
        sourceUuid: request.uuid,
        sourceKind: 'system_change',
        threadUuid: request.thread_uuid,
        turnUuid: null,
        seq: numberOrNull(request.start_event_sequence),
        eventSequence: numberOrNull(request.start_event_sequence),
        kind: 'system',
        requestUuid: request.uuid,
        requestOrdinal: numberOrUndefined(request.request_ordinal),
        status: 'completed',
        startedAt: timestamp(request.created_at),
        preview: historyComplete ? 'Initial System Prompt' : 'System Prompt Snapshot',
        output: { system_prompt_digest: request.system_prompt_digest, tool_catalog_digest: request.tool_catalog_digest },
        source: request,
        systemChangeType: historyComplete ? 'initial' : 'snapshot',
        orderingAccuracy: request.ordering_accuracy || 'legacy_unlinked',
        sortAt: timestamp(request.created_at),
        sortRank: 1,
      }))
      previous = request
      continue
    }
    const systemChanged = Boolean(previous.system_prompt_digest && request.system_prompt_digest && previous.system_prompt_digest !== request.system_prompt_digest)
    const toolsChanged = Boolean(previous.tool_catalog_digest && request.tool_catalog_digest && previous.tool_catalog_digest !== request.tool_catalog_digest)
    if (systemChanged || toolsChanged) {
      const preview = systemChanged && toolsChanged
        ? 'System Prompt and Tools Updated'
        : systemChanged ? 'System Prompt Updated' : 'Tools Updated'
      result.push(trajectoryItem({
        id: request.uuid,
        sourceUuid: request.uuid,
        sourceKind: 'system_change',
        threadUuid: request.thread_uuid,
        turnUuid: request.turn_uuid || null,
        seq: numberOrNull(request.start_event_sequence),
        eventSequence: numberOrNull(request.start_event_sequence),
        kind: 'system',
        requestUuid: request.uuid,
        previousRequestUuid: previous.uuid,
        requestOrdinal: numberOrUndefined(request.request_ordinal),
        status: 'completed',
        startedAt: timestamp(request.created_at),
        preview,
        input: { system_prompt_digest: previous.system_prompt_digest, tool_catalog_digest: previous.tool_catalog_digest },
        output: { system_prompt_digest: request.system_prompt_digest, tool_catalog_digest: request.tool_catalog_digest },
        source: request,
        systemChangeType: 'update',
        orderingAccuracy: request.ordering_accuracy || 'legacy_unlinked',
        sortAt: timestamp(request.created_at),
        sortRank: 1,
      }))
    }
    previous = request
  }
  return result
}

export function buildTrajectoryRows(turns, requests, items) {
  const contentByTurn = new Map()
  const threadLevel = []
  const append = (turnUuid, value) => {
    if (!turnUuid) {
      threadLevel.push(value)
      return
    }
    const current = contentByTurn.get(turnUuid) || []
    current.push(value)
    contentByTurn.set(turnUuid, current)
  }
  requests.forEach((request) => append(request.turnUuid, request))
  items.forEach((item) => append(item.turnUuid, { ...item, rowType: 'item' }))

  const rows = []
  threadLevel.sort(compareExecutionFacts).forEach((value) => rows.push(value.rowType ? value : { ...value, rowType: 'item' }))
  for (const turn of sortTurns(turns)) {
    const content = contentByTurn.get(turn.uuid)
    if (!content?.length) continue
    rows.push({ rowType: 'turn', id: turn.uuid, key: trajectoryKey('turn', turn.uuid), sourceUuid: turn.uuid, turnUuid: turn.uuid, turn })
    content.sort(compareExecutionFacts).forEach((value) => rows.push(value.rowType ? value : { ...value, rowType: 'item' }))
  }
  return rows.map((row, index) => ({ ...row, ariaRowIndex: index + 1 }))
}

function trajectoryItem(value) {
  const kind = ITEM_KINDS.has(value.kind) ? value.kind : 'error'
  const sourceUuid = String(value.sourceUuid || value.id || '').trim()
  return {
    ...value,
    id: value.id || sourceUuid,
    key: trajectoryKey(value.sourceKind || kind, sourceUuid),
    sourceUuid,
    turnUuid: value.turnUuid || null,
    kind,
    status: normalizeStatus(value.status, kind),
    preview: oneLine(value.preview),
  }
}

function itemKind(source) {
  if (source.item_type === 'user_message') return 'user'
  if (source.item_type === 'assistant_message') return 'assistant'
  if (source.item_type === 'context_summary') return 'context'
  if (source.item_type === 'error') return 'error'
  if (source.item_type === 'tool_call' || source.item_type === 'tool_result' || source.item_type === 'user_input_request') return 'tool'
  if (source.role === 'system') return 'system'
  return null
}

export function deriveToolStatus(tool, turn) {
  const resultFailed = toolResultFailed(tool?.result)
  if (resultFailed || tool?.status === 'error' || tool?.status === 'failed' || tool?.error_code) return 'error'
  if (TERMINAL_TURN_STATUSES.has(turn?.status) && ['pending', 'running', 'intent', 'executing', 'in_progress'].includes(tool?.status)) return 'interrupted'
  return normalizeStatus(tool?.status, 'tool')
}

function toolResultFailed(value) {
  const parsed = machineValue(value)
  return parsed && typeof parsed === 'object' && parsed.success === false
}

function normalizeStatus(status, kind) {
  const value = String(status || '').toLowerCase()
  if (value === 'failed' || value === 'error') return 'error'
  if (value === 'cancelled' || value === 'interrupted') return 'interrupted'
  if (value === 'in_progress' || value === 'executing' || value === 'running') return 'running'
  if (value === 'queued' || value === 'intent' || value === 'pending') return 'pending'
  if (value === 'completed') return 'completed'
  return kind === 'error' ? 'error' : 'completed'
}

function compareExecutionFacts(left, right) {
  const leftAt = Number(left?.sortAt ?? left?.startedAt)
  const rightAt = Number(right?.sortAt ?? right?.startedAt)
  if (Number.isFinite(leftAt) && Number.isFinite(rightAt) && leftAt !== rightAt) return leftAt - rightAt
  const leftEvent = numberOrNull(left?.eventSequence ?? left?.seq)
  const rightEvent = numberOrNull(right?.eventSequence ?? right?.seq)
  if (leftEvent !== null && rightEvent !== null && leftEvent !== rightEvent) return leftEvent - rightEvent
  const leftItem = numberOrNull(left?.itemSequence)
  const rightItem = numberOrNull(right?.itemSequence)
  if (leftItem !== null && rightItem !== null && leftItem !== rightItem) return leftItem - rightItem
  const rank = Number(left?.sortRank || 0) - Number(right?.sortRank || 0)
  if (rank) return rank
  return String(left?.key || left?.sourceUuid || '').localeCompare(String(right?.key || right?.sourceUuid || ''))
}

function compareSourceRequests(left, right) {
  return compareExecutionFacts(
    { sortAt: timestamp(left.created_at), eventSequence: left.start_event_sequence, key: left.uuid },
    { sortAt: timestamp(right.created_at), eventSequence: right.start_event_sequence, key: right.uuid },
  )
}

function uniqueByStableKey(items) {
  const byKey = new Map()
  items.forEach((item) => { if (item?.key) byKey.set(item.key, item) })
  return [...byKey.values()]
}

function sortTurns(turns) {
  return [...(turns || [])].sort((left, right) => Number(left?.queue_sequence || 0) - Number(right?.queue_sequence || 0) || String(left?.uuid || '').localeCompare(String(right?.uuid || '')))
}

function oneLine(value) {
  const text = String(value ?? '').replace(/\s+/g, ' ').trim()
  return text.length > 280 ? `${text.slice(0, 279)}…` : text
}

function compactMachineValue(value) {
  const parsed = machineValue(value)
  if (parsed === undefined || parsed === null) return ''
  try {
    const text = JSON.stringify(sortMachineValue(parsed))
    return oneLine(text).slice(0, 160)
  } catch {
    return oneLine(value).slice(0, 160)
  }
}

function machineValue(value) {
  if (value === null || value === undefined || value === '') return value
  if (typeof value !== 'string') return value
  try { return JSON.parse(value) } catch { return value }
}

function sortMachineValue(value) {
  if (Array.isArray(value)) return value.map(sortMachineValue)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(Object.keys(value).sort().map((key) => [key, sortMachineValue(value[key])]))
}

function timestamp(value) {
  if (value === null || value === undefined || value === '') return undefined
  const number = typeof value === 'number' ? value : Date.parse(value)
  return Number.isFinite(number) ? number : undefined
}

function numberOrNull(value) {
  if (value === null || value === undefined || value === '') return null
  const number = Number(value)
  return Number.isFinite(number) ? number : null
}

function numberOrUndefined(value) {
  const number = numberOrNull(value)
  return number === null ? undefined : number
}

function emptyCursor() {
  return { per_page: 0, next_cursor: '', prev_cursor: '', has_more: false }
}

function emptyOverview() {
  return { turn_count: 0, item_count: 0, model_request_count: 0, tool_count: 0, compaction_count: 0, active_turn_count: 0, active_request_count: 0, active_tool_count: 0, timeline: [] }
}
