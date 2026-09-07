import { useLayoutEffect, useRef } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

import { useI18n } from '../../i18n/useI18n.js'
import { trajectoryRequestOrigin, trajectoryWorkflowTitle } from './trajectoryRequestOrigin.js'

const statusKeys = {
  pending: 'trajectory.status.pending',
  running: 'trajectory.status.running',
  completed: 'trajectory.status.completed',
  error: 'trajectory.status.error',
  interrupted: 'trajectory.status.interrupted',
}

function VirtualTrajectoryRow({ entry, onMeasure, children }) {
  const ref = useRef(null)
  useLayoutEffect(() => onMeasure?.(entry.key, ref.current), [entry.key, onMeasure])
  return <div className="trajectory-virtual-row" data-virtual-row-key={entry.key} ref={ref} style={{ transform: `translateY(${entry.start}px)` }}>{children}</div>
}

export default function TrajectoryLedger({ pictureBook, rows, virtualEntries, totalSize = 0, scrollRef, onMeasureRow, selectedUuid, selectedKey = '', collapsedTurns = new Set(), collapsedToolGroups = new Set(), toolGroups = new Map(), onToggleTurn, onToggleToolGroup, onSelect, onLoadEarlier, loadingEarlier = false, canLoadEarlier = false, filtered = false }) {
  const { formatCount, formatNumber, t } = useI18n()
  const entries = virtualEntries || rows.map((row, index) => ({ key: row.key, row, index, start: 0 }))

  const renderRequestBoundary = (request, detached = false) => {
    const selected = selectedKey ? selectedKey === request.key : selectedUuid === request.sourceUuid
    const label = [
      t('trajectory.ledger.request', { number: request.requestOrdinal || '—' }),
      trajectoryRequestOrigin(request.source, t, pictureBook),
      request.requestType ? t(`trajectory.request_type.${request.requestType}`) : '',
      request.source?.model,
      t(statusKeys[request.status] || 'common.status.unknown_with_code', { code: request.status || '—' }),
      request.durationMs != null ? t('trajectory.request.duration', { seconds: formatNumber(request.durationMs / 1000, { maximumFractionDigits: 3 }) }) : '',
      request.attempt > 1 ? t('trajectory.request.attempt', { number: request.attempt }) : '',
      request.source?.error_code,
    ].filter(Boolean).join(' · ')
    return (
      <button
        type="button"
        className={`trajectory-request-boundary${detached ? ' trajectory-request-boundary--detached' : ''}${selected ? ' is-active' : ''}`}
        key={request.key}
        aria-label={label}
        aria-pressed={selected}
        data-label={label}
        data-request-status={request.status}
        data-source-uuid={request.sourceUuid}
        data-trajectory-selection-key={request.key}
        style={{ '--trajectory-request-offset': `${(request.runIndex || 0) * 20}px` }}
        onKeyDown={(event) => event.stopPropagation()}
        onClick={(event) => { event.stopPropagation(); onSelect(request) }}
      />
    )
  }

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
    const requestOnly = row.rowType === 'request'
    const preview = row.kind === 'workflow' ? trajectoryWorkflowTitle(row.source, t, pictureBook) : row.preview
    return (
      <div
        className={`trajectory-row ${requestOnly ? 'trajectory-row--request-marker' : `trajectory-row--${row.rowType} trajectory-row--kind-${row.kind || 'error'} trajectory-row--status-${row.status || 'completed'}`}${row.turnStart ? ' trajectory-row--turn-start' : ''}${row.turnEnd ? ' trajectory-row--turn-end' : ''}`}
        data-row-key={row.key}
        data-source-uuid={row.sourceUuid}
        data-trajectory-selection-key={requestOnly ? undefined : row.key}
        data-turn-start={row.turnStart || undefined}
        data-turn-end={row.turnEnd || undefined}
        role="row"
        aria-rowindex={row.ariaRowIndex}
        aria-selected={requestOnly ? undefined : selected}
        tabIndex={requestOnly ? undefined : '0'}
        onClick={requestOnly ? undefined : () => onSelect(row)}
        onKeyDown={requestOnly ? undefined : (event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onSelect(row) } }}
      >
        <span className={`trajectory-row__event${requestOnly ? ' trajectory-row__event--requests' : ''}`} role="cell">
          {row.turnUuid ? <span className={`trajectory-turn-rail trajectory-turn-rail--${row.turn?.status || 'completed'}`} aria-hidden="true" /> : null}
          {selected && !requestOnly ? <span className="trajectory-selection-rail" aria-hidden="true" /> : null}
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
          {requestOnly ? (row.requestBoundaries?.length ? row.requestBoundaries : [row]).map((request) => renderRequestBoundary(request, true)) : (row.requestBoundaries || []).map((request) => renderRequestBoundary(request))}
          {!requestOnly ? <span className="trajectory-row__kind-slot"><strong>{t(`trajectory.kind.${row.kind || 'error'}`)}</strong></span> : null}
          {row.isSteering ? <small>{t('trajectory.steering')}</small> : null}
        </span>
        {!requestOnly ? <span className="trajectory-row__content" role="cell"><span>{preview || '—'}</span><span className="trajectory-row__trailing">{row.kind === 'workflow' && row.durationMs != null ? <span>{t('trajectory.request.duration', { seconds: formatNumber(row.durationMs / 1000, { maximumFractionDigits: 3 }) })}</span> : null}<em>{t(statusKeys[row.status] || 'common.status.unknown_with_code', { code: row.status || '—' })}</em>{group ? <button type="button" className="trajectory-tool-group-toggle" aria-pressed={toolGroupCollapsed} title={t(toolGroupCollapsed ? 'trajectory.expand.tools' : 'trajectory.collapse.tools', { count: group.count })} aria-label={t(toolGroupCollapsed ? 'trajectory.expand.tools' : 'trajectory.collapse.tools', { count: group.count })} onKeyDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); onToggleToolGroup?.(row.key) }}>{toolGroupCollapsed ? <ChevronRight size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}</button> : null}</span></span> : null}
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
