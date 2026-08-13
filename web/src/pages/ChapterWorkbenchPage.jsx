import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  Download,
  Eye,
  GripVertical,
  History,
  ImagePlus,
  Layers3,
  Plus,
  RefreshCw,
  Save,
  Settings,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react'
import { Link, useParams, useSearchParams } from 'react-router-dom'

import { createAssetUpload } from '../api/assets.js'
import {
  createComicSection,
  createStoryboard,
  deleteComicSection,
  generateSectionImage,
  getComicSnapshot,
  getComicState,
  importSectionImage,
  listComicSections,
  listComicSnapshots,
  listImageVariants,
  listProductionTasks,
  listStoryboards,
  reorderComicSections,
  restoreComicSnapshot,
  selectImageVariant,
  selectStoryboard,
  updateComicSection,
} from '../api/production.js'
import { getChapter, getStoryProject, listChapterStories } from '../api/story.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { sourceTypeLabel, statusLabel } from '../i18n/labels.js'
import { useI18n } from '../i18n/useI18n.js'
import MarkdownEditor from '../components/MarkdownEditor.jsx'
import MarkdownPreview from '../components/MarkdownPreview.jsx'
import LumiDialog from '../components/LumiDialog.jsx'
import ComicExportDialog from '../components/ComicExportDialog.jsx'
import ImageRatioNotice from '../components/ImageRatioNotice.jsx'
import PromptCatalogEditor from '../components/PromptCatalogEditor.jsx'
import { ProductionImage, ProductionTaskStrip } from './ProductionWorkspaces.jsx'
import {
  comicImageDimensions,
  comicImageFileSize,
  comicImageModelLabel,
  comicImageTitle,
} from './comicImagePresentation.js'
import { activeTaskFor, moveSection } from './productionWorkspaceState.js'
import { comicExportDialogRequest } from './comicExportState.js'
import { readImageFileDimensions } from './pictureBookProfile.js'
import {
  enterTimelineMultiSelect,
  filterTimelineSelection,
  normalizedChapterTab,
  normalizedPreviewTab,
  patchWorkbenchSearch,
  reorderedTimelineUuids,
  sectionImageGenerationActive,
  timelineDragScrollDelta,
  timelineDragTransition,
  timelineManageDisabledState,
  timelineSectionDropIntent,
  timelineSelectableSectionUuids,
  timelineSelectionControls,
  toggleTimelineSelection,
} from './chapterWorkbenchState.js'

const newKey = (prefix) => `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2)}`

function WorkbenchDialog({ children, className = '', dismissDisabled = false, onClose, ...props }) {
  return (
    <LumiDialog {...props} className={`chapter-workbench-dialog ${className}`} dismissDisabled={dismissDisabled} onClose={onClose}>
      {children}
    </LumiDialog>
  )
}

function updateSearchParams(searchParams, setSearchParams, patch, replace = false) {
  setSearchParams(patchWorkbenchSearch(searchParams, patch), { replace })
}

function snapshotReason(t, reason, unit = 'section') {
  if (unit === 'page') {
    const pageKey = `comic.workbench.snapshot.reason_page.${reason}`
    const pageLocalized = t(pageKey)
    if (pageLocalized !== pageKey) return pageLocalized
  }
  const key = `comic.workbench.snapshot.reason.${reason}`
  const localized = t(key)
  return localized === key ? reason : localized
}

function snapshotSource(t, source) {
  const key = `comic.workbench.snapshot.source.${source}`
  const localized = t(key)
  return localized === key ? source : localized
}

function snapshotMediaStatus(t, status) {
  const key = `comic.workbench.snapshot.media.${status || 'none'}`
  const localized = t(key)
  return localized === key ? statusLabel(t, status || 'missing') : localized
}

export default function ChapterWorkbenchPage({ projectUuid, renderBody }) {
  const { chapterUuid } = useParams()
  const { formatDateTime, t } = useI18n()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const [historyOpen, setHistoryOpen] = useState(() => searchParams.get('history') === 'snapshots')
  const [selectedSnapshotUuid, setSelectedSnapshotUuid] = useState(() => searchParams.get('snapshot_uuid') || '')
  const [error, setError] = useState(null)
  const activeTab = normalizedChapterTab(searchParams.get('workspace_tab'))
  const projectQuery = useQuery({ queryKey: ['story-project', projectUuid], queryFn: () => getStoryProject(projectUuid) })
  const chapterQuery = useQuery({ queryKey: ['story-chapter', projectUuid, chapterUuid], queryFn: () => getChapter(projectUuid, chapterUuid) })
  const chapterHistoryQuery = useQuery({ queryKey: ['story-chapter-history', projectUuid, chapterUuid], queryFn: () => listChapterStories(projectUuid, chapterUuid) })
  const sectionsQuery = useQuery({ queryKey: ['comic-sections', projectUuid, chapterUuid], queryFn: () => listComicSections(projectUuid, chapterUuid) })
  const snapshotsQuery = useQuery({ queryKey: ['comic-snapshots', projectUuid, chapterUuid], queryFn: () => listComicSnapshots(projectUuid, chapterUuid) })
  const snapshots = snapshotsQuery.data?.items || []
  const selectedSnapshot = snapshots.find((snapshot) => snapshot.uuid === selectedSnapshotUuid) || snapshots[0] || null
  const snapshotDetailQuery = useQuery({
    queryKey: ['comic-snapshot', projectUuid, chapterUuid, selectedSnapshot?.uuid],
    queryFn: () => getComicSnapshot(projectUuid, chapterUuid, selectedSnapshot.uuid),
    enabled: Boolean(historyOpen && selectedSnapshot?.uuid),
  })
  const refreshComic = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['comic-sections', projectUuid, chapterUuid] })
    queryClient.invalidateQueries({ queryKey: ['comic-state', projectUuid, chapterUuid] })
    queryClient.invalidateQueries({ queryKey: ['comic-snapshots', projectUuid, chapterUuid] })
    queryClient.invalidateQueries({ queryKey: ['comic-snapshot', projectUuid, chapterUuid] })
    queryClient.invalidateQueries({ queryKey: ['comic-storyboards', projectUuid, chapterUuid] })
    queryClient.invalidateQueries({ queryKey: ['comic-images', projectUuid, chapterUuid] })
    queryClient.invalidateQueries({ queryKey: ['production-tasks', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['story-tasks', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['story-chapter', projectUuid, chapterUuid] })
  }, [chapterUuid, projectUuid, queryClient])

  useEffect(() => {
    if (!snapshots.length) {
      if (selectedSnapshotUuid) setSelectedSnapshotUuid('')
      return
    }
    if (!selectedSnapshotUuid || !snapshots.some((snapshot) => snapshot.uuid === selectedSnapshotUuid)) setSelectedSnapshotUuid(snapshots[0].uuid)
  }, [selectedSnapshotUuid, snapshots])

  const restoreMutation = useMutation({
    mutationFn: (snapshot) => restoreComicSnapshot(projectUuid, chapterUuid, snapshot.uuid),
    onSuccess: (result) => {
      refreshComic()
      const firstSection = result?.items?.[0]
      updateSearchParams(searchParams, setSearchParams, { history: null, snapshot_uuid: null, section_uuid: firstSection?.uuid || null, preview_tab: null })
      setHistoryOpen(false)
      setSelectedSnapshotUuid('')
      setError(null)
    },
    onError: setError,
  })
  const restoreSnapshot = () => {
    const detail = snapshotDetailQuery.data
    if (!selectedSnapshot || !detail || detail.uuid !== selectedSnapshot.uuid) return
    if (window.confirm(t(unit === 'page' ? 'comic.workbench.snapshot.restore_confirm_pages' : 'comic.workbench.snapshot.restore_confirm', { version: selectedSnapshot.version_no, count: detail.sections?.length || 0 }))) restoreMutation.mutate(selectedSnapshot)
  }
  const closeHistory = () => {
    if (restoreMutation.isPending) return
    setHistoryOpen(false)
    updateSearchParams(searchParams, setSearchParams, { history: null, snapshot_uuid: null }, true)
  }
  const setTab = (tab) => updateSearchParams(searchParams, setSearchParams, { workspace_tab: tab === 'storyboard' ? null : tab })
  const chapter = chapterQuery.data
  const sections = sectionsQuery.data?.items || []
  const pictureBook = projectQuery.data?.picture_book
  const unit = pictureBook?.format && pictureBook.format !== 'vertical_strip' ? 'page' : 'section'
  const bodyVersions = chapterHistoryQuery.data?.items || []
  const chapterListSearch = useMemo(() => {
    const next = new URLSearchParams(searchParams)
    next.delete('workspace_tab')
    next.delete('section_uuid')
    next.delete('preview_tab')
    return next.toString()
  }, [searchParams])

  if (chapterQuery.isLoading) return <p className="workspace-loading">{t('story.chapter.loading')}</p>
  if (chapterQuery.isError && !chapter) return <LocalizedErrorMessage error={chapterQuery.error} />

  return (
    <div className="chapter-workbench">
      <header className="chapter-workbench__header">
        <Link className="chapter-workbench__back" to={{ pathname: `/projects/${encodeURIComponent(projectUuid)}/chapters`, search: chapterListSearch ? `?${chapterListSearch}` : '' }} aria-label={t('story.chapters.list')}><ArrowLeft size={14} aria-hidden="true" />{t('story.chapters.list')}</Link>
        <div className="chapter-workbench__header-controls">
          <button type="button" className="button-secondary chapter-workbench__history" onClick={() => setHistoryOpen(true)}><History size={16} aria-hidden="true" />{t('comic.workbench.history')}</button>
          <Link className="button-secondary chapter-workbench__preview-link" to={{ pathname: `/projects/${encodeURIComponent(projectUuid)}/chapters/${encodeURIComponent(chapterUuid)}/preview`, search: searchParams.toString() ? `?${searchParams.toString()}` : '' }}><Eye size={16} aria-hidden="true" />{t(unit === 'page' ? 'comic.workbench.preview_page.open_pages' : 'comic.workbench.preview_page.open')}</Link>
          <div className="chapter-workbench__tabs" role="tablist" aria-label={t('projects.group_navigation', { section: t('story.chapter') })}>
            <button id="chapter-tab-storyboard" type="button" role="tab" aria-selected={activeTab === 'storyboard'} aria-controls="chapter-panel-storyboard" onClick={() => setTab('storyboard')}>{t(unit === 'page' ? 'comic.workbench.tabs.pages' : 'comic.workbench.tabs.storyboard')}<span>{sections.length}</span></button>
            <button id="chapter-tab-body" type="button" role="tab" aria-selected={activeTab === 'body'} aria-controls="chapter-panel-body" onClick={() => setTab('body')}>{t('comic.workbench.tabs.body')}<span>{bodyVersions.length}</span></button>
            <button id="chapter-tab-prompts" type="button" role="tab" aria-selected={activeTab === 'prompts'} aria-controls="chapter-panel-prompts" onClick={() => setTab('prompts')}>{t('comic.workbench.tabs.prompts')}</button>
          </div>
        </div>
      </header>

      <LocalizedErrorMessage error={error || sectionsQuery.error || snapshotsQuery.error} onDismiss={error ? () => setError(null) : undefined} />
      {activeTab === 'storyboard' ? <section className="chapter-workbench__panel" id="chapter-panel-storyboard" role="tabpanel" aria-labelledby="chapter-tab-storyboard">
        {sectionsQuery.isLoading ? <ChapterComicSkeleton t={t} /> : null}
		{!sectionsQuery.isLoading && !(sectionsQuery.isError && !sectionsQuery.data) ? <ChapterComicWorkbench projectUuid={projectUuid} chapterUuid={chapterUuid} chapterLabel={chapter ? `${chapter.chapter_code} · ${chapter.title || t('projects.unnamed_chapter')}` : ''} sections={sections} searchParams={searchParams} setSearchParams={setSearchParams} refreshComic={refreshComic} pictureBook={pictureBook} unit={unit} /> : null}
        {sectionsQuery.isError && !sectionsQuery.data ? <div className="chapter-workbench__empty"><Layers3 size={30} aria-hidden="true" /><h2>{t('common.status.unavailable')}</h2></div> : null}
      </section> : null}
      {activeTab === 'body' ? <section className="chapter-workbench__panel" id="chapter-panel-body" role="tabpanel" aria-labelledby="chapter-tab-body">{renderBody?.()}</section> : null}
      {activeTab === 'prompts' ? <section className="chapter-workbench__panel" id="chapter-panel-prompts" role="tabpanel" aria-labelledby="chapter-tab-prompts"><ChapterPromptWorkbench projectUuid={projectUuid} /></section> : null}

      {historyOpen ? (
        <WorkbenchDialog className="chapter-history-dialog" dismissDisabled={restoreMutation.isPending} onClose={closeHistory}>
          <header className="lumi-dialog__header">
            <div><h2>{t('comic.workbench.snapshot.title')}</h2><p>{t(unit === 'page' ? 'comic.workbench.snapshot.body_pages' : 'comic.workbench.snapshot.body')}</p></div>
            <button type="button" className="button-quiet" disabled={restoreMutation.isPending} aria-label={t('common.action.close')} onClick={closeHistory}><X size={18} aria-hidden="true" /></button>
          </header>
          <div className="lumi-dialog__body chapter-history-dialog__body">
            <aside className="chapter-history-dialog__list" aria-label={t('comic.workbench.snapshot.list')}>
              {snapshotsQuery.isLoading ? <p className="workspace-loading">{t('common.loading')}</p> : null}
              {snapshots.map((snapshot) => (
                <button type="button" className={`chapter-history-row${snapshot.uuid === selectedSnapshot?.uuid ? ' is-selected' : ''}`} aria-pressed={snapshot.uuid === selectedSnapshot?.uuid} disabled={restoreMutation.isPending} key={snapshot.uuid} onClick={() => setSelectedSnapshotUuid(snapshot.uuid)}>
                  <span><strong>v{snapshot.version_no}</strong><em>{snapshotReason(t, snapshot.reason, unit)}</em></span>
                  <small>{snapshotSource(t, snapshot.source)} · {formatDateTime(snapshot.created_at)}</small>
                  <small>{t(unit === 'page' ? 'comic.workbench.snapshot.pages' : 'comic.workbench.snapshot.sections', { count: snapshot.section_count || 0 })}</small>
                </button>
              ))}
              {!snapshotsQuery.isLoading && snapshots.length === 0 ? <div className="workspace-empty"><History size={24} aria-hidden="true" /><h2>{t('comic.workbench.snapshot.empty')}</h2></div> : null}
            </aside>
            <section className="chapter-history-dialog__preview" aria-live="polite">
              {snapshotDetailQuery.isLoading && selectedSnapshot ? <p className="workspace-loading">{t('comic.workbench.snapshot.loading')}</p> : null}
              {snapshotDetailQuery.isError ? <LocalizedErrorMessage error={snapshotDetailQuery.error} /> : null}
              {restoreMutation.isError ? <LocalizedErrorMessage error={restoreMutation.error} /> : null}
			  {snapshotDetailQuery.data ? <SnapshotDetail projectUuid={projectUuid} detail={snapshotDetailQuery.data} t={t} unit={unit} /> : null}
            </section>
          </div>
          <footer className="lumi-dialog__actions chapter-history-dialog__actions">
            <button type="button" className="button-secondary" disabled={restoreMutation.isPending} onClick={closeHistory}>{t('common.action.close')}</button>
            <button type="button" disabled={!snapshotDetailQuery.isSuccess || snapshotDetailQuery.data?.uuid !== selectedSnapshot?.uuid || restoreMutation.isPending} onClick={restoreSnapshot}><RefreshCw size={14} aria-hidden="true" />{t(restoreMutation.isPending ? 'comic.workbench.snapshot.restoring' : 'comic.workbench.snapshot.restore')}</button>
          </footer>
        </WorkbenchDialog>
      ) : null}
    </div>
  )
}

function SnapshotDetail({ projectUuid, detail, t, unit = 'section' }) {
  return (
    <div className="chapter-snapshot-detail">
      <header>
        <div><p className="eyebrow">{detail.chapter?.chapter_code}</p><h3>{detail.chapter?.title || t('comic.unnamed')}</h3></div>
        <dl><div><dt>{t('comic.workbench.snapshot.version')}</dt><dd>v{detail.version_no}</dd></div><div><dt>{t('common.label.source')}</dt><dd>{snapshotSource(t, detail.source)}</dd></div><div><dt>{t('comic.workbench.snapshot.reason')}</dt><dd>{snapshotReason(t, detail.reason, unit)}</dd></div></dl>
      </header>
      <div className="chapter-snapshot-detail__sections">
        {(detail.sections || []).map((section, index) => (
          <article className="chapter-snapshot-section" key={section.uuid || `${detail.uuid}-${index}`}>
			<header><strong>{t(unit === 'page' ? 'comic.workbench.page_label' : 'comic.workbench.section_label', { number: section.section_no || index + 1 })}</strong><span>{section.title || t(unit === 'page' ? 'comic.page.untitled' : 'comic.section.untitled')}</span></header>
            {section.storyboard_md ? <MarkdownPreview value={section.storyboard_md} /> : <p className="chapter-snapshot-section__empty">{t(unit === 'page' ? 'comic.workbench.snapshot.storyboard_missing_page' : 'comic.workbench.snapshot.storyboard_missing')}</p>}
            <div className="chapter-snapshot-section__media">
              <SnapshotMedia projectUuid={projectUuid} media={section.current_image} label={t('comic.workbench.snapshot.current_image')} t={t} />
              <SnapshotMedia projectUuid={projectUuid} media={section.premise_reference} label={t('comic.workbench.snapshot.premise_reference')} t={t} />
            </div>
          </article>
        ))}
        {!detail.sections?.length ? <div className="workspace-empty"><Layers3 size={24} aria-hidden="true" /><h2>{t(unit === 'page' ? 'comic.workbench.snapshot.no_pages' : 'comic.workbench.snapshot.no_sections')}</h2></div> : null}
      </div>
    </div>
  )
}

function SnapshotMedia({ projectUuid, media, label, t }) {
  const ready = media?.status === 'ready' && media.asset
  return (
    <figure className={`chapter-snapshot-media chapter-snapshot-media--${media?.status || 'none'}`}>
      {ready ? <ProductionImage projectUuid={projectUuid} asset={media.asset} alt={label} /> : <div className="chapter-snapshot-media__placeholder"><ImagePlus size={22} aria-hidden="true" /><span>{snapshotMediaStatus(t, media?.status)}</span></div>}
      <figcaption><strong>{label}</strong><span>{media?.asset?.original_filename || media?.asset_uuid || snapshotMediaStatus(t, media?.status)}</span></figcaption>
    </figure>
  )
}

function ChapterComicSkeleton({ t }) {
  return (
    <div className="chapter-comic-skeleton" aria-busy="true" aria-label={t('common.loading')}>
      <div className="chapter-comic-skeleton__stage"><section><span /><i /><i /><i /><i /><i /></section><section><span /><div /></section></div>
      <footer><span /><span /><span /><span /><span /><span /></footer>
    </div>
  )
}

function ChapterComicWorkbench({ projectUuid, chapterUuid, chapterLabel, sections, searchParams, setSearchParams, refreshComic, pictureBook, unit = 'section' }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const requestedSectionUuid = searchParams.get('section_uuid') || ''
  const selected = sections.find((section) => section.uuid === requestedSectionUuid) || sections[0] || null
  const previewTab = normalizedPreviewTab(searchParams.get('preview_tab'))
  const [storyboard, setStoryboard] = useState('')
  const [title, setTitle] = useState('')
  const [multiSelect, setMultiSelect] = useState(false)
  const [manageMode, setManageMode] = useState(false)
  const [checkedSections, setCheckedSections] = useState(() => new Set())
  const [storyboardDialogOpen, setStoryboardDialogOpen] = useState(false)
  const [imageFile, setImageFile] = useState(null)
  const [imageFileDimensions, setImageFileDimensions] = useState(null)
  const [error, setError] = useState(null)
  const [notice, setNotice] = useState('')
  const [exportRequest, setExportRequest] = useState(null)
  const [imageDialog, setImageDialog] = useState(null)
  const [sectionDragState, setSectionDragState] = useState(null)
  const storyboardRef = useRef(null)
  const sectionDragRef = useRef(null)

  const storyboardsQuery = useQuery({
    queryKey: ['comic-storyboards', projectUuid, chapterUuid, selected?.uuid],
    queryFn: () => listStoryboards(projectUuid, chapterUuid, selected.uuid),
    enabled: Boolean(selected),
  })
  const imagesQuery = useQuery({
    queryKey: ['comic-images', projectUuid, chapterUuid, selected?.uuid],
    queryFn: () => listImageVariants(projectUuid, chapterUuid, selected.uuid),
    enabled: Boolean(selected),
  })
  const tasksQuery = useQuery({
    queryKey: ['production-tasks', projectUuid],
    queryFn: () => listProductionTasks(projectUuid),
  })
  const comicStateQuery = useQuery({ queryKey: ['comic-state', projectUuid, chapterUuid], queryFn: () => getComicState(projectUuid, chapterUuid) })
  const storyboards = storyboardsQuery.data?.items || []
  const images = imagesQuery.data?.items || []
  const tasks = tasksQuery.data?.items || []
  const selectableSectionUuids = useMemo(
    () => timelineSelectableSectionUuids(sections, tasks),
    [sections, tasksQuery.data?.items],
  )
  const currentStoryboard = selected?.current_storyboard?.content_md || selected?.description_md || ''
  const storyboardDirty = Boolean(selected && storyboard.trim() && storyboard !== currentStoryboard)
  const selectedIndex = sections.findIndex((section) => section.uuid === selected?.uuid)
  const imageTask = selected ? activeTaskFor(tasks, 'comic_image_generation', selected.uuid) : null
  const latestImageTask = selected ? tasks.find((task) => task.kind === 'comic_image_generation' && task.resource_uuid === selected.uuid) : null
  const showImageTask = Boolean(latestImageTask && ['queued', 'running', 'failed', 'interrupted'].includes(latestImageTask.status))
  const pageMode = unit === 'page'

  useEffect(() => {
    setStoryboard(currentStoryboard)
    setTitle(selected?.title || '')
	setImageFile(null)
	setImageFileDimensions(null)
    setNotice('')
  }, [currentStoryboard, selected?.title, selected?.uuid])

  useEffect(() => {
    setCheckedSections((current) => filterTimelineSelection(current, selectableSectionUuids))
  }, [selectableSectionUuids])

  const refreshTasks = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['production-tasks', projectUuid] })
    queryClient.invalidateQueries({ queryKey: ['comic-exports', projectUuid] })
  }, [projectUuid, queryClient])
  const selectSection = (sectionUuid) => updateSearchParams(searchParams, setSearchParams, { section_uuid: sectionUuid })
	const selectImageFile = async (file) => {
		setImageFile(file)
		setImageFileDimensions(null)
		if (!file) return
		try { setImageFileDimensions(await readImageFileDimensions(file)) } catch { setImageFileDimensions(null) }
	}
  const setPreviewTab = (tab) => updateSearchParams(searchParams, setSearchParams, { preview_tab: tab === 'current' ? null : tab })
  const mutationOptions = { onSuccess: () => { refreshComic(); setError(null) }, onError: setError }
  const saveStoryboard = useMutation({
    mutationFn: () => createStoryboard(projectUuid, chapterUuid, selected.uuid, { content_md: storyboard, source_type: 'manual', expected_revision: selected.revision }),
    onSuccess: () => { mutationOptions.onSuccess(); setNotice(t('comic.workbench.storyboard_saved')) },
    onError: setError,
  })
  const storyboardSelect = useMutation({
    mutationFn: (variant) => selectStoryboard(projectUuid, chapterUuid, selected.uuid, variant.uuid, selected.revision),
    onSuccess: () => { mutationOptions.onSuccess(); setStoryboardDialogOpen(false) },
    onError: setError,
  })
  const imageGenerate = useMutation({
    mutationFn: (section) => generateSectionImage(projectUuid, chapterUuid, section.uuid, { prompt: section.current_storyboard?.content_md || section.description_md || '', parameters: {}, idempotency_key: newKey('comic-image') }),
    onSuccess: () => { refreshTasks(); refreshComic(); setError(null) },
    onError: setError,
  })
  const batchGenerate = useMutation({
    mutationFn: async () => {
      const targets = sections.filter((section) => checkedSections.has(section.uuid) && section.current_storyboard)
      for (const section of targets) {
        await generateSectionImage(projectUuid, chapterUuid, section.uuid, { prompt: section.current_storyboard.content_md || section.description_md || '', parameters: {}, idempotency_key: newKey('comic-image-batch') })
      }
    },
    onSuccess: () => { refreshTasks(); setMultiSelect(false); setCheckedSections(new Set()); setError(null) },
    onError: setError,
  })
  const imageSelect = useMutation({
    mutationFn: (variant) => selectImageVariant(projectUuid, chapterUuid, selected.uuid, variant.uuid, selected.revision),
    onSuccess: (section, variant) => {
      mutationOptions.onSuccess()
      setImageDialog((current) => current ? { ...current, variant: section.current_image || variant } : null)
      setPreviewTab('current')
    },
    onError: setError,
  })
  const imageImport = useMutation({
    mutationFn: async () => {
      const upload = await createAssetUpload(projectUuid, { purpose: 'comic_section_image', displayName: selected.title, file: imageFile })
      return importSectionImage(projectUuid, chapterUuid, selected.uuid, { upload_uuid: upload.uuid, expected_revision: selected.revision })
    },
    onSuccess: () => { mutationOptions.onSuccess(); setImageFile(null); setImageFileDimensions(null) },
    onError: setError,
  })
  const sectionCreate = useMutation({
    mutationFn: () => createComicSection(projectUuid, chapterUuid, { title: '', description_md: '', storyboard_md: '' }),
    onSuccess: (section) => { mutationOptions.onSuccess(); selectSection(section.uuid) },
    onError: setError,
  })
  const sectionUpdate = useMutation({
    mutationFn: () => updateComicSection(projectUuid, chapterUuid, selected.uuid, { title, description_md: selected.description_md || '', expected_revision: selected.revision }),
    ...mutationOptions,
  })
  const sectionDelete = useMutation({
    mutationFn: (section) => deleteComicSection(projectUuid, chapterUuid, section.uuid, section.revision),
    onSuccess: () => { mutationOptions.onSuccess(); updateSearchParams(searchParams, setSearchParams, { section_uuid: null }, true) },
    onError: setError,
  })
  const reorder = useMutation({ mutationFn: (uuids) => reorderComicSections(projectUuid, chapterUuid, uuids), ...mutationOptions })
  const isSectionManagePending = sectionCreate.isPending || sectionUpdate.isPending || sectionDelete.isPending || reorder.isPending
  const selectionControls = timelineSelectionControls(checkedSections, selectableSectionUuids, batchGenerate.isPending)
  const selectedManageState = timelineManageDisabledState({
    createPending: sectionCreate.isPending || sectionUpdate.isPending,
    deletePending: sectionDelete.isPending,
    reorderPending: reorder.isPending,
    imageGenerationActive: sectionImageGenerationActive(tasks, selected?.uuid),
    index: selectedIndex,
    total: sections.length,
  })

  const clearSectionDrag = (type = 'cancel') => {
    sectionDragRef.current = null
    setSectionDragState((current) => timelineDragTransition(current, { type }))
  }
  const enterManageMode = () => {
    if (isSectionManagePending) return
    setManageMode(true)
    setMultiSelect(false)
    setCheckedSections(new Set())
    clearSectionDrag()
  }
  const exitManageMode = () => {
    if (isSectionManagePending) return
    setManageMode(false)
    clearSectionDrag()
  }
  const enterMultiSelectMode = () => {
    setManageMode(false)
    clearSectionDrag()
    setMultiSelect(true)
    setCheckedSections(enterTimelineMultiSelect(selected?.uuid, selectableSectionUuids))
  }
  const exitMultiSelectMode = () => {
    setMultiSelect(false)
    setCheckedSections(new Set())
  }

  const confirmDelete = (section) => {
    const state = timelineManageDisabledState({
      createPending: sectionCreate.isPending || sectionUpdate.isPending,
      deletePending: sectionDelete.isPending,
      reorderPending: reorder.isPending,
      imageGenerationActive: sectionImageGenerationActive(tasks, section.uuid),
    })
    if (state.deleteDisabled) return
	if (window.confirm(t(pageMode ? 'comic.workbench.pages.delete_confirm' : 'comic.workbench.sections.delete_confirm', { title: section.title || t(pageMode ? 'comic.page.untitled' : 'comic.section.untitled') }))) sectionDelete.mutate(section)
  }
  const moveSelected = (direction) => {
    if (selectedIndex < 0 || isSectionManagePending) return
    reorder.mutate(moveSection(sections.map((section) => section.uuid), selectedIndex, direction))
  }
  const timelineIntentForPointer = (timelineElement, clientX, clientY, draggingUuid) => {
    if (!timelineElement) return null
    const timelineRect = timelineElement.getBoundingClientRect()
    const sectionRects = [...timelineElement.querySelectorAll('[data-section-uuid]')].map((element) => {
      const rect = element.getBoundingClientRect()
      return { uuid: element.dataset.sectionUuid, left: rect.left, width: rect.width, height: rect.height }
    })
    return timelineSectionDropIntent({ timelineRect, sectionRects, clientX, clientY, draggingUuid })
  }
  const startTimelineSectionDrag = (event, section, imageState) => {
    if (!manageMode || isSectionManagePending) return
    event.preventDefault()
    event.stopPropagation()
    const itemElement = event.currentTarget.closest('[data-section-uuid]')
    const previewElement = itemElement?.querySelector('.section-timeline-card__main') || itemElement
    const previewRect = previewElement?.getBoundingClientRect()
    const dragOffsetX = previewRect ? event.clientX - previewRect.left : 0
    const dragOffsetY = previewRect ? event.clientY - previewRect.top : 0
    sectionDragRef.current = {
      sectionUuid: section.uuid,
      pointerId: event.pointerId,
      timelineElement: event.currentTarget.closest('.section-timeline__track'),
      offsetX: dragOffsetX,
      offsetY: dragOffsetY,
      previewWidth: previewRect?.width || 88,
      previewHeight: previewRect?.height || 98,
      targetUuid: null,
      placement: null,
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
    setSectionDragState((current) => timelineDragTransition(current, {
      type: 'start',
      sectionUuid: section.uuid,
      preview: {
        sectionNo: section.section_no,
		title: section.title || t(pageMode ? 'comic.page.untitled' : 'comic.section.untitled'),
        imageState,
        left: event.clientX - dragOffsetX,
        top: event.clientY - dragOffsetY,
        width: previewRect?.width || 88,
        height: previewRect?.height || 98,
      },
    }))
  }
  const updateTimelineSectionDrag = (event) => {
    const drag = sectionDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.preventDefault()
    event.stopPropagation()
    const timelineRect = drag.timelineElement?.getBoundingClientRect()
    if (drag.timelineElement) drag.timelineElement.scrollLeft += timelineDragScrollDelta(timelineRect, event.clientX)
    const intent = timelineIntentForPointer(drag.timelineElement, event.clientX, event.clientY, drag.sectionUuid)
    drag.targetUuid = intent?.targetUuid || null
    drag.placement = intent?.placement || null
    setSectionDragState((current) => timelineDragTransition(current, {
      type: 'move',
      targetUuid: drag.targetUuid,
      placement: drag.placement,
      preview: {
        left: event.clientX - drag.offsetX,
        top: event.clientY - drag.offsetY,
      },
    }))
  }
  const finishTimelineSectionDrag = (event) => {
    const drag = sectionDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.preventDefault()
    event.stopPropagation()
    const intent = timelineIntentForPointer(drag.timelineElement, event.clientX, event.clientY, drag.sectionUuid)
    const targetUuid = intent?.targetUuid || drag.targetUuid
    const placement = intent?.placement || drag.placement
    clearSectionDrag('complete')
    const next = reorderedTimelineUuids(sections.map((section) => section.uuid), drag.sectionUuid, targetUuid, placement)
    if (next && !isSectionManagePending) reorder.mutate(next)
  }
  const cancelTimelineSectionDrag = (event) => {
    const drag = sectionDragRef.current
    if (drag && drag.pointerId !== event.pointerId) return
    clearSectionDrag()
  }
  const stepSection = (direction) => {
    const nextIndex = selectedIndex + direction
    if (nextIndex >= 0 && nextIndex < sections.length) selectSection(sections[nextIndex].uuid)
  }
  const selectTimelineSection = (section) => {
    selectSection(section.uuid)
    if (multiSelect) setCheckedSections((current) => toggleTimelineSelection(current, section.uuid, selectableSectionUuids))
  }
  const openStoryboardSearch = () => storyboardRef.current?.openSearchPanel()
  const openStoryboardReference = async () => {
    if (!selected || !storyboard.trim() || saveStoryboard.isPending) return
    if (storyboardDirty) {
      try {
        await saveStoryboard.mutateAsync()
      } catch {
        return
      }
    }
    updateSearchParams(searchParams, setSearchParams, {
      chat_scope: 'project',
      chat_thread_uuid: null,
      workflow_uuid: null,
      chat_new: '1',
      chat_scene: 'storyboard_reference',
      chat_subject_uuid: selected.uuid,
      chat_subject_title: t(pageMode ? 'comic.workbench.ai.page_subject_title' : 'comic.workbench.ai.subject_title', { number: selected.section_no, title: selected.title || t(pageMode ? 'comic.page.untitled' : 'comic.section.untitled') }),
    })
  }
  const availableBatchCount = selectionControls.selectedCount

  useEffect(() => setImageDialog(null), [selected?.uuid])

  useEffect(() => {
    if (!imageDialog?.variant) return
    const updated = images.find((variant) => variant.uuid === imageDialog.variant.uuid)
      || (selected?.current_image?.uuid === imageDialog.variant.uuid ? selected.current_image : null)
    if (updated && updated !== imageDialog.variant) setImageDialog((current) => current ? { ...current, variant: updated } : null)
  }, [imageDialog, images, selected?.current_image])

  if (!selected && sections.length === 0) {
    return (
      <section className="chapter-workbench__empty">
        <Layers3 size={30} aria-hidden="true" />
        <h2>{t(pageMode ? 'comic.page.empty' : 'comic.section.empty')}</h2>
        <p>{t(pageMode ? 'comic.page.empty_body' : 'comic.section.empty_body')}</p>
        <button type="button" disabled={sectionCreate.isPending} onClick={() => sectionCreate.mutate()}><Plus size={16} aria-hidden="true" />{t(pageMode ? 'comic.page.add' : 'comic.section.add')}</button>
      </section>
    )
  }

  return (
    <div className="chapter-comic-workbench">
      <LocalizedErrorMessage error={error || storyboardsQuery.error || imagesQuery.error || tasksQuery.error || comicStateQuery.error} onDismiss={error ? () => setError(null) : undefined} />
      {notice ? <div className="chapter-workbench__notice" role="status"><Check size={15} aria-hidden="true" />{notice}</div> : null}
      {comicStateQuery.data && !comicStateQuery.data.has_premise_assets ? <PremiseAssetsWarning projectUuid={projectUuid} searchParams={searchParams} compact={false} t={t} /> : null}
      <div className="chapter-comic-workbench__stage">
        <section className="storyboard-workbench" aria-label={t(pageMode ? 'comic.workbench.page_storyboard_title' : 'comic.workbench.storyboard_title')}>
          <header className="storyboard-workbench__header">
            <h2>{t(pageMode ? 'comic.workbench.page_storyboard_heading' : 'comic.workbench.storyboard_heading')}</h2>
            <div className="storyboard-workbench__actions">
              <button type="button" className="button-secondary" onClick={openStoryboardSearch} disabled={saveStoryboard.isPending}>{t('comic.workbench.search.open')}</button>
              <button type="button" className="button-secondary" onClick={() => setStoryboardDialogOpen(true)}>{t('comic.workbench.storyboard_candidates_short')}</button>
              <button type="button" className="button-secondary" disabled={!selected.current_storyboard || Boolean(imageTask) || imageGenerate.isPending} onClick={() => imageGenerate.mutate(selected)}><RefreshCw size={14} aria-hidden="true" />{t(selected.current_image ? 'comic.workbench.preview.regenerate_short' : 'comic.workbench.preview.generate_first')}</button>
              {storyboardDirty ? <button type="button" className="button-secondary" disabled={saveStoryboard.isPending} onClick={() => saveStoryboard.mutate()}><Save size={14} aria-hidden="true" />{t(saveStoryboard.isPending ? 'common.status.saving' : 'common.action.save')}</button> : null}
            </div>
          </header>
          <div className="storyboard-workbench__editor">
            <button type="button" className="storyboard-workbench__ai" onClick={openStoryboardReference} disabled={!storyboard.trim() || saveStoryboard.isPending} aria-label={t('comic.workbench.ai.open')} title={t('comic.workbench.ai.open')}><Sparkles size={14} aria-hidden="true" /></button>
            <MarkdownEditor ref={storyboardRef} className="storyboard-code-editor" value={storyboard} onChange={(value) => { setStoryboard(value); setNotice('') }} disabled={saveStoryboard.isPending} enableSearch placeholderText={t('comic.storyboard.placeholder')} ariaLabel={t(pageMode ? 'comic.workbench.page_storyboard_title' : 'comic.workbench.storyboard_title')} />
          </div>
        </section>

        <section className="section-preview" aria-label={t(pageMode ? 'comic.images.page_title' : 'comic.images.title')}>
          <header className="section-preview__header">
            <div className="section-preview__title">
			  <h2>{t(pageMode ? 'comic.workbench.page_label' : 'comic.workbench.section_label', { number: selected.section_no })}</h2>
              <div className="section-preview__navigation" aria-label={t(pageMode ? 'comic.workbench.preview.page_navigation' : 'comic.workbench.preview.navigation')}>
                <button type="button" className="button-secondary" aria-label={t(pageMode ? 'common.action.previous_page' : 'comic.workbench.preview.previous')} disabled={selectedIndex <= 0} onClick={() => stepSection(-1)}><ChevronLeft size={15} aria-hidden="true" /></button>
                <button type="button" className="button-secondary" aria-label={t(pageMode ? 'common.action.next_page' : 'comic.workbench.preview.next')} disabled={selectedIndex >= sections.length - 1} onClick={() => stepSection(1)}><ChevronRight size={15} aria-hidden="true" /></button>
              </div>
            </div>
            <div className="section-preview__tabs" role="tablist" aria-label={t(pageMode ? 'comic.images.page_title' : 'comic.images.title')}>
              <button type="button" role="tab" aria-selected={previewTab === 'current'} onClick={() => setPreviewTab('current')}>{t('comic.workbench.preview.current')}</button>
              <button type="button" role="tab" aria-selected={previewTab === 'reference'} onClick={() => setPreviewTab('reference')}>{t('comic.workbench.preview.reference')}</button>
              <button type="button" role="tab" aria-selected={previewTab === 'candidates'} onClick={() => setPreviewTab('candidates')}>{t('comic.workbench.preview.candidates')}<span>{images.length}</span></button>
            </div>
            <div className="section-preview__header-actions">
              <button type="button" className="button-secondary" disabled={!selected.current_storyboard || Boolean(imageTask) || imageGenerate.isPending} onClick={() => imageGenerate.mutate(selected)}><RefreshCw size={14} aria-hidden="true" />{t(selected.current_image ? 'comic.workbench.preview.generate' : 'comic.workbench.preview.generate_first')}</button>
              <button type="button" className="button-secondary" onClick={() => setExportRequest(comicExportDialogRequest('chapter', chapterUuid, chapterLabel))}><Download size={14} aria-hidden="true" />{t('comic.workbench.export.download')}</button>
            </div>
          </header>
          <div className="section-preview__canvas" role="tabpanel">
			{previewTab === 'current' ? <CurrentImage projectUuid={projectUuid} section={selected} onOpen={(variant) => setImageDialog({ mode: 'preview', variant })} t={t} pictureBook={pictureBook} unit={unit} /> : null}
            {previewTab === 'reference' ? <ReferenceImage projectUuid={projectUuid} section={selected} t={t} unit={unit} /> : null}
            {previewTab === 'candidates' ? <div className="section-preview__candidate-view">
              <div className="section-preview__candidate-tools">
				<label className="button-secondary section-preview__import">{t('comic.images.replace')}<input name="section_image_upload" type="file" accept="image/*" onChange={(event) => selectImageFile(event.target.files?.[0] || null)} /></label>
				{imageFile ? <button type="button" className="button-secondary" disabled={imageImport.isPending} onClick={() => imageImport.mutate()}>{t('comic.images.import')}</button> : null}
			  </div>
			  {imageFileDimensions ? <ImageRatioNotice pictureBook={pictureBook} width={imageFileDimensions.width} height={imageFileDimensions.height} beforeImport showCompatible /> : null}
			  <ImageCandidates projectUuid={projectUuid} section={selected} images={images} pending={imageSelect.isPending} onOpen={(variant) => setImageDialog({ mode: 'candidate', variant })} onSelect={(variant) => imageSelect.mutate(variant)} t={t} />
            </div> : null}
          </div>
          {showImageTask ? <div className="section-preview__task"><ProductionTaskStrip projectUuid={projectUuid} tasks={tasks} resourceUuid={selected.uuid} kind="comic_image_generation" refresh={refreshTasks} /></div> : null}
        </section>
      </div>

      <section className="section-timeline">
		{comicStateQuery.data && !comicStateQuery.data.has_premise_assets ? <PremiseAssetsWarning projectUuid={projectUuid} searchParams={searchParams} compact t={t} /> : null}
        <header className="section-timeline__header">
		  <h2>{t(pageMode ? 'comic.workbench.pages.heading' : 'comic.workbench.sections.heading')}</h2>
          <div className="section-timeline__actions">
            <span>{multiSelect ? `${selectionControls.selectedCount}/${selectionControls.selectableCount}` : sections.length}</span>
            {manageMode ? <>
			  <button type="button" className="button-secondary" disabled={isSectionManagePending} onClick={() => sectionCreate.mutate()}><Plus size={14} aria-hidden="true" />{t(pageMode ? 'comic.workbench.pages.add' : 'comic.workbench.sections.add')}</button>
              <button type="button" className="button-secondary" disabled={isSectionManagePending} onClick={exitManageMode}>{t('comic.workbench.sections.done')}</button>
            </> : multiSelect ? <>
              <button type="button" className="button-secondary" aria-pressed="true" disabled={batchGenerate.isPending} onClick={exitMultiSelectMode}>{t('comic.workbench.sections.exit_multi')}</button>
              <button type="button" className="button-secondary" disabled={selectionControls.selectAllDisabled} onClick={() => setCheckedSections(new Set(selectableSectionUuids))}>{t('comic.workbench.sections.select_all')}</button>
              <button type="button" className="button-secondary" disabled={selectionControls.clearDisabled} onClick={() => setCheckedSections(new Set())}>{t('comic.workbench.sections.clear')}</button>
              <button type="button" className="section-timeline__batch-action" disabled={selectionControls.generateDisabled} onClick={() => batchGenerate.mutate()}>{t(availableBatchCount ? 'comic.workbench.sections.batch_generate_count' : 'comic.workbench.sections.batch_generate', { count: availableBatchCount })}</button>
            </> : <>
              <button type="button" className="button-secondary" aria-pressed="false" disabled={!selectableSectionUuids.length} onClick={enterMultiSelectMode}>{t('comic.workbench.sections.multi_select')}</button>
              <button type="button" className="button-secondary" aria-pressed="false" disabled={isSectionManagePending} onClick={enterManageMode}><Settings size={14} aria-hidden="true" />{t('comic.workbench.sections.manage')}</button>
            </>}
          </div>
        </header>
		{manageMode ? <p className="section-timeline__hint">{t(pageMode ? 'comic.workbench.pages.manage_hint' : 'comic.workbench.sections.manage_hint')}</p> : null}
        <div className={`section-timeline__track${manageMode ? ' is-managing' : ''}${multiSelect ? ' is-multi-selecting' : ''}`}>
          {sections.map((section, index) => {
            const task = tasks.find((item) => item.kind === 'comic_image_generation' && item.resource_uuid === section.uuid)
            const isProcessing = sectionImageGenerationActive(tasks, section.uuid)
            const imageState = isProcessing ? 'is-processing' : ['failed', 'interrupted'].includes(task?.status) ? 'is-failed' : section.current_image ? 'is-completed' : ''
            const isActive = section.uuid === selected.uuid
            const isChecked = checkedSections.has(section.uuid)
            const isSelectable = selectableSectionUuids.includes(section.uuid)
            const isDragging = sectionDragState?.sectionUuid === section.uuid
            const isDropBefore = isDragging === false && sectionDragState?.targetUuid === section.uuid && sectionDragState?.placement === 'before'
            const isDropAfter = isDragging === false && sectionDragState?.targetUuid === section.uuid && sectionDragState?.placement === 'after'
            const manageState = timelineManageDisabledState({
              createPending: sectionCreate.isPending || sectionUpdate.isPending,
              deletePending: sectionDelete.isPending,
              reorderPending: reorder.isPending,
              imageGenerationActive: isProcessing,
              index,
              total: sections.length,
            })
            const isDeleting = sectionDelete.isPending && sectionDelete.variables?.uuid === section.uuid
            const cardClassName = [
              'section-timeline-card',
              isActive ? 'is-active' : '',
              isChecked ? 'is-checked' : '',
              manageMode ? 'is-manageable' : '',
              multiSelect ? 'is-multi-selectable' : '',
              isProcessing ? 'is-processing' : '',
              isDragging ? 'is-dragging' : '',
              isDropBefore ? 'is-drop-before' : '',
              isDropAfter ? 'is-drop-after' : '',
              isDeleting ? 'is-pending' : '',
            ].filter(Boolean).join(' ')
            return (
            <article
              key={section.uuid}
              className={cardClassName}
              data-section-uuid={section.uuid}
              aria-busy={isDeleting ? 'true' : undefined}
            >
              <button
                type="button"
                className="section-timeline-card__main"
                disabled={multiSelect && !isSelectable}
                aria-current={isActive ? 'true' : undefined}
                aria-pressed={multiSelect ? isChecked : undefined}
                title={multiSelect && !isSelectable ? t(pageMode ? 'comic.workbench.pages.selection_unavailable' : 'comic.workbench.sections.selection_unavailable', { number: section.section_no }) : undefined}
                onClick={() => selectTimelineSection(section)}
              >
                {multiSelect ? <span className="section-timeline-card__check" aria-hidden="true" /> : null}
                <span className="section-timeline-card__copy">
				  <strong>{t(pageMode ? 'comic.workbench.page_label' : 'comic.workbench.section_label', { number: section.section_no })}</strong>
				  <span>{section.title || t(pageMode ? 'comic.page.untitled' : 'comic.section.untitled')}</span>
                </span>
              </button>
              <i className={`section-timeline-card__status ${imageState}`} role="img" aria-label={t(isProcessing ? 'comic.task.syncing' : section.current_image ? 'comic.section.has_image' : 'comic.section.has_storyboard')} />
              {manageMode ? <>
                <button
                  type="button"
                  className="section-timeline-card__drag"
                  disabled={manageState.dragDisabled}
                  aria-label={t(pageMode ? 'comic.workbench.pages.drag' : 'comic.workbench.sections.drag', { number: section.section_no })}
                  title={t('comic.workbench.sections.drag_title')}
                  onPointerDown={(event) => startTimelineSectionDrag(event, section, imageState)}
                  onPointerMove={updateTimelineSectionDrag}
                  onPointerUp={finishTimelineSectionDrag}
                  onPointerCancel={cancelTimelineSectionDrag}
                  onClick={(event) => event.preventDefault()}
                ><GripVertical size={13} aria-hidden="true" /></button>
                <button
                  type="button"
                  className="section-timeline-card__delete"
                  disabled={manageState.deleteDisabled}
                  aria-label={t(pageMode ? 'comic.workbench.pages.delete_numbered' : 'comic.workbench.sections.delete_numbered', { number: section.section_no })}
                  title={t(isProcessing ? (pageMode ? 'comic.workbench.pages.delete_generating_title' : 'comic.workbench.sections.delete_generating_title') : (pageMode ? 'comic.workbench.pages.delete_title' : 'comic.workbench.sections.delete_title'))}
                  onClick={() => confirmDelete(section)}
                ><Trash2 size={13} aria-hidden="true" /></button>
              </> : null}
            </article>
            )
          })}
        </div>
      </section>

      {sectionDragState?.preview ? (
        <div
          className="section-timeline-card__drag-preview"
          style={{
            left: `${sectionDragState.preview.left}px`,
            top: `${sectionDragState.preview.top}px`,
            width: `${sectionDragState.preview.width}px`,
            height: `${sectionDragState.preview.height}px`,
          }}
          aria-hidden="true"
        >
          <i className={`section-timeline-card__status ${sectionDragState.preview.imageState}`} />
          <span className="section-timeline-card__copy">
			<strong>{t(pageMode ? 'comic.workbench.page_label' : 'comic.workbench.section_label', { number: sectionDragState.preview.sectionNo })}</strong>
            <span>{sectionDragState.preview.title}</span>
          </span>
        </div>
      ) : null}

      {manageMode ? (
        <section className="section-meta-editor">
		  <label>{t(pageMode ? 'comic.page.title' : 'comic.section.title')}<input name="section_title" value={title} onChange={(event) => setTitle(event.target.value)} /></label>
          <button type="button" disabled={title === selected.title || selectedManageState.pending} onClick={() => sectionUpdate.mutate()}>{t(pageMode ? 'comic.page.save' : 'comic.section.save')}</button>
          <button type="button" className="button-secondary" aria-label={t(pageMode ? 'comic.workbench.pages.move_before' : 'comic.workbench.sections.move_before')} disabled={selectedManageState.moveBeforeDisabled} onClick={() => moveSelected(-1)}><ChevronUp size={16} aria-hidden="true" /></button>
          <button type="button" className="button-secondary" aria-label={t(pageMode ? 'comic.workbench.pages.move_after' : 'comic.workbench.sections.move_after')} disabled={selectedManageState.moveAfterDisabled} onClick={() => moveSelected(1)}><ChevronDown size={16} aria-hidden="true" /></button>
        </section>
      ) : null}

      {storyboardDialogOpen ? (
        <WorkbenchDialog className="storyboard-candidates-dialog" dismissDisabled={storyboardSelect.isPending} onClose={() => setStoryboardDialogOpen(false)}>
		  <header className="lumi-dialog__header"><div><h2>{t('comic.workbench.storyboard_candidates')}</h2><p>{selected.title || t(pageMode ? 'comic.page.untitled' : 'comic.section.untitled')}</p></div><button type="button" className="button-quiet" disabled={storyboardSelect.isPending} aria-label={t('common.action.close')} onClick={() => setStoryboardDialogOpen(false)}><X size={18} aria-hidden="true" /></button></header>
          <div className="lumi-dialog__body storyboard-candidates-dialog__body">
            {storyboards.map((variant) => <article key={variant.uuid} className={variant.uuid === selected.current_storyboard?.uuid ? 'is-current' : ''}><header><strong>v{variant.version_no}</strong><span>{sourceTypeLabel(t, variant.source_type)}</span></header><pre data-user-content>{variant.content_md}</pre><button type="button" className="button-secondary" disabled={variant.uuid === selected.current_storyboard?.uuid || storyboardSelect.isPending} onClick={() => storyboardSelect.mutate(variant)}>{t(variant.uuid === selected.current_storyboard?.uuid ? 'comic.workbench.preview.selected' : 'common.action.restore')}</button></article>)}
            {!storyboardsQuery.isLoading && storyboards.length === 0 ? <div className="workspace-empty"><h2>{t(pageMode ? 'comic.workbench.page_storyboard_empty' : 'comic.workbench.storyboard_empty')}</h2></div> : null}
          </div>
        </WorkbenchDialog>
      ) : null}
	  {imageDialog?.mode === 'preview' ? <CurrentImagePreviewDialog projectUuid={projectUuid} section={selected} variant={imageDialog.variant} onClose={() => setImageDialog(null)} pictureBook={pictureBook} unit={unit} /> : null}
	  {imageDialog?.mode === 'candidate' ? <ImageVariantDetailDialog projectUuid={projectUuid} variant={imageDialog.variant} current={selected.current_image?.uuid === imageDialog.variant.uuid || (imageSelect.isSuccess && imageSelect.variables?.uuid === imageDialog.variant.uuid)} pending={imageSelect.isPending} onClose={() => setImageDialog(null)} onSelect={() => imageSelect.mutate(imageDialog.variant)} pictureBook={pictureBook} /> : null}
      {exportRequest ? <ComicExportDialog projectUuid={projectUuid} request={exportRequest} onClose={() => setExportRequest(null)} /> : null}
    </div>
  )
}

function PremiseAssetsWarning({ projectUuid, searchParams, compact, t }) {
  const search = searchParams.toString()
  return <aside className={`comic-premise-warning${compact ? ' comic-premise-warning--compact' : ''}`} role="status"><div><strong>{t('comic.workbench.premise_warning.title')}</strong><span>{t('comic.workbench.premise_warning.body')}</span></div><Link className="button-secondary" to={{ pathname: `/projects/${encodeURIComponent(projectUuid)}/premise`, search: search ? `?${search}` : '' }}>{t('comic.workbench.premise_warning.add')}</Link></aside>
}

function CurrentImage({ projectUuid, section, onOpen, t, pictureBook, unit = 'section' }) {
  const variant = section.current_image
  if (variant?.asset) {
	    const asset = variant.asset
	const alt = section.title || t(unit === 'page' ? 'comic.page' : 'comic.section')
    return (
      <figure className="comic-section-visual comic-section-visual--image">
        <ProductionImage
          projectUuid={projectUuid}
          asset={asset}
          alt={alt}
          profile="detail_1024"
          renderReady={(image) => <button className="comic-section-visual__media comic-section-visual__media-button" type="button" aria-haspopup="dialog" aria-label={t(unit === 'page' ? 'comic.workbench.image_detail.open_current_page' : 'comic.workbench.image_detail.open_current')} aria-busy="false" onClick={() => onOpen(variant)}>{image}</button>}
        />
		<ImageMetadata as="figcaption" items={[comicImageDimensions(asset), comicImageModelLabel(variant), comicImageTitle(variant)]} />
		<ImageRatioNotice pictureBook={pictureBook} width={asset.width} height={asset.height} />
      </figure>
    )
  }
  return <div className="section-preview__empty"><ImagePlus size={30} aria-hidden="true" /><strong>{t(unit === 'page' ? 'comic.workbench.preview.page_current_empty' : 'comic.workbench.preview.current_empty')}</strong><p>{t(unit === 'page' ? 'comic.workbench.preview.page_current_empty_body' : 'comic.workbench.preview.current_empty_body')}</p></div>
}

function ReferenceImage({ projectUuid, section, t, unit = 'section' }) {
  const premise = section.current_image?.section_premise
  if (premise?.asset) {
    const titles = premise.selected_titles?.length ? premise.selected_titles : (premise.selected_assets || []).map((asset) => asset.title).filter(Boolean)
    return (
      <figure className="comic-section-visual comic-section-visual--premise">
        <div className="comic-section-visual__media"><ProductionImage projectUuid={projectUuid} asset={premise.asset} alt={t(unit === 'page' ? 'comic.workbench.image_detail.reference_alt_page' : 'comic.workbench.image_detail.reference_alt', { number: section.section_no })} profile="detail_1024" /></div>
        <ImageMetadata as="figcaption" items={[comicImageDimensions(premise.asset), comicImageTitle({ asset: premise.asset })]} />
        {titles.length ? <div className="comic-section-visual__files" aria-label={t('comic.workbench.image_detail.selected_references')}>{titles.map((title) => <span key={title}>{title}</span>)}</div> : null}
        {premise.selection_reason ? <p className="comic-section-visual__reason" data-user-content>{premise.selection_reason}</p> : null}
      </figure>
    )
  }
  return <div className="section-preview__empty"><Layers3 size={30} aria-hidden="true" /><strong>{t('comic.workbench.preview.reference_empty')}</strong><p>{t('comic.workbench.preview.reference_body')}</p></div>
}

function ImageCandidates({ projectUuid, section, images, pending, onOpen, onSelect, t }) {
  const { formatDateTime, formatNumber } = useI18n()
  if (!images.length) return <div className="section-preview__empty"><ImagePlus size={30} aria-hidden="true" /><strong>{t('comic.workbench.preview.candidates_empty')}</strong></div>
  return (
    <div className="section-preview__candidate-grid">
      {images.map((variant) => {
        const current = variant.uuid === section.current_image?.uuid
        const title = comicImageTitle(variant, `v${variant.version_no}`)
        const source = current ? t('comic.workbench.image_detail.current') : imageVariantSourceLabel(t, variant.source_type)
        return (
          <article key={variant.uuid} className={`section-image-variant-item${current ? ' is-current' : ''}`}>
            <ProductionImage
              projectUuid={projectUuid}
              asset={variant.asset}
              alt={title}
              renderReady={(image) => (
                <button className="section-image-variant-item__details" type="button" aria-haspopup="dialog" aria-label={t('comic.workbench.image_detail.open_candidate', { filename: title })} onClick={() => onOpen(variant)}>
                  <span className="section-image-variant-item__media">{image}</span>
                  <span className="section-image-variant-item__body">
                    <ImageMetadata as="span" className="section-variant-item__meta" items={[source, formatDateTime(variant.created_at), comicImageDimensions(variant.asset)]} />
                    <strong>{title}</strong>
                    <ImageMetadata as="span" items={[comicImageFileSize(variant.asset?.byte_size, formatNumber), comicImageModelLabel(variant)]} />
                  </span>
                </button>
              )}
            />
            {!current ? <button type="button" className="button-secondary section-variant-item__select" disabled={pending} onClick={() => onSelect(variant)}><Check size={14} aria-hidden="true" />{t('comic.workbench.image_detail.select')}</button> : null}
          </article>
        )
      })}
    </div>
  )
}

function CurrentImagePreviewDialog({ projectUuid, section, variant, onClose, pictureBook, unit = 'section' }) {
  const { t } = useI18n()
  const asset = variant.asset
  const titleId = 'section-image-dialog-title'
  return (
    <WorkbenchDialog className="section-image-dialog" onClose={onClose} aria-labelledby={titleId}>
      <header className="section-image-dialog__header">
        <div><p className="eyebrow">{t('comic.workbench.image_detail.preview_eyebrow')}</p><h2 id={titleId}>{t(unit === 'page' ? 'comic.workbench.image_detail.preview_title_page' : 'comic.workbench.image_detail.preview_title')}</h2></div>
        <button className="section-image-dialog__close" type="button" aria-label={t('common.action.close')} onClick={onClose}>×</button>
      </header>
	  <div className="section-image-dialog__media"><ProductionImage projectUuid={projectUuid} asset={asset} alt={section.title || t(unit === 'page' ? 'comic.page' : 'comic.section')} profile="detail_1024" /></div>
	  <ImageRatioNotice pictureBook={pictureBook} width={asset.width} height={asset.height} />
      <div className="section-image-dialog__footer">
        <ImageMetadata items={[comicImageDimensions(asset), comicImageModelLabel(variant), comicImageTitle(variant)]} />
        {asset.content_url ? <a className="button-secondary" href={asset.content_url} target="_blank" rel="noreferrer">{t('comic.workbench.image_detail.open_original')}</a> : null}
      </div>
    </WorkbenchDialog>
  )
}

function ImageVariantDetailDialog({ projectUuid, variant, current, pending, onClose, onSelect, pictureBook }) {
  const { formatDateTime, formatNumber, t } = useI18n()
  const asset = variant.asset
  const title = comicImageTitle(variant, `v${variant.version_no}`)
  const source = imageVariantSourceLabel(t, variant.source_type)
  const titleId = 'section-image-variant-dialog-title'
  const facts = [
    [t('comic.workbench.image_detail.filename'), title],
    [t('common.label.source'), source],
    [t('comic.workbench.image_detail.created_at'), variant.created_at ? formatDateTime(variant.created_at) : '—'],
    [t('comic.workbench.image_detail.updated_at'), asset.created_at ? formatDateTime(asset.created_at) : '—'],
    [t('comic.workbench.image_detail.page_dimensions'), comicImageDimensions(asset) || '—'],
    [t('comic.workbench.image_detail.file_size'), comicImageFileSize(asset.byte_size, formatNumber) || '—'],
    [t('comic.workbench.image_detail.media_type'), asset.kind || '—'],
    [t('comic.workbench.image_detail.model'), comicImageModelLabel(variant) || '—'],
    [t('comic.workbench.image_detail.variant_uuid'), variant.uuid || '—'],
    [t('comic.workbench.image_detail.asset_uuid'), asset.uuid || '—'],
    [t('comic.workbench.image_detail.generation_uuid'), variant.generation?.uuid || variant.generation_uuid || '—'],
  ]
  return (
    <WorkbenchDialog className="section-image-variant-dialog" dismissDisabled={pending} onClose={onClose} aria-labelledby={titleId}>
      <header className="section-image-dialog__header">
        <div><p className="eyebrow">{t('comic.workbench.image_detail.candidate_eyebrow')}</p><h2 id={titleId}>{title}</h2></div>
        <button className="section-image-dialog__close" type="button" disabled={pending} aria-label={t('common.action.close')} onClick={onClose}>×</button>
      </header>
	  <div className="section-image-variant-dialog__body">
		<div className="section-image-variant-dialog__media"><ProductionImage projectUuid={projectUuid} asset={asset} alt={title} profile="detail_1024" /></div>
		<aside className="section-image-variant-dialog__side" aria-label={t('comic.workbench.image_detail.candidate_info')}>
		  <ImageRatioNotice pictureBook={pictureBook} width={asset.width} height={asset.height} />
          <div className="section-image-variant-dialog__actions">
            <span className={`section-image-variant-dialog__status${current ? ' is-current' : ''}`}>{current ? t('comic.workbench.image_detail.current') : source}</span>
            <button className="button-secondary section-variant-item__select" type="button" disabled={pending || current} onClick={onSelect}><Check size={14} aria-hidden="true" />{t(current ? 'comic.workbench.image_detail.selected_action' : 'comic.workbench.image_detail.select')}</button>
            {asset.content_url ? <a className="button-secondary" href={asset.content_url} target="_blank" rel="noreferrer">{t('comic.workbench.image_detail.open_original')}</a> : null}
          </div>
          <dl className="section-image-variant-dialog__info">{facts.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
        </aside>
      </div>
    </WorkbenchDialog>
  )
}

function ImageMetadata({ as: Tag = 'div', className = 'comic-section-visual__meta', items }) {
  const visible = items.filter(Boolean)
  if (!visible.length) return null
  return <Tag className={className}>{visible.map((item, index) => <span key={`${item}-${index}`}>{item}</span>)}</Tag>
}

function imageVariantSourceLabel(t, sourceType) {
  if (['ai', 'ai_generated', 'generated'].includes(sourceType)) return t('comic.workbench.image_detail.source_generated')
  if (['external_import', 'file_import', 'imported', 'upload'].includes(sourceType)) return t('comic.workbench.image_detail.source_uploaded')
  return sourceTypeLabel(t, sourceType)
}

function ChapterPromptWorkbench({ projectUuid }) {
	return <PromptCatalogEditor projectUuid={projectUuid} groups={['chapter']} />
}
