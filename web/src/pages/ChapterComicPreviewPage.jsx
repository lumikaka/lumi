import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, ChevronLeft, ChevronRight, ExternalLink, ImageOff } from 'lucide-react'
import { Link, useParams, useSearchParams } from 'react-router-dom'

import { listComicSections } from '../api/production.js'
import { getChapter, getStoryProject } from '../api/story.js'
import ImageRatioNotice from '../components/ImageRatioNotice.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { ProductionImage } from './ProductionWorkspaces.jsx'
import { formatTerminologyMessageKey } from './pictureBookProfile.js'

export default function ChapterComicPreviewPage({ projectUuid }) {
  const { chapterUuid } = useParams()
  const [searchParams] = useSearchParams()
  const { t } = useI18n()
  const [pageIndex, setPageIndex] = useState(0)
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
  const targetRatio = pictureBook?.aspect_ratio
  const currentPage = sections[pageIndex]

  useEffect(() => {
    setPageIndex((index) => Math.max(0, Math.min(index, Math.max(0, sections.length - 1))))
  }, [sections.length])

  useEffect(() => {
    if (verticalStrip || sections.length < 2) return undefined
    const onKeyDown = (event) => {
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || ['INPUT', 'TEXTAREA', 'SELECT'].includes(event.target?.tagName)) return
      if (event.key === 'ArrowLeft') {
        event.preventDefault()
        setPageIndex((index) => Math.max(0, index - 1))
      }
      if (event.key === 'ArrowRight') {
        event.preventDefault()
        setPageIndex((index) => Math.min(sections.length - 1, index + 1))
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [sections.length, verticalStrip])

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
		verticalStrip ? (
		  <section className="chapter-preview__strip" aria-label={t('comic.workbench.preview_page.region')}>
		    {sections.map((section) => <LazyPreviewSection key={section.uuid} projectUuid={projectUuid} section={section} unit="section" targetRatio={targetRatio} pictureBook={pictureBook} />)}
		  </section>
		) : (
		  <section className="chapter-preview__pager" aria-label={t('comic.workbench.preview_page.page_region')}>
		    <nav className="chapter-preview__pager-controls" aria-label={t('comic.workbench.preview_page.pagination')}>
		      <button className="button-secondary" type="button" disabled={pageIndex === 0} onClick={() => setPageIndex((index) => Math.max(0, index - 1))}><ChevronLeft size={16} aria-hidden="true" />{t('common.action.previous_page')}</button>
		      <strong>{t('comic.workbench.preview_page.page_position', { current: pageIndex + 1, total: sections.length })}</strong>
		      <button className="button-secondary" type="button" disabled={pageIndex === sections.length - 1} onClick={() => setPageIndex((index) => Math.min(sections.length - 1, index + 1))}>{t('common.action.next_page')}<ChevronRight size={16} aria-hidden="true" /></button>
		    </nav>
		    {currentPage ? <LazyPreviewSection key={currentPage.uuid} projectUuid={projectUuid} section={currentPage} unit="page" targetRatio={targetRatio} pictureBook={pictureBook} /> : null}
		  </section>
		)
      )}
    </main>
  )
}

function LazyPreviewSection({ projectUuid, section, unit = 'section', targetRatio, pictureBook }) {
  const { t } = useI18n()
  const sectionRef = useRef(null)
  const [visible, setVisible] = useState(false)
  const asset = section.current_image?.asset
  const ready = Boolean(asset?.status === 'ready' && asset?.content_url)
  const title = section.title || t(unit === 'page' ? 'comic.page.untitled' : 'comic.section.untitled')

  useEffect(() => {
    if (visible || !ready) return undefined
    const target = sectionRef.current
    if (!target) return undefined
    if (typeof IntersectionObserver === 'undefined') {
      setVisible(true)
      return undefined
    }
    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return
      setVisible(true)
      observer.disconnect()
    }, { rootMargin: '0px 0px 160px 0px', threshold: 0.01 })
    observer.observe(target)
    return () => observer.disconnect()
  }, [ready, visible])

  return (
    <article className="chapter-preview__section" ref={sectionRef}>
      <header>
        <div><p>{t(unit === 'page' ? 'comic.workbench.page_label' : 'comic.workbench.section_label', { number: section.section_no })}</p><h2>{title}</h2></div>
        {ready ? <a href={asset.content_url} target="_blank" rel="noreferrer" aria-label={t('comic.workbench.preview_page.open_source', { title })}><ExternalLink size={16} aria-hidden="true" /></a> : null}
      </header>
      <div className="chapter-preview__media" style={{ aspectRatio: previewAspectRatio(asset, targetRatio) }}>
        {ready && visible ? <ProductionImage projectUuid={projectUuid} asset={asset} alt={title} profile="detail_1024" /> : <div className="chapter-preview__placeholder"><ImageOff size={22} aria-hidden="true" /><span>{t(ready ? (unit === 'page' ? 'comic.workbench.preview_page.page_lazy' : 'comic.workbench.preview_page.lazy') : 'comic.workbench.preview_page.no_image')}</span></div>}
      </div>
      {ready ? <ImageRatioNotice pictureBook={pictureBook} width={asset.width} height={asset.height} /> : null}
    </article>
  )
}

function sectionHasReadyImage(section) {
  const asset = section.current_image?.asset
  return Boolean(asset?.status === 'ready' && asset?.content_url)
}

function previewAspectRatio(asset, targetRatio) {
  const width = Number(asset?.width) || Number(targetRatio?.width) || 1
  const height = Number(asset?.height) || Number(targetRatio?.height) || 3
  return `${width} / ${height}`
}
