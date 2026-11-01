import { useMemo } from 'react'
import BookPreview from '../components/book-preview/BookPreview.jsx'
import { webBookPages, loadWebBookImage, webBookLabels } from '../components/book-preview/webAdapter.js'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, ExternalLink, ImageOff } from 'lucide-react'
import { Link, useParams, useSearchParams } from 'react-router-dom'

import { listComicSections } from '../api/production.js'
import { getChapter, getStoryProject } from '../api/story.js'
import ImageRatioNotice from '../components/ImageRatioNotice.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { formatTerminologyMessageKey } from './pictureBookProfile.js'

export default function ChapterComicPreviewPage({ projectUuid }) {
  const { chapterUuid } = useParams()
  const [searchParams] = useSearchParams()
  const { t } = useI18n()
  const projectQuery = useQuery({ queryKey: ['story-project', projectUuid], queryFn: () => getStoryProject(projectUuid) })
  const chapterQuery = useQuery({
    queryKey: ['story-chapter', projectUuid, chapterUuid],
    queryFn: () => getChapter(projectUuid, chapterUuid),
  })
  const sectionsQuery = useQuery({
    queryKey: ['comic-sections', projectUuid, chapterUuid],
    queryFn: () => listComicSections(projectUuid, chapterUuid),
  })
  const sections = sectionsQuery.data?.items || []
  const imageCount = useMemo(() => sections.filter(sectionHasReadyImage).length, [sections])
  const preservedSearch = searchParams.toString()
  const verticalStrip = !projectQuery.data?.picture_book || projectQuery.data.picture_book.format === 'vertical_strip'
  const pictureBook = projectQuery.data?.picture_book
  const term = (key, values) => t(formatTerminologyMessageKey(pictureBook, key), values)
  if ((projectQuery.isLoading && !projectQuery.data) || (chapterQuery.isLoading && !chapterQuery.data) || (sectionsQuery.isLoading && !sectionsQuery.data)) {
    return <div className="chapter-preview"><p className="workspace-loading">{t('common.loading')}</p></div>
  }

  if (chapterQuery.isError && !chapterQuery.data) {
    return <div className="chapter-preview"><LocalizedErrorMessage error={chapterQuery.error} /></div>
  }

  const chapter = chapterQuery.data

  return (
    <main className={`chapter-preview ${verticalStrip ? 'chapter-preview--strip' : 'chapter-preview--paged'}`}>
      <LocalizedErrorMessage error={projectQuery.error || sectionsQuery.error} />
      <header className="chapter-preview__header">
        <Link className="button-secondary chapter-preview__back" to={{ pathname: `/projects/${encodeURIComponent(projectUuid)}/chapters/${encodeURIComponent(chapterUuid)}`, search: preservedSearch ? `?${preservedSearch}` : '' }}>
          <ArrowLeft size={14} aria-hidden="true" />
          {t(verticalStrip ? 'comic.workbench.preview_page.back' : 'comic.workbench.preview_page.back_pages')}
        </Link>
        <div className="chapter-preview__title">
          <p>{chapter?.chapter_code || term('story.chapter')}</p>
          <h1>{chapter?.title || t(verticalStrip ? 'comic.workbench.preview_page.title' : 'comic.workbench.preview_page.title_pages')}</h1>
        </div>
        <div className="chapter-preview__stats" aria-label={t('comic.workbench.preview_page.stats')}>
          <span>{t(verticalStrip ? 'comic.workbench.preview_page.section_count' : 'comic.workbench.preview_page.page_count', { count: sections.length })}</span>
          <span>{t('comic.workbench.preview_page.image_count', { count: imageCount })}</span>
        </div>
      </header>

      {sections.length === 0 && !sectionsQuery.isLoading ? (
        <section className="chapter-preview__empty">
          <ImageOff size={28} aria-hidden="true" />
          <h2>{t('comic.workbench.preview_page.empty')}</h2>
          <p>{t(verticalStrip ? 'comic.workbench.preview_page.empty_body' : 'comic.workbench.preview_page.empty_body_pages')}</p>
        </section>
      ) : (
        <BookPreview key={chapterUuid} title={chapter?.title} groups={[{ uuid: chapterUuid, title: '', pages: webBookPages(sections, t, { verticalStrip, targetRatio: pictureBook?.aspect_ratio }) }]} mode={verticalStrip ? 'strip' : 'paged'} loadImage={loadWebBookImage} labels={{ ...webBookLabels(t), missing: t('comic.workbench.preview_page.no_image'), waiting: t(verticalStrip ? 'comic.workbench.preview_page.lazy' : 'comic.workbench.preview_page.page_lazy') }}
          renderPageActions={(page) => page.current_image?.asset?.content_url ? <a href={page.current_image.asset.content_url} target="_blank" rel="noreferrer" aria-label={t('comic.workbench.preview_page.open_source', { title: page.title })}><ExternalLink size={16} aria-hidden="true" /></a> : null}
          renderPageDetails={(page) => page.current_image?.asset ? <ImageRatioNotice pictureBook={pictureBook} width={page.current_image.asset.width} height={page.current_image.asset.height} /> : null}
        />
      )}
    </main>
  )
}

function sectionHasReadyImage(section) {
  const asset = section.current_image?.asset
  return Boolean(asset?.status === 'ready' && asset?.content_url)
}
