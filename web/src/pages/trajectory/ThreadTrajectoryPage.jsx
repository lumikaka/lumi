import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useParams, useSearchParams } from 'react-router-dom'

import { getChatTrajectory } from '../../api/chat.js'
import LocalizedErrorMessage from '../../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../../i18n/useI18n.js'
import TrajectoryInspector from './TrajectoryInspector.jsx'
import TrajectoryLedger from './TrajectoryLedger.jsx'
import TrajectoryStats from './TrajectoryStats.jsx'
import TrajectoryTimeline from './TrajectoryTimeline.jsx'
import TrajectoryToolbar from './TrajectoryToolbar.jsx'
import { applyTrajectoryCollapse, buildAssistantToolGroups } from './trajectoryCollapse.js'
import { applyTrajectoryUpserts, combineTrajectoryPages } from './trajectoryProjector.js'
import { filterTrajectoryRows, reconcileTrajectorySearchIndex, updateTrajectoryRequestSearchDocument } from './trajectorySearch.js'
import { trajectoryTimelineEntries } from './trajectoryTimeline.js'
import {
  captureTrajectoryVirtualAnchor,
  isTrajectoryAtTail,
  measureTrajectoryRows,
  restoreTrajectoryVirtualAnchor,
  shouldFollowTrajectoryTail,
  shouldLoadEarlierTrajectory,
  trajectoryVirtualWindow,
} from './trajectoryVirtualRows.js'

const TRAJECTORY_PAGE_SIZE = 80
const INSPECTOR_WIDTH_KEY = 'lumi.trajectory.inspectorWidth'

function initialInspectorWidth() {
  if (typeof window === 'undefined') return 390
  try {
    const stored = window.localStorage?.getItem(INSPECTOR_WIDTH_KEY)
    if (stored == null || stored === '') return 390
    const value = Number(stored)
    return Number.isFinite(value) ? Math.min(620, Math.max(320, value)) : 390
  } catch { return 390 }
}

function resolveSelectedRow(rows, sourceUuid, sourceKind = '') {
  if (!sourceUuid) return null
  const candidates = rows.filter((row) => row.sourceUuid === sourceUuid
    || row.source?.uuid === sourceUuid
    || row.source?.call_item_uuid === sourceUuid
    || row.source?.result_item_uuid === sourceUuid)
  if (sourceKind) return candidates.find((row) => row.sourceKind === sourceKind) || candidates[0] || null
  return candidates.find((row) => row.rowType === 'request') || candidates[0] || null
}

export default function ThreadTrajectoryPage({ projectUuid }) {
  const { threadUuid } = useParams()
  const { t } = useI18n()
  const [searchParams, setSearchParams] = useSearchParams()
  const initialItemUuidRef = useRef(searchParams.get('item_uuid') || '')
  const [inspectorWidth, setInspectorWidth] = useState(initialInspectorWidth)
  const [collapsedTurns, setCollapsedTurns] = useState(() => new Set())
  const [collapsedToolGroups, setCollapsedToolGroups] = useState(() => new Set())
  const [search, setSearch] = useState('')
  const [kindFilter, setKindFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [timelineRange, setTimelineRange] = useState(null)
  const [searchIndex, setSearchIndex] = useState(() => new Map())
  const [rowHeights, setRowHeights] = useState(() => new Map())
  const [viewport, setViewport] = useState({ scrollTop: 0, height: 600 })
  const resizeRef = useRef(null)
  const ledgerRef = useRef(null)
  const pendingHistoryAnchorRef = useRef(null)
  const atTailRef = useRef(true)
  const initialPositionedRef = useRef(false)
  const previousLastKeyRef = useRef('')
  const selectedUuid = searchParams.get('item_uuid') || ''
  const selectedSourceKind = searchParams.get('item_kind') || ''

  const trajectoryQuery = useInfiniteQuery({
    queryKey: ['chat-trajectory', projectUuid, threadUuid],
    initialPageParam: { before: '', itemUuid: initialItemUuidRef.current },
    queryFn: ({ pageParam }) => getChatTrajectory(projectUuid, threadUuid, {
      before: pageParam?.before || '',
      itemUuid: pageParam?.itemUuid || '',
      limit: TRAJECTORY_PAGE_SIZE,
    }),
    getPreviousPageParam: (firstPage) => firstPage?.history_complete || !firstPage?.cursor_pagination?.has_more
      ? undefined
      : { before: firstPage.cursor_pagination.prev_cursor },
    getNextPageParam: () => undefined,
  })

  const baseProjection = useMemo(() => combineTrajectoryPages(trajectoryQuery.data?.pages || []), [trajectoryQuery.data?.pages])
  const baseSelected = useMemo(() => resolveSelectedRow(baseProjection.rows, selectedUuid, selectedSourceKind), [baseProjection.rows, selectedSourceKind, selectedUuid])
  const anchorQuery = useQuery({
    queryKey: ['chat-trajectory', projectUuid, threadUuid, 'anchor', selectedUuid],
    queryFn: () => getChatTrajectory(projectUuid, threadUuid, { itemUuid: selectedUuid, limit: TRAJECTORY_PAGE_SIZE }),
    enabled: Boolean(selectedUuid && trajectoryQuery.isSuccess && !baseSelected),
  })
  const projection = useMemo(() => anchorQuery.data ? applyTrajectoryUpserts(baseProjection, anchorQuery.data) : baseProjection, [anchorQuery.data, baseProjection])
  const selected = useMemo(() => resolveSelectedRow(projection.rows, selectedUuid, selectedSourceKind), [projection.rows, selectedSourceKind, selectedUuid])
  const toolGroups = useMemo(() => buildAssistantToolGroups(projection.rows), [projection.rows])
  const timelineEntries = useMemo(() => trajectoryTimelineEntries(projection), [projection])
  const collapsibleTurnUuids = useMemo(() => projection.turns.map((turn) => turn.uuid), [projection.turns])
  const collapsibleToolGroupKeys = useMemo(() => [...toolGroups.keys()], [toolGroups])
  const allTurnsCollapsed = collapsibleTurnUuids.length > 0 && collapsibleTurnUuids.every((uuid) => collapsedTurns.has(uuid))
  const allToolGroupsCollapsed = collapsibleToolGroupKeys.length > 0 && collapsibleToolGroupKeys.every((key) => collapsedToolGroups.has(key))
  const searchedRows = useMemo(() => filterTrajectoryRows(projection.rows, searchIndex, { query: search, kind: kindFilter, status: statusFilter, rangeKeys: timelineRange?.sourceUuids || null }), [kindFilter, projection.rows, search, searchIndex, statusFilter, timelineRange])
  const visibleRows = useMemo(() => {
    return applyTrajectoryCollapse(searchedRows, projection.rows, { collapsedTurns, collapsedToolGroups, toolGroups })
  }, [collapsedToolGroups, collapsedTurns, projection.rows, searchedRows, toolGroups])
  const rowMeasurement = useMemo(() => measureTrajectoryRows(visibleRows, rowHeights), [rowHeights, visibleRows])
  const virtualEntries = useMemo(() => trajectoryVirtualWindow(rowMeasurement, viewport.scrollTop, viewport.height), [rowMeasurement, viewport.height, viewport.scrollTop])

  useEffect(() => {
    setSearchIndex((current) => reconcileTrajectorySearchIndex(current, projection.rows))
  }, [projection.rows])

  const recordRequestDetail = useCallback((requestUuid, detail) => {
    setSearchIndex((current) => updateTrajectoryRequestSearchDocument(current, requestUuid, detail))
  }, [])

  const measureRow = useCallback((key, node) => {
    if (!node) return undefined
    const update = () => {
      const height = node.getBoundingClientRect().height
      if (!Number.isFinite(height) || height <= 0) return
      setRowHeights((current) => {
        if (Math.abs((current.get(key) || 0) - height) < 0.5) return current
        const next = new Map(current)
        next.set(key, height)
        return next
      })
    }
    update()
    if (typeof ResizeObserver === 'undefined') return undefined
    const observer = new ResizeObserver(update)
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  const selectTrajectoryUuid = useCallback((sourceUuid, sourceKind = '') => {
    const next = new URLSearchParams(searchParams)
    next.set('item_uuid', sourceUuid)
    if (sourceKind) next.set('item_kind', sourceKind)
    else next.delete('item_kind')
    setSearchParams(next)
  }, [searchParams, setSearchParams])

  const selectRow = useCallback((row) => selectTrajectoryUuid(row.sourceUuid, row.sourceKind), [selectTrajectoryUuid])

  const closeInspector = useCallback(() => {
    const next = new URLSearchParams(searchParams)
    next.delete('item_uuid')
    next.delete('item_kind')
    setSearchParams(next)
  }, [searchParams, setSearchParams])

  const toggleTurn = useCallback((turnUuid) => {
    setCollapsedTurns((current) => {
      const next = new Set(current)
      if (next.has(turnUuid)) next.delete(turnUuid)
      else next.add(turnUuid)
      return next
    })
  }, [])

  const toggleToolGroup = useCallback((assistantKey) => {
    setCollapsedToolGroups((current) => {
      const next = new Set(current)
      if (next.has(assistantKey)) next.delete(assistantKey)
      else next.add(assistantKey)
      return next
    })
  }, [])

  const toggleAllTurns = useCallback(() => {
    setCollapsedTurns((current) => {
      const next = new Set(current)
      if (collapsibleTurnUuids.length && collapsibleTurnUuids.every((uuid) => next.has(uuid))) collapsibleTurnUuids.forEach((uuid) => next.delete(uuid))
      else collapsibleTurnUuids.forEach((uuid) => next.add(uuid))
      return next
    })
  }, [collapsibleTurnUuids])

  const toggleAllToolGroups = useCallback(() => {
    setCollapsedToolGroups((current) => {
      const next = new Set(current)
      if (collapsibleToolGroupKeys.length && collapsibleToolGroupKeys.every((key) => next.has(key))) collapsibleToolGroupKeys.forEach((key) => next.delete(key))
      else collapsibleToolGroupKeys.forEach((key) => next.add(key))
      return next
    })
  }, [collapsibleToolGroupKeys])

  const loadEarlier = useCallback(() => {
    const element = ledgerRef.current
    if (!element || !trajectoryQuery.hasPreviousPage || trajectoryQuery.isFetchingPreviousPage || pendingHistoryAnchorRef.current) return
    pendingHistoryAnchorRef.current = {
      anchor: captureTrajectoryVirtualAnchor(rowMeasurement, element.scrollTop),
      previousCount: rowMeasurement.entries.length,
      previousFirstKey: rowMeasurement.entries[0]?.key || '',
      previousTotalSize: rowMeasurement.totalSize,
      previousScrollTop: element.scrollTop,
      previousPageCount: trajectoryQuery.data?.pages?.length || 0,
    }
    atTailRef.current = false
    void trajectoryQuery.fetchPreviousPage().catch(() => { pendingHistoryAnchorRef.current = null })
  }, [rowMeasurement, trajectoryQuery])

  const startResize = useCallback((event) => {
    event.preventDefault()
    resizeRef.current = { startX: event.clientX, startWidth: inspectorWidth }
  }, [inspectorWidth])

  useEffect(() => {
    const resize = (event) => {
      if (!resizeRef.current) return
      const next = Math.min(620, Math.max(320, resizeRef.current.startWidth + resizeRef.current.startX - event.clientX))
      setInspectorWidth(next)
    }
    const finish = () => {
      if (!resizeRef.current) return
      resizeRef.current = null
      try { window.localStorage.setItem(INSPECTOR_WIDTH_KEY, String(inspectorWidth)) } catch { /* restricted browser */ }
    }
    window.addEventListener('pointermove', resize)
    window.addEventListener('pointerup', finish)
    return () => {
      window.removeEventListener('pointermove', resize)
      window.removeEventListener('pointerup', finish)
    }
  }, [inspectorWidth])

  useEffect(() => {
    const element = ledgerRef.current
    if (!element) return undefined
    const syncViewport = () => {
      const next = { scrollTop: element.scrollTop, height: element.clientHeight || 600 }
      setViewport((current) => current.scrollTop === next.scrollTop && current.height === next.height ? current : next)
      atTailRef.current = isTrajectoryAtTail({ scrollTop: element.scrollTop, scrollHeight: element.scrollHeight, clientHeight: element.clientHeight })
      if (shouldLoadEarlierTrajectory({ scrollTop: element.scrollTop, hasPreviousPage: trajectoryQuery.hasPreviousPage, fetchingPreviousPage: trajectoryQuery.isFetchingPreviousPage })) loadEarlier()
    }
    syncViewport()
    element.addEventListener('scroll', syncViewport, { passive: true })
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(syncViewport)
    observer?.observe(element)
    return () => {
      element.removeEventListener('scroll', syncViewport)
      observer?.disconnect()
    }
  }, [loadEarlier, trajectoryQuery.hasPreviousPage, trajectoryQuery.isFetchingPreviousPage])

  useLayoutEffect(() => {
    const pending = pendingHistoryAnchorRef.current
    const element = ledgerRef.current
    if (!pending || !element) return
    const firstKey = rowMeasurement.entries[0]?.key || ''
    const historyChanged = rowMeasurement.entries.length !== pending.previousCount
      || firstKey !== pending.previousFirstKey
      || (trajectoryQuery.data?.pages?.length || 0) > pending.previousPageCount
    if (!historyChanged) return
    const anchored = restoreTrajectoryVirtualAnchor(rowMeasurement, pending.anchor)
    element.scrollTop = anchored ?? Math.max(0, pending.previousScrollTop + rowMeasurement.totalSize - pending.previousTotalSize)
    pendingHistoryAnchorRef.current = null
    setViewport({ scrollTop: element.scrollTop, height: element.clientHeight || 600 })
  }, [rowMeasurement, trajectoryQuery.data?.pages?.length])

  useEffect(() => {
    if (!trajectoryQuery.isFetchingPreviousPage && !trajectoryQuery.hasPreviousPage) pendingHistoryAnchorRef.current = null
  }, [trajectoryQuery.hasPreviousPage, trajectoryQuery.isFetchingPreviousPage])

  useLayoutEffect(() => {
    const element = ledgerRef.current
    if (!element || !rowMeasurement.totalSize || initialPositionedRef.current) return
    if (selectedUuid) return
    element.scrollTop = rowMeasurement.totalSize
    initialPositionedRef.current = true
    atTailRef.current = true
    setViewport({ scrollTop: element.scrollTop, height: element.clientHeight || 600 })
  }, [rowMeasurement.totalSize, selectedUuid])

  useLayoutEffect(() => {
    const nextLastKey = visibleRows.at(-1)?.key || ''
    const element = ledgerRef.current
    if (element && initialPositionedRef.current && shouldFollowTrajectoryTail({ wasAtTail: atTailRef.current, previousLastKey: previousLastKeyRef.current, nextLastKey, prepending: Boolean(pendingHistoryAnchorRef.current) })) {
      element.scrollTop = rowMeasurement.totalSize
      setViewport({ scrollTop: element.scrollTop, height: element.clientHeight || 600 })
    }
    previousLastKeyRef.current = nextLastKey
  }, [rowMeasurement.totalSize, visibleRows])

  useEffect(() => {
    if (!selected) return
    const turnUuid = selected.turnUuid
    if (turnUuid && collapsedTurns.has(turnUuid)) {
      setCollapsedTurns((current) => {
        const next = new Set(current)
        next.delete(turnUuid)
        return next
      })
      return
    }
    const owningGroup = [...toolGroups.values()].find((group) => group.toolKeys.has(selected.key))
    if (owningGroup && collapsedToolGroups.has(owningGroup.assistantKey)) {
      setCollapsedToolGroups((current) => {
        const next = new Set(current)
        next.delete(owningGroup.assistantKey)
        return next
      })
      return
    }
    const entry = rowMeasurement.entries.find((candidate) => candidate.key === selected.key
      || candidate.row.requestBoundaries?.some((request) => request.sourceUuid === selected.sourceUuid))
    if (entry && ledgerRef.current) {
      const target = Math.max(0, entry.start - Math.max(0, ledgerRef.current.clientHeight - entry.size) / 2)
      ledgerRef.current.scrollTop = target
      setViewport({ scrollTop: target, height: ledgerRef.current.clientHeight || 600 })
      initialPositionedRef.current = true
      atTailRef.current = isTrajectoryAtTail({ scrollTop: target, scrollHeight: rowMeasurement.totalSize, clientHeight: ledgerRef.current.clientHeight })
    }
    const frame = window.requestAnimationFrame(() => {
      const row = [...(ledgerRef.current?.querySelectorAll('[data-trajectory-selection-key]') || [])].find((element) => element.dataset.trajectorySelectionKey === selected.key)
      row?.scrollIntoView({ block: 'center' })
      row?.focus?.({ preventScroll: true })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [collapsedToolGroups, collapsedTurns, rowMeasurement, selected, toolGroups])

  if (trajectoryQuery.isLoading) return <div className="trajectory-page trajectory-page--loading"><p>{t('trajectory.loading')}</p></div>

  return (
    <div className="trajectory-page" style={{ '--trajectory-inspector-width': `${inspectorWidth}px` }}>
      <TrajectoryToolbar projection={projection} fetching={trajectoryQuery.isFetching} search={search} kind={kindFilter} status={statusFilter} onSearchChange={setSearch} onKindChange={setKindFilter} onStatusChange={setStatusFilter} onRefresh={() => trajectoryQuery.refetch()} />
      <TrajectoryTimeline
        entries={timelineEntries}
        range={timelineRange}
        selectedUuid={selected?.sourceUuid || selectedUuid}
        selectedSourceKind={selected?.sourceKind || selectedSourceKind}
        allTurnsCollapsed={allTurnsCollapsed}
        allToolGroupsCollapsed={allToolGroupsCollapsed}
        onToggleAllTurns={toggleAllTurns}
        onToggleAllToolGroups={toggleAllToolGroups}
        onRangeChange={setTimelineRange}
        onSelect={(item) => selectTrajectoryUuid(item.sourceUuid, item.sourceKind)}
      />
      <div className="trajectory-page__error"><LocalizedErrorMessage error={trajectoryQuery.error || anchorQuery.error} /></div>
      <div className={`trajectory-workbench ${selected ? 'trajectory-workbench--inspector-open' : ''}`}>
        <div className="trajectory-ledger-shell">
          <TrajectoryLedger
            rows={visibleRows}
            virtualEntries={virtualEntries}
            totalSize={rowMeasurement.totalSize}
            scrollRef={ledgerRef}
            onMeasureRow={measureRow}
            selectedUuid={selected?.sourceUuid || selectedUuid}
            selectedKey={selected?.key || ''}
            collapsedTurns={collapsedTurns}
            collapsedToolGroups={collapsedToolGroups}
            toolGroups={toolGroups}
            onToggleTurn={toggleTurn}
            onToggleToolGroup={toggleToolGroup}
            onSelect={selectRow}
            filtered={Boolean(search || kindFilter || statusFilter || timelineRange)}
            canLoadEarlier={Boolean(trajectoryQuery.hasPreviousPage)}
            loadingEarlier={trajectoryQuery.isFetchingPreviousPage}
            onLoadEarlier={loadEarlier}
          />
        </div>
        <TrajectoryInspector projectUuid={projectUuid} selected={selected} onClose={closeInspector} onResizeStart={startResize} onRequestDetailLoaded={recordRequestDetail} />
      </div>
      <TrajectoryStats overview={projection.overview} />
    </div>
  )
}
