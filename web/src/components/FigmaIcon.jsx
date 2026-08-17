export default function FigmaIcon({ src, size = 16, leafWidth = size, leafHeight = size, className = '', alt = '' }) {
  return (
    <span
      className={`figma-icon ${className}`.trim()}
      style={{ width: size, height: size }}
      role={alt ? 'img' : undefined}
      aria-label={alt || undefined}
      aria-hidden={alt ? undefined : 'true'}
    >
      <img src={src} width={leafWidth} height={leafHeight} alt="" draggable="false" />
    </span>
  )
}
