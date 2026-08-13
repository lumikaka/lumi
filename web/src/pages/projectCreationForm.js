export const MAX_OVERALL_STYLE_CHARACTERS = 12000

export function projectDefaultOverallStyle(defaultOverallStyles, generationLanguage) {
  return String(defaultOverallStyles?.[generationLanguage] || '')
}

export function overallStyleForLanguage({ currentStyle, dirty, defaultOverallStyles, generationLanguage }) {
  return dirty ? currentStyle : projectDefaultOverallStyle(defaultOverallStyles, generationLanguage)
}

export function overallStyleUsesDefault(style, defaultStyle) {
  const normalized = String(style || '').trim()
  return !normalized || normalized === String(defaultStyle || '').trim()
}

export function projectCreationErrors({ name, parentPath, storyPrompt, createMode, pictureBookValid, overallStyle }) {
  const errors = {}
  if (!String(name || '').trim()) errors.name = 'projects.validation.name_required'
  if (createMode === 'yolo' && !String(storyPrompt || '').trim()) errors.storyPrompt = 'projects.validation.story_idea_required'
  if (!String(parentPath || '').trim()) errors.parentPath = 'projects.validation.parent_path_required'
  if ([...String(overallStyle || '').trim()].length > MAX_OVERALL_STYLE_CHARACTERS) errors.overallStyle = 'projects.validation.overall_style_too_long'
  if (!pictureBookValid) errors.pictureBook = 'projects.picture_book.custom.invalid'
  return errors
}
