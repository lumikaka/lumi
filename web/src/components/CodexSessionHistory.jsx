import { useInfiniteQuery } from '@tanstack/react-query'
import { listCodexSessionItems } from '../api/chat.js'
import { useI18n } from '../i18n/useI18n.js'

export default function CodexSessionHistory({ projectUuid, threadUuid }) {
  const { t } = useI18n()
  const history = useInfiniteQuery({
    queryKey: ['codex-session-items', projectUuid, threadUuid],
    queryFn: ({ pageParam, signal }) => listCodexSessionItems(projectUuid, threadUuid, { after: pageParam, signal }),
    initialPageParam: '',
    getNextPageParam: (page) => page.cursor_pagination.has_more ? page.cursor_pagination.next_cursor : undefined,
  })
  const items = history.data?.pages.flatMap((page) => page.items) || []
  return <section className="codex-session-history" aria-label={t('chat.codex.history')}>
    <p className="chat-muted">{t('chat.codex.read_only')}</p>
    {history.isPending ? <p className="chat-muted">{t('chat.loading')}</p> : null}
    {history.error ? <p role="alert">{history.error.message} <button type="button" className="button-quiet" onClick={() => history.isFetchNextPageError ? history.fetchNextPage() : history.refetch()}>{t('chat.codex.retry')}</button></p> : null}
    <ol className="codex-session-history__items">
      {items.map((item) => <li key={item.uuid} className={`codex-session-history__item codex-session-history__item--${item.kind}`}>
        <div className="codex-session-history__meta"><span>{t(`chat.codex.${item.kind}`)}</span><time dateTime={item.recorded_at}>{new Date(item.recorded_at).toLocaleString()}</time></div>
        <p>{item.text}</p>
      </li>)}
    </ol>
    {history.hasNextPage ? <button type="button" className="button-quiet" disabled={history.isFetching} onClick={() => history.fetchNextPage()}>{t('chat.mcp.more')}</button> : null}
  </section>
}
