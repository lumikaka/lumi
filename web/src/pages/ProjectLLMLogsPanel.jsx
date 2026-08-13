import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'

import { getProjectLLMLog, listProjectLLMLogs } from '../api/ai.js'
import LumiDialog from '../components/LumiDialog.jsx'
import { useI18n } from '../i18n/useI18n.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { localizedErrorPresentation } from '../i18n/errorLocalization.js'
import { extractReadablePrompt, extractReadableResponse } from './llmLogPrompt.js'

const pageSize = 12
const emptyFilters = Object.freeze({
  providerUuid: '',
  providerType: '',
  model: '',
  scenario: '',
  status: '',
  requestType: '',
  keyword: '',
})

function formatDuration(value, formatNumber) {
  const duration = Number(value) || 0
  if (duration < 1000) return `${formatNumber(duration)} ms`
  return `${formatNumber(duration / 1000, { maximumFractionDigits: duration < 10000 ? 2 : 1 })} s`
}

function scenarioLabel(t, log) {
  if (log.scene === 'premise_asset_generation' || log.scenario === 'premise_asset_generation') return t('settings.llm_logs.scenario.premise_asset_generation')
  if (log.scene === 'asset_reference' || log.scenario === 'asset_reference') return t('settings.llm_logs.scenario.asset_reference')
  if (log.source_type === 'project_chat') return t(log.scope === 'premise' ? 'settings.llm_logs.scenario.premise_chat' : 'settings.llm_logs.scenario.project_chat')
  const known = new Set([
    'story_chapter_generation', 'story_profile_generation', 'story_profile_from_chapters', 'story_chapter_batch_plan',
    'comic_storyboard_generation', 'premise_setting_generation', 'premise_asset_breakdown', 'premise_asset_generation',
    'comic_reference_selection', 'comic_image_generation',
  ])
  if (known.has(log.scenario)) return t(`settings.llm_logs.scenario.${log.scenario}`)
  return log.scenario ? t('common.status.unknown_with_code', { code: log.scenario }) : t('settings.llm_logs.scenario.default')
}

function requestTypeLabel(t, requestType) {
  const key = requestType === 'image' ? 'image' : 'text'
  return t(`settings.llm_logs.request_type.${key}`)
}

function tokenValue(log, value, formatNumber) {
  return log.request_type === 'image' ? '—' : formatNumber(value)
}

function metricValue(log, value, formatNumber, options) {
  return log.request_type === 'image' || value == null ? '—' : formatNumber(value, options)
}

function providerLabel(log) {
  return log.provider_type || log.provider_uuid?.slice(0, 13) || '—'
}

function statusLabel(t, status) {
  const key = { pending: 'common.status.in_progress', completed: 'common.status.completed', failed: 'common.status.failed', cancelled: 'common.status.cancelled' }[status]
  return key ? t(key) : t('common.status.unknown_with_code', { code: status || '—' })
}

function formatPayload(value) {
  return value == null ? '' : JSON.stringify(value, null, 2)
}

function LLMLogReaderDialog({ content, descriptionKey, onClose, titleKey }) {
  const { t } = useI18n()

  return (
    <LumiDialog className="overview-llm-reader-dialog" aria-labelledby="overview-llm-reader-dialog-title" onClose={onClose}>
      <header className="lumi-dialog__header">
        <div>
          <h2 id="overview-llm-reader-dialog-title">{t(titleKey)}</h2>
          <p>{t(descriptionKey)}</p>
        </div>
        <button type="button" className="button-quiet" aria-label={t('common.action.close')} onClick={onClose}>×</button>
      </header>
      <div className="lumi-dialog__body overview-llm-reader-dialog__body">
        <pre className="overview-llm-readable-log" data-user-content>{content}</pre>
      </div>
    </LumiDialog>
  )
}

function LLMLogDetailDialog({ projectUuid, log, onClose }) {
  const { formatDateTime, formatNumber, t } = useI18n()
  const [openReader, setOpenReader] = useState('')
  const detailsQuery = useQuery({
    queryKey: ['project-llm-log', projectUuid, log.uuid],
    queryFn: () => getProjectLLMLog(projectUuid, log.uuid),
  })

  const detail = detailsQuery.data
  const displayLog = detail || log
  const readablePrompt = extractReadablePrompt(detail?.request_payload)
  const readableResponse = extractReadableResponse(detail?.response)
  const legacyFallback = Boolean(detail && detail.request_payload == null)
  const loadFallback = detailsQuery.isError
  const useSummaryFallback = legacyFallback || loadFallback
  const requestText = detailsQuery.isLoading
    ? t('settings.llm_logs.detail_loading')
    : detail?.request_payload != null
      ? formatPayload(detail.request_payload)
      : log.input_summary || t('settings.llm_logs.input_summary_empty')
  const responseText = detailsQuery.isLoading
    ? t('settings.llm_logs.detail_loading')
    : detail?.response != null
      ? formatPayload(detail.response)
      : useSummaryFallback
        ? log.output_summary || t('settings.llm_logs.output_summary_empty')
        : displayLog.status === 'pending'
          ? t('settings.llm_logs.response_pending')
          : t('settings.llm_logs.response_unavailable')
  const totalTokens = displayLog.input_tokens + displayLog.output_tokens
  const closeDetail = () => {
    setOpenReader('')
    onClose()
  }
  return (
    <>
      <LumiDialog className="overview-llm-dialog" onClose={closeDetail}>
        <header className="lumi-dialog__header">
          <div><p className="eyebrow">{scenarioLabel(t, displayLog)}</p><h2>{displayLog.model || t('settings.llm_logs.scenario.default')}</h2><code>{displayLog.uuid}</code></div>
          <button type="button" className="button-quiet" aria-label={t('common.action.close')} onClick={closeDetail}>×</button>
        </header>
        <div className="lumi-dialog__body overview-llm-dialog__body">
          <dl className="overview-llm-detail-facts">
            <div><dt>{t('common.label.status')}</dt><dd>{statusLabel(t, displayLog.status)}</dd></div>
            <div><dt>{t('common.label.type')}</dt><dd>{requestTypeLabel(t, displayLog.request_type)}</dd></div>
            <div><dt>{t('settings.provider')}</dt><dd>{providerLabel(displayLog)}</dd></div>
            <div><dt>{t('settings.llm_logs.attempt')}</dt><dd>{formatNumber(displayLog.attempt)}</dd></div>
            <div><dt>{t('settings.llm_logs.input_tokens')}</dt><dd>{tokenValue(displayLog, displayLog.input_tokens, formatNumber)}</dd></div>
            <div><dt>{t('settings.llm_logs.output_tokens')}</dt><dd>{tokenValue(displayLog, displayLog.output_tokens, formatNumber)}</dd></div>
            <div><dt>{t('settings.llm_logs.total_tokens')}</dt><dd>{tokenValue(displayLog, totalTokens, formatNumber)}</dd></div>
            <div><dt>{t('settings.llm_logs.cached_input_tokens')}</dt><dd>{metricValue(displayLog, displayLog.cached_input_tokens, formatNumber)}</dd></div>
            <div><dt>{t('settings.llm_logs.input_characters')}</dt><dd>{metricValue(displayLog, displayLog.input_characters, formatNumber)}</dd></div>
            <div><dt>{t('settings.llm_logs.output_characters')}</dt><dd>{metricValue(displayLog, displayLog.output_characters, formatNumber)}</dd></div>
            <div><dt>{t('settings.llm_logs.output_tokens_per_second')}</dt><dd>{metricValue(displayLog, displayLog.output_tokens_per_second, formatNumber, { maximumFractionDigits: 2 })}</dd></div>
            <div><dt>{t('settings.llm_logs.output_characters_per_second')}</dt><dd>{metricValue(displayLog, displayLog.output_characters_per_second, formatNumber, { maximumFractionDigits: 2 })}</dd></div>
            <div><dt>{t('settings.llm_logs.duration')}</dt><dd>{formatDuration(displayLog.duration_ms, formatNumber)}</dd></div>
            <div><dt>{t('settings.llm_logs.finish_reason')}</dt><dd>{displayLog.finish_reason || '—'}</dd></div>
            <div><dt>{t('settings.llm_logs.occurred_at')}</dt><dd>{formatDateTime(displayLog.created_at)}</dd></div>
          </dl>
          {displayLog.error_code ? <p className="workspace-notice workspace-notice--error"><strong>{localizedErrorPresentation(t, { code: displayLog.error_code }).message}</strong><small>{t('errors.diagnostic_code', { code: displayLog.error_code })}</small>{displayLog.error_message ? <small>{t('settings.llm_logs.provider_error_message')}: {displayLog.error_message}</small> : null}</p> : null}
          <section className="overview-llm-context" aria-label={t('settings.llm_logs.context')}>
            <h3>{t('settings.llm_logs.context_heading')}</h3>
            <dl>
              {displayLog.task_uuid ? <div><dt>Task UUID</dt><dd>{displayLog.task_uuid}</dd></div> : null}
              {displayLog.thread_uuid ? <div><dt>Thread UUID</dt><dd>{displayLog.thread_uuid}</dd></div> : null}
              {displayLog.run_uuid ? <div><dt>Run UUID</dt><dd>{displayLog.run_uuid}</dd></div> : null}
              {displayLog.workflow_uuid ? <div><dt>{t('settings.llm_logs.workflow_uuid')}</dt><dd>{displayLog.workflow_uuid}</dd></div> : null}
              {displayLog.workflow_step_uuid ? <div><dt>{t('settings.llm_logs.workflow_step_uuid')}</dt><dd>{displayLog.workflow_step_uuid}</dd></div> : null}
              {displayLog.http_status ? <div><dt>{t('settings.llm_logs.http_status')}</dt><dd>{displayLog.http_status}</dd></div> : null}
              {displayLog.provider_error_code ? <div><dt>{t('settings.llm_logs.provider_error_code')}</dt><dd>{displayLog.provider_error_code}</dd></div> : null}
              {displayLog.provider_request_id ? <div><dt>{t('settings.llm_logs.provider_request_id')}</dt><dd>{displayLog.provider_request_id}</dd></div> : null}
            </dl>
          </section>
          <LocalizedErrorMessage error={detailsQuery.error} compact />
          {legacyFallback ? <p className="overview-llm-payload-note">{t('settings.llm_logs.legacy_payload_notice')}</p> : null}
          {loadFallback ? <p className="overview-llm-payload-note">{t('settings.llm_logs.detail_load_failed')}</p> : null}
          <div className="overview-llm-payloads">
            <article>
              <header className="overview-llm-payload-heading">
                <h3>{t('settings.llm_logs.request_payload')}</h3>
                {readablePrompt ? <button type="button" className="button-secondary" onClick={() => setOpenReader('prompt')}>{t('settings.llm_logs.read_log')}</button> : null}
              </header>
              <pre data-user-content={requestText ? '' : undefined}>{requestText}</pre>
            </article>
            <article>
              <header className="overview-llm-payload-heading">
                <h3>{t('settings.llm_logs.response')}</h3>
                {readableResponse ? <button type="button" className="button-secondary" onClick={() => setOpenReader('response')}>{t('settings.llm_logs.read_log')}</button> : null}
              </header>
              <pre data-user-content={responseText ? '' : undefined}>{responseText}</pre>
            </article>
          </div>
          <p className="overview-llm-cost-note">{t('settings.llm_logs.cost_note')}</p>
        </div>
      </LumiDialog>
      {openReader === 'prompt' && readablePrompt ? <LLMLogReaderDialog content={readablePrompt} titleKey="settings.llm_logs.prompt_reader_title" descriptionKey="settings.llm_logs.prompt_reader_description" onClose={() => setOpenReader('')} /> : null}
      {openReader === 'response' && readableResponse ? <LLMLogReaderDialog content={readableResponse} titleKey="settings.llm_logs.response_reader_title" descriptionKey="settings.llm_logs.response_reader_description" onClose={() => setOpenReader('')} /> : null}
    </>
  )
}

export default function ProjectLLMLogsPanel({ projectUuid, scope = '', title, description, standalone = false }) {
  const { formatCount, formatDateTime, formatNumber, t } = useI18n()
  const [page, setPage] = useState(1)
  const [filters, setFilters] = useState(emptyFilters)
  const [keywordDraft, setKeywordDraft] = useState('')
  const [selectedLog, setSelectedLog] = useState(null)
  const logsQuery = useQuery({
    queryKey: ['project-llm-logs', projectUuid, scope, page, filters],
    queryFn: () => listProjectLLMLogs(projectUuid, { page, perPage: pageSize, scope, ...filters }),
  })
  const logs = logsQuery.data?.items || []
  const pagination = logsQuery.data?.pagination || { per_page: pageSize, current_page: page, last_page: 1, total: 0 }
  const filterGroups = logsQuery.data?.filter_groups || { providers: [], provider_types: [], models: [], scenarios: [], statuses: [], request_types: [] }
  const updateFilter = (key, value) => {
    setPage(1)
    setFilters((current) => ({ ...current, [key]: value }))
  }
  const resetFilters = () => {
    setPage(1)
    setKeywordDraft('')
    setFilters(emptyFilters)
  }
  const openLog = (log) => setSelectedLog(log)

  return (
    <div
      className={`project-overview project-overview--llm-logs ${standalone ? 'project-overview--standalone' : ''}`}
      role={standalone ? undefined : 'tabpanel'}
      id={standalone ? undefined : 'overview-panel-llm-logs'}
      aria-labelledby={standalone ? undefined : 'overview-tab-llm-logs'}
    >
      <LocalizedErrorMessage error={logsQuery.error} />
      <section className="overview-card overview-llm-panel">
        <header className="overview-card__header"><div><h1>{title || t('settings.llm_logs.project_title')}</h1><p>{description || t('settings.llm_logs.panel_description')}</p></div><span>{formatCount('common.count.items', pagination.total)}</span></header>
        <form className="overview-llm-filters" aria-label={t('settings.llm_logs.filters')} aria-busy={logsQuery.isFetching} onSubmit={(event) => { event.preventDefault(); updateFilter('keyword', keywordDraft.trim()) }}>
          <label className="overview-llm-filter overview-llm-filter--keyword"><span>{t('settings.llm_logs.filter.keyword')}</span><input type="search" value={keywordDraft} onChange={(event) => setKeywordDraft(event.target.value)} placeholder={t('settings.llm_logs.filter.keyword_placeholder')} /></label>
          <label className="overview-llm-filter"><span>{t('settings.provider')}</span><select value={filters.providerUuid} onChange={(event) => updateFilter('providerUuid', event.target.value)}><option value="">{t('common.label.all')}</option>{filterGroups.providers.map((provider) => <option key={provider.uuid} value={provider.uuid}>{provider.type || t('settings.llm_logs.provider_unknown')} · {provider.uuid.slice(0, 13)}</option>)}</select></label>
          <label className="overview-llm-filter"><span>{t('settings.llm_logs.filter.provider_type')}</span><select value={filters.providerType} onChange={(event) => updateFilter('providerType', event.target.value)}><option value="">{t('common.label.all')}</option>{filterGroups.provider_types.map((value) => <option key={value} value={value}>{value}</option>)}</select></label>
          <label className="overview-llm-filter"><span>{t('settings.llm_logs.filter.model')}</span><select value={filters.model} onChange={(event) => updateFilter('model', event.target.value)}><option value="">{t('common.label.all')}</option>{filterGroups.models.map((value) => <option key={value} value={value}>{value}</option>)}</select></label>
          <label className="overview-llm-filter"><span>{t('settings.llm_logs.scenario')}</span><select value={filters.scenario} onChange={(event) => updateFilter('scenario', event.target.value)}><option value="">{t('common.label.all')}</option>{filterGroups.scenarios.map((value) => <option key={value} value={value}>{scenarioLabel(t, { scenario: value })}</option>)}</select></label>
          <label className="overview-llm-filter"><span>{t('common.label.status')}</span><select value={filters.status} onChange={(event) => updateFilter('status', event.target.value)}><option value="">{t('common.label.all')}</option>{filterGroups.statuses.map((value) => <option key={value} value={value}>{statusLabel(t, value)}</option>)}</select></label>
          <label className="overview-llm-filter"><span>{t('common.label.type')}</span><select value={filters.requestType} onChange={(event) => updateFilter('requestType', event.target.value)}><option value="">{t('common.label.all')}</option>{filterGroups.request_types.map((value) => <option key={value} value={value}>{requestTypeLabel(t, value)}</option>)}</select></label>
          <div className="overview-llm-filter-actions"><button type="submit" disabled={logsQuery.isFetching}>{t('common.action.search')}</button><button type="button" className="button-secondary" disabled={logsQuery.isFetching || (Object.values(filters).every((value) => !value) && !keywordDraft)} onClick={resetFilters}>{t('settings.llm_logs.filter.reset')}</button></div>
        </form>
        <div className="overview-llm-table-wrap">
          <table className="overview-llm-table">
            <thead><tr><th>{t('settings.llm_logs.time')}</th><th>{t('settings.llm_logs.model_provider')}</th><th>{t('settings.llm_logs.scenario')}</th><th>{t('settings.llm_logs.input')}</th><th>{t('settings.llm_logs.output')}</th><th>{t('settings.llm_logs.duration')}</th><th>{t('common.label.status')}</th></tr></thead>
            <tbody>
              {logsQuery.isLoading ? <tr><td colSpan="7"><p className="overview-card__loading">{t('settings.llm_logs.loading')}</p></td></tr> : null}
              {!logsQuery.isLoading && !logsQuery.isError && logs.length === 0 ? <tr><td colSpan="7"><p className="overview-llm-empty">{t('settings.llm_logs.empty')}</p></td></tr> : null}
              {!logsQuery.isLoading ? logs.map((log) => (
                <tr key={log.uuid} role="button" tabIndex="0" onClick={() => openLog(log)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); openLog(log) } }}>
                  <td><time dateTime={log.created_at}>{formatDateTime(log.created_at, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</time></td>
                  <td><strong>{log.model || '—'}</strong><small>{providerLabel(log)}</small></td>
                  <td>{scenarioLabel(t, log)}<small>{requestTypeLabel(t, log.request_type)}</small></td>
                  <td><strong>{tokenValue(log, log.input_tokens, formatNumber)}{log.request_type === 'image' ? '' : ` ${t('settings.llm_logs.token_unit')}`}</strong><small>{t('settings.llm_logs.cached_short')}: {metricValue(log, log.cached_input_tokens, formatNumber)}</small><small>{t('settings.llm_logs.characters_short')}: {metricValue(log, log.input_characters, formatNumber)}</small></td>
                  <td><strong>{tokenValue(log, log.output_tokens, formatNumber)}{log.request_type === 'image' ? '' : ` ${t('settings.llm_logs.token_unit')}`}</strong><small>{t('settings.llm_logs.characters_short')}: {metricValue(log, log.output_characters, formatNumber)}</small><small>{t('settings.llm_logs.speed_short')}: {metricValue(log, log.output_tokens_per_second, formatNumber, { maximumFractionDigits: 2 })} {t('settings.llm_logs.tokens_per_second_unit')} · {metricValue(log, log.output_characters_per_second, formatNumber, { maximumFractionDigits: 2 })} {t('settings.llm_logs.characters_per_second_unit')}</small></td>
                  <td>{formatDuration(log.duration_ms, formatNumber)}</td>
                  <td><span className={`overview-llm-status overview-llm-status--${log.status}`}>{statusLabel(t, log.status)}</span></td>
                </tr>
              )) : null}
            </tbody>
          </table>
        </div>
        {pagination.last_page > 1 ? <nav className="overview-llm-pagination" aria-label={t('settings.llm_logs.pagination')}><button type="button" className="button-secondary" disabled={page <= 1 || logsQuery.isFetching} onClick={() => setPage((current) => Math.max(1, current - 1))}>{t('common.action.previous_page')}</button><span>{pagination.current_page} / {pagination.last_page}</span><button type="button" className="button-secondary" disabled={page >= pagination.last_page || logsQuery.isFetching} onClick={() => setPage((current) => current + 1)}>{t('common.action.next_page')}</button></nav> : null}
      </section>
      {selectedLog ? <LLMLogDetailDialog projectUuid={projectUuid} log={selectedLog} onClose={() => setSelectedLog(null)} /> : null}
    </div>
  )
}
