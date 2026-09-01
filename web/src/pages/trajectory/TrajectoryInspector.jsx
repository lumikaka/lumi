import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { X } from 'lucide-react'

import { getProjectLLMLog } from '../../api/ai.js'
import LocalizedErrorMessage from '../../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../../i18n/useI18n.js'
import { jsonSyntaxSegments } from './trajectoryMachineValue.js'

function systemPrompt(payload) {
  if (!payload || typeof payload !== 'object') return null
  if (typeof payload.system === 'string') return payload.system
  const message = Array.isArray(payload.messages) ? payload.messages.find((item) => item?.role === 'system') : null
  if (typeof message?.content === 'string') return message.content
  if (Array.isArray(message?.content)) return message.content.map((part) => part?.text || '').filter(Boolean).join('\n') || null
  return null
}

function toolCatalog(payload, toolName = '') {
  const tools = Array.isArray(payload?.tools) ? payload.tools : []
  if (!toolName) return tools
  const matching = tools.filter((tool) => tool?.name === toolName || tool?.function?.name === toolName)
  return matching.length ? matching : tools
}

function requestUuidFor(selected) {
  if (!selected) return ''
  return selected.sourceKind === 'model_request' ? selected.sourceUuid : selected.requestUuid || ''
}

function inspectorTabs(selected) {
  if (!selected) return []
  if (selected.sourceKind === 'model_request') return ['summary', 'payload', 'result', 'timing', 'raw']
  if (selected.kind === 'tool') return ['summary', 'payload', 'result', 'schema', 'timing', 'raw']
  if (selected.kind === 'system') return selected.previousRequestUuid
    ? ['summary', 'diff', 'prompt', 'tools', 'raw']
    : ['summary', 'prompt', 'tools', 'raw']
  if (selected.kind === 'compaction') return ['summary', 'compaction', 'raw']
  return ['summary', 'raw']
}

function formatDuration(value, formatNumber, fallback) {
  if (value == null) return fallback
  const duration = Number(value)
  if (!Number.isFinite(duration)) return fallback
  if (duration < 1000) return `${formatNumber(duration)} ms`
  return `${formatNumber(duration / 1000, { maximumFractionDigits: 2 })} s`
}

function factValue(value, fallback) {
  return value == null || value === '' ? fallback : value
}

export default function TrajectoryInspector({ projectUuid, selected, onClose, onResizeStart, onRequestDetailLoaded }) {
  const { formatDateTime, formatNumber, t } = useI18n()
  const tabs = useMemo(() => inspectorTabs(selected), [selected])
  const [tab, setTab] = useState('summary')
  const requestUuid = requestUuidFor(selected)
  const requestQuery = useQuery({
    queryKey: ['project-llm-log', projectUuid, requestUuid],
    queryFn: () => getProjectLLMLog(projectUuid, requestUuid),
    enabled: Boolean(projectUuid && requestUuid),
  })
  const previousRequestUuid = selected?.kind === 'system' ? selected.previousRequestUuid || '' : ''
  const previousQuery = useQuery({
    queryKey: ['project-llm-log', projectUuid, previousRequestUuid],
    queryFn: () => getProjectLLMLog(projectUuid, previousRequestUuid),
    enabled: Boolean(projectUuid && previousRequestUuid),
  })

  useEffect(() => { setTab('summary') }, [selected?.key])
  useEffect(() => { if (!tabs.includes(tab)) setTab(tabs[0] || 'summary') }, [tab, tabs])
  useEffect(() => { if (requestUuid && requestQuery.data) onRequestDetailLoaded?.(requestUuid, requestQuery.data) }, [onRequestDetailLoaded, requestQuery.data, requestUuid])
  useEffect(() => { if (previousRequestUuid && previousQuery.data) onRequestDetailLoaded?.(previousRequestUuid, previousQuery.data) }, [onRequestDetailLoaded, previousQuery.data, previousRequestUuid])

  const detail = requestQuery.data
  const requestSource = selected?.sourceKind === 'model_request' ? selected.source : selected?.source
  const displayRequest = detail || requestSource || {}
  const notRecorded = t('trajectory.inspector.not_recorded')
  const isUserWait = selected?.kind === 'tool' && selected.source?.tool_name === 'request_user_input'
  const timingStartedAt = selected?.kind === 'tool' ? selected.startedAt : displayRequest.created_at || selected?.startedAt
  const timingCompletedAt = selected?.kind === 'tool' ? selected.completedAt : displayRequest.completed_at || selected?.completedAt
  const timingDuration = selected?.kind === 'tool' ? selected.durationMs : displayRequest.duration_ms ?? selected?.durationMs
  const currentPrompt = systemPrompt(detail?.request_payload)
  const previousPrompt = systemPrompt(previousQuery.data?.request_payload)

  if (!selected) {
    return (
      <aside className="trajectory-inspector trajectory-inspector--empty" aria-label={t('trajectory.inspector.title')}>
        <button type="button" className="trajectory-inspector__resizer" aria-label={t('trajectory.inspector.resize')} onPointerDown={onResizeStart} />
        <p>{t('trajectory.inspector.empty')}</p>
      </aside>
    )
  }

  const summaryFacts = [
    [t('trajectory.inspector.source_uuid'), selected.sourceUuid],
    [t('trajectory.inspector.turn_uuid'), selected.turnUuid || notRecorded],
    [t('trajectory.inspector.request_uuid'), requestUuid || notRecorded],
    [t('trajectory.inspector.call_uuid'), selected.callUuid || notRecorded],
    [t('common.label.status'), t(`trajectory.status.${selected.status || 'completed'}`)],
    [t('trajectory.inspector.provider'), displayRequest.provider_type || displayRequest.provider_uuid || notRecorded],
    [t('trajectory.inspector.model'), displayRequest.model || notRecorded],
    [t('trajectory.inspector.ordinal'), selected.requestOrdinal || displayRequest.attempt || notRecorded],
  ]
  const toolSummaryFacts = [
    [t('trajectory.inspector.hierarchy'), `${t('trajectory.ledger.request', { number: selected.requestOrdinal || displayRequest.attempt || '—' })} › ${selected.source?.tool_name || t('trajectory.kind.tool')}`],
    [t('trajectory.inspector.activity'), t(isUserWait ? 'trajectory.inspector.user_wait' : 'trajectory.inspector.tool_execution')],
    [t('common.label.status'), t(`trajectory.status.${selected.status || 'completed'}`)],
  ]

  return (
    <aside className="trajectory-inspector" aria-label={t('trajectory.inspector.title')}>
      <button type="button" className="trajectory-inspector__resizer" aria-label={t('trajectory.inspector.resize')} onPointerDown={onResizeStart} />
      <header className="trajectory-inspector__header">
        <div><p className="eyebrow">{t('trajectory.inspector.title')}</p><h2>{selected.rowType === 'request' ? t('trajectory.ledger.request', { number: selected.requestOrdinal || '—' }) : t(`trajectory.kind.${selected.kind || 'error'}`)}</h2><code>{selected.sourceUuid}</code></div>
        <button type="button" className="button-quiet" aria-label={t('trajectory.inspector.close')} onClick={onClose}><X size={17} aria-hidden="true" /></button>
      </header>
      <div className="trajectory-inspector__tabs" role="tablist" aria-label={t('trajectory.inspector.title')}>
        {tabs.map((name) => <button type="button" role="tab" key={name} aria-selected={tab === name} aria-pressed={tab === name} onClick={() => setTab(name)}>{t(`trajectory.inspector.${name}`)}</button>)}
      </div>
      <div className="trajectory-inspector__body" role="tabpanel">
        {tab === 'summary' ? (
          <>
            {selected.kind !== 'tool' ? <p className="trajectory-inspector__preview">{selected.preview || '—'}</p> : null}
            <dl className="trajectory-inspector__facts">{(selected.kind === 'tool' ? toolSummaryFacts : summaryFacts).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
            {selected.kind === 'tool' ? (
              <>
                <div className="trajectory-tool-summary">
                  <MachineValue title={t('trajectory.inspector.payload')} value={selected.input} />
                  <MachineValue title={t('trajectory.inspector.result')} value={selected.output ?? notRecorded} />
                </div>
                <details className="trajectory-inspector__metadata">
                  <summary>{t('trajectory.inspector.metadata')}</summary>
                  <dl className="trajectory-inspector__facts">{summaryFacts.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
                </details>
              </>
            ) : null}
            {selected.derivedReason ? <p className="trajectory-inspector__notice"><strong>{t('trajectory.inspector.derived_reason')}</strong>{selected.derivedReason}</p> : null}
            {selected.orderingAccuracy ? <p className="trajectory-inspector__note">{t(`trajectory.ordering.${selected.orderingAccuracy}`)}</p> : null}
          </>
        ) : null}
        {tab === 'raw' ? <MachineValue title={t('trajectory.inspector.safe_json')} value={selected.source || selected} /> : null}
        {tab === 'payload' ? <DetailValue query={requestQuery} title={t('trajectory.inspector.payload')} value={selected.kind === 'tool' ? selected.input : detail?.request_payload} /> : null}
        {tab === 'result' ? <DetailValue query={requestQuery} title={t('trajectory.inspector.result')} value={selected.kind === 'tool' ? selected.output : detail?.response} /> : null}
        {tab === 'schema' ? <DetailValue query={requestQuery} title={t('trajectory.inspector.schema')} value={toolCatalog(detail?.request_payload, selected.source?.tool_name)} /> : null}
        {tab === 'timing' ? (
          <dl className="trajectory-inspector__facts trajectory-inspector__facts--timing">
            <div><dt>{t('trajectory.inspector.created_at')}</dt><dd>{timingStartedAt ? formatDateTime(timingStartedAt) : notRecorded}</dd></div>
            <div><dt>{t('trajectory.inspector.completed_at')}</dt><dd>{timingCompletedAt ? formatDateTime(timingCompletedAt) : selected.status === 'pending' || selected.status === 'running' ? t('trajectory.inspector.pending') : notRecorded}</dd></div>
            <div><dt>{t(isUserWait ? 'trajectory.inspector.wait_duration' : 'trajectory.inspector.duration')}</dt><dd>{formatDuration(timingDuration, formatNumber, selected.status === 'pending' || selected.status === 'running' ? t('trajectory.inspector.pending') : notRecorded)}</dd></div>
            <div><dt>{t('trajectory.inspector.finish_reason')}</dt><dd>{factValue(displayRequest.finish_reason, notRecorded)}</dd></div>
            <div><dt>{t('trajectory.inspector.input_tokens')}</dt><dd>{factValue(displayRequest.input_tokens, notRecorded)}</dd></div>
            <div><dt>{t('trajectory.inspector.cached_tokens')}</dt><dd>{factValue(displayRequest.cached_input_tokens, notRecorded)}</dd></div>
            <div><dt>{t('trajectory.inspector.output_tokens')}</dt><dd>{factValue(displayRequest.output_tokens, notRecorded)}</dd></div>
            <div><dt>{t('trajectory.inspector.ttft')}</dt><dd>{notRecorded}</dd></div>
            <div><dt>{t('trajectory.inspector.generation')}</dt><dd>{notRecorded}</dd></div>
            <div><dt>{t('trajectory.inspector.reasoning')}</dt><dd>{notRecorded}</dd></div>
            <div><dt>{t('trajectory.inspector.cache_write')}</dt><dd>{notRecorded}</dd></div>
          </dl>
        ) : null}
        {tab === 'diff' ? (
          <div className="trajectory-system-diff">
            <MachineValue title={t('trajectory.inspector.system_before')} value={previousPrompt || selected.input} />
            <MachineValue title={t('trajectory.inspector.system_after')} value={currentPrompt || selected.output} />
          </div>
        ) : null}
        {tab === 'prompt' ? <DetailValue query={requestQuery} title={t('trajectory.inspector.prompt')} value={currentPrompt} /> : null}
        {tab === 'tools' ? <DetailValue query={requestQuery} title={t('trajectory.inspector.tools')} value={toolCatalog(detail?.request_payload)} /> : null}
        {tab === 'compaction' ? (
          <div className="trajectory-compaction-detail">
            {!selected.turnUuid ? <p className="trajectory-inspector__notice">{t('trajectory.inspector.assignment_unknown')}</p> : null}
            <dl className="trajectory-inspector__facts"><div><dt>{t('trajectory.inspector.through_sequence')}</dt><dd>{factValue(selected.source?.through_item_sequence, notRecorded)}</dd></div><div><dt>{t('trajectory.inspector.source_bytes')}</dt><dd>{factValue(selected.source?.source_bytes, notRecorded)}</dd></div></dl>
            <MachineValue title={t('trajectory.inspector.compaction')} value={selected.output} />
          </div>
        ) : null}
        <LocalizedErrorMessage error={requestQuery.error || previousQuery.error} compact />
      </div>
    </aside>
  )
}

function DetailValue({ query, title, value }) {
  const { t } = useI18n()
  if (query.isLoading) return <p className="trajectory-inspector__note">{t('trajectory.inspector.detail_loading')}</p>
  if (query.isError) return <p className="trajectory-inspector__note">{t('trajectory.inspector.detail_unavailable')}</p>
  return <MachineValue title={title} value={value ?? t('trajectory.inspector.not_recorded')} />
}

function MachineValue({ title, value }) {
  const segments = jsonSyntaxSegments(value)
  return (
    <section className="trajectory-machine-value">
      <h3>{title}</h3>
      <pre><code data-user-content>{segments.map((segment) => <span className={`trajectory-json-token trajectory-json-token--${segment.kind}`} key={`${segment.offset}:${segment.kind}`}>{segment.text}</span>)}</code></pre>
    </section>
  )
}
