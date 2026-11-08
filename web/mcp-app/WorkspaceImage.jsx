import { useEffect, useMemo, useRef, useState } from 'react'
import { Sparkles } from 'lucide-react'
import { detailEdge, PreviewRequest } from './images.js'

// Keep the same direct img/placeholder DOM as Web. Parent card and image
// styles rely on those children; an extra wrapper would become a caption.
export function createAppImage(previews, project, detailPreviews) {
  return function AppImage({ asset, alt, fallbackIcon, fallbackText, detail }) {
    const ref = useRef(null)
    const [visible, setVisible] = useState(false)
    const [edge, setEdge] = useState(384)
    const [state, setState] = useState({ state: asset ? 'waiting' : 'missing' })
    const [attempt, setAttempt] = useState(0)
    const cache = detail && detailPreviews ? detailPreviews : previews
    const loader = useMemo(() => new PreviewRequest(cache, setState), [cache])
    useEffect(() => {
      if (!ref.current || !asset) return
      if (cache.detail) {
        const measure = () => {
          const width = ref.current?.clientWidth
          if (!width) return
          const required = detailEdge(width, window.devicePixelRatio || 1, asset)
          setEdge(previous => Math.max(previous, required))
          setVisible(true)
        }
        measure()
        const observer = new ResizeObserver(measure)
        observer.observe(ref.current.parentElement)
        return () => observer.disconnect()
      }
      const show = () => { setEdge(ref.current.clientWidth > 480 ? 1024 : 384); setVisible(true) }
      if (!globalThis.IntersectionObserver) { show(); return }
      const observer = new IntersectionObserver(entries => {
        if (entries.some(entry => entry.isIntersecting)) { show(); observer.disconnect() }
      }, { rootMargin: '160px' })
      observer.observe(ref.current)
      return () => observer.disconnect()
    }, [asset?.uuid, asset?.width, asset?.height, cache])
    useEffect(() => {
      loader.show(project, asset?.uuid, edge, visible)
      return () => loader.cancel()
    }, [loader, asset?.uuid, edge, visible, attempt])
    const retry = event => {
      event.preventDefault(); event.stopPropagation()
      cache.evict(project, asset?.uuid, edge); setAttempt(value => value + 1)
    }
    if ((state.state === 'ready' || state.state === 'decoded') && state.data.asset_uuid === asset?.uuid) {
      return <img ref={ref} src={state.data.data_uri} alt={alt || ''} onLoad={() => setState(value => ({ ...value, state: 'decoded' }))} onError={() => setState({ state: 'error' })} />
    }
    const failed = state.state === 'error'
    const errorLabel = state.error?.code === 'cli_upgrade_required' ? '请使用新版 CLI 重启 Lumi 服务后重试' : '图片读取失败 · 重试'
    return <div ref={ref} className="simple-image-placeholder" role={failed ? 'button' : 'img'} tabIndex={failed ? 0 : undefined} aria-label={failed ? errorLabel : alt || fallbackText} onClick={failed ? retry : undefined} onKeyDown={failed ? event => { if (event.key === 'Enter' || event.key === ' ') retry(event) } : undefined}>{fallbackIcon || <Sparkles size={22} aria-hidden="true" />}{(failed || fallbackText) && <span>{failed ? errorLabel : fallbackText}</span>}</div>
  }
}
