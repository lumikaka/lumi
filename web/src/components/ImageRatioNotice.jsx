import { useI18n } from '../i18n/useI18n.js'
import { aspectRatioMismatch, pictureBookRatio, reducedRatioValue } from '../pages/pictureBookProfile.js'

export default function ImageRatioNotice({ pictureBook, width, height, beforeImport = false, showCompatible = false }) {
  const { t } = useI18n()
  const target = pictureBookRatio(pictureBook)
  const actualRatio = reducedRatioValue(width, height)
  if (!target || !actualRatio) return null
  const mismatch = aspectRatioMismatch(width, height, pictureBook)
  if (!mismatch && !showCompatible) return null
  const actual = `${width}×${height} · ${actualRatio}`
  return (
    <div className={`image-ratio-notice ${mismatch ? 'is-warning' : 'is-compatible'}`} role={mismatch ? 'alert' : 'status'}>
      <strong>{t(mismatch ? 'comic.images.ratio_mismatch' : 'comic.images.ratio_match')}</strong>
      <span>{t('comic.images.ratio_details', { target: target.value, actual })}</span>
      {mismatch ? <small>{t(beforeImport ? 'comic.images.ratio_import_warning' : 'comic.images.ratio_preview_warning')}</small> : null}
    </div>
  )
}
