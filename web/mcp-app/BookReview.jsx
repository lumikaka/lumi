import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import BookPreview from '../src/components/book-preview/BookPreview.jsx'
import { readWholeBook, bookGroups } from './book.js'
import { AppRealtime } from './realtime.js'
import { bindReviewSync } from './sync.js'

export default function BookReview({ seed, bridge, previews, client, OpenLumi }) {
  const [status, setStatus] = useState('正在连接实时提示…')
  const [connectionAttempt, setConnectionAttempt] = useState(0)
  const seedRef = useRef(seed)
  const query = useQuery({
    queryKey: ['app-review', seed.project_uuid, 'complete-book', seed.request.arguments.chapter_uuid || ''],
    queryFn: ({ signal }) => readWholeBook((...args) => bridge.call(...args), seed, signal),
    refetchOnMount: 'always',
  })
  useEffect(() => {
    if (seedRef.current === seed) return
    seedRef.current = seed
    void query.refetch()
  }, [seed, query.refetch])
  useEffect(() => bindReviewSync({ socket: new AppRealtime(bridge, seed.project_uuid), project: seed.project_uuid, queryClient: client, onStatus: setStatus }), [bridge, client, seed.project_uuid, connectionAttempt])
  const loadImage = useCallback(async (page, { large, signal, attempt }) => {
    const edge = large ? 1024 : 384
    if (attempt) previews.evict(seed.project_uuid, page.asset_uuid, edge)
    const data = await previews.load(seed.project_uuid, page.asset_uuid, edge, signal)
    return { src: data.data_uri }
  }, [previews, seed.project_uuid])
  const data = query.data || seed
  return <main className="final-book-preview">
    <header><div><p className="eyebrow">LUMI / {seed.request.arguments.chapter_uuid ? '章节预览' : '全书预览'}</p><h1>{data.title}</h1></div><OpenLumi url={data.open_url} /></header>
    <div className="toolbar"><span role="status">{query.isFetching ? '正在读取全部页面…' : query.data ? `${status} · ${query.data.items.length} 页` : '页面尚未读全'}</span><button onClick={() => query.refetch()}>刷新内容</button><button onClick={() => setConnectionAttempt((value) => value + 1)}>重新连接</button></div>
    <BookPreview appearance="app" title={data.title} groups={bookGroups(query.data)} loadImage={loadImage} loading={query.isPending} error={query.error ? `读取失败${query.data ? '，保留上次完整内容' : '，尚未显示完整范围'}。${query.error.message}` : ''} onRetry={() => query.refetch()} />
    <footer>仅展示当前采用的页面图片 · 原图请在 Lumi 中打开</footer>
  </main>
}
