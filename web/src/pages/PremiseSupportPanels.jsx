import { useInfiniteQuery } from '@tanstack/react-query'
import { MessageSquare, Plus } from 'lucide-react'

import { listChatThreads } from '../api/chat.js'
import PromptCatalogEditor from '../components/PromptCatalogEditor.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { statusLabel as fallbackStatusLabel } from '../i18n/labels.js'

const statusCopy = {
  idle: 'premise.threads.status.idle',
  busy: 'premise.threads.status.busy',
  waiting_for_input: 'common.status.waiting_for_input',
  completed: 'common.status.completed',
  failed: 'common.status.failed',
  cancelled: 'common.status.cancelled',
  interrupted: 'common.status.interrupted',
}

export function usePremiseThreads(projectUuid, enabled = true) {
  return useInfiniteQuery({
    queryKey: ['chat-threads', projectUuid, 'premise'],
    queryFn: ({ pageParam }) => listChatThreads(projectUuid, { scope: 'premise', page: pageParam, perPage: 30 }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) => lastPage.pagination?.current_page < lastPage.pagination?.last_page
      ? lastPage.pagination.current_page + 1
      : undefined,
    enabled,
    refetchInterval: (query) => query.state.data?.pages?.some((page) => page.items?.some((thread) => ['busy', 'waiting_for_input'].includes(thread.status))) ? 1500 : false,
  })
}

function flattenThreads(pages = []) {
  const seen = new Set()
  return pages.flatMap((page) => page.items || []).filter((thread) => {
    if (seen.has(thread.uuid)) return false
    seen.add(thread.uuid)
    return true
  })
}

export function PremiseThreadsPanel({ projectUuid, onOpenThread, onNewThread }) {
  const { formatDateTime, t } = useI18n()
  const threadsQuery = usePremiseThreads(projectUuid)
  const threads = flattenThreads(threadsQuery.data?.pages)
  const total = threadsQuery.data?.pages?.[0]?.pagination?.total ?? threads.length
  return (
    <section className="premise-content-panel premise-thread-panel" role="tabpanel">
      <header className="premise-content-heading premise-content-heading--actions">
        <div><div><h1>{t('premise.threads.title')}</h1><span>{total}</span></div><p>{t('premise.threads.description')}</p></div>
        <button type="button" onClick={onNewThread}><Plus size={15} aria-hidden="true" />{t('premise.threads.new')}</button>
      </header>
      {threadsQuery.isLoading ? <p className="premise-panel-loading">{t('premise.threads.loading')}</p> : null}
      {threadsQuery.isError && !threads.length && !threadsQuery.isFetchNextPageError ? (
        <div className="premise-empty-state">
          <MessageSquare size={30} aria-hidden="true" />
          <h2>{t('premise.threads.load_failed')}</h2>
          <LocalizedErrorMessage error={threadsQuery.error} fallback={t('premise.threads.load_failed_body')} />
          <div><button type="button" onClick={() => threadsQuery.refetch()}>{t('common.action.retry')}</button></div>
        </div>
      ) : null}
      {!threadsQuery.isLoading && !threadsQuery.isError && threads.length === 0 ? <div className="premise-empty-state"><MessageSquare size={30} aria-hidden="true" /><h2>{t('premise.threads.empty_title')}</h2><p>{t('premise.threads.empty_body')}</p><div><button type="button" onClick={onNewThread}>{t('premise.threads.new')}</button></div></div> : null}
      <div className="premise-thread-list">
        {threads.map((thread) => (
          <button type="button" className="premise-thread-item" key={thread.uuid} onClick={() => onOpenThread(thread)}>
            <span className="premise-thread-item__icon"><MessageSquare size={17} aria-hidden="true" /></span>
            <span><strong>{thread.title}</strong><small>{t(thread.scene === 'premise_asset_generation' ? 'premise.threads.scene.generate' : thread.scene === 'asset_reference' ? 'premise.threads.scene.reference' : 'premise.threads.scene.chat')} · {formatDateTime(thread.updated_at)}</small></span>
            <em className={`premise-thread-status premise-thread-status--${thread.status}`}>{statusCopy[thread.status] ? t(statusCopy[thread.status]) : fallbackStatusLabel(t, thread.status)}</em>
          </button>
        ))}
      </div>
      {!threadsQuery.isLoading && threads.length ? (
        <div className="premise-history-pagination">
          {threadsQuery.isFetchNextPageError
            ? <button type="button" className="button-quiet" onClick={() => threadsQuery.fetchNextPage()}>{t('premise.history.retry_more')}</button>
            : threadsQuery.hasNextPage
              ? <button type="button" className="button-secondary" disabled={threadsQuery.isFetchingNextPage} onClick={() => threadsQuery.fetchNextPage()}>{t(threadsQuery.isFetchingNextPage ? 'premise.history.loading_more' : 'premise.history.load_more')}</button>
              : <span>{t('premise.history.end', { count: total })}</span>}
        </div>
      ) : null}
    </section>
  )
}

export function PremisePromptsPanel({ projectUuid }) {
  return (
    <section className="premise-content-panel premise-prompt-panel" role="tabpanel">
      <PromptCatalogEditor projectUuid={projectUuid} groups={['premise', 'premise_style']} />
    </section>
  )
}
