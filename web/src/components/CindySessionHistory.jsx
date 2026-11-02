import { useInfiniteQuery } from '@tanstack/react-query'
import { listCindySessionItems } from '../api/chat.js'
import { useI18n } from '../i18n/useI18n.js'

export default function CindySessionHistory({ projectUuid, threadUuid }) {
  const { t } = useI18n()
  const history = useInfiniteQuery({
    queryKey: ['cindy-session-items', projectUuid, threadUuid],
    queryFn: ({ pageParam, signal }) => listCindySessionItems(projectUuid, threadUuid, { after: pageParam, signal }),
    initialPageParam: '',
    getNextPageParam: (page) => page.cursor_pagination.has_more ? page.cursor_pagination.next_cursor : undefined,
  })
  const items = history.data?.pages.flatMap((page) => page.items) || []
  const coverage = history.data?.pages[0]?.coverage
  return <section className="codex-session-history" aria-label={t('chat.cindy.history')}>
    <p className="chat-muted">{t('chat.cindy.read_only')}</p>
    {coverage ? <div className="chat-muted">
      <p>{t('chat.cindy.cutoff')} <time dateTime={coverage.cutoff_at}>{new Date(coverage.cutoff_at).toLocaleString()}</time></p>
      <p>{t('chat.cindy.coverage')}</p>
      {coverage.omitted_messages > 0 ? <p>{t('chat.cindy.omitted', { count: coverage.omitted_messages })}</p> : null}
      {coverage.warnings.length ? <p>{t('chat.cindy.filtered')}</p> : null}
    </div> : null}
    {history.isPending ? <p className="chat-muted">{t('chat.loading')}</p> : null}
    {history.error ? <p role="alert">{history.error.message} <button type="button" className="button-quiet" onClick={() => history.isFetchNextPageError ? history.fetchNextPage() : history.refetch()}>{t('chat.codex.retry')}</button></p> : null}
    {!history.isPending && !history.error && !items.length ? <p className="chat-muted">{t('chat.cindy.empty')}</p> : null}
    <ol className="codex-session-history__items">
      {items.map((item) => <li key={item.uuid} className={`codex-session-history__item codex-session-history__item--${item.kind}`}>
        <div className="codex-session-history__meta"><span>{t(`chat.cindy.${item.kind}`)}</span><time dateTime={item.recorded_at}>{new Date(item.recorded_at).toLocaleString()}</time></div>
        <p>{item.text}</p>
      </li>)}
    </ol>
    {history.hasNextPage ? <button type="button" className="button-quiet" disabled={history.isFetching} onClick={() => history.fetchNextPage()}>{t('chat.mcp.more')}</button> : null}
  </section>
}
