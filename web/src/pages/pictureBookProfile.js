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

export function isVerticalStripPictureBook(profile) {
  return profile?.format === 'vertical_strip'
}

export function formatTerminologyKey(profile, pictureBookKey, verticalStripKey) {
  return isVerticalStripPictureBook(profile) ? verticalStripKey : pictureBookKey
}

const PICTURE_BOOK_TERMINOLOGY_KEYS = Object.freeze({
  'projects.section.chapters': 'projects.section.picture_books',
  'projects.overview.work.chapters.title': 'projects.overview.work.picture_books.title',
  'projects.overview.work.chapters.body': 'projects.overview.work.picture_books.body',
  'projects.overview.active_chapters': 'projects.overview.active_picture_books',
  'projects.exports.chapter_records': 'projects.exports.picture_book_records',
  'projects.exports.description': 'projects.exports.description_pages',
  'projects.exports.pagination': 'projects.exports.pagination_pages',
  'projects.exports.new_chapter': 'projects.exports.new_picture_book',
  'projects.exports.empty_chapter': 'projects.exports.empty_picture_book',
  'projects.exports.chapter_select': 'projects.exports.picture_book_select',
  'projects.exports.chapter_label': 'projects.exports.chapter_label_pages',
  'projects.exports.project_label': 'projects.exports.project_label_pages',
  'projects.unnamed_chapter': 'projects.unnamed_picture_book',
  'story.chapter': 'story.picture_book',
  'story.chapters': 'story.picture_books',
  'story.generation.default_prompt': 'story.generation.default_prompt_picture_book',
  'story.generation.title': 'story.generation.title_picture_book',
  'story.chapter.loading': 'story.picture_book.loading',
  'story.chapter.back': 'story.picture_book.back',
  'story.chapter.title': 'story.picture_book.title',
  'story.chapter.placeholder': 'story.picture_book.placeholder',
  'story.profile.context_body': 'story.profile.context_body_picture_book',
  'story.profile.reconstruct': 'story.profile.reconstruct_picture_books',
  'story.prompts.description': 'story.prompts.description_picture_book',
  'story.trash.title': 'story.trash.title_picture_books',
  'story.trash.description': 'story.trash.description_picture_books',
  'story.trash.empty_confirm_title': 'story.trash.empty_confirm_title_picture_books',
  'story.trash.empty_confirm_body': 'story.trash.empty_confirm_body_picture_books',
  'story.trash.empty_done': 'story.trash.empty_done_picture_books',
  'story.trash.empty_partial': 'story.trash.empty_partial_picture_books',
  'story.chapters.create.batch': 'story.picture_books.create.batch',
  'story.chapters.create.next': 'story.picture_books.create.next',
  'story.chapters.create.continue': 'story.picture_books.create.continue',
  'story.chapters.create.manual': 'story.picture_books.create.manual',
  'story.chapters.create.upload': 'story.picture_books.create.upload',
  'story.chapters.partial_generation': 'story.picture_books.partial_generation',
  'story.chapters.generation_failed': 'story.picture_books.generation_failed',
  'story.chapters.count_label': 'story.picture_books.count_label',
  'story.chapters.add': 'story.picture_books.add',
  'story.chapters.add_wait': 'story.picture_books.add_wait',
  'story.chapters.empty': 'story.picture_books.empty',
  'story.chapters.list': 'story.picture_books.list',
  'story.chapters.delete_disabled': 'story.picture_books.delete_disabled',
  'story.chapters.count': 'story.picture_books.count',
  'story.chapters.count_hint': 'story.picture_books.count_hint',
  'story.chapters.files_hint': 'story.picture_books.files_hint',
  'story.chapters.import': 'story.picture_books.import',
  'story.chapters.next_hint': 'story.picture_books.next_hint',
  'story.chapters.source_chapter': 'story.picture_books.source_chapter',
  'story.chapters.target_chapter': 'story.picture_books.target_chapter',
  'story.chapters.none': 'story.picture_books.none',
  'story.chapters.continue_hint': 'story.picture_books.continue_hint',
  'story.chapters.continue_missing': 'story.picture_books.continue_missing',
  'story.chapters.delete_title': 'story.picture_books.delete_title',
  'story.chapters.delete_hint': 'story.picture_books.delete_hint',
  'story.chapters.code': 'story.picture_books.code',
  'story.chapters.operation_generation': 'story.picture_books.operation_generation',
  'story.chapters.operation_import': 'story.picture_books.operation_import',
  'chat.workflow.step.comic_sections': 'chat.workflow.step.pages',
  'chat.workflow.step.save_section_premise': 'chat.workflow.step.save_page_premise',
  'chat.workflow.step.generate_section_image': 'chat.workflow.step.generate_page_image',
  'chat.workflow.step.save_section_image': 'chat.workflow.step.save_page_image',
  'chat.workflow.step.comic_storyboard': 'chat.workflow.step.page_scripts',
  'chat.workflow.step.story_chapter': 'chat.workflow.step.picture_book',
  'chat.workflow.step.chapter_batch_plan': 'chat.workflow.step.picture_book_batch_plan',
  'chat.workflow.kind.comic_section_image_generation': 'chat.workflow.kind.page_image_generation',
  'chat.workflow.kind.comic_storyboard_generation': 'chat.workflow.kind.page_script_generation',
  'chat.workflow.kind.story_chapter_generation': 'chat.workflow.kind.picture_book_generation',
  'chat.workflow.kind.story_chapter_batch_plan': 'chat.workflow.kind.picture_book_batch_plan',
  'chat.workflow.kind.story_chapter': 'chat.workflow.kind.picture_book',
  'chat.workflow.kind.story_chapter_with_code': 'chat.workflow.kind.picture_book_with_code',
  'chat.workflow.kind.next_story_chapter': 'chat.workflow.kind.next_picture_book',
  'chat.workflow.kind.next_story_chapter_with_code': 'chat.workflow.kind.next_picture_book_with_code',
  'chat.workflow.kind.chapter_batch_plan': 'chat.workflow.kind.picture_book_batch_plan',
  'chat.workflow.kind.chapter_batch_plan_with_count': 'chat.workflow.kind.picture_book_batch_plan_with_count',
  'chat.workflow.conflict.title': 'chat.workflow.conflict.title_pages',
  'chat.workflow.conflict.body': 'chat.workflow.conflict.body_pages',
  'chat.workflow.conflict.snapshot_notice': 'chat.workflow.conflict.snapshot_notice_pages',
  'chat.workflow.conflict.keep_existing': 'chat.workflow.conflict.keep_existing_pages',
  'chat.workflow.conflict.overwrite': 'chat.workflow.conflict.overwrite_pages',
  'premise.threads.empty_body': 'premise.threads.empty_body_picture_book',
})

export function formatTerminologyMessageKey(profile, key) {
  if (isVerticalStripPictureBook(profile)) return key
  return PICTURE_BOOK_TERMINOLOGY_KEYS[key] || key
}

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
