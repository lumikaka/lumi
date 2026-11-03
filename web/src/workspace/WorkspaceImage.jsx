import { useEffect, useState } from 'react'
import { Sparkles } from 'lucide-react'

export default function WorkspaceImage({ asset, alt, fallbackIcon, fallbackText }) {
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [asset?.uuid, asset?.content_url])
  if (!asset || asset.status !== 'ready' || !asset.content_url || failed) return <div className="simple-image-placeholder" role="img" aria-label={alt || fallbackText}>{fallbackIcon || <Sparkles size={22} aria-hidden="true" />}{fallbackText ? <span>{fallbackText}</span> : null}</div>
  return <img src={asset.content_url} alt={alt || ''} loading="lazy" onError={() => setFailed(true)} />
}
