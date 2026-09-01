import { useEffect, useMemo, useRef, useState } from 'react'
import { SquareMinus, SquarePlus } from 'lucide-react'

import { useI18n } from '../../i18n/useI18n.js'
import { formatTrajectoryDuration } from './trajectoryStats.js'
import {
  buildTrajectoryTimeline,
  normalizeTrajectoryRange,
  panTrajectoryView,
  trajectoryRangeSourceUuids,
  trajectoryTimelineTicks,
  zoomTrajectoryView,
} from './trajectoryTimeline.js'

const laneMessageKeys = ['trajectory.timeline.lane.input', 'trajectory.timeline.lane.model', 'trajectory.timeline.lane.tools']
const timelineModes = ['duration', 'time', 'actual', 'sequence']
const kindMessageKeys = {
  model_request: 'trajectory.filter.request',
  system: 'trajectory.kind.system',
  user: 'trajectory.kind.user',
  context: 'trajectory.kind.context',
  assistant: 'trajectory.kind.assistant',
  tool: 'trajectory.kind.tool',
  compaction: 'trajectory.kind.compaction',
  error: 'trajectory.kind.error',
}
const statusMessageKeys = {
  pending: 'trajectory.status.pending',
  running: 'trajectory.status.running',
  completed: 'trajectory.status.completed',
  error: 'trajectory.status.error',
  interrupted: 'trajectory.status.interrupted',
}

function viewSignature(domain, mode) {
  return `${mode}:${domain.min}:${domain.max}`
}

function fullView(domain) {
  return { start: domain.min, end: domain.max }
}

function itemTitle(item, t) {
  const kind = item.activity === 'user_wait'
    ? t('trajectory.timeline.user_wait')
    : t(kindMessageKeys[item.kind] || 'trajectory.kind.system')
  const duration = item.durationMs == null
    ? ''
    : t('trajectory.timeline.item_duration', { duration: formatTrajectoryDuration(item.durationMs) })
  return [kind, item.preview || t(statusMessageKeys[item.status] || 'trajectory.status.completed'), duration].filter(Boolean).join(' · ')
}

function tickLabel(value, mode) {
  if (mode === 'sequence') return `#${Math.max(1, Math.floor(value) + 1)}`
  return formatTrajectoryDuration(Math.max(0, value), '0s')
}

function timelineSummary(model, view, mode, t) {
  if (mode === 'sequence') return t('trajectory.timeline.summary.events', { count: model.items.length })
  if (mode === 'duration') return t('trajectory.timeline.summary.recorded', { duration: formatTrajectoryDuration(model.recordedDurationMs, '0s') })
  const duration = formatTrajectoryDuration(Math.max(0, view.end - view.start), '0s')
  return t(mode === 'actual' ? 'trajectory.timeline.summary.actual' : 'trajectory.timeline.summary.compressed', { duration })
}

function clamp(value, min = 0, max = 1) {
  return Math.min(max, Math.max(min, value))
}

function centeredRange(center, width, domain) {
  const size = Math.min(domain.max - domain.min, Math.max(0, width))
  const start = Math.min(Math.max(center - size / 2, domain.min), domain.max - size)
  return { start, end: start + size }
}

export default function TrajectoryTimeline({
  entries = [],
  range,
  selectedUuid = '',
  selectedSourceKind = '',
  allTurnsCollapsed = false,
  allToolGroupsCollapsed = false,
  onToggleAllTurns,
  onToggleAllToolGroups,
  onRangeChange,
  onSelect,
}) {
  const { t } = useI18n()
  const [mode, setMode] = useState('duration')
  const model = useMemo(() => buildTrajectoryTimeline(entries, mode), [entries, mode])
  const [view, setView] = useState(() => fullView(model.domain))
  const [hoverFraction, setHoverFraction] = useState(null)
  const [hoveredKey, setHoveredKey] = useState('')
  const dragRef = useRef(null)
  const trackRef = useRef(null)
  const signature = viewSignature(model.domain, mode)

  useEffect(() => { setView(fullView(model.domain)) }, [signature])

  const activeView = normalizeTrajectoryRange(view, model.domain) || fullView(model.domain)
  const activeRange = normalizeTrajectoryRange(range?.mode === mode ? range : null, model.domain)
  const domainSize = Math.max(1e-9, activeView.end - activeView.start)
  const valueAtClientX = (clientX) => {
    const rect = trackRef.current?.getBoundingClientRect()
    const fraction = rect ? clamp((clientX - rect.left) / Math.max(1, rect.width)) : 0.5
    return activeView.start + fraction * domainSize
  }

  const selectMode = (nextMode) => {
    setMode(nextMode)
    onRangeChange?.(null)
  }

  const publishRange = (nextRange) => {
    const normalized = normalizeTrajectoryRange(nextRange, model.domain)
    onRangeChange?.(normalized ? { ...normalized, mode, sourceUuids: trajectoryRangeSourceUuids(model, normalized) } : null)
  }

  const handleWheel = (event) => {
    event.preventDefault()
    const rect = trackRef.current?.getBoundingClientRect()
    const ratio = rect ? clamp((event.clientX - rect.left) / Math.max(1, rect.width)) : 0.5
    setView(zoomTrajectoryView(activeView, model.domain, event.deltaY, ratio))
  }

  const handlePointerDown = (event) => {
    if (event.button !== 0 && event.button !== 2) return
    event.currentTarget.setPointerCapture?.(event.pointerId)
    const start = valueAtClientX(event.clientX)
    dragRef.current = {
      kind: event.button === 2 ? 'pan' : 'range',
      pointerId: event.pointerId,
      start,
      startX: event.clientX,
      view: activeView,
    }
  }

  const handlePointerMove = (event) => {
    const rect = trackRef.current?.getBoundingClientRect()
    if (rect) setHoverFraction(clamp((event.clientX - rect.left) / Math.max(1, rect.width)))
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    if (drag.kind === 'range') {
      publishRange({ start: drag.start, end: valueAtClientX(event.clientX) })
      return
    }
    const delta = rect ? -(event.clientX - drag.startX) / Math.max(1, rect.width) * (drag.view.end - drag.view.start) : 0
    setView(panTrajectoryView(drag.view, model.domain, delta))
  }

  const handlePointerEnd = (event) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    dragRef.current = null
    if (drag.kind === 'pan') return
    const end = valueAtClientX(event.clientX)
    if (Math.abs(event.clientX - drag.startX) < 3) {
      const minimum = Math.max(domainSize / Math.max(1, model.items.length), domainSize / 200)
      publishRange(centeredRange(end, minimum, model.domain))
    } else publishRange({ start: drag.start, end })
  }

  const TurnIcon = allTurnsCollapsed ? SquarePlus : SquareMinus
  const CallsIcon = allToolGroupsCollapsed ? SquarePlus : SquareMinus
  const visibleItems = model.items.filter((item) => item.start <= activeView.end && Math.max(item.start, item.end) >= activeView.start)
  const leftPercent = (value) => (value - activeView.start) / domainSize * 100
  const ticks = trajectoryTimelineTicks(activeView, mode)
  const hoveredItem = hoveredKey ? model.items.find((item) => item.key === hoveredKey) : null
  const hasUserWait = model.items.some((item) => item.activity === 'user_wait')

  return (
    <section className="trajectory-timeline" aria-label={t('trajectory.timeline.title')}>
      <header className="trajectory-timeline__header" role="toolbar" aria-label={t('trajectory.timeline.mode')}>
        <div className="trajectory-timeline__mode-switch" role="group" aria-label={t('trajectory.timeline.mode')}>
          {timelineModes.map((name) => (
            <button
              type="button"
              key={name}
              aria-pressed={mode === name}
              title={t(`trajectory.timeline.mode.${name}.description`)}
              onClick={() => selectMode(name)}
            >{t(`trajectory.timeline.mode.${name}`)}</button>
          ))}
        </div>
        <div className="trajectory-timeline__header-actions">
          <button
            type="button"
            className="trajectory-timeline__toolbar-button"
            aria-pressed={allTurnsCollapsed}
            title={t(allTurnsCollapsed ? 'trajectory.timeline.expand_turns' : 'trajectory.timeline.collapse_turns')}
            onClick={onToggleAllTurns}
          ><TurnIcon size={12} aria-hidden="true" />{t('trajectory.timeline.turns')}</button>
          <button
            type="button"
            className="trajectory-timeline__toolbar-button"
            aria-pressed={allToolGroupsCollapsed}
            title={t(allToolGroupsCollapsed ? 'trajectory.timeline.expand_calls' : 'trajectory.timeline.collapse_calls')}
            onClick={onToggleAllToolGroups}
          ><CallsIcon size={12} aria-hidden="true" />{t('trajectory.timeline.calls')}</button>
        </div>
        {hasUserWait ? <span className="trajectory-timeline__wait-legend"><i aria-hidden="true" />{t('trajectory.timeline.user_wait')}</span> : null}
        <output className="trajectory-timeline__readout" aria-live="polite">
          {hoveredItem ? itemTitle(hoveredItem, t) : timelineSummary(model, activeView, mode, t)}
        </output>
      </header>
      {model.items.length ? (
        <div className="trajectory-timeline__plot">
          <div className="trajectory-timeline__labels" aria-hidden="true">{laneMessageKeys.map((key) => <span key={key}>{t(key)}</span>)}</div>
          <div
            className="trajectory-timeline__track"
            ref={trackRef}
            tabIndex="0"
            title={t('trajectory.timeline.interaction')}
            onWheel={handleWheel}
            onPointerDown={handlePointerDown}
            onPointerMove={handlePointerMove}
            onPointerUp={handlePointerEnd}
            onPointerCancel={() => { dragRef.current = null; setHoverFraction(null) }}
            onPointerLeave={() => { if (!dragRef.current) setHoverFraction(null) }}
            onDoubleClick={() => { setView(fullView(model.domain)); onRangeChange?.(null) }}
            onContextMenu={(event) => event.preventDefault()}
            onKeyDown={(event) => { if (event.key === 'Escape') { setView(fullView(model.domain)); onRangeChange?.(null) } }}
          >
            {hoverFraction != null && !dragRef.current ? <span className="trajectory-timeline__hover-line" aria-hidden="true" style={{ '--trajectory-hover-left': `${hoverFraction * 100}%` }} /> : null}
            <span className="trajectory-timeline__axis" aria-hidden="true">
              {ticks.map((value, index) => {
                const percent = leftPercent(value)
                const atStartEdge = index === 0 && percent < 4
                const atEndEdge = index === ticks.length - 1 && percent > 96
                return <span className="trajectory-timeline__tick" data-edge-start={atStartEdge || undefined} data-edge-end={atEndEdge || undefined} key={`${mode}:${value}`} style={{ '--trajectory-tick-left': `${percent}%` }}><em>{tickLabel(value, mode)}</em></span>
              })}
            </span>
            {activeRange ? (
              <>
                <span className="trajectory-timeline__selection" aria-hidden="true" style={{ '--trajectory-selection-left': `${leftPercent(activeRange.start)}%`, '--trajectory-selection-width': `${(activeRange.end - activeRange.start) / domainSize * 100}%` }} />
                <span className="trajectory-timeline__selection-edges" aria-hidden="true" style={{ '--trajectory-selection-left': `${leftPercent(activeRange.start)}%`, '--trajectory-selection-width': `${(activeRange.end - activeRange.start) / domainSize * 100}%` }} />
              </>
            ) : null}
            <span className="trajectory-timeline__turn-boundaries" aria-hidden="true">
              {model.turnBoundaries.filter((boundary) => boundary.position > activeView.start && boundary.position <= activeView.end).map((boundary) => <span className="trajectory-timeline__turn-boundary" key={boundary.key} style={{ '--trajectory-turn-left': `${leftPercent(boundary.position)}%` }} />)}
            </span>
            <span className="trajectory-timeline__lanes">
              {visibleItems.map((item) => {
                const itemSelected = activeRange ? item.start <= activeRange.end && item.end >= activeRange.start : true
                const current = selectedUuid === item.sourceUuid && (!selectedSourceKind || selectedSourceKind === item.sourceKind)
                const width = Math.max(0, item.end - item.start) / domainSize * 100
                return (
                  <button
                    type="button"
                    className={`trajectory-timeline__span trajectory-timeline__span--${item.kind}`}
                    data-status={item.status}
                    data-activity={item.activity}
                    data-marker={!item.span || undefined}
                    data-current={current || undefined}
                    data-selected={activeRange ? itemSelected ? 'true' : 'false' : undefined}
                    key={item.key}
                    aria-label={itemTitle(item, t)}
                    title={itemTitle(item, t)}
                    style={{ '--trajectory-span-left': `${leftPercent(item.start)}%`, '--trajectory-span-width': `${width}%`, '--trajectory-span-lane': item.lane }}
                    onPointerDown={(event) => event.stopPropagation()}
                    onPointerEnter={() => setHoveredKey(item.key)}
                    onPointerLeave={() => setHoveredKey('')}
                    onFocus={() => setHoveredKey(item.key)}
                    onBlur={() => setHoveredKey('')}
                    onClick={(event) => { event.stopPropagation(); onSelect?.(item) }}
                  />
                )
              })}
            </span>
          </div>
        </div>
      ) : <p className="trajectory-timeline__empty">{t('trajectory.timeline.empty')}</p>}
      <span className="trajectory-timeline__sr-status" aria-live="polite">{activeRange ? t('trajectory.filter.range_active') : ''}</span>
    </section>
  )
}

export function timelineRangeKeys(entries, mode, range) {
  return trajectoryRangeSourceUuids(buildTrajectoryTimeline(entries, mode), range)
}
