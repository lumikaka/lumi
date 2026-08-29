export const COMIC_PAGE_ROLES = Object.freeze(['front_cover', 'body', 'back_cover'])

export function comicPageRole(section) {
  return COMIC_PAGE_ROLES.includes(section?.page_role) ? section.page_role : 'body'
}

export function comicBodySections(sections = []) {
  return sections.filter((section) => comicPageRole(section) === 'body')
}

export function comicBodyPageNumber(sections = [], section) {
  if (comicPageRole(section) !== 'body') return 0
  const explicit = Number(section?.body_page_no)
  if (Number.isInteger(explicit) && explicit > 0) return explicit
  const index = comicBodySections(sections).findIndex((item) => item.uuid === section?.uuid)
  if (index >= 0) return index + 1
  const sectionNo = Number(section?.section_no)
  return Number.isInteger(sectionNo) && sectionNo > 0 ? sectionNo : 1
}

export function comicPageLabel(t, sections, section) {
  const role = comicPageRole(section)
  if (role === 'front_cover') return t('comic.page_role.front_cover')
  if (role === 'back_cover') return t('comic.page_role.back_cover')
  return t('comic.page_role.body_label', { number: comicBodyPageNumber(sections, section) })
}

export function comicPageFallbackTitle(t, section) {
  const role = comicPageRole(section)
  if (role === 'front_cover') return t('comic.page_role.front_cover_untitled')
  if (role === 'back_cover') return t('comic.page_role.back_cover_untitled')
  return t('comic.page.untitled')
}

export function comicPageRoleOptionDisabled(sections = [], role, selectedUuid = '') {
  if (role === 'body') return false
  if (comicBodySections(sections).length === 0) return true
  const selected = sections.find((section) => section.uuid === selectedUuid)
  if (selected && comicPageRole(selected) === 'body' && comicBodySections(sections).length <= 1) return true
  return sections.some((section) => section.uuid !== selectedUuid && comicPageRole(section) === role)
}

export function comicBodyReorderUuids(sections = [], sectionUuid, direction) {
  const bodyUuids = comicBodySections(sections).map((section) => section.uuid)
  const index = bodyUuids.indexOf(sectionUuid)
  const target = index + direction
  if (index < 0 || target < 0 || target >= bodyUuids.length) return null
  const next = [...bodyUuids]
  ;[next[index], next[target]] = [next[target], next[index]]
  return next
}

export function reorderedComicBodyUuids(sections = [], sectionUuid, targetUuid, placement) {
  if (!sectionUuid || !targetUuid || sectionUuid === targetUuid || !['before', 'after'].includes(placement)) return null
  const bodyUuids = comicBodySections(sections).map((section) => section.uuid)
  if (!bodyUuids.includes(sectionUuid) || !bodyUuids.includes(targetUuid)) return null
  const remaining = bodyUuids.filter((uuid) => uuid !== sectionUuid)
  const targetIndex = remaining.indexOf(targetUuid)
  remaining.splice(targetIndex + (placement === 'after' ? 1 : 0), 0, sectionUuid)
  return remaining.every((uuid, index) => uuid === bodyUuids[index]) ? null : remaining
}
