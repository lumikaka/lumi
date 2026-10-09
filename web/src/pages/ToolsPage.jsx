import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, FlaskConical, Plus } from 'lucide-react'
import { Link, useLocation, useSearchParams } from 'react-router-dom'

import { listModelTests, modelTestKeys } from '../api/modelTests.js'
import AppPageShell from '../components/AppPageShell.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { modelTestPage } from './modelTestState.js'
import './modelTests.sass'

export function TestStatus({ status }) {
  const { t } = useI18n()
  return <span className={`model-test-status model-test-status--${status}`}>{t(`tools.status.${status}`)}</span>
}

export default function ToolsPage() {
  const { t, formatDateTime } = useI18n()
  const location = useLocation()
  const [params, setParams] = useSearchParams()
  const page = modelTestPage(params.get('page'))
  const history = useQuery({ queryKey: modelTestKeys.list(page), queryFn: () => listModelTests(page) })
  const pagination = history.data?.pagination

  return <AppPageShell title={t('tools.title')}>
    <div className="model-tools-page">
      <header className="model-tools-heading">
        <Link className="model-tools-back" to="/"><ArrowLeft size={15} aria-hidden="true" />{t('common.not_found.action')}</Link>
        <h1>{t('tools.title')}</h1><p>{t('tools.description')}</p>
      </header>
      <section aria-label={t('tools.available')}>
        <Link className="model-tool-card" to={{ pathname: '/settings/tools/model-tests/new', search: location.search }}>
          <FlaskConical size={24} aria-hidden="true" />
          <span><strong>{t('tools.model_test')}</strong><span>{t('tools.model_test_description')}</span></span>
          <Plus size={20} aria-hidden="true" />
        </Link>
      </section>
      <LocalizedErrorMessage error={history.error} />
      <section className="model-test-history" aria-labelledby="model-test-history-title">
        <header><h2 id="model-test-history-title">{t('tools.history')}</h2><span>{t('tools.history_count', { total: pagination?.total || 0 })}</span></header>
        {history.isLoading ? <p>{t('tools.loading')}</p> : null}
        {!history.isLoading && !history.isError && !history.data?.items.length ? <p>{t('tools.history_empty')}</p> : null}
        <ul>{(history.data?.items || []).map((test) => <li key={test.uuid}>
          <Link to={{ pathname: `/settings/tools/model-tests/${test.uuid}`, search: location.search }} className="model-test-history-row">
            <span className="model-test-history-copy"><strong>{formatDateTime(test.created_at, { dateStyle: 'medium', timeStyle: 'medium' })}</strong><span data-no-i18n>{test.text_model.model || '—'} · {test.image_model.model || '—'}</span></span>
            <TestStatus status={test.status} /><ArrowRight size={16} aria-hidden="true" />
          </Link>
        </li>)}</ul>
        {pagination && (pagination.last_page > 1 || page > 1) ? <nav className="model-test-pagination" aria-label={t('tools.pagination')}>
          <button type="button" className="button-secondary" disabled={page <= 1 || history.isFetching} onClick={() => setParams({ page: String(page - 1) })}>{t('tools.previous')}</button>
          <span>{t('tools.page', { page, last: pagination.last_page })}</span>
          <button type="button" className="button-secondary" disabled={page >= pagination.last_page || history.isFetching} onClick={() => setParams({ page: String(page + 1) })}>{t('tools.next')}</button>
        </nav> : null}
      </section>
    </div>
  </AppPageShell>
}
