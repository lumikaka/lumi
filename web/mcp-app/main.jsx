import React, { useEffect, useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { AppRealtime } from './realtime.js'
import { AppBridge } from './bridge.js'
import { reviewContext, failureKind, failureLabels, acceptReviewResult } from './contracts.js'
import { PreviewCache, PreviewRequest } from './images.js'
import { bindReviewSync } from './sync.js'
import './review.scss'

const bridge = new AppBridge()
const previews = new PreviewCache((...args) => bridge.call(...args))
const client = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, refetchOnReconnect: false, staleTime: Infinity, gcTime: 5 * 60 * 1000 } } })
function OpenLumi({ url }) {
  const [error, setError] = useState(false)
  return <span className="open-lumi"><a href={url} target="_blank" rel="noreferrer" onClick={(event) => {
    event.preventDefault(); bridge.request('ui/open-link', { url }).then((r) => { if (r?.isError) setError(true) }).catch(() => setError(true))
  }}>在 Lumi 中打开 ↗</a>{error && <small>请复制入口：<a href={url} target="_blank" rel="noreferrer">{url}</a>，或在项目目录运行 lumi open。</small>}</span>
}
function Preview({ project, asset, title, large = false }) {
  const ref = useRef(null)
  const [visible, setVisible] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [preview, setPreview] = useState({ state: asset ? 'waiting' : 'missing' })
  const loader = useRef(null)
  if (!loader.current) loader.current = new PreviewRequest(previews, setPreview)
  useEffect(() => {
    if (!asset) return undefined
    if (!globalThis.IntersectionObserver) { setVisible(true); return undefined }
    const observer = new IntersectionObserver((entries) => { if (entries.some((v) => v.isIntersecting)) { setVisible(true); observer.disconnect() } })
    observer.observe(ref.current); return () => observer.disconnect()
  }, [asset])
  useEffect(() => { loader.current.show(project, asset, large ? 1024 : 384, visible); return () => loader.current.cancel() }, [project, asset, large, visible, attempt])
  useEffect(() => {
    if (preview.state !== 'ready') return undefined
    const timer = setTimeout(() => setPreview((p) => p.state === 'ready' ? { state: 'error', error: { kind: 'timeout' } } : p), 15000)
    return () => clearTimeout(timer)
  }, [preview.state, preview.data])
  const labels = { missing: '暂无图片', waiting: '图片将在可见时加载', loading: '正在读取预览…' }
  const ready = preview.state === 'ready' || preview.state === 'decoded'
  return <div className={`preview ${large ? 'large' : ''}`} ref={ref} aria-busy={preview.state === 'loading'}>
    {ready ? <img key={`${asset}:${attempt}`} src={preview.data.data_uri} alt={title} onLoad={() => setPreview((p) => ({ ...p, state: 'decoded' }))} onError={() => setPreview({ state: 'error', error: { kind: 'decode_error' } })} /> :
      <div className="image-placeholder" role="status"><span className="image-symbol" aria-hidden="true">◩</span><span>{preview.state === 'error' ? (failureLabels[failureKind(preview.error)] ?? '预览失败') : labels[preview.state]}</span>
        {preview.state === 'error' && <button onClick={() => setAttempt((n) => n + 1)}>重试这张图片</button>}</div>}
  </div>
}
function PageCard({ item, project, detailed, navigate }) {
  const [variant, setVariant] = useState('')
  const selected = item.variants?.find((v) => v.uuid === variant)
  return <article className={`card ${detailed ? 'detail' : ''}`}>
    <Preview key={selected?.asset_uuid ?? item.asset_uuid ?? item.uuid} project={project} asset={selected?.asset_uuid ?? item.asset_uuid} title={item.title} large={detailed} />
    <div className="card-copy"><p className="eyebrow">{item.kind} · r{item.revision}</p><h2>{item.title}</h2>
      {detailed && item.variants?.length > 0 && <label className="versions">图片版本<select value={variant} onChange={(e) => setVariant(e.target.value)}><option value="">当前图片</option>{item.variants.map((v) => <option key={v.uuid} value={v.uuid}>{v.title}（仅预览）</option>)}</select></label>}
      <p className="page-text">{!detailed && item.text.length > 180 ? `${item.text.slice(0,180)}…` : item.text || '暂无正文'}</p>
      {!detailed && item.text.length > 180 && <details><summary>完整正文</summary><p className="page-text">{item.text}</p></details>}
      {item.scene && <details open={detailed}><summary>画面描述</summary><p className="page-text">{item.scene}</p></details>}
      <OpenLumi url={item.open_url} />
      <div className="card-actions">{item.chapter_uuid && !detailed && <button onClick={() => navigate('review_page', {chapter_uuid:item.chapter_uuid,page_uuid:item.uuid})}>预览本页</button>}
        {(detailed || !item.chapter_uuid) && <button onClick={() => navigate('review_candidates', item.chapter_uuid ? {kind:'page',chapter_uuid:item.chapter_uuid,page_uuid:item.uuid} : {kind:'setting',setting_uuid:item.uuid})}>查看图片候选</button>}</div>
    </div>
  </article>
}
function Candidates({ data }) {
  const [selected, setSelected] = useState(null)
  const item = data.items[0]
  return <><p className="notice">{data.selection_notice}</p><div className="grid choices">{item.variants?.map((v) => <article className="card" key={v.uuid || v.number}>
    {v.asset_uuid && <Preview project={data.project_uuid} asset={v.asset_uuid} title={`候选 ${v.number}：${v.title}`} />}
    <div className="card-copy"><button className="candidate" aria-pressed={selected === v.number} onClick={() => setSelected(v.number)}><span className="number">{String(v.number).padStart(2, '0')}</span><span>{v.title}</span><span aria-hidden="true">{selected === v.number ? '●' : '○'}</span></button><p className="page-text">{v.text}</p>{v.asset_uuid === item.asset_uuid && <small>当前使用</small>}</div>
  </article>)}</div>{!item.variants?.length && <p>暂无候选图片，可在对话中继续创作。</p>}
    {selected !== null && <p role="status" className="selection">已在界面选中 {selected}，尚未保存。请在对话中回复“选择 {selected}”。</p>}</>
}
function Review({ seed, navigate }) {
  const [request, setRequest] = useState(seed.request)
  const [lastData, setLastData] = useState(seed)
  const [status, setStatus] = useState('正在连接实时提示…')
  const [connectionAttempt, setConnectionAttempt] = useState(0)
  const query = useQuery({ queryKey: ['app-review', seed.project_uuid, request], queryFn: async () => {
    const next = reviewContext(await bridge.call(request.name, request.arguments))
    if (next.project_uuid !== seed.project_uuid || next.view !== seed.view || (seed.view !== 'book' && seed.view !== 'settings' && next.target_uuid !== seed.target_uuid)) throw new Error('审阅目标发生变化')
    return next
  }, initialData: JSON.stringify(request) === JSON.stringify(seed.request) ? seed : undefined })
  useEffect(() => { if (query.data) setLastData(query.data) }, [query.data])
  useEffect(() => {
    return bindReviewSync({ socket: new AppRealtime(bridge, seed.project_uuid), project: seed.project_uuid, queryClient: client, onStatus: setStatus })
  }, [seed.project_uuid, connectionAttempt])
  const data = query.data ?? lastData
  const labels = { book: '整本概览', page: '单页预览', candidates: '候选审阅', settings: '设定卡片' }
  const pagination = data.cursor_pagination
  return <main><header><div><p className="eyebrow">LUMI / {labels[data.view]}</p><h1>{data.title}</h1></div><OpenLumi url={data.open_url} /></header>
    <div className="toolbar"><span role="status">{query.isFetching ? '正在校准最新内容…' : status}</span><button onClick={() => query.refetch()}>刷新内容</button><button onClick={() => setConnectionAttempt((n) => n + 1)}>重新连接</button></div>
    <nav className="view-nav" aria-label="审阅视图"><button aria-pressed={data.view === 'book'} onClick={() => navigate('review_book',{})}>整本</button><button aria-pressed={data.view === 'settings'} onClick={() => navigate('review_settings',{})}>设定</button>
      {data.view === 'candidates' && data.request.arguments.kind === 'page' && <button onClick={() => navigate('review_page',{chapter_uuid:data.request.arguments.chapter_uuid,page_uuid:data.request.arguments.page_uuid})}>返回单页</button>}</nav>
    {query.error && <p role="alert" className="notice">读取失败，保留上次内容。{query.error.message} <OpenLumi url={data.open_url} /></p>}
    {data.view === 'candidates' ? <Candidates key={`${data.target_uuid}:${data.revision}`} data={data} /> : <div className={data.view === 'page' ? 'single' : 'grid'}>{data.items.map((item) => <PageCard key={`${item.uuid}:${item.revision}`} item={item} project={data.project_uuid} detailed={data.view === 'page'} navigate={navigate} />)}</div>}
    {data.items.length === 0 && <p className="empty">还没有可审阅的内容。可以在对话中继续创作。</p>}
    {pagination && <nav aria-label="审阅分页"><button disabled={pagination.prev_cursor === '0'} onClick={() => setRequest({ ...request, arguments: { ...request.arguments, before: pagination.prev_cursor, after: undefined } })}>上一组</button><span>每组 {pagination.per_page} 项</span><button disabled={!pagination.has_more} onClick={() => setRequest({ ...request, arguments: { ...request.arguments, after: pagination.next_cursor, before: undefined } })}>下一组</button></nav>}
    <footer>缩略图供审阅使用 · 原图与完整编辑请在 Lumi 中打开</footer>
  </main>
}
function App() {
  const [seed, setSeed] = useState(null); const [fallback, setFallback] = useState('等待审阅工具结果…'); const [error, setError] = useState('')
  const sequence = useRef(0)
  const navigation = useRef(0)
  const [theme, setTheme] = useState('auto')
  const [effectiveTheme, setEffectiveTheme] = useState(() => window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
  const themeRef = useRef('auto'); const hostTheme = useRef('light')
  const showReview = async (name, args) => {
    const token = ++navigation.current
    try { const result = await bridge.call(name,args); if (token !== navigation.current) return; const next=acceptReviewResult(client,result);setSeed({context:next,sequence:++sequence.current});setError('') }
    catch(e) { if(token === navigation.current) setError(e.message) }
  }
  useEffect(() => {
    const applyTheme = (context) => { if (['dark', 'light'].includes(context?.theme)) {hostTheme.current=context.theme;if(themeRef.current==='auto'){document.documentElement.dataset.theme=context.theme;setEffectiveTheme(context.theme)}} }
    const off = bridge.onNotification((method, params) => {
      if (method === 'ui/notifications/host-context-changed') applyTheme(params)
      if (method === 'ui/resource-teardown') {reactRoot.unmount();bridge.dispose();return}
      if (method !== 'ui/notifications/tool-result') return
      setFallback(params?.content?.filter((b) => b.type === 'text').map((b) => b.text).join('\n') || '工具未返回文本，请查看对话中的结果。')
      navigation.current++
      try { setSeed({ context: acceptReviewResult(client, params), sequence: ++sequence.current }); setError('') } catch (e) { setSeed(null); setError(e.message) }
    })
    bridge.initialize().then((r) => applyTheme(r.hostContext)).catch((e) => setError(failureLabels[failureKind(e)] ?? e.message))
    const observer = new ResizeObserver(() => bridge.notify('ui/notifications/size-changed', { height: document.documentElement.scrollHeight }))
    observer.observe(document.body)
    return () => { off(); observer.disconnect() }
  }, [])
  return <div className="lumi-app" data-theme={effectiveTheme}><label className="theme-control">外观<select aria-label="审阅外观" value={theme} onChange={e=>{const value=e.target.value;themeRef.current=value;setTheme(value);const next=value==='auto'?hostTheme.current:value;document.documentElement.dataset.theme=next;setEffectiveTheme(next)}}><option value="auto">跟随宿主</option><option value="light">浅色</option><option value="dark">深色</option></select></label>{seed ? <>{error && <p role="alert" className="notice">{error}</p>}<Review key={seed.sequence} seed={seed.context} navigate={showReview} /></> : <main><p className="eyebrow">LUMI / 创作审阅</p><h1>审阅内容</h1>{error && <p role="alert">{error}</p>}<p>正文、候选编号和 Lumi 入口保留在工具文本中。也可在项目目录运行 <code>lumi open</code>。</p><pre>{fallback}</pre></main>}</div>
}
const reactRoot=createRoot(document.getElementById('root'))
reactRoot.render(<QueryClientProvider client={client}><App /></QueryClientProvider>)
