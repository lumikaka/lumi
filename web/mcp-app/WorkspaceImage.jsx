import { useEffect, useRef, useState } from 'react'
import { Sparkles } from 'lucide-react'
import { PreviewRequest } from './images.js'

// Keep the same direct img/placeholder DOM as Web. Parent card and image
// styles rely on those children; an extra wrapper would become a caption.
export function createAppImage(previews, project) {
  return function AppImage({ asset, alt, fallbackIcon, fallbackText }) {
    const ref = useRef(null)
    const [visible, setVisible] = useState(false)
    const [edge, setEdge] = useState(384)
    const [state, setState] = useState({ state: asset ? 'waiting' : 'missing' })
    const [attempt, setAttempt] = useState(0)
    const loader = useRef(null)
    if (!loader.current) loader.current = new PreviewRequest(previews, setState)
    useEffect(() => {
      if (!ref.current || !asset) return
      const show = () => { setEdge(ref.current.clientWidth > 480 ? 1024 : 384); setVisible(true) }
      if (!globalThis.IntersectionObserver) { show(); return }
      const observer = new IntersectionObserver(entries => {
        if (entries.some(entry => entry.isIntersecting)) { show(); observer.disconnect() }
      }, { rootMargin: '160px' })
      observer.observe(ref.current)
      return () => observer.disconnect()
    }, [asset?.uuid])
    useEffect(() => {
      loader.current.show(project, asset?.uuid, edge, visible)
      return () => loader.current.cancel()
    }, [asset?.uuid, edge, visible, attempt])
    const retry = event => {
      event.preventDefault(); event.stopPropagation()
      previews.evict(project, asset?.uuid, edge); setAttempt(value => value + 1)
    }
    if ((state.state === 'ready' || state.state === 'decoded') && state.data.asset_uuid === asset?.uuid) {
      return <img ref={ref} src={state.data.data_uri} alt={alt || ''} onLoad={() => setState(value => ({ ...value, state: 'decoded' }))} onError={() => setState({ state: 'error' })} />
    }
    const failed = state.state === 'error'
    return <div ref={ref} className="simple-image-placeholder" role={failed ? 'button' : 'img'} tabIndex={failed ? 0 : undefined} aria-label={failed ? '图片读取失败，点击重试' : alt || fallbackText} onClick={failed ? retry : undefined} onKeyDown={failed ? event => { if (event.key === 'Enter' || event.key === ' ') retry(event) } : undefined}>{fallbackIcon || <Sparkles size={22} aria-hidden="true" />}{(failed || fallbackText) && <span>{failed ? '图片读取失败 · 重试' : fallbackText}</span>}</div>
  }
}
