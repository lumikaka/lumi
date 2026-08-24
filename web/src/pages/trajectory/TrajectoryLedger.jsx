import { useLayoutEffect, useRef } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

import { useI18n } from '../../i18n/useI18n.js'

const statusKeys = {
  pending: 'trajectory.status.pending',
  running: 'trajectory.status.running',
  completed: 'trajectory.status.completed',
  error: 'trajectory.status.error',
  interrupted: 'trajectory.status.interrupted',
}

function rowEvent(row, t) {
  return t(`trajectory.kind.${row.kind || 'error'}`)
}

function rowContent(row) {
  return row.preview || '—'
}

function VirtualTrajectoryRow({ entry, onMeasure, children }) {
  const ref = useRef(null)
  useLayoutEffect(() => onMeasure?.(entry.key, ref.current), [entry.key, onMeasure])
  return <div className="trajectory-virtual-row" data-virtual-row-key={entry.key} ref={ref} style={{ transform: `translateY(${entry.start}px)` }}>{children}</div>
}

export default function TrajectoryLedger({ rows, virtualEntries, totalSize = 0, scrollRef, onMeasureRow, selectedUuid, selectedKey = '', collapsedTurns = new Set(), collapsedToolGroups = new Set(), toolGroups = new Map(), onToggleTurn, onToggleToolGroup, onSelect, onLoadEarlier, loadingEarlier = false, canLoadEarlier = false, filtered = false }) {
  const { formatCount, t } = useI18n()
  const entries = virtualEntries || rows.map((row, index) => ({ key: row.key, row, index, start: 0 }))

  const renderRow = (row) => {
    const selected = selectedKey ? selectedKey === row.key : selectedUuid === row.sourceUuid
    if (row.rowType === 'summary') {
      const family = row.summaryKind === 'turn' ? 'trajectory.summary.turn' : 'trajectory.summary.tools'
      const turnCollapsed = collapsedTurns.has(row.turnUuid)
      return (
        <div
          className={`trajectory-row trajectory-row--summary trajectory-row--summary-${row.summaryKind} trajectory-row--kind-${row.kind}${row.turnStart ? ' trajectory-row--turn-start' : ''}${row.turnEnd ? ' trajectory-row--turn-end' : ''}`}
          data-row-key={row.key}
          role="row"
          aria-rowindex={row.ariaRowIndex}
          tabIndex={row.summaryKind === 'turn' ? '0' : undefined}
          onClick={row.summaryKind === 'turn' ? () => onToggleTurn?.(row.turnUuid) : undefined}
          onKeyDown={row.summaryKind === 'turn' ? (event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onToggleTurn?.(row.turnUuid) } } : undefined}
        >
          <span className="trajectory-row__event" role="cell">
            {row.turnUuid ? <span className={`trajectory-turn-rail trajectory-turn-rail--${row.turn?.status || 'completed'}`} aria-hidden="true" /> : null}
            {row.turnStart ? (
              <button
                type="button"
                className="trajectory-turn-label"
                aria-expanded={!turnCollapsed}
                aria-label={t('trajectory.ledger.turn', { number: row.turn?.queue_sequence ?? '—' })}
                title={t(turnCollapsed ? 'trajectory.expand.turn' : 'trajectory.collapse.turn')}
                onKeyDown={(event) => event.stopPropagation()}
                onClick={(event) => { event.stopPropagation(); onToggleTurn?.(row.turnUuid) }}
              >{t('trajectory.ledger.turn', { number: row.turn?.queue_sequence ?? '—' })}</button>
            ) : null}
            <span className="trajectory-row__kind-slot"><strong>{t(`trajectory.kind.${row.kind}`)}</strong></span>
          </span>
          <span className="trajectory-row__content" role="cell"><span>{formatCount(family, row.hiddenCount)}</span></span>
        </div>
      )
    }
    const group = toolGroups.get(row.key)
    const toolGroupCollapsed = collapsedToolGroups.has(row.key)
    const turnCollapsed = collapsedTurns.has(row.turnUuid)
    return (
      <div
        className={`trajectory-row trajectory-row--${row.rowType} trajectory-row--kind-${row.kind || 'error'} trajectory-row--status-${row.status || 'completed'}${row.turnStart ? ' trajectory-row--turn-start' : ''}${row.turnEnd ? ' trajectory-row--turn-end' : ''}`}
        data-row-key={row.key}
        data-source-uuid={row.sourceUuid}
        data-trajectory-selection-key={row.key}
        data-turn-start={row.turnStart || undefined}
        data-turn-end={row.turnEnd || undefined}
        role="row"
        aria-rowindex={row.ariaRowIndex}
        aria-selected={selected}
        tabIndex="0"
        onClick={() => onSelect(row)}
        onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onSelect(row) } }}
      >
        <span className="trajectory-row__event" role="cell">
          {row.turnUuid ? <span className={`trajectory-turn-rail trajectory-turn-rail--${row.turn?.status || 'completed'}`} aria-hidden="true" /> : null}
          {selected ? <span className="trajectory-selection-rail" aria-hidden="true" /> : null}
          {row.turnStart ? (
            <button
              type="button"
              className="trajectory-turn-label"
              aria-expanded={!turnCollapsed}
              aria-label={t('trajectory.ledger.turn', { number: row.turn?.queue_sequence ?? '—' })}
              title={t(turnCollapsed ? 'trajectory.expand.turn' : 'trajectory.collapse.turn')}
              onKeyDown={(event) => event.stopPropagation()}
              onClick={(event) => { event.stopPropagation(); onToggleTurn?.(row.turnUuid) }}
            >{t('trajectory.ledger.turn', { number: row.turn?.queue_sequence ?? '—' })}</button>
          ) : null}
          {(row.requestBoundaries || []).map((request) => {
            const requestSelected = selectedKey ? selectedKey === request.key : selectedUuid === request.sourceUuid
            const label = t('trajectory.ledger.request', { number: request.requestOrdinal || '—' })
            return (
              <button
                type="button"
                className={`trajectory-request-boundary${requestSelected ? ' is-active' : ''}`}
                key={request.key}
                aria-label={label}
                aria-pressed={requestSelected}
                data-label={label}
                data-request-status={request.status}
                data-source-uuid={request.sourceUuid}
                data-trajectory-selection-key={request.key}
                style={{ '--trajectory-request-offset': `${request.runIndex * 8}px` }}
                onKeyDown={(event) => event.stopPropagation()}
                onClick={(event) => { event.stopPropagation(); onSelect(request) }}
              />
            )
          })}
          <span className="trajectory-row__kind-slot"><strong>{rowEvent(row, t)}</strong></span>
          {row.isSteering ? <small>{t('trajectory.steering')}</small> : null}
        </span>
        <span className="trajectory-row__content" role="cell"><span>{rowContent(row)}</span><span className="trajectory-row__trailing"><em>{t(statusKeys[row.status] || 'common.status.unknown_with_code', { code: row.status || '—' })}</em>{group ? <button type="button" className="trajectory-tool-group-toggle" aria-pressed={toolGroupCollapsed} title={t(toolGroupCollapsed ? 'trajectory.expand.tools' : 'trajectory.collapse.tools', { count: group.count })} aria-label={t(toolGroupCollapsed ? 'trajectory.expand.tools' : 'trajectory.collapse.tools', { count: group.count })} onKeyDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); onToggleToolGroup?.(row.key) }}>{toolGroupCollapsed ? <ChevronRight size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}</button> : null}</span></span>
      </div>
    )
  }

  return (
    <section className="trajectory-ledger" aria-label={t('trajectory.ledger.label')}>
      <div className="trajectory-ledger__head" role="row">
        <span role="columnheader">{t('trajectory.ledger.sequence')}</span>
        <span role="columnheader">{t('trajectory.ledger.event')}</span>
        <span role="columnheader">{t('trajectory.ledger.content')}</span>
      </div>
      {canLoadEarlier || loadingEarlier ? (
        <div className="trajectory-ledger__history">
          <button type="button" className="button-secondary" disabled={loadingEarlier || !canLoadEarlier} onClick={onLoadEarlier}>
            {t(loadingEarlier ? 'trajectory.history.loading_earlier' : 'trajectory.history.load_earlier')}
          </button>
        </div>
      ) : null}
      <div className="trajectory-ledger__scroll" ref={scrollRef}>
        <div className="trajectory-ledger__rows trajectory-ledger__rows--virtual" role="rowgroup" style={{ height: `${totalSize}px` }}>
          {entries.map((entry) => <VirtualTrajectoryRow entry={entry} key={entry.key} onMeasure={onMeasureRow}>{renderRow(entry.row)}</VirtualTrajectoryRow>)}
          {!rows.length ? <p className="trajectory-ledger__empty">{t(filtered ? 'trajectory.search.no_results' : 'trajectory.empty')}</p> : null}
        </div>
      </div>
    </section>
  )
}
