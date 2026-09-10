import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { listMCPActivity } from '../api/chat.js'
import { loadMCPActivityHistory } from '../pages/mcpActivityHistory.js'
import { useI18n } from '../i18n/useI18n.js'

export default function MCPActivityHistory({ projectUuid, threadUuid }) {
  const { t } = useI18n()
  const [pageCount, setPageCount] = useState(1)
  const history = useQuery({
    queryKey: ['mcp-thread-activity', projectUuid, threadUuid, pageCount],
    queryFn: ({ signal }) => loadMCPActivityHistory(
      (after) => listMCPActivity(projectUuid, threadUuid, { after, signal }), pageCount,
    ),
    placeholderData: (previous) => previous,
  })
  const data = history.data
  return <section className="mcp-activity-history" aria-label={t('chat.mcp.history')}>
    <>{history.error ? <p role="alert" className="chat-muted">{history.error.message}</p> : null}</>
    {history.isPending ? <p className="chat-muted">{t('chat.loading')}</p> : null}
    {data ? <>
      <p className="chat-muted">{data.client_name} · {new Date(data.started_at).toLocaleString()} — {new Date(data.last_call_at).toLocaleTimeString()}</p>
      <ol className="mcp-activity-history__entries">
        {data.items.map((item) => <li key={item.uuid} className={['failed', 'rejected', 'expired', 'interrupted'].includes(item.status) ? 'mcp-activity-history__error' : ''}>
          <p>{item.text}</p>
          <time dateTime={item.admitted_at}>{new Date(item.admitted_at).toLocaleTimeString()}</time>
          {item.status === 'pending_confirmation' ? <Link to="/settings/mcp">{t('chat.mcp.confirmation')}</Link> : null}
        </li>)}
      </ol>
      {!data.items.length ? <p className="chat-muted">{t('chat.mcp.empty')}</p> : null}
      {data.cursor_pagination?.has_more ? <button className="button-quiet" type="button" disabled={history.isFetching} onClick={() => setPageCount((value) => value + 1)}>{t('chat.mcp.more')}</button> : null}
    </> : null}
  </section>
}
