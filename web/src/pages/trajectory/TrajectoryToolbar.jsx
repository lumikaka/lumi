import { RefreshCw } from 'lucide-react'

import { useI18n } from '../../i18n/useI18n.js'

const kindOptions = [
  ['system', 'trajectory.kind.system'],
  ['user', 'trajectory.kind.user'],
  ['context', 'trajectory.kind.context'],
  ['assistant', 'trajectory.kind.assistant'],
  ['tool', 'trajectory.kind.tool'],
  ['compaction', 'trajectory.kind.compaction'],
  ['error', 'trajectory.kind.error'],
  ['request', 'trajectory.filter.request'],
]
const statusOptions = ['pending', 'running', 'completed', 'error', 'interrupted']

export default function TrajectoryToolbar({ projection, fetching = false, search = '', kind = '', status = '', onSearchChange, onKindChange, onStatusChange, onRefresh }) {
  const { formatNumber, t } = useI18n()
  const thread = projection.thread
  const overview = projection.overview || {}
  const activeCount = Number(overview.active_turn_count || 0) + Number(overview.active_request_count || 0) + Number(overview.active_tool_count || 0)
  const facts = [
    ['turns', overview.turn_count],
    ['items', overview.item_count],
    ['requests', overview.model_request_count],
    ['tools', overview.tool_count],
    ['compactions', overview.compaction_count],
    ['active', activeCount],
  ]

  return (
    <header className="trajectory-toolbar">
      <div className="trajectory-toolbar__heading">
        <p className="eyebrow">{t('trajectory.title')}</p>
        <h1>{thread?.title || t('trajectory.title')}</h1>
        <p>{t('trajectory.subtitle')}</p>
        {thread?.uuid ? <code title={t('trajectory.thread_uuid')}>{thread.uuid}</code> : null}
      </div>
      <div className="trajectory-toolbar__actions">
        <span className={`trajectory-history-state ${projection.historyComplete ? 'is-complete' : 'is-partial'}`}>
          {t(projection.historyComplete ? 'trajectory.history.complete' : 'trajectory.history.partial')}
        </span>
        <button type="button" className="button-secondary trajectory-refresh" disabled={fetching} onClick={onRefresh}>
          <RefreshCw size={15} className={fetching ? 'is-spinning' : ''} aria-hidden="true" />
          {t('trajectory.refresh')}
        </button>
      </div>
      <dl className="trajectory-overview-facts">
        {facts.map(([key, value]) => (
          <div key={key} className={key === 'active' && Number(value) > 0 ? 'is-active' : ''}>
            <dt>{t(`trajectory.overview.${key}`)}</dt>
            <dd>{formatNumber(Number(value) || 0)}</dd>
          </div>
        ))}
      </dl>
      <div className="trajectory-search-controls">
        <label className="trajectory-search-field">
          <span>{t('trajectory.search.label')}</span>
          <input type="search" value={search} onChange={(event) => onSearchChange?.(event.target.value)} placeholder={t('trajectory.search.placeholder')} />
        </label>
        <label>
          <span>{t('trajectory.filter.kind')}</span>
          <select value={kind} onChange={(event) => onKindChange?.(event.target.value)}><option value="">{t('trajectory.filter.all_kinds')}</option>{kindOptions.map(([value, messageKey]) => <option value={value} key={value}>{t(messageKey)}</option>)}</select>
        </label>
        <label>
          <span>{t('trajectory.filter.status')}</span>
          <select value={status} onChange={(event) => onStatusChange?.(event.target.value)}><option value="">{t('trajectory.filter.all_statuses')}</option>{statusOptions.map((value) => <option value={value} key={value}>{t(`trajectory.status.${value}`)}</option>)}</select>
        </label>
        {search && !projection.historyComplete ? <p className="trajectory-search-notice">{t('trajectory.search.loaded_only')}</p> : null}
      </div>
    </header>
  )
}
