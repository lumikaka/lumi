import { useEffect, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight, ImageOff, Sparkles } from 'lucide-react'
import './book-preview.scss'

const defaults = {
  thumbnails: '页面缩略图', previous: '上一页', next: '下一页', back: '返回缩略图',
  loading: '正在读取页面…', waiting: '图片将在可见时加载', missing: '暂无图片',
  imageError: '图片加载失败', retry: '重试', empty: '暂无页面',
  position: (current, total, label) => `${label} · ${current} / ${total}`,
}

// Shared Web presentation. Adapters own data access, image transport and links.
// Web keeps its original grid links or paged/strip layout; Apps opens the same
// page reader locally. No router, REST, MCP or project-context dependencies.
export default function BookPreview({ groups, loadImage, title, labels, loading, error, onRetry, renderPageLink, renderThumbnailActions, renderPageActions, renderPageDetails, mode = 'grid', appearance = 'web' }) {
  const copy = { ...defaults, ...labels }
  const pages = groups.flatMap((group) => group.pages.map((page) => ({ ...page, groupTitle: group.title })))
  const [selected, setSelected] = useState(null)
  const lastIndex = useRef(0)
  const reader = useRef(null)
  const reading = mode === 'paged' || (selected !== null && mode !== 'strip')
  const found = pages.findIndex((page) => page.uuid === selected)
  const index = found >= 0 ? found : Math.max(0, Math.min(lastIndex.current, pages.length - 1))
  const current = reading ? pages[index] : null
  useEffect(() => {
    if (current) { lastIndex.current = index; setSelected(current.uuid) }
  }, [current?.uuid, index])
  const choose = (page) => {
    setSelected(page.uuid)
    if (appearance === 'app') requestAnimationFrame(() => { reader.current?.scrollIntoView({ block: 'nearest' }); reader.current?.focus({ preventScroll: true }) })
  }
  useEffect(() => {
    if (!reading || pages.length < 2) return undefined
    const onKeyDown = (event) => {
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.target?.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(event.target?.tagName)) return
      if (event.key === 'ArrowLeft' && index > 0) { event.preventDefault(); setSelected(pages[index - 1].uuid) }
      if (event.key === 'ArrowRight' && index < pages.length - 1) { event.preventDefault(); setSelected(pages[index + 1].uuid) }
      if (event.key === 'Escape' && mode !== 'paged') setSelected(null)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [reading, pages, index, mode])
  const image = (page, large) => <BookPreviewImage key={`${page.uuid}:${page.image_key || 'missing'}:${large}`} page={page} large={large} loadImage={loadImage} labels={copy} />
  const pageView = (page) => <article className="chapter-preview__section" key={page.uuid}>
    <header><div><p>{page.label}</p><h2>{page.title}</h2></div>{renderPageActions?.(page)}</header>
    <div className="chapter-preview__media" style={appearance === 'web' ? { aspectRatio: page.aspect_ratio } : undefined}>{image(page, true)}</div>
    {renderPageDetails?.(page)}
  </article>
  return <section className={`book-preview book-preview--${appearance}`} aria-label={title || copy.thumbnails}>
    {loading && <p role="status">{copy.loading}</p>}
    {error && <p role="alert" className="book-preview__notice">{error} {onRetry && <button type="button" onClick={onRetry}>{copy.retry}</button>}</p>}
    {!loading && !pages.length && <p className="book-preview__empty">{copy.empty}</p>}
    {current ? <section className="chapter-preview__pager" ref={reader} tabIndex={-1} aria-label={title || copy.thumbnails}>
      {mode !== 'paged' && <button type="button" className="button-secondary book-preview__back" onClick={() => setSelected(null)}>{copy.back}</button>}
      <nav className="chapter-preview__pager-controls" aria-label={copy.thumbnails}>
        <button className="button-secondary" type="button" disabled={index === 0} onClick={() => choose(pages[index - 1])}><ChevronLeft size={16} aria-hidden="true" />{copy.previous}</button>
        <strong aria-live="polite">{copy.position(index + 1, pages.length, current.label)}</strong>
        <button className="button-secondary" type="button" disabled={index === pages.length - 1} onClick={() => choose(pages[index + 1])}>{copy.next}<ChevronRight size={16} aria-hidden="true" /></button>
      </nav>
      {appearance === 'app' && current.groupTitle && <p className="book-preview__group-title">{current.groupTitle}</p>}
      {pageView(current)}
    </section> : groups.map((group) => <section key={group.uuid}>
      {group.title && <h2 className="book-preview__group-title">{group.title}</h2>}
      {mode === 'strip' ? <section className="chapter-preview__strip">{group.pages.map(pageView)}</section> : <div className="simple-book-grid">{group.pages.map((page) => <article key={page.uuid}>
        <div className="book-preview__thumbnail">
          {image(page, false)}<span>{page.label}</span>
          {renderPageLink ? renderPageLink(page, { className: 'book-preview__hit-area' }) : <button className="book-preview__hit-area" type="button" onClick={() => choose(page)} aria-label={`${page.label} · ${page.title}`} />}
        </div>
        {renderThumbnailActions?.(page)}
      </article>)}</div>}
    </section>)}
  </section>
}

export function BookPreviewImage({ page, large, loadImage, labels = defaults }) {
  const ref = useRef(null)
  const [visible, setVisible] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [state, setState] = useState({ status: page.image_key ? 'waiting' : 'missing' })
  useEffect(() => {
    if (!page.image_key || visible) return undefined
    if (!globalThis.IntersectionObserver) { setVisible(true); return undefined }
    const observer = new IntersectionObserver((entries) => { if (entries.some((entry) => entry.isIntersecting)) { setVisible(true); observer.disconnect() } }, { rootMargin: '160px' })
    observer.observe(ref.current)
    return () => observer.disconnect()
  }, [page.image_key, visible])
  useEffect(() => {
    if (!page.image_key || !visible) return undefined
    const controller = new AbortController()
    let active = true
    setState({ status: 'loading' })
    const timer = setTimeout(() => { active = false; controller.abort(); setState({ status: 'error' }) }, 30000)
    Promise.resolve().then(() => loadImage(page, { large, signal: controller.signal, attempt })).then(
      (result) => { if (active) { clearTimeout(timer); setState({ status: 'ready', src: result.src }) } },
      () => { if (active) { clearTimeout(timer); setState({ status: 'error' }) } },
    )
    return () => { active = false; clearTimeout(timer); controller.abort() }
  }, [page.image_key, visible, large, loadImage, attempt])
  useEffect(() => {
    if (state.status !== 'ready') return undefined
    const timer = setTimeout(() => setState({ status: 'error' }), 15000)
    return () => clearTimeout(timer)
  }, [state.status, state.src])
  return <div ref={ref} className="book-preview__image" aria-busy={state.status === 'loading'}>
    {['ready', 'decoded'].includes(state.status) ? <img key={attempt} src={state.src} alt={page.title || page.label} onLoad={() => setState((value) => ({ ...value, status: 'decoded' }))} onError={() => setState({ status: 'error' })} /> : <div className={large ? 'chapter-preview__placeholder' : 'simple-image-placeholder'} role="status">
      {large ? <ImageOff size={22} aria-hidden="true" /> : <Sparkles size={22} aria-hidden="true" />}<span>{state.status === 'error' ? labels.imageError : labels[state.status]}</span>
      {state.status === 'error' && <button type="button" onClick={(event) => { event.stopPropagation(); setAttempt((value) => value + 1) }}>{labels.retry}</button>}
    </div>}
  </div>
}
