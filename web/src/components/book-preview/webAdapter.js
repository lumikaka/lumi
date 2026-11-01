import { comicPageLabel, comicPageFallbackTitle } from '../../pages/comicPageRoles.js'

export function webBookPages(sections, t, { verticalStrip = false, targetRatio } = {}) {
  return sections.map((section) => ({
    ...section, title: section.title || (verticalStrip ? t('comic.section.untitled') : comicPageFallbackTitle(t, section)),
    label: verticalStrip ? t('comic.workbench.section_label', { number: section.section_no }) : comicPageLabel(t, sections, section),
    image_key: section.current_image?.asset?.status === 'ready' && section.current_image.asset.content_url ? section.current_image.asset.uuid : null,
    aspect_ratio: `${Number(section.current_image?.asset?.width) || Number(targetRatio?.width) || 1} / ${Number(section.current_image?.asset?.height) || Number(targetRatio?.height) || 3}`,
  }))
}
export async function loadWebBookImage(page, { attempt }) {
  const url = page.current_image?.asset?.content_url
  if (!url) throw new Error('Missing image URL')
  return { src: attempt ? `${url}${url.includes('?') ? '&' : '?'}preview_retry=${attempt}` : url }
}
export function webBookLabels(t) {
  return {
    thumbnails: t('simple.book.reader'), previous: t('common.action.previous_page'), next: t('common.action.next_page'),
    back: t('simple.pages.book_view'), loading: t('common.loading'), waiting: t('comic.workbench.preview_page.page_lazy'),
    missing: t('simple.pages.no_image'), imageError: t('comic.image_unavailable'), retry: t('common.action.reload'), empty: t('simple.pages.empty_manage'),
    position: (current, total, label) => t('comic.workbench.preview_page.page_position_role', { label, current, total }),
  }
}
