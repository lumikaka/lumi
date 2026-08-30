import { defaultPictureBookDraft, pictureBookPayload } from '../pages/pictureBookProfile.js'
import { projectDefaultOverallStyle } from '../pages/projectCreationForm.js'

export function sidebarProjectCreateInput(name, defaults, locale) {
  const generationLanguage = locale === 'en' ? 'en' : 'zh-Hans'
  return {
    name: name.trim(),
    parentPath: defaults?.parent_path || '',
    generationLanguage,
    pictureBook: pictureBookPayload(defaultPictureBookDraft()),
    overallStyle: projectDefaultOverallStyle(defaults?.default_overall_styles, generationLanguage),
  }
}
