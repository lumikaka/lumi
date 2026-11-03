import { useEffect, useRef, useState } from 'react'
import { ImageOff, Sparkles } from 'lucide-react'

const defaults = { waiting: '图片将在可见时加载', loading: '正在读取图片…', missing: '暂无图片', imageError: '图片加载失败', retry: '重试' }

export default function SectionImage({ page, large, loadImage, labels = defaults }) {
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
  return <div ref={ref} className="review-section-rail__image-content" aria-busy={state.status === 'loading'}>
    {['ready', 'decoded'].includes(state.status) ? <img key={attempt} src={state.src} alt={page.title || page.label} onLoad={() => setState((value) => ({ ...value, status: 'decoded' }))} onError={() => setState({ status: 'error' })} /> : <div className={'simple-image-placeholder'} role="status">
      {large ? <ImageOff size={22} aria-hidden="true" /> : <Sparkles size={22} aria-hidden="true" />}<span>{state.status === 'error' ? labels.imageError : labels[state.status]}</span>
      {state.status === 'error' && <button type="button" onClick={(event) => { event.stopPropagation(); setAttempt((value) => value + 1) }}>{labels.retry}</button>}
    </div>}
  </div>
}
