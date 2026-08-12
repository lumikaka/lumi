export function comicImageTitle(variant, fallback = '') {
  const asset = variant?.asset
  return asset?.original_filename || asset?.display_name || fallback || (variant?.version_no ? `v${variant.version_no}` : '')
}

export function comicImageDimensions(asset) {
  if (!asset?.width || !asset?.height) return ''
  return `${asset.width}x${asset.height}`
}

export function comicImageModelLabel(variant) {
  const provider = variant?.generation?.provider_type?.trim() || ''
  const model = variant?.generation?.model?.trim() || ''
  if (!model) return provider
  if (!provider || model.includes('/')) return model
  return `${provider}/${model}`
}

export function comicImageFileSize(byteSize, formatNumber = String) {
  const bytes = Number(byteSize)
  if (!Number.isFinite(bytes) || bytes < 0) return ''
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let value = bytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${formatNumber(value, { maximumFractionDigits: unitIndex === 0 ? 0 : 1 })} ${units[unitIndex]}`
}
