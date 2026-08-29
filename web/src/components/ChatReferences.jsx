import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BookOpen, Check, ChevronDown, Image, Layers3, Paperclip, X } from 'lucide-react'

import { listPremiseAssets, listComicSections } from '../api/production.js'
import { listChapters } from '../api/story.js'
import { MAX_PROJECT_CHAT_REFERENCES, referenceKey } from '../pages/projectChatAttachments.js'
import { formatTerminologyKey } from '../pages/pictureBookProfile.js'
import { useI18n } from '../i18n/useI18n.js'

function snapshotOf(reference) {
  if (reference?.snapshot && typeof reference.snapshot === 'object') return reference.snapshot
  return reference?.snapshot_data && typeof reference.snapshot_data === 'object' ? reference.snapshot_data : {}
}

export function referenceTitle(reference, fallback = '') {
  const snapshot = snapshotOf(reference)
  return reference?.title || snapshot.title || snapshot.chapter_code || snapshot.name || snapshot.original_filename || fallback || reference?.resource_uuid || ''
}

function referenceTypeLabel(reference, t, pictureBook) {
  const key = {
    file: 'chat.reference.type.file',
    premise_asset: 'chat.reference.type.premise_asset',
    chapter: formatTerminologyKey(pictureBook, 'chat.reference.type.picture_book', 'chat.reference.type.chapter'),
    comic_section: formatTerminologyKey(pictureBook, 'chat.reference.type.page', 'chat.reference.type.comic_section'),
  }[reference?.resource_type]
  return key ? t(key) : reference?.resource_type || t('chat.reference.type.unknown')
}

function imageFileUuid(reference) {
  const snapshot = snapshotOf(reference)
  if (reference?.image_available === false) return ''
  return reference?.image_file_uuid
    || (reference?.resource_type === 'file' ? reference.resource_uuid : '')
    || snapshot.current_file_uuid
    || snapshot.current_image_file_uuid
    || ''
}

function referenceImageUrl(projectUuid, reference) {
  const fileUuid = imageFileUuid(reference)
  return fileUuid ? `/media/projects/${encodeURIComponent(projectUuid)}/assets/${encodeURIComponent(fileUuid)}/content` : ''
}

export function ReferenceStrip({ projectUuid, references = [], onRemove, canRemove, compact = false, pictureBook }) {
  const { t } = useI18n()
  if (!references.length) return null
  return (
    <div className={`chat-reference-strip${compact ? ' chat-reference-strip--compact' : ''}`} aria-label={t('chat.reference.selected')}>
      {references.map((reference) => {
        const imageUrl = reference.previewUrl || referenceImageUrl(projectUuid, reference)
        const title = referenceTitle(reference, t('chat.reference.untitled'))
        return (
          <span className={`chat-reference-chip chat-reference-chip--${reference.status || 'ready'}`} key={reference.localId || referenceKey(reference)} title={title}>
            {imageUrl ? <img src={imageUrl} alt="" loading="lazy" /> : <span className="chat-reference-chip__icon">{['chapter', 'comic_section'].includes(reference.resource_type) ? <BookOpen size={14} /> : reference.resource_type === 'premise_asset' ? <Layers3 size={14} /> : <Image size={14} />}</span>}
            <span className="chat-reference-chip__copy"><b>{title}</b><small>{reference.status === 'uploading' ? t('chat.reference.uploading') : reference.status === 'error' ? t('chat.reference.upload_failed') : referenceTypeLabel(reference, t, pictureBook)}</small></span>
            {onRemove && (!canRemove || canRemove(reference)) ? <button type="button" onClick={() => onRemove(reference.localId || referenceKey(reference))} aria-label={t('chat.reference.remove', { title })}><X size={12} /></button> : null}
          </span>
        )
      })}
    </div>
  )
}

function premiseImageUuid(asset) {
  return asset?.current_variant?.asset?.uuid || asset?.current_variant?.file_uuid || ''
}

function sectionImageUuid(section) {
  return section?.current_image?.asset?.uuid || section?.current_image?.file_uuid || section?.current_image_file_uuid || ''
}

export function ReferencePicker({ projectUuid, references = [], disabled = false, onToggle, pictureBook }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState('premise_asset')
  const [chapterUuid, setChapterUuid] = useState('')
  const rootRef = useRef(null)
  const selected = useMemo(() => new Set(references.map(referenceKey)), [references])
  const atLimit = references.filter((item) => item.status !== 'error').length >= MAX_PROJECT_CHAT_REFERENCES
  const premiseQuery = useQuery({
    queryKey: ['premise-assets', projectUuid, '', false],
    queryFn: () => listPremiseAssets(projectUuid, { state: 'active' }),
    enabled: open,
  })
  const chaptersQuery = useQuery({
    queryKey: ['story-chapters', projectUuid, 'active'],
    queryFn: () => listChapters(projectUuid, 'active'),
    enabled: open && ['chapter', 'comic_section'].includes(tab),
  })
  const chapters = chaptersQuery.data?.items || []
  useEffect(() => {
    if (tab === 'comic_section' && !chapters.some((chapter) => chapter.uuid === chapterUuid)) setChapterUuid(chapters[0]?.uuid || '')
  }, [chapterUuid, chapters, tab])
  const sectionsQuery = useQuery({
    queryKey: ['comic-sections', projectUuid, chapterUuid],
    queryFn: () => listComicSections(projectUuid, chapterUuid),
    enabled: open && tab === 'comic_section' && Boolean(chapterUuid),
  })
  useEffect(() => {
    if (!open) return undefined
    const close = (event) => {
      if (event.key === 'Escape' || (event.type === 'pointerdown' && !rootRef.current?.contains(event.target))) setOpen(false)
    }
    document.addEventListener('pointerdown', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('pointerdown', close)
      document.removeEventListener('keydown', close)
    }
  }, [open])

  const candidates = tab === 'premise_asset' ? premiseQuery.data?.items || [] : tab === 'chapter' ? chapters : sectionsQuery.data?.items || []
  const loading = tab === 'premise_asset' ? premiseQuery.isLoading : tab === 'chapter' ? chaptersQuery.isLoading : chaptersQuery.isLoading || sectionsQuery.isLoading
  const error = tab === 'premise_asset' ? premiseQuery.error : tab === 'chapter' ? chaptersQuery.error : chaptersQuery.error || sectionsQuery.error
  const choose = (candidate) => {
    const resourceType = tab
    const imageUuid = resourceType === 'premise_asset' ? premiseImageUuid(candidate) : resourceType === 'comic_section' ? sectionImageUuid(candidate) : ''
    onToggle({
      localId: `${resourceType}:${candidate.uuid}`,
      resource_type: resourceType,
      resource_uuid: candidate.uuid,
      title: resourceType === 'chapter' ? [candidate.chapter_code, candidate.title].filter(Boolean).join(' · ') : candidate.title || candidate.chapter_code || candidate.uuid,
      image_file_uuid: imageUuid,
      image_available: Boolean(imageUuid),
      status: 'ready',
    })
  }

  return (
    <div className="chat-reference-picker" ref={rootRef}>
      <button className="chat-reference-picker__trigger" type="button" disabled={disabled} aria-expanded={open} onClick={() => setOpen((value) => !value)} title={t('chat.reference.add')}><Paperclip size={16} /><span>{t('chat.reference.add')}</span><ChevronDown size={13} /></button>
      {open ? (
        <section className="chat-reference-picker__popover" aria-label={t('chat.reference.picker')}>
          <header><strong>{t('chat.reference.picker')}</strong><small>{t('chat.reference.count', { count: references.filter((item) => item.status !== 'error').length, max: MAX_PROJECT_CHAT_REFERENCES })}</small></header>
          <div className="chat-reference-picker__tabs" role="tablist">
            <button type="button" role="tab" aria-selected={tab === 'premise_asset'} aria-pressed={tab === 'premise_asset'} onClick={() => setTab('premise_asset')}><Layers3 size={14} />{t('chat.reference.premise_assets')}</button>
            <button type="button" role="tab" aria-selected={tab === 'chapter'} aria-pressed={tab === 'chapter'} onClick={() => setTab('chapter')}><BookOpen size={14} />{t(formatTerminologyKey(pictureBook, 'chat.reference.picture_books', 'chat.reference.chapters'))}</button>
            <button type="button" role="tab" aria-selected={tab === 'comic_section'} aria-pressed={tab === 'comic_section'} onClick={() => setTab('comic_section')}><BookOpen size={14} />{t(formatTerminologyKey(pictureBook, 'chat.reference.pages', 'chat.reference.comic_sections'))}</button>
          </div>
          {tab === 'comic_section' ? <label className="chat-reference-picker__chapter"><span>{t(formatTerminologyKey(pictureBook, 'chat.reference.picture_book', 'chat.reference.chapter'))}</span><select value={chapterUuid} onChange={(event) => setChapterUuid(event.target.value)}>{chapters.map((chapter) => <option value={chapter.uuid} key={chapter.uuid}>{chapter.chapter_code ? `${chapter.chapter_code} · ` : ''}{chapter.title || chapter.uuid}</option>)}</select></label> : null}
          {atLimit ? <p className="chat-reference-picker__notice" role="status">{t('chat.reference.limit')}</p> : null}
          {error ? <p className="chat-reference-picker__notice" role="alert">{error.message}</p> : null}
          <div className="chat-reference-picker__list">
            {loading ? <p>{t('chat.loading')}</p> : null}
            {!loading && !error && candidates.length === 0 ? <p>{t('chat.reference.empty')}</p> : null}
            {candidates.map((candidate) => {
              const key = `${tab}:${candidate.uuid}`
              const pressed = selected.has(key)
              return <button type="button" aria-pressed={pressed} disabled={!pressed && atLimit} onClick={() => choose(candidate)} key={candidate.uuid}><span>{tab === 'comic_section' && candidate.section_no ? `${candidate.section_no}. ` : ''}{tab === 'chapter' && candidate.chapter_code ? `${candidate.chapter_code} · ` : ''}{candidate.title || t('chat.reference.untitled')}</span>{pressed ? <Check size={14} /> : null}</button>
            })}
          </div>
        </section>
      ) : null}
    </div>
  )
}
