export function projectCreationErrors({ name, parentPath, storyPrompt, createMode, pictureBookValid }) {
  const errors = {}
  if (!String(name || '').trim()) errors.name = 'projects.validation.name_required'
  if (createMode === 'yolo' && !String(storyPrompt || '').trim()) errors.storyPrompt = 'projects.validation.story_idea_required'
  if (!String(parentPath || '').trim()) errors.parentPath = 'projects.validation.parent_path_required'
  if (!pictureBookValid) errors.pictureBook = 'projects.picture_book.custom.invalid'
  return errors
}
