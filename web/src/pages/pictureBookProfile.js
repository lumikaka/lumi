export const PICTURE_BOOK_FORMATS = Object.freeze([
  'classic_picture_book',
  'wordless_picture_book',
  'interactive_picture_book',
  'comic_story',
  'vertical_strip',
])

export const ASPECT_RATIO_MODES = Object.freeze(['landscape', 'square', 'portrait', 'custom'])
export const INTERACTION_MODES = Object.freeze(['find_it', 'make_a_choice', 'guess', 'follow_along'])
export const COMIC_LAYOUTS = Object.freeze(['four_panel', 'page_comic'])

export function defaultPictureBookDraft() {
  return {
    format: 'classic_picture_book',
    aspectMode: 'landscape',
    customWidth: 4,
    customHeight: 3,
    largeImageMinimalText: false,
    interactionMode: 'find_it',
    comicLayout: 'page_comic',
  }
}

export function draftForPictureBookFormat(format) {
  return { ...defaultPictureBookDraft(), format: PICTURE_BOOK_FORMATS.includes(format) ? format : 'classic_picture_book' }
}

export function pictureBookPayload(draft) {
  const format = PICTURE_BOOK_FORMATS.includes(draft?.format) ? draft.format : 'classic_picture_book'
  if (format === 'vertical_strip') return { format }
  const payload = { format }
  if (format !== 'interactive_picture_book') {
    const mode = ASPECT_RATIO_MODES.includes(draft?.aspectMode) ? draft.aspectMode : 'landscape'
    payload.aspect_ratio = mode === 'custom'
      ? { mode, width: Number(draft.customWidth), height: Number(draft.customHeight) }
      : { mode }
  }
  if (format === 'classic_picture_book') payload.large_image_minimal_text = Boolean(draft?.largeImageMinimalText)
  if (format === 'interactive_picture_book') payload.interaction_mode = INTERACTION_MODES.includes(draft?.interactionMode) ? draft.interactionMode : 'find_it'
  if (format === 'comic_story') payload.comic_layout = COMIC_LAYOUTS.includes(draft?.comicLayout) ? draft.comicLayout : 'page_comic'
  return payload
}

export function pictureBookDraftIsValid(draft) {
  if (draft?.format === 'vertical_strip' || draft?.format === 'interactive_picture_book' || draft?.aspectMode !== 'custom') return true
  const width = Number(draft.customWidth)
  const height = Number(draft.customHeight)
  return Number.isInteger(width) && Number.isInteger(height) && width >= 1 && width <= 100 && height >= 1 && height <= 100 && width * 3 >= height && height * 3 >= width
}

export function pictureBookFormatKey(format) {
  return `projects.picture_book.format.${PICTURE_BOOK_FORMATS.includes(format) ? format : 'classic_picture_book'}`
}

export function pictureBookAspectKey(mode) {
  return `projects.picture_book.aspect.${['landscape', 'square', 'portrait', 'custom', 'fixed'].includes(mode) ? mode : 'custom'}`
}

export function pictureBookRatio(profile) {
  const width = Number(profile?.aspect_ratio?.width)
  const height = Number(profile?.aspect_ratio?.height)
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return null
  return { width, height, value: `${width}:${height}` }
}

export function pictureBookProfileDetails(t, profile) {
  if (!profile) return []
  const ratio = pictureBookRatio(profile)
  const details = [
    { label: t('projects.picture_book.field.format'), value: t(pictureBookFormatKey(profile.format)) },
  ]
  if (profile.format !== 'interactive_picture_book') {
    details.push({ label: t('projects.picture_book.field.aspect_ratio'), value: ratio ? `${t(pictureBookAspectKey(profile.aspect_ratio?.mode))} · ${ratio.value}` : '—' })
  }
  if (profile.format === 'classic_picture_book') {
    details.push({ label: t('projects.picture_book.field.large_image_minimal_text'), value: t(profile.large_image_minimal_text ? 'common.answer.yes' : 'common.answer.no') })
  }
  if (profile.format === 'interactive_picture_book' && profile.interaction_mode) {
    details.push({ label: t('projects.picture_book.field.interaction_mode'), value: t(`projects.picture_book.interaction.${profile.interaction_mode}`) })
  }
  if (profile.format === 'comic_story' && profile.comic_layout) {
    details.push({ label: t('projects.picture_book.field.comic_layout'), value: t(`projects.picture_book.comic_layout.${profile.comic_layout}`) })
  }
  return details
}

export function aspectRatioMismatch(actualWidth, actualHeight, profile, tolerance = 0.005) {
  const ratio = pictureBookRatio(profile)
  const width = Number(actualWidth)
  const height = Number(actualHeight)
  if (!ratio || !Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return false
  const expected = ratio.width / ratio.height
  const actual = width / height
  return Math.abs(actual - expected) / expected > tolerance
}

export function reducedRatioValue(widthValue, heightValue) {
  let width = Math.round(Number(widthValue))
  let height = Math.round(Number(heightValue))
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0) return ''
  let left = width
  let right = height
  while (right) [left, right] = [right, left % right]
  return `${width / left}:${height / left}`
}

export async function readImageFileDimensions(file) {
  if (!file) return null
  if (typeof createImageBitmap === 'function') {
    const bitmap = await createImageBitmap(file)
    try { return { width: bitmap.width, height: bitmap.height } } finally { bitmap.close?.() }
  }
	if (typeof Image === 'undefined' || typeof URL === 'undefined' || typeof URL.createObjectURL !== 'function') return null
  const url = URL.createObjectURL(file)
  try {
    return await new Promise((resolve, reject) => {
      const image = new Image()
      image.onload = () => resolve({ width: image.naturalWidth, height: image.naturalHeight })
      image.onerror = reject
      image.src = url
    })
  } finally {
    URL.revokeObjectURL(url)
  }
}
