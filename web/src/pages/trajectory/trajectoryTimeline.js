import { trajectoryKey } from './trajectoryIdentity.js'

export const TRAJECTORY_TIMELINE_MODES = Object.freeze(['sequence', 'duration', 'time', 'actual'])

export function trajectoryTimelineLane(kind) {
  if (kind === 'tool') return 2
  if (kind === 'assistant' || kind === 'model_request' || kind === 'compaction') return 1
  return 0
}

function timestamp(value) {
  if (value == null || value === '') return null
  const number = typeof value === 'number' ? value : Date.parse(value)
  return Number.isFinite(number) ? number : null
}

function duration(value) {
  if (value == null || value === '') return null
  const number = Number(value)
  return Number.isFinite(number) && number >= 0 ? number : null
}

function sourceKey(entry) {
  return trajectoryKey(entry.source_kind || entry.kind || 'timeline', entry.uuid)
}

function median(values) {
  if (!values.length) return 1000
  const sorted = [...values].sort((left, right) => left - right)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2
}

function normalizeEntries(entries = []) {
  return entries.filter((entry) => entry?.uuid).map((entry, index) => ({
    key: sourceKey(entry),
    sourceUuid: entry.uuid,
    sourceKind: entry.source_kind || entry.kind || 'timeline',
    kind: entry.kind || 'system',
    lane: trajectoryTimelineLane(entry.kind),
    status: entry.status || 'completed',
    preview: entry.preview || '',
    turnUuid: entry.turn_uuid || '',
    requestUuid: entry.request_uuid || '',
    requestOrdinal: Number(entry.request_ordinal) || 0,
    eventSequence: entry.event_sequence == null ? null : Number(entry.event_sequence),
    itemSequence: entry.item_sequence == null ? null : Number(entry.item_sequence),
    startedAt: timestamp(entry.started_at),
    completedAt: timestamp(entry.completed_at),
    durationMs: duration(entry.duration_ms),
    orderingAccuracy: entry.ordering_accuracy || 'approximate',
    source: entry,
    sourceIndex: index,
  }))
}

function orderEntries(entries) {
  return [...entries].sort((left, right) => {
    if (left.startedAt != null && right.startedAt != null && left.startedAt !== right.startedAt) return left.startedAt - right.startedAt
    if (left.eventSequence != null && right.eventSequence != null && left.eventSequence !== right.eventSequence) return left.eventSequence - right.eventSequence
    if (left.itemSequence != null && right.itemSequence != null && left.itemSequence !== right.itemSequence) return left.itemSequence - right.itemSequence
    return left.key.localeCompare(right.key)
  })
}

function sequencePositions(entries) {
  return orderEntries(entries).map((entry, index) => {
    return { ...entry, start: index, end: index + 1, span: true }
  })
}

function durationPositions(entries) {
  const known = entries.map((entry) => entry.durationMs).filter((value) => value != null && value > 0)
  const markerStep = Math.max(1, median(known) * 0.08)
  let cursor = 0
  return orderEntries(entries).map((entry) => {
    const start = cursor
    const span = entry.durationMs != null
    const end = span ? start + Math.max(entry.durationMs, markerStep * 0.15) : start
    cursor += span ? Math.max(entry.durationMs, markerStep) : markerStep
    return { ...entry, start, end, span }
  })
}

function wallClockPositions(entries, compressGaps) {
  const ordered = orderEntries(entries)
  const starts = ordered.map((entry) => entry.startedAt).filter((value) => value != null)
  const baseline = starts.length ? Math.min(...starts) : 0
  const known = ordered.map((entry) => entry.durationMs).filter((value) => value != null && value > 0)
  const gapCap = Math.max(1000, median(known) * 4)
  let previousActual = baseline
  let previousMapped = 0
  return ordered.map((entry, index) => {
    const actual = entry.startedAt ?? previousActual
    let start
    if (!compressGaps) start = actual - baseline
    else if (index === 0) start = 0
    else start = previousMapped + Math.min(Math.max(0, actual - previousActual), gapCap)
    previousActual = actual
    previousMapped = start
    const span = entry.durationMs != null
    return { ...entry, start, end: span ? start + entry.durationMs : start, span }
  })
}

function timelineDomain(items) {
  if (!items.length) return { min: 0, max: 1 }
  const min = Math.min(...items.map((item) => item.start))
  const maxFact = Math.max(...items.map((item) => Math.max(item.start, item.end)))
  return { min, max: maxFact > min ? maxFact : min + 1 }
}

function timelineTurnBoundaries(items) {
  const seen = new Set()
  const result = []
  for (const item of orderEntries(items)) {
    if (!item.turnUuid || seen.has(item.turnUuid)) continue
    seen.add(item.turnUuid)
    result.push({ key: item.turnUuid, turnUuid: item.turnUuid, position: item.start })
  }
  return result
}

export function trajectoryTimelineEntries(projection) {
  const source = projection?.overview?.timeline || []
  const requestUuids = new Set(source.filter((entry) => entry?.source_kind === 'model_request').map((entry) => entry.uuid))
  const assistantRequestByUuid = new Map(
    (projection?.items || [])
      .filter((item) => item?.kind === 'assistant' && item?.sourceUuid && item?.requestUuid)
      .map((item) => [item.sourceUuid, item.requestUuid]),
  )
  const entries = source.filter((entry) => {
    if (entry?.kind !== 'assistant') return true
    const requestUuid = entry.request_uuid || assistantRequestByUuid.get(entry.uuid)
    return !requestUuid || !requestUuids.has(requestUuid)
  })
  const existingKeys = new Set(entries.map((entry) => trajectoryKey(entry.source_kind || entry.kind || 'timeline', entry.uuid)))
  const threadStartedAt = projection?.thread?.created_at || ''

  for (const item of projection?.items || []) {
    if (item.kind !== 'system') continue
    const key = trajectoryKey(item.sourceKind || 'system_change', item.sourceUuid)
    if (existingKeys.has(key)) continue
    entries.push({
      uuid: item.sourceUuid,
      source_kind: item.sourceKind || 'system_change',
      kind: 'system',
      turn_uuid: item.turnUuid || '',
      event_sequence: item.eventSequence,
      request_uuid: item.requestUuid || '',
      request_ordinal: item.requestOrdinal || 0,
      status: item.status,
      preview: item.preview,
      started_at: item.systemChangeType === 'initial' && threadStartedAt ? threadStartedAt : item.startedAt,
      ordering_accuracy: item.orderingAccuracy,
    })
    existingKeys.add(key)
  }
  return entries
}

export function buildTrajectoryTimeline(entries = [], mode = 'sequence') {
  const normalizedMode = TRAJECTORY_TIMELINE_MODES.includes(mode) ? mode : 'sequence'
  const normalized = normalizeEntries(entries)
  let items
  if (normalizedMode === 'duration') items = durationPositions(normalized)
  else if (normalizedMode === 'time') items = wallClockPositions(normalized, true)
  else if (normalizedMode === 'actual') items = wallClockPositions(normalized, false)
  else items = sequencePositions(normalized)
  return { mode: normalizedMode, items, domain: timelineDomain(items), turnBoundaries: timelineTurnBoundaries(items) }
}

export function normalizeTrajectoryRange(range, domain) {
  if (!range || range.start == null || range.end == null) return null
  const start = Math.max(domain.min, Math.min(Number(range.start), Number(range.end)))
  const end = Math.min(domain.max, Math.max(Number(range.start), Number(range.end)))
  return end > start ? { start, end } : null
}

export function trajectoryRangeSourceUuids(model, range) {
  const normalized = normalizeTrajectoryRange(range, model.domain)
  if (!normalized) return null
  return new Set(model.items.filter((item) => item.start <= normalized.end && Math.max(item.start, item.end) >= normalized.start).map((item) => item.sourceUuid))
}

export function zoomTrajectoryView(view, domain, wheelDelta, anchorRatio = 0.5) {
  const current = normalizeTrajectoryRange(view, domain) || { start: domain.min, end: domain.max }
  const domainSize = domain.max - domain.min
  const currentSize = current.end - current.start
  const factor = wheelDelta > 0 ? 1.22 : 0.82
  const nextSize = Math.min(domainSize, Math.max(domainSize / 200, currentSize * factor))
  const anchor = current.start + currentSize * Math.min(1, Math.max(0, anchorRatio))
  let start = anchor - nextSize * Math.min(1, Math.max(0, anchorRatio))
  start = Math.min(domain.max - nextSize, Math.max(domain.min, start))
  return { start, end: start + nextSize }
}

export function panTrajectoryView(view, domain, delta) {
  const current = normalizeTrajectoryRange(view, domain) || { start: domain.min, end: domain.max }
  const size = current.end - current.start
  let start = current.start + Number(delta || 0)
  start = Math.min(domain.max - size, Math.max(domain.min, start))
  return { start, end: start + size }
}
